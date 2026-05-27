// Hermetic unit tests for the 10-gate pre-flight validator (T6).
//
// Each gate is exercised twice: a PASS path (every gate green) and a
// FAIL path (the gate's precondition violated). A no-op validator that
// returned nil would FAIL every gate-FAIL assertion below — the tests
// assert on the *PreflightError's Gate + ExitCode + Reason, drawn from
// a real Preflight() round-trip simulated by an httptest server (G2)
// or a configurable stub (G3..G10) plus real os.Stat / env reads
// (G7/G8/G6/G10).
//
// §107 anti-bluff: there is no "absence-of-error" PASS — the PASS test
// asserts runPreflight returns nil ONLY when a complete, real report
// satisfies all gates, and each FAIL test asserts the SPECIFIC gate
// tag + exit code fired (not merely "some error").
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// preflightStub is a configurable MessengerClient whose Preflight
// return value is driven per-test. Distinct from fakeMessenger (whose
// Preflight is fixed-green) so gate-FAIL paths can inject violations.
type preflightStub struct {
	report messenger.PreflightReport
	err    error
}

func (s *preflightStub) Me(context.Context) (string, int64, error) {
	return s.report.Username, s.report.UserID, nil
}
func (s *preflightStub) Send(context.Context, string) (messenger.MessageID, error) {
	return "1", nil
}
func (s *preflightStub) SendPhoto(context.Context, string, string) (messenger.MessageID, error) {
	return "1", nil
}
func (s *preflightStub) SendDocument(context.Context, string, string) (messenger.MessageID, error) {
	return "1", nil
}
func (s *preflightStub) SendVoice(context.Context, string) (messenger.MessageID, error) {
	return "1", nil
}
func (s *preflightStub) WaitForReply(context.Context, messenger.MessageID, messenger.Predicate, time.Duration) (messenger.Reply, error) {
	return messenger.Reply{}, context.DeadlineExceeded
}
func (s *preflightStub) GetUpdates(_ context.Context, offset int64) ([]messenger.Reply, int64, error) {
	return nil, offset, nil
}
func (s *preflightStub) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("preflightStub: Download unused")
}
func (s *preflightStub) Preflight(context.Context, int64) (messenger.PreflightReport, error) {
	return s.report, s.err
}
func (s *preflightStub) Close() error { return nil }

var _ messenger.MessengerClient = (*preflightStub)(nil)

// greenReport is the all-gates-pass baseline. Individual tests clone +
// mutate one field to drive a single gate to FAIL.
func greenReport() messenger.PreflightReport {
	return messenger.PreflightReport{
		Username:                "herald_qa_bot",
		UserID:                  999,
		CanReadAllGroupMessages: true,
		InChat:                  true,
		ChatType:                "supergroup",
		PheraldBotPresent:       true,
	}
}

// preflightDocsDir creates a temp docs dir with Issues.md + Fixed.md so
// G7 is satisfied by default. Returns the dir.
func preflightDocsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Issues.md"), []byte("| HRD-001 | bug |\n"), 0o644); err != nil {
		t.Fatalf("write Issues.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Fixed.md"), []byte("| HRD-000 | bug |\n"), 0o644); err != nil {
		t.Fatalf("write Fixed.md: %v", err)
	}
	return dir
}

// baseCfg returns a Config wired to a docs dir with both .md files and
// no OTel-port liveness requirement (HERALD_OTEL_PORT is unset by the
// test harness — t.Setenv is used by the gates that need it).
func baseCfg(t *testing.T, docsDir string) Config {
	return Config{
		QABotToken:         "qa:TOKEN",
		ChatID:             999,
		PheraldBotUsername: "pherald_bot",
		DocsDir:            docsDir,
	}
}

// assertGate runs runPreflight and asserts a *PreflightError with the
// expected Gate + ExitCode fired. discard sink for writeEvent keeps the
// transcript closure exercised (so a writeEvent panic would surface).
func assertGate(t *testing.T, msgr, msgrNonOp messenger.MessengerClient, cfg Config, wantGate string, wantExit int) {
	t.Helper()
	err := runPreflight(context.Background(), msgr, msgrNonOp, cfg, func(string, string, any) {})
	if err == nil {
		t.Fatalf("gate %s: expected *PreflightError, got nil", wantGate)
	}
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("gate %s: error is not *PreflightError: %v", wantGate, err)
	}
	if pe.Gate != wantGate {
		t.Fatalf("gate: got %q, want %q (reason: %s)", pe.Gate, wantGate, pe.Reason)
	}
	if pe.ExitCode != wantExit {
		t.Fatalf("gate %s exit code: got %d, want %d", wantGate, pe.ExitCode, wantExit)
	}
	if code, ok := PreflightExitCode(err); !ok || code != wantExit {
		t.Fatalf("PreflightExitCode unwrap: got (%d,%v), want (%d,true)", code, ok, wantExit)
	}
}

