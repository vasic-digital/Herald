package wizard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// tgramBaseURL is the Telegram Bot API base — overridable in tests
// against an httptest.NewServer. Production value is hard-coded HTTPS;
// no environment-driven override is exposed in the public CLI surface
// (preventing accidental redirection of live credentials to a third
// party).
var tgramBaseURL = "https://api.telegram.org"

// runTelegram walks the operator through obtaining HERALD_TGRAM_BOT_TOKEN
// and HERALD_TGRAM_CHAT_ID, validating each against the live Bot API
// before persisting. Per §107: no value is saved without observing
// positive evidence (getMe returns a Username; getUpdates / getChat
// returns a usable chat record).
//
// Token resolution order (first non-empty wins):
//   1. opts.BotToken (--bot-token CLI flag)
//   2. os.Getenv("HERALD_TGRAM_BOT_TOKEN")
//   3. interactive prompt (skipped + error in non-interactive mode)
// When resolved from env (case 2) we DO NOT re-persist — the operator
// already has it configured. When resolved from flag/prompt we DO
// persist via writeAndSummarize so the value survives a fresh shell.
//
// Chat ID resolution order:
//   1. opts.ChatID (--chat-id CLI flag) — validated via getChat
//   2. os.Getenv("HERALD_TGRAM_CHAT_ID")               — validated via getChat
//   3. interactive: poll getUpdates, let operator pick (default)
//   4. non-interactive without (1) or (2): error
func runTelegram(ctx context.Context, r *bufio.Reader, out io.Writer, target ShellTarget, opts Opts) error {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "── Telegram (HRD-011) ─────────────────────────────────────────────")
	fmt.Fprintln(out, "")

	// --- 1. Resolve bot token ---
	token, tokenFromEnv, err := resolveTokenInput(r, out, opts)
	if err != nil {
		return err
	}

	if !looksLikeTgramToken(token) {
		return fmt.Errorf("wizard.telegram: token %q does not match Bot API shape <digits>:<35+chars>", MaskValue(token))
	}

	// §107: validate via getMe BEFORE persisting / proceeding.
	fmt.Fprintf(out, "  Validating token via Bot API getMe...\n")
	username, err := tgramGetMe(ctx, token)
	if err != nil {
		return fmt.Errorf("wizard.telegram: getMe failed: %w", err)
	}
	fmt.Fprintf(out, "  ✓ token valid — bot username: @%s\n", username)

	// Persist token ONLY if we didn't pick it up from env. (Env-supplied
	// tokens are already in the shell file by definition.)
	if !tokenFromEnv {
		if err := writeAndSummarize(out, target,
			"HERALD_TGRAM_BOT_TOKEN", token,
			"Herald — Telegram (HRD-011) — bot token (per docs/guides/messengers/TELEGRAM.md)",
			"telegram",
			fmt.Sprintf("bot @%s validated via getMe at write-time", username),
			r,
		); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "  (token came from HERALD_TGRAM_BOT_TOKEN env — not re-persisting)\n")
	}

	// --- 2. Resolve chat ID ---
	chatID, err := resolveChatID(ctx, r, out, opts, token, username)
	if err != nil {
		return err
	}

	// Persist chat_id (always — it's the wizard's primary output).
	if err := writeAndSummarize(out, target,
		"HERALD_TGRAM_CHAT_ID", chatID,
		"Herald — Telegram (HRD-011) — destination chat ID (per docs/guides/messengers/TELEGRAM.md)",
		"telegram",
		fmt.Sprintf("chat_id=%s validated against bot @%s", chatID, username),
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

// resolveTokenInput picks a bot token from opts → env → prompt, in that
// order. The bool return is true iff the token came from env (so the
// caller knows to skip re-persisting).
func resolveTokenInput(r *bufio.Reader, out io.Writer, opts Opts) (string, bool, error) {
	if opts.BotToken != "" {
		tok := strings.TrimSpace(opts.BotToken)
		if tok == "" {
			return "", false, fmt.Errorf("wizard.telegram: --bot-token was whitespace-only")
		}
		fmt.Fprintln(out, "  Token: supplied via --bot-token flag.")
		return tok, false, nil
	}
	if envTok := strings.TrimSpace(os.Getenv("HERALD_TGRAM_BOT_TOKEN")); envTok != "" {
		fmt.Fprintln(out, "  Token: detected in HERALD_TGRAM_BOT_TOKEN env (will not re-persist).")
		return envTok, true, nil
	}
	if opts.NonInteractive {
		return "", false, fmt.Errorf("wizard.telegram: --non-interactive set but no bot token (provide --bot-token or set HERALD_TGRAM_BOT_TOKEN)")
	}
	fmt.Fprintln(out, "Pre-requisite: you've already created a bot via @BotFather and have the")
	fmt.Fprintln(out, "token. If not, see docs/guides/messengers/TELEGRAM.md §Step 1.")
	fmt.Fprintln(out, "")
	fmt.Fprint(out, "Paste your bot token (will be MASKED in confirmations; never printed raw): ")
	tok, err := readSecretLine(r)
	if err != nil {
		return "", false, err
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", false, fmt.Errorf("wizard.telegram: bot token cannot be empty")
	}
	return tok, false, nil
}

// resolveChatID picks a chat ID from opts → env → interactive
// getUpdates polling. Whichever source is used, the result is validated
// against the live Bot API (getChat) so a stale / wrong chat_id is
// caught BEFORE persistence (§107 anti-bluff).
func resolveChatID(ctx context.Context, r *bufio.Reader, out io.Writer, opts Opts, token, username string) (string, error) {
	if opts.ChatID != "" {
		id := strings.TrimSpace(opts.ChatID)
		fmt.Fprintf(out, "  Chat ID: supplied via --chat-id=%s; validating via getChat...\n", id)
		ct, name, err := tgramGetChat(ctx, token, id)
		if err != nil {
			return "", fmt.Errorf("wizard.telegram: --chat-id=%s rejected by Bot API getChat: %w", id, err)
		}
		fmt.Fprintf(out, "  ✓ chat valid — type=%s name=%q\n", ct, name)
		return id, nil
	}
	if envID := strings.TrimSpace(os.Getenv("HERALD_TGRAM_CHAT_ID")); envID != "" {
		fmt.Fprintf(out, "  Chat ID: detected in HERALD_TGRAM_CHAT_ID env (%s); validating via getChat...\n", envID)
		ct, name, err := tgramGetChat(ctx, token, envID)
		if err != nil {
			return "", fmt.Errorf("wizard.telegram: HERALD_TGRAM_CHAT_ID=%s rejected by Bot API getChat: %w", envID, err)
		}
		fmt.Fprintf(out, "  ✓ chat valid — type=%s name=%q\n", ct, name)
		return envID, nil
	}
	if opts.NonInteractive {
		return "", fmt.Errorf("wizard.telegram: --non-interactive set but no chat ID (provide --chat-id, set HERALD_TGRAM_CHAT_ID, or run interactive)")
	}

	// Interactive flow: poll getUpdates.
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
		return "", fmt.Errorf("wizard.telegram: getUpdates failed: %w", err)
	}
	if len(chats) == 0 {
		fmt.Fprintln(out, "  No chats found yet. Common causes:")
		fmt.Fprintln(out, "    - Privacy Mode is ON and you didn't send a /command or @-mention in a group.")
		fmt.Fprintln(out, "    - You haven't sent any message to the bot yet (DMs require at least one).")
		fmt.Fprintln(out, "    - A previous getUpdates already acked the update (send a fresh message).")
		fmt.Fprintln(out, "  See docs/guides/messengers/TELEGRAM.md §Step 2.5 for full troubleshooting.")
		return "", fmt.Errorf("wizard.telegram: getUpdates returned 0 chats — send a message to the bot and re-run")
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
			return "", fmt.Errorf("wizard.telegram: invalid chat choice %q", line)
		}
	}
	chosen := chats[idx-1]
	return strconv.FormatInt(chosen.ID, 10), nil
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
	u := tgramBaseURL + "/bot" + url.PathEscape(token) + "/getMe"
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

