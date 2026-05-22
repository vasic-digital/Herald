package wizard

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// synthetic token shape (NOT a real token — same precedent as shell_test.go).
const fakeTgramToken = "0000000000:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

// newTgramFixture spins up an httptest.Server that fakes the subset of
// the Telegram Bot API the wizard touches: getMe, getChat, getUpdates.
// Tests override tgramBaseURL to point at it. The fixture asserts the
// token in the URL path matches the expected value so tests can prove
// the wizard actually used the supplied token.
type tgramFixture struct {
	server   *httptest.Server
	getMeHits int
	getChatHits int
	getUpdatesHits int
	wantToken string

	// Knobs:
	chatID   int64  // result.id for getChat
	chatType string // result.type for getChat ("private" / "group" / etc.)
	chatName string // result.title or first_name for getChat
	failGetMe bool
	failGetChat bool
	emptyUpdates bool
}

func newTgramFixture(t *testing.T, wantToken string) *tgramFixture {
	t.Helper()
	f := &tgramFixture{
		wantToken: wantToken,
		chatID:    424242,
		chatType:  "private",
		chatName:  "Test Operator",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/bot"+wantToken+"/getMe", func(w http.ResponseWriter, r *http.Request) {
		f.getMeHits++
		if f.failGetMe {
			_, _ = w.Write([]byte(`{"ok":false,"description":"forced-fail"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Fixture","username":"fixturebot","can_join_groups":true}}`))
	})
	mux.HandleFunc("/bot"+wantToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		f.getChatHits++
		if f.failGetChat {
			_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":` + itoa(f.chatID) + `,"type":"` + f.chatType + `","title":"` + f.chatName + `","first_name":"` + f.chatName + `"}}`))
	})
	mux.HandleFunc("/bot"+wantToken+"/getUpdates", func(w http.ResponseWriter, r *http.Request) {
		f.getUpdatesHits++
		if f.emptyUpdates {
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"chat":{"id":` + itoa(f.chatID) + `,"type":"` + f.chatType + `","title":"` + f.chatName + `","first_name":"` + f.chatName + `"}}}]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected fixture hit: %s %s (token mismatch suspected)", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func itoa(n int64) string {
	// avoid importing strconv just for one call
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func withTgramBaseURL(t *testing.T, url string) {
	t.Helper()
	prev := tgramBaseURL
	tgramBaseURL = url
	t.Cleanup(func() { tgramBaseURL = prev })
}

func newTmpShellTarget(t *testing.T) ShellTarget {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(path, []byte("# Test fixture .zshrc\n"), 0600); err != nil {
		t.Fatalf("seed shell file: %v", err)
	}
	return ShellTarget{Path: path, DisplayName: ".zshrc", ShellKind: "zshrc"}
}

func unsetTgramEnv(t *testing.T) {
	t.Helper()
	prev := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	prev2 := os.Getenv("HERALD_TGRAM_CHAT_ID")
	_ = os.Unsetenv("HERALD_TGRAM_BOT_TOKEN")
	_ = os.Unsetenv("HERALD_TGRAM_CHAT_ID")
	t.Cleanup(func() {
		if prev != "" {
			_ = os.Setenv("HERALD_TGRAM_BOT_TOKEN", prev)
		}
		if prev2 != "" {
			_ = os.Setenv("HERALD_TGRAM_CHAT_ID", prev2)
		}
	})
}

// TestRunTelegram_BotTokenAndChatIDFromOpts proves that when BOTH
// --bot-token and --chat-id are supplied, the wizard runs without ANY
// prompt input — stdin is empty — and persists the chat_id (but NOT
// re-persisting the token, since the wizard only persists token when
// supplied via flag, not via env; here it's via flag so it DOES persist).
//
// Wait — re-check: per resolveTokenInput, flag-supplied token IS persisted
// (only env-supplied skips). So this test verifies the token line ends up
// in the temp .zshrc.
func TestRunTelegram_BotTokenAndChatIDFromOpts(t *testing.T) {
	unsetTgramEnv(t)
	f := newTgramFixture(t, fakeTgramToken)
	withTgramBaseURL(t, f.server.URL)
	target := newTmpShellTarget(t)

	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader("")) // EMPTY stdin — wizard must not block on Read
	opts := Opts{
		BotToken:       fakeTgramToken,
		ChatID:         "424242",
		NonInteractive: true,
	}
	if err := runTelegram(context.Background(), in, &out, target, opts); err != nil {
		t.Fatalf("runTelegram returned error: %v; output:\n%s", err, out.String())
	}
	if f.getMeHits != 1 {
		t.Errorf("getMe hits = %d, want 1", f.getMeHits)
	}
	if f.getChatHits != 1 {
		t.Errorf("getChat hits = %d, want 1 (validate chat_id BEFORE persist)", f.getChatHits)
	}
	if f.getUpdatesHits != 0 {
		t.Errorf("getUpdates hits = %d, want 0 (chat_id from opts → polling skipped)", f.getUpdatesHits)
	}
	body, _ := os.ReadFile(target.Path)
	if !strings.Contains(string(body), "HERALD_TGRAM_BOT_TOKEN") {
		t.Errorf(".zshrc lacks HERALD_TGRAM_BOT_TOKEN — flag-supplied token must persist; body:\n%s", body)
	}
	if !strings.Contains(string(body), "HERALD_TGRAM_CHAT_ID=") || !strings.Contains(string(body), "424242") {
		t.Errorf(".zshrc lacks HERALD_TGRAM_CHAT_ID=424242; body:\n%s", body)
	}
}