// ---- All-gates-green PASS ------------------------------------------

func TestPreflight_AllGatesGreen_PASS(t *testing.T) {
	docs := preflightDocsDir(t)
	cfg := baseCfg(t, docs)
	msgr := &preflightStub{report: greenReport()}
	if err := runPreflight(context.Background(), msgr, nil, cfg, func(string, string, any) {}); err != nil {
		t.Fatalf("expected nil (all gates green), got %v", err)
	}
}

// ---- G2 — qa-bot getMe ---------------------------------------------

func TestPreflight_G2_GetMeError_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	msgr := &preflightStub{err: errors.New("401 Unauthorized")}
	assertGate(t, msgr, nil, cfg, "G2", 2)
}

func TestPreflight_G2_EmptyUsername_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	rep := greenReport()
	rep.Username = ""
	assertGate(t, &preflightStub{report: rep}, nil, cfg, "G2", 2)
}

// ---- G5 — qa-bot != pherald-bot ------------------------------------

func TestPreflight_G5_UsernameCollision_FAIL(t *testing.T) {
	docs := preflightDocsDir(t)
	cfg := baseCfg(t, docs)
	cfg.PheraldBotUsername = "herald_qa_bot" // same as report.Username
	assertGate(t, &preflightStub{report: greenReport()}, nil, cfg, "G5", 5)
}

// ---- G6 — tokens distinct ------------------------------------------

func TestPreflight_G6_TokenCollision_FAIL(t *testing.T) {
	docs := preflightDocsDir(t)
	cfg := baseCfg(t, docs)
	cfg.QABotToken = "shared:TOKEN"
	t.Setenv("HERALD_TGRAM_BOT_TOKEN", "shared:TOKEN")
	assertGate(t, &preflightStub{report: greenReport()}, nil, cfg, "G6", 5)
}

func TestPreflight_G6_DistinctTokens_PASS(t *testing.T) {
	docs := preflightDocsDir(t)
	cfg := baseCfg(t, docs)
	cfg.QABotToken = "qa:TOKEN"
	t.Setenv("HERALD_TGRAM_BOT_TOKEN", "pherald:TOKEN")
	if err := runPreflight(context.Background(), &preflightStub{report: greenReport()}, nil, cfg, func(string, string, any) {}); err != nil {
		t.Fatalf("distinct tokens should pass G6, got %v", err)
	}
}

// ---- G3 — privacy mode ---------------------------------------------

func TestPreflight_G3_PrivacyEnabled_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	rep := greenReport()
	rep.CanReadAllGroupMessages = false
	assertGate(t, &preflightStub{report: rep}, nil, cfg, "G3", 3)
}

// ---- G4 — qa-bot in chat + type ------------------------------------

func TestPreflight_G4_NotInChat_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	rep := greenReport()
	rep.InChat = false
	assertGate(t, &preflightStub{report: rep}, nil, cfg, "G4", 4)
}

func TestPreflight_G4_WrongChatType_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	rep := greenReport()
	rep.ChatType = "private"
	assertGate(t, &preflightStub{report: rep}, nil, cfg, "G4", 4)
}

// ---- G7 — docs dirs exist ------------------------------------------

func TestPreflight_G7_MissingIssuesMd_FAIL(t *testing.T) {
	dir := t.TempDir() // no Issues.md / Fixed.md
	cfg := baseCfg(t, dir)
	assertGate(t, &preflightStub{report: greenReport()}, nil, cfg, "G7", 6)
}

func TestPreflight_G7_MissingFixedMd_FAIL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Issues.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fixed.md absent.
	cfg := baseCfg(t, dir)
	assertGate(t, &preflightStub{report: greenReport()}, nil, cfg, "G7", 6)
}

// ---- G8 — pherald-qa-out-dir exists --------------------------------

func TestPreflight_G8_QAOutDirMissing_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	cfg.PheraldQAOutDir = filepath.Join(t.TempDir(), "does-not-exist")
	assertGate(t, &preflightStub{report: greenReport()}, nil, cfg, "G8", 6)
}

func TestPreflight_G8_QAOutDirExists_PASS(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	cfg.PheraldQAOutDir = t.TempDir() // exists
	if err := runPreflight(context.Background(), &preflightStub{report: greenReport()}, nil, cfg, func(string, string, any) {}); err != nil {
		t.Fatalf("existing qa-out-dir should pass G8, got %v", err)
	}
}

// ---- G1 — pherald-bot present + OTel-port liveness -----------------

func TestPreflight_G1_PheraldBotAbsent_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	rep := greenReport()
	rep.PheraldBotPresent = false
	assertGate(t, &preflightStub{report: rep}, nil, cfg, "G1", 2)
}

