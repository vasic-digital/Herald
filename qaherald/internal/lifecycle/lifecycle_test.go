// Unit tests for the lifecycle package (T3+T4+T5).
//
// Tests are hermetic — they use httptest to impersonate the Telegram
// Bot API + writes canned pherald transcripts and Issues.md/Fixed.md
// state under t.TempDir(). No live network or real bot tokens.
//
// §107 anti-bluff posture: each test asserts on observable byte
// streams (the report.md contents, the transcript JSONL bytes, the
// scenario Result struct fields) — never on log lines or stub
// metadata. A scenario that silently no-ops the inner call would
// fail because the captured Result has empty InboundMessageID.
package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// TestRegistry_15Scenarios is the load-bearing §107 anchor: the
// scenario count MUST equal 15. A mutation that drops a scenario
// (or duplicates one without changing the count) fails here.
func TestRegistry_15Scenarios(t *testing.T) {
	scenarios := Registry()
	if got, want := len(scenarios), 15; got != want {
		t.Fatalf("Registry size: got %d, want %d", got, want)
	}
	for i, s := range scenarios {
		wantID := fmt.Sprintf("S%02d", i+1)
		if s.ID != wantID {
			t.Errorf("scenarios[%d].ID = %q, want %q", i, s.ID, wantID)
		}
		if s.Run == nil {
			t.Errorf("scenarios[%d] (%s): Run is nil", i, s.ID)
		}
	}
}

// TestByID_LookupHits ensures every registered ID is resolvable.
func TestByID_LookupHits(t *testing.T) {
	for _, s := range Registry() {
		got, ok := ByID(s.ID)
		if !ok {
			t.Errorf("ByID(%q) miss", s.ID)
			continue
		}
		if got.Name != s.Name {
			t.Errorf("ByID(%q).Name = %q, want %q", s.ID, got.Name, s.Name)
		}
	}
}

// TestByName_LookupHits ensures every registered Name resolves.
func TestByName_LookupHits(t *testing.T) {
	for _, s := range Registry() {
		got, ok := ByName(s.Name)
		if !ok {
			t.Errorf("ByName(%q) miss", s.Name)
		}
		if got.ID != s.ID {
			t.Errorf("ByName(%q).ID = %q, want %q", s.Name, got.ID, s.ID)
		}
	}
}

// TestExtractHRDID covers the happy path + the no-match + ambiguous
// cases. The ambiguous case is the §107 anti-bluff anchor — pherald
// is expected to reply with EXACTLY one HRD-NNN.
func TestExtractHRDID(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wantID string
		errSub string
	}{
		{"clean-single", "Opened HRD-123 for telemetry", "HRD-123", ""},
		{"no-match", "no ticket id here", "", "no HRD-NNN"},
		{"ambiguous", "Closed HRD-1 reopened HRD-2", "HRD-1", "ambiguous"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractHRDID(tc.input)
			if got != tc.wantID {
				t.Errorf("ID: got %q, want %q", got, tc.wantID)
			}
			if tc.errSub == "" && err != nil {
				t.Errorf("unexpected err: %v", err)
			}
			if tc.errSub != "" {
				if err == nil {
					t.Fatalf("expected err containing %q, got nil", tc.errSub)
				}
				if !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("err %q missing %q", err.Error(), tc.errSub)
				}
			}
		})
	}
}