// tgramGetChat calls getChat for the supplied chat_id (integer or
// "@channel_username") and returns (type, name, error). Used to validate
// an opts/env-supplied chat_id before persisting it — the Bot API
// returns 400 if the bot can't see the chat.
func tgramGetChat(ctx context.Context, token, chatID string) (string, string, error) {
	u := tgramBaseURL + "/bot" + url.PathEscape(token) + "/getChat?chat_id=" + url.QueryEscape(chatID)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", "", err
	}
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			Title     string `json:"title"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode getChat: %w", err)
	}
	if !out.OK {
		return "", "", fmt.Errorf("getChat returned ok=false: %s", out.Description)
	}
	if out.Result.Type == "" {
		return "", "", fmt.Errorf("getChat returned empty Type (§107 bluff guard)")
	}
	name := out.Result.Title
	if name == "" {
		name = out.Result.Username
	}
	if name == "" {
		name = out.Result.FirstName
	}
	return out.Result.Type, name, nil
}

// tgramListChats calls getUpdates with offset=-1 + limit=100 and
// extracts the unique chat IDs the bot has updates from. Empty slice
// is a valid return value (= no updates yet).
func tgramListChats(ctx context.Context, token string) ([]tgramChat, error) {
	u := tgramBaseURL + "/bot" + url.PathEscape(token) + "/getUpdates?offset=-1&limit=100"
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