func TestPreflight_G1_OTelPortUnreachable_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	// Pick a port that is almost certainly closed: bind+close to learn a
	// free port, then point HERALD_OTEL_PORT at it (now closed).
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // now closed → DialTimeout fails
	t.Setenv("HERALD_OTEL_PORT", fmt.Sprintf("%d", port))
	assertGate(t, &preflightStub{report: greenReport()}, nil, cfg, "G1", 2)
}

func TestPreflight_G1_OTelPortReachable_PASS(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	// srv.URL is http://127.0.0.1:PORT — extract the port.
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host-port: %v", err)
	}
	t.Setenv("HERALD_OTEL_PORT", portStr)
	if err := runPreflight(context.Background(), &preflightStub{report: greenReport()}, nil, cfg, func(string, string, any) {}); err != nil {
		t.Fatalf("reachable OTel port should pass G1, got %v", err)
	}
}

// ---- G10 — non-op bot distinct + not in operator allowlist ---------

func TestPreflight_G10_NonOpSameUserID_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	main := &preflightStub{report: greenReport()}                     // UserID 999
	nonOp := &preflightStub{report: mutateReport(greenReport(), 999)} // same UserID
	assertGate(t, main, nonOp, cfg, "G10", 7)
}

func TestPreflight_G10_NonOpInOperatorAllowlist_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	main := &preflightStub{report: greenReport()}                     // 999
	nonOp := &preflightStub{report: mutateReport(greenReport(), 555)} // distinct user-id
	t.Setenv("HERALD_OPERATOR_IDS", "111,555,222")                    // 555 is the non-op bot → defeats S9
	assertGate(t, main, nonOp, cfg, "G10", 7)
}

func TestPreflight_G10_NonOpDistinctAndNotInAllowlist_PASS(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	main := &preflightStub{report: greenReport()}                     // 999
	nonOp := &preflightStub{report: mutateReport(greenReport(), 555)} // distinct
	t.Setenv("HERALD_OPERATOR_IDS", "111,222")                        // 555 absent → OK
	if err := runPreflight(context.Background(), main, nonOp, cfg, func(string, string, any) {}); err != nil {
		t.Fatalf("distinct non-op bot not in allowlist should pass G10, got %v", err)
	}
}

func TestPreflight_G10_NonOpGetMeError_FAIL(t *testing.T) {
	cfg := baseCfg(t, preflightDocsDir(t))
	main := &preflightStub{report: greenReport()}
	nonOp := &preflightStub{err: errors.New("non-op token invalid")}
	assertGate(t, main, nonOp, cfg, "G10", 7)
}

// mutateReport clones r with a distinct UserID + a distinct username
// (so the non-op bot does not also trip G5/G10-username collisions).
func mutateReport(r messenger.PreflightReport, userID int64) messenger.PreflightReport {
	r.UserID = userID
	r.Username = fmt.Sprintf("herald_qa_bot_%d", userID)
	return r
}

// TestPreflight_G2_RealHTTPStub_FAIL drives the FULL TelegramClient
// against an httptest server that returns a 401 on getMe — proving the
// validator's G2 gate fires on a REAL wire round-trip, not just a stub
// struct. §107 anchor: a no-op validator returning nil would FAIL here.
func TestPreflight_G2_RealHTTPStub_FAIL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	}))
	defer ts.Close()

	msgr, err := messenger.NewTelegramClient("TOK", 999, ts.URL)
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer msgr.Close()

	cfg := baseCfg(t, preflightDocsDir(t))
	cfg.QABotToken = "TOK"
	err = runPreflight(context.Background(), msgr, nil, cfg, func(string, string, any) {})
	if err == nil {
		t.Fatal("expected G2 failure against a 401 getMe, got nil")
	}
	var pe *PreflightError
	if !errors.As(err, &pe) || pe.Gate != "G2" {
		t.Fatalf("expected *PreflightError Gate=G2, got %v", err)
	}
}

// TestPreflight_G2_RealHTTPStub_PASS drives the FULL TelegramClient
// against an httptest server with green getMe + getChat +
// getChatAdministrators, proving every gate that reads the messenger
// report is satisfied by REAL wire bytes.
func TestPreflight_G2_RealHTTPStub_PASS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":999,"is_bot":true,"username":"herald_qa_bot","can_read_all_group_messages":true}}`))
		case strings.Contains(r.URL.Path, "getChatAdministrators"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"user":{"id":111,"is_bot":true,"username":"pherald_bot"}}]}`))
		case strings.Contains(r.URL.Path, "getChat"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":999,"type":"supergroup","title":"QA"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	msgr, err := messenger.NewTelegramClient("TOK", 999, ts.URL)
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer msgr.Close()

	cfg := baseCfg(t, preflightDocsDir(t))
	cfg.QABotToken = "TOK"
	if err := runPreflight(context.Background(), msgr, nil, cfg, func(string, string, any) {}); err != nil {
		t.Fatalf("expected green preflight against real wire stub, got %v", err)
	}
}