// TestAssertIssuesDelta covers add/remove/zero deltas + the hunk
// rendering. Synthetic Issues.md fragments avoid touching the real
// docs/Issues.md.
func TestAssertIssuesDelta(t *testing.T) {
	before := []byte(`| HRD-001 | bug | open | low | foo | 2026 | 2026 | x |
| HRD-002 | task | open | low | bar | 2026 | 2026 | x |
`)
	afterAdd := []byte(string(before) + `| HRD-003 | bug | open | low | baz | 2026 | 2026 | x |
`)
	afterRm := []byte(`| HRD-001 | bug | open | low | foo | 2026 | 2026 | x |
`)

	if _, err := assertIssuesDelta(before, before, 0); err != nil {
		t.Errorf("zero-delta: %v", err)
	}
	if _, err := assertIssuesDelta(before, afterAdd, 1); err != nil {
		t.Errorf("add-1-delta: %v", err)
	}
	if _, err := assertIssuesDelta(before, afterRm, -1); err != nil {
		t.Errorf("remove-1-delta: %v", err)
	}

	// Wrong expectation: actual=+1, expected=0 → MUST fail.
	if hunk, err := assertIssuesDelta(before, afterAdd, 0); err == nil {
		t.Errorf("expected delta-mismatch error, got nil; hunk=%q", hunk)
	}
}

// TestBuildHRDHunk verifies the hunk contains the added/removed row
// markers we render in the report.
func TestBuildHRDHunk(t *testing.T) {
	before := []byte(`| HRD-001 | bug | open | low | foo | 2026 | 2026 | x |
`)
	after := []byte(`| HRD-001 | bug | open | low | foo | 2026 | 2026 | x |
| HRD-002 | task | open | low | bar | 2026 | 2026 | x |
`)
	hunk := buildHRDHunk(before, after)
	if !strings.Contains(hunk, "+ ") || !strings.Contains(hunk, "HRD-002") {
		t.Errorf("hunk missing + HRD-002 marker: %q", hunk)
	}
}

// fakeMessenger is a hermetic MessengerClient stub used by the
// scenario tests. It records what was sent and pre-feeds canned
// Reply responses to WaitForReply.
type fakeMessenger struct {
	mu         sync.Mutex
	sent       []string
	sentPhotos []string
	sentDocs   []string
	sentVoice  []string
	repliesQ   []messenger.Reply
	nextMsgID  int64
}