// TestRunTelegram_EnvTokenSkipsPersistence proves the env-supplied
// token is used but NOT re-written to .zshrc (since the operator
// already has it configured by definition).
func TestRunTelegram_EnvTokenSkipsPersistence(t *testing.T) {
	unsetTgramEnv(t)
	_ = os.Setenv("HERALD_TGRAM_BOT_TOKEN", fakeTgramToken)
	t.Cleanup(func() { _ = os.Unsetenv("HERALD_TGRAM_BOT_TOKEN") })

	f := newTgramFixture(t, fakeTgramToken)
	withTgramBaseURL(t, f.server.URL)
	target := newTmpShellTarget(t)

	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader(""))
	opts := Opts{ChatID: "555555", NonInteractive: true}
	if err := runTelegram(context.Background(), in, &out, target, opts); err != nil {
		t.Fatalf("runTelegram error: %v; output:\n%s", err, out.String())
	}
	body, _ := os.ReadFile(target.Path)
	if strings.Contains(string(body), "HERALD_TGRAM_BOT_TOKEN") {
		t.Errorf("env-supplied token MUST NOT be re-persisted to .zshrc; body:\n%s", body)
	}
	if !strings.Contains(string(body), "HERALD_TGRAM_CHAT_ID=") {
		t.Errorf(".zshrc must contain chat_id; body:\n%s", body)
	}
	if !strings.Contains(out.String(), "detected in HERALD_TGRAM_BOT_TOKEN env") {
		t.Errorf("wizard output must announce env-supplied token; got:\n%s", out.String())
	}
}

// TestRunTelegram_NonInteractiveMissingToken proves --non-interactive
// without a token (and no env) fails LOUD before any network call.
func TestRunTelegram_NonInteractiveMissingToken(t *testing.T) {
	unsetTgramEnv(t)
	f := newTgramFixture(t, fakeTgramToken)
	withTgramBaseURL(t, f.server.URL)
	target := newTmpShellTarget(t)

	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader(""))
	opts := Opts{NonInteractive: true} // no token, no chat_id, no env
	err := runTelegram(context.Background(), in, &out, target, opts)
	if err == nil {
		t.Fatalf("expected error for non-interactive without token; got nil")
	}
	if !strings.Contains(err.Error(), "no bot token") {
		t.Errorf("error must name the missing input; got: %v", err)
	}
	if f.getMeHits != 0 {
		t.Errorf("no network call should fire when prerequisite missing; getMe=%d", f.getMeHits)
	}
}

// TestRunTelegram_ChatIDValidationRejection proves that an opts/env
// chat_id that the Bot API rejects (getChat returns ok=false) causes
// the wizard to error out BEFORE persisting anything.
func TestRunTelegram_ChatIDValidationRejection(t *testing.T) {
	unsetTgramEnv(t)
	f := newTgramFixture(t, fakeTgramToken)
	f.failGetChat = true // forced rejection
	withTgramBaseURL(t, f.server.URL)
	target := newTmpShellTarget(t)

	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader(""))
	opts := Opts{BotToken: fakeTgramToken, ChatID: "999", NonInteractive: true}
	err := runTelegram(context.Background(), in, &out, target, opts)
	if err == nil {
		t.Fatalf("expected error when getChat returns ok=false; got nil")
	}
	if !strings.Contains(err.Error(), "999") || !strings.Contains(err.Error(), "getChat") {
		t.Errorf("error must name the chat_id + getChat; got: %v", err)
	}
	body, _ := os.ReadFile(target.Path)
	if strings.Contains(string(body), "HERALD_TGRAM_CHAT_ID") {
		t.Errorf("chat_id MUST NOT be persisted when getChat rejected it; body:\n%s", body)
	}
}

// TestRunTelegram_FlagTokenWinsOverEnv proves that when BOTH --bot-token
// flag AND env are set, the flag wins (and the wizard treats it as
// flag-supplied → persists it).
func TestRunTelegram_FlagTokenWinsOverEnv(t *testing.T) {
	unsetTgramEnv(t)
	_ = os.Setenv("HERALD_TGRAM_BOT_TOKEN", "9999999999:ENVTOKEN_____________________________")
	t.Cleanup(func() { _ = os.Unsetenv("HERALD_TGRAM_BOT_TOKEN") })

	f := newTgramFixture(t, fakeTgramToken) // fixture only accepts the FLAG-supplied token
	withTgramBaseURL(t, f.server.URL)
	target := newTmpShellTarget(t)

	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader(""))
	opts := Opts{BotToken: fakeTgramToken, ChatID: "424242", NonInteractive: true}
	if err := runTelegram(context.Background(), in, &out, target, opts); err != nil {
		t.Fatalf("runTelegram error: %v; output:\n%s", err, out.String())
	}
	if f.getMeHits != 1 {
		t.Errorf("flag-supplied token should be the one validated; getMe hits = %d", f.getMeHits)
	}
	if !strings.Contains(out.String(), "supplied via --bot-token flag") {
		t.Errorf("wizard must announce flag-supplied token; got:\n%s", out.String())
	}
}
