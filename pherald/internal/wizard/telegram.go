package wizard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// runTelegram walks the operator through obtaining HERALD_TGRAM_BOT_TOKEN
// and HERALD_TGRAM_CHAT_ID, validating each against the live Bot API
// before persisting. Per §107: no value is saved without observing
// positive evidence (getMe returns a Username; getUpdates returns a chat).
func runTelegram(ctx context.Context, r *bufio.Reader, out io.Writer, target ShellTarget) error {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "── Telegram (HRD-011) ─────────────────────────────────────────────")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Pre-requisite: you've already created a bot via @BotFather and have the")
	fmt.Fprintln(out, "token. If not, see docs/guides/messengers/TELEGRAM.md §Step 1.")
	fmt.Fprintln(out, "")
	fmt.Fprint(out, "Paste your bot token (will be MASKED in confirmations; never printed raw): ")
	token, err := readSecretLine(r)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("wizard.telegram: bot token cannot be empty")
	}
	if !looksLikeTgramToken(token) {
		return fmt.Errorf("wizard.telegram: token %q does not match Bot API token shape <digits>:<35+chars>", MaskValue(token))
	}

	// §107: validate via getMe BEFORE persisting.
	fmt.Fprintf(out, "  Validating token via Bot API getMe...\n")
	username, err := tgramGetMe(ctx, token)
	if err != nil {
		return fmt.Errorf("wizard.telegram: getMe failed: %w", err)
	}
	fmt.Fprintf(out, "  ✓ token valid — bot username: @%s\n", username)

	// Save the token (with masking + summary).
	if err := writeAndSummarize(out, target,
		"HERALD_TGRAM_BOT_TOKEN", token,
		"Herald — Telegram (HRD-011) — bot token (per docs/guides/messengers/TELEGRAM.md)",
		"telegram",
		fmt.Sprintf("bot @%s validated via getMe at write-time", username),
		r,
	); err != nil {
		return err
	}

	// Chat ID discovery.
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Now we need a chat ID — the destination Herald will send messages to.")
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "  Recommended flow: in Telegram, search @%s and tap Start (or send /start) to open a DM.\n", username)
	fmt.Fprintln(out, "  Then send ANY message in that DM. Once you've done that, press Enter here.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Tip: for GROUP chats, see docs/guides/messengers/TELEGRAM.md §Step 2.5 —")
	fmt.Fprintln(out, "       promoting the bot to group admin bypasses Privacy Mode entirely.")
	fmt.Fprint(out, "[Press Enter when ready to poll getUpdates] ")
	_, _ = r.ReadString('\n')

	fmt.Fprintln(out, "  Polling getUpdates...")
	chats, err := tgramListChats(ctx, token)
	if err != nil {
		return fmt.Errorf("wizard.telegram: getUpdates failed: %w", err)
	}
	if len(chats) == 0 {
		fmt.Fprintln(out, "  No chats found yet. Common causes:")
		fmt.Fprintln(out, "    - Privacy Mode is ON and you didn't send a /command or @-mention in a group.")
		fmt.Fprintln(out, "    - You haven't sent any message to the bot yet (DMs require at least one).")
		fmt.Fprintln(out, "    - A previous getUpdates already acked the update (send a fresh message).")
		fmt.Fprintln(out, "  See docs/guides/messengers/TELEGRAM.md §Step 2.5 for full troubleshooting.")
		fmt.Fprint(out, "  Continue anyway and set HERALD_TGRAM_CHAT_ID manually later? [y/N]: ")
		line, _ := r.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			return fmt.Errorf("wizard.telegram: aborted by operator — no chat_id available")
		}
		return nil
	}
	fmt.Fprintln(out, "  Chats discovered:")
	for i, c := range chats {
		fmt.Fprintf(out, "    [%d] chat_id=%d  type=%s  name=%s\n", i+1, c.ID, c.Type, c.Name)
	}
	fmt.Fprint(out, "  Pick a number to save as HERALD_TGRAM_CHAT_ID [default=1]: ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	idx := 1
	if line != "" {
		if _, err := fmt.Sscanf(line, "%d", &idx); err != nil || idx < 1 || idx > len(chats) {
			return fmt.Errorf("wizard.telegram: invalid chat choice %q", line)
		}
	}
	chosen := chats[idx-1]
	chatIDStr := fmt.Sprintf("%d", chosen.ID)

	if err := writeAndSummarize(out, target,
		"HERALD_TGRAM_CHAT_ID", chatIDStr,
		fmt.Sprintf("Herald — Telegram (HRD-011) — chat_id (type=%s, name=%q)", chosen.Type, chosen.Name),
		"telegram",
		fmt.Sprintf("type=%s name=%q (discovered via getUpdates)", chosen.Type, chosen.Name),
		r,
	); err != nil {
		return err
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Telegram setup complete. To verify end-to-end (HRD-011 E17):")
	fmt.Fprintln(out, "    source ~/.zshrc   # or your chosen shell file")
	fmt.Fprintln(out, "    go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSend_PersistsDeliveryEvidence -count=1 -timeout=300s")
	return nil
}

// readSecretLine reads a line from `r` without echoing. (For real
// terminal stdin we'd use golang.org/x/term.ReadPassword for true
// no-echo; without that dep we still avoid leaking via wizard output —
// the operator's terminal will show what they paste, but the wizard
// itself never prints the raw value back. Acceptable per §107.)
func readSecretLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// looksLikeTgramToken returns true if s matches the canonical Bot API
// token shape: <digits>:<35+chars-of-base64ish>. Rejects obvious
// typos/non-tokens before hitting the API.
func looksLikeTgramToken(s string) bool {
	colon := strings.Index(s, ":")
	if colon < 1 {
		return false
	}
	prefix, suffix := s[:colon], s[colon+1:]
	if len(suffix) < 30 {
		return false
	}
	for _, c := range prefix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type tgramChat struct {
	ID   int64
	Type string
	Name string
}

// tgramGetMe calls the Telegram Bot API's getMe and returns the bot's
// Username on success. The token is sent in the URL path (per Bot API
// convention) over HTTPS only. Per §107: returns an error if the API
// reports !ok OR if Username is empty.
func tgramGetMe(ctx context.Context, token string) (string, error) {
	u := "https://api.telegram.org/bot" + url.PathEscape(token) + "/getMe"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode getMe: %w", err)
	}
	if !out.OK {
		return "", fmt.Errorf("getMe returned ok=false: %s", out.Description)
	}
	if out.Result.Username == "" {
		return "", fmt.Errorf("getMe returned empty Username (§107 bluff guard)")
	}
	return out.Result.Username, nil
}

// tgramListChats calls getUpdates with offset=-1 + limit=100 and
// extracts the unique chat IDs the bot has updates from. Empty slice
// is a valid return value (= no updates yet).
func tgramListChats(ctx context.Context, token string) ([]tgramChat, error) {
	u := "https://api.telegram.org/bot" + url.PathEscape(token) + "/getUpdates?offset=-1&limit=100"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		OK     bool `json:"ok"`
		Result []struct {
			Message *struct {
				Chat struct {
					ID        int64  `json:"id"`
					Type      string `json:"type"`
					Title     string `json:"title"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"chat"`
			} `json:"message"`
			EditedMessage *struct {
				Chat struct {
					ID        int64  `json:"id"`
					Type      string `json:"type"`
					Title     string `json:"title"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"chat"`
			} `json:"edited_message"`
			ChannelPost *struct {
				Chat struct {
					ID        int64  `json:"id"`
					Type      string `json:"type"`
					Title     string `json:"title"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"chat"`
			} `json:"channel_post"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode getUpdates: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("getUpdates returned ok=false: %s", out.Description)
	}
	seen := map[int64]bool{}
	var chats []tgramChat
	for _, r := range out.Result {
		for _, m := range []struct {
			Chat struct {
				ID        int64  `json:"id"`
				Type      string `json:"type"`
				Title     string `json:"title"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"chat"`
		}{} {
			_ = m // placeholder; the real loop is below using concrete pointers
		}
		extractChat := func(c *struct {
			Chat struct {
				ID        int64  `json:"id"`
				Type      string `json:"type"`
				Title     string `json:"title"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"chat"`
		}) {
			if c == nil {
				return
			}
			if seen[c.Chat.ID] {
				return
			}
			seen[c.Chat.ID] = true
			name := c.Chat.Title
			if name == "" {
				name = c.Chat.Username
			}
			if name == "" {
				name = c.Chat.FirstName
			}
			chats = append(chats, tgramChat{ID: c.Chat.ID, Type: c.Chat.Type, Name: name})
		}
		extractChat(r.Message)
		extractChat(r.EditedMessage)
		extractChat(r.ChannelPost)
	}
	return chats, nil
}