func (f *fakeMessenger) Me(ctx context.Context) (string, int64, error) {
	return "fake_qa_bot", 999, nil
}
func (f *fakeMessenger) Send(ctx context.Context, text string) (messenger.MessageID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	id := atomic.AddInt64(&f.nextMsgID, 1)
	return messenger.MessageID(fmt.Sprintf("%d", id)), nil
}
func (f *fakeMessenger) SendPhoto(ctx context.Context, path, caption string) (messenger.MessageID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentPhotos = append(f.sentPhotos, path+"|"+caption)
	id := atomic.AddInt64(&f.nextMsgID, 1)
	return messenger.MessageID(fmt.Sprintf("%d", id)), nil
}
func (f *fakeMessenger) SendDocument(ctx context.Context, path, caption string) (messenger.MessageID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentDocs = append(f.sentDocs, path+"|"+caption)
	id := atomic.AddInt64(&f.nextMsgID, 1)
	return messenger.MessageID(fmt.Sprintf("%d", id)), nil
}
func (f *fakeMessenger) SendVoice(ctx context.Context, path string) (messenger.MessageID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentVoice = append(f.sentVoice, path)
	id := atomic.AddInt64(&f.nextMsgID, 1)
	return messenger.MessageID(fmt.Sprintf("%d", id)), nil
}
func (f *fakeMessenger) WaitForReply(ctx context.Context, _ messenger.MessageID, pred messenger.Predicate, timeout time.Duration) (messenger.Reply, error) {
	deadline := time.Now().Add(timeout)
	for {
		f.mu.Lock()
		for i, r := range f.repliesQ {
			if pred(r) {
				f.repliesQ = append(f.repliesQ[:i], f.repliesQ[i+1:]...)
				f.mu.Unlock()
				return r, nil
			}
		}
		f.mu.Unlock()
		if time.Now().After(deadline) {
			return messenger.Reply{}, context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return messenger.Reply{}, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
func (f *fakeMessenger) GetUpdates(ctx context.Context, offset int64) ([]messenger.Reply, int64, error) {
	return nil, offset, nil
}
func (f *fakeMessenger) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("fake: Download not supported")
}
func (f *fakeMessenger) Preflight(ctx context.Context, expectedChatID int64) (messenger.PreflightReport, error) {
	return messenger.PreflightReport{
		Username:                "fake_qa_bot",
		UserID:                  999,
		CanReadAllGroupMessages: true,
		InChat:                  true,
		ChatType:                "supergroup",
		PheraldBotPresent:       true,
	}, nil
}
func (f *fakeMessenger) Close() error { return nil }

// Ensure the fake satisfies the interface at compile time.
var _ messenger.MessengerClient = (*fakeMessenger)(nil)

// writeCannedPheraldTranscript creates an Issues/Fixed/transcript
// state under qaDir that a scenario can scan.
func writeCannedPheraldTranscript(t *testing.T, qaDir string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(qaDir, 0o755); err != nil {
		t.Fatalf("mkdir qa-dir: %v", err)
	}
	path := filepath.Join(qaDir, "transcript.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	for _, l := range lines {
		fmt.Fprintln(f, l)
	}
	f.Close()
}

// TestRunS01_HappyPath drives a single scenario end-to-end using a
// fakeMessenger + canned pherald transcript + temporary Issues/Fixed.
// Asserts the scenario PASSes and the Result carries the expected
// classification + evidence fragments.
func TestRunS01_HappyPath(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	qaOut := filepath.Join(dir, "qa-out")
	_ = os.MkdirAll(docs, 0o755)
	_ = os.WriteFile(filepath.Join(docs, "Issues.md"), []byte(`| HRD-001 | bug | open | low | x | 2026 | 2026 | x |
`), 0o644)
	_ = os.WriteFile(filepath.Join(docs, "Fixed.md"), []byte{}, 0o644)

	writeCannedPheraldTranscript(t, qaOut, []string{
		`{"ts":"2026-05-23T13:00:00Z","direction":"out","kind":"cc.dispatch","payload":{"classification":{"Type":"query","Criticality":"middle","Confidence":0}}}`,
	})

	fm := &fakeMessenger{}
	fm.repliesQ = []messenger.Reply{
		{MessageID: "1001", SenderUsername: "pherald_bot", Text: "ok"},
	}

	env := &Env{
		Msgr:           fm,
		PheraldBotUser: "pherald_bot",
		DocsDir:        docs,
		PheraldQADir:   qaOut,
		ChatID:         1,
		PerTimeout:     2 * time.Second,
	}
	scenarioStartOffset.Store(env, int64(0))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := runS01(ctx, env)
	if !res.PASS {
		t.Fatalf("S01 expected PASS, got FAIL: %s", res.FailureReason)
	}
	if res.ClassificationSeen != "query" {
		t.Errorf("ClassificationSeen = %q, want query", res.ClassificationSeen)
	}
	if len(res.Evidence) == 0 {
		t.Errorf("Evidence empty — §107 anchor missing")
	}
}

// TestRunS09_SkipsWhenNonOpNil asserts S09 emits a SKIP-with-reason
// Result when env.MsgrNonOp is nil — per §11.4.5.
func TestRunS09_SkipsWhenNonOpNil(t *testing.T) {
	env := &Env{
		Msgr:         &fakeMessenger{},
		MsgrNonOp:    nil,
		LastOpenedHRD: "HRD-999",
		PerTimeout:   1 * time.Second,
	}
	res := runS09(context.Background(), env)
	if res.PASS {
		t.Errorf("S09 with nil MsgrNonOp must not PASS")
	}
	if !strings.HasPrefix(res.FailureReason, "SKIP:") {
		t.Errorf("FailureReason: got %q, want SKIP: prefix", res.FailureReason)
	}
}

// TestWriteReport_RendersAllSections constructs synthetic Results
// (PASS, FAIL, SKIP) and asserts the rendered Markdown contains the
// per-scenario sections, the summary counts, and the aggregate table.
func TestWriteReport_RendersAllSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	results := []Result{
		{ScenarioID: "S01", PASS: true, ClassificationSeen: "query",
			InboundMessageID: "10", ReplyMessageID: "11",
			Evidence: []EvidenceFragment{{Kind: "reply-text", Content: "ok"}}},
		{ScenarioID: "S05", PASS: false, FailureReason: "Issues.md delta: expected +1 got 0"},
		{ScenarioID: "S09", FailureReason: "SKIP: HERALD_QA_BOT_TOKEN_NON_OPERATOR unset"},
	}
	if err := writeReport(path, "test-run", nil, "pherald_bot", results); err != nil {
		t.Fatalf("writeReport: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	s := string(body)
	for _, want := range []string{
		"# Lifecycle Report",
		"test-run",
		"pherald_bot",
		"1 PASS",
		"1 FAIL",
		"1 SKIP",
		"### S01 — PASS",
		"### S05 — FAIL",
		"### S09 — SKIP",
		"Aggregate",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report missing %q\n--- report ---\n%s", want, s)
		}
	}
}

// TestFilterScenarios_ByID_CaseInsensitive verifies the orchestrator's
// scenario-subset filter is case-insensitive and trims whitespace.
func TestFilterScenarios_ByID_CaseInsensitive(t *testing.T) {
	all := Registry()
	got := filterScenarios(all, []string{"s01", " S15 ", "s07"})
	if len(got) != 3 {
		t.Fatalf("filter got %d, want 3 (%v)", len(got), got)
	}
	wantIDs := map[string]bool{"S01": true, "S07": true, "S15": true}
	for _, s := range got {
		if !wantIDs[s.ID] {
			t.Errorf("unexpected scenario ID in filter result: %s", s.ID)
		}
	}
}

// TestMatchesClassification_CCAndFastPath covers both branches:
// cc.dispatch + Classification.Type direct match, and tgram.send_reply
// + payload.text substring match.
func TestMatchesClassification_CCAndFastPath(t *testing.T) {
	// CC dispatch case.
	cc := mustJSON(t, map[string]any{
		"kind": "cc.dispatch",
		"payload": map[string]any{
			"classification": map[string]any{
				"Type":        "bug",
				"Criticality": "middle",
				"Confidence":  1,
			},
		},
	})
	if !matchesClassification(cc, "bug") {
		t.Error("cc.dispatch + Type=bug should match expected=bug")
	}
	if matchesClassification(cc, "task") {
		t.Error("cc.dispatch + Type=bug must NOT match expected=task")
	}

	// Fast-path tgram.send_reply case.
	fp := mustJSON(t, map[string]any{
		"kind": "tgram.send_reply",
		"payload": map[string]any{
			"text": "Command catalogue (§32.6): Bug:, Task:, ...",
		},
	})
	if !matchesClassification(fp, "Command catalogue") {
		t.Error("tgram.send_reply with catalogue substring should match")
	}
	if matchesClassification(fp, "Nonexistent") {
		t.Error("tgram.send_reply without matching substring must NOT match")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestOrchestrator_HappyPath_NoneScenarios drives Run with --scenarios
// filtered to a subset that doesn't exist — proves the orchestrator
// runs zero scenarios, writes report.md + transcript.jsonl, and
// returns nil.
//
// Uses a real TelegramClient configured against an httptest server
// that 404s any unexpected call. Preflight is skipped.
func TestOrchestrator_HappyPath_NoneScenarios(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	tmpOut := t.TempDir()
	cfg := Config{
		QABotToken:         "fake:TOKEN",
		ChatID:             1,
		PheraldBotUsername: "pherald_bot",
		OutDir:             tmpOut,
		RunID:              "test-run",
		DocsDir:            t.TempDir(),
		Scenarios:          []string{"NONEXISTENT"},
		PerScenarioTimeout: 1 * time.Second,
		SkipPreflight:      true,
		BotAPIBaseURL:      ts.URL,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{
		filepath.Join(tmpOut, "report.md"),
		filepath.Join(tmpOut, "transcript.jsonl"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("missing artefact %q: %v", want, err)
		}
	}
}
