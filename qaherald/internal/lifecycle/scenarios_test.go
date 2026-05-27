// Full hermetic per-scenario coverage (T7).
//
// Every one of S01..S15 gets at least:
//   - one PASS test (canned pherald transcript + queued reply makes the
//     scenario's assertions all green), and
//   - one mismatch-FAIL test (a classification mismatch, a missing
//     reply, or an fs-delta mismatch makes the scenario FAIL LOUDLY).
//
// Plus the §107-load-bearing checks the dispatch calls out explicitly:
//   - S05/S06/S08/S10 Issues.md / Fixed.md fs-mutation assertions
//     (canned before/after docs trees with the exact +1/-1 row deltas).
//   - S11/S12/S13 inbound attachment download evidence (mime substring
//     in the canned pherald transcript).
//   - S14 outbound-attachment sha256 round-trip (real TelegramClient
//     upload → download → sha256 match; a faked download FAILs).
//
// §107 anti-bluff posture: every assertion is on Result struct bytes
// (PASS bool, ClassificationSeen string, Evidence content) or on real
// wire bytes (the sha256 round-trip). A scenario that no-oped its inner
// IO would surface as an empty InboundMessageID or a missing Evidence
// fragment, failing these tests.
package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// scenarioHarness wires a temp docs tree + a canned pherald transcript
// + a fakeMessenger into an *Env ready to drive a scenario. The
// scenarioStartOffset map is seeded to 0 so awaitClassification scans
// the whole canned transcript.
type scenarioHarness struct {
	env   *Env
	fm    *fakeMessenger
	docs  string
	qaOut string
}

// newScenarioHarness builds the harness. issuesBefore/fixedBefore are
// the docs file contents BEFORE the scenario runs. transcriptLines are
// the canned pherald journal lines the scenario will observe. replies
// are queued for WaitForReply.
func newScenarioHarness(t *testing.T, issuesBefore, fixedBefore string, transcriptLines []string, replies []messenger.Reply) *scenarioHarness {
	t.Helper()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	qaOut := filepath.Join(dir, "qa-out")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docs, "Issues.md"), []byte(issuesBefore), 0o644); err != nil {
		t.Fatalf("write Issues.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docs, "Fixed.md"), []byte(fixedBefore), 0o644); err != nil {
		t.Fatalf("write Fixed.md: %v", err)
	}
	writeCannedPheraldTranscript(t, qaOut, transcriptLines)

	fm := &fakeMessenger{}
	fm.repliesQ = append(fm.repliesQ, replies...)

	env := &Env{
		Msgr:           fm,
		PheraldBotUser: "pherald_bot",
		DocsDir:        docs,
		PheraldQADir:   qaOut,
		ChatID:         1,
		PerTimeout:     2 * time.Second,
	}
	scenarioStartOffset.Store(env, int64(0))
	return &scenarioHarness{env: env, fm: fm, docs: docs, qaOut: qaOut}
}

// mutateDocsAfter schedules a rewrite of Issues.md / Fixed.md to
// simulate pherald's fs mutation that lands DURING the scenario — the
// rewrite fires on the FIRST outbound Send* call (via fakeMessenger's
// onSend hook), which is strictly AFTER the scenario's snapshot-before
// read and strictly BEFORE its snapshot-after read. This reproduces the
// real ordering: snapshot → send → pherald mutates docs → snapshot.
func (h *scenarioHarness) mutateDocsAfter(t *testing.T, issuesAfter, fixedAfter string) {
	t.Helper()
	h.fm.onSend = func() {
		if err := os.WriteFile(filepath.Join(h.docs, "Issues.md"), []byte(issuesAfter), 0o644); err != nil {
			t.Errorf("rewrite Issues.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(h.docs, "Fixed.md"), []byte(fixedAfter), 0o644); err != nil {
			t.Errorf("rewrite Fixed.md: %v", err)
		}
	}
}

// ccLine builds a canned cc.dispatch journal line with the given
// classification Type (matches matchesClassification's CC branch).
func ccLine(typ string) string {
	return fmt.Sprintf(`{"ts":"2026-05-23T13:00:00Z","direction":"out","kind":"cc.dispatch","payload":{"classification":{"Type":%q,"Criticality":"middle","Confidence":1}}}`, typ)
}

// fastPathLine builds a canned tgram.send_reply line whose payload.text
// carries substr (matches matchesClassification's fast-path branch).
func fastPathLine(text string) string {
	b, _ := json.Marshal(map[string]any{
		"ts":        "2026-05-23T13:00:00Z",
		"direction": "out",
		"kind":      "tgram.send_reply",
		"payload":   map[string]any{"text": text},
	})
	return string(b)
}

// mimeLine builds a canned line carrying an attachments mime substring
// (S11/S12/S13 scan for "image/", "application/", "audio/").
func mimeLine(mimeType string) string {
	return fmt.Sprintf(`{"ts":"2026-05-23T13:00:00Z","direction":"in","kind":"tgram.message","payload":{"attachments":[{"mime":%q,"sha256":"deadbeef"}]}}`, mimeType)
}

const oneRow = "| HRD-001 | bug | open | low | x | 2026 | 2026 | x |\n"
const twoRows = oneRow + "| HRD-002 | task | open | low | y | 2026 | 2026 | x |\n"

func pheraldReply(text string) messenger.Reply {
	return messenger.Reply{MessageID: "1001", SenderUsername: "pherald_bot", Text: text}
}

func runScenario(t *testing.T, run ScenarioRun, env *Env) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return run(ctx, env)
}

func mustPASS(t *testing.T, r Result) {
	t.Helper()
	if !r.PASS {
		t.Fatalf("%s expected PASS, got FAIL: %s", r.ScenarioID, r.FailureReason)
	}
	if len(r.Evidence) == 0 {
		t.Errorf("%s PASS but no Evidence fragments — §107 anchor missing", r.ScenarioID)
	}
}

func mustFAIL(t *testing.T, r Result, reasonSub string) {
	t.Helper()
	if r.PASS {
		t.Fatalf("%s expected FAIL, got PASS", r.ScenarioID)
	}
	if strings.HasPrefix(r.FailureReason, "SKIP:") {
		t.Fatalf("%s expected FAIL, got SKIP: %s", r.ScenarioID, r.FailureReason)
	}
	if reasonSub != "" && !strings.Contains(r.FailureReason, reasonSub) {
		t.Errorf("%s FailureReason %q missing %q", r.ScenarioID, r.FailureReason, reasonSub)
	}
}

// ===================================================================
// S01 — plain greeting → query fallthrough
// ===================================================================

func TestS01_PASS(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("query")}, []messenger.Reply{pheraldReply("hi back")})
	r := runScenario(t, runS01, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "query" {
		t.Errorf("ClassificationSeen = %q, want query", r.ClassificationSeen)
	}
}

func TestS01_ClassificationMismatch_FAIL(t *testing.T) {
	// Transcript says "bug" but S01 expects "query" → awaitClassification times out.
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("bug")}, []messenger.Reply{pheraldReply("hi")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS01, h.env)
	mustFAIL(t, r, "await-classification")
}

// ===================================================================
// S02/S03/S04 — fast-path Help/Status/Continue
// ===================================================================

func TestS02_PASS(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{fastPathLine("Command catalogue (§32.6): Bug:, Task:")},
		[]messenger.Reply{pheraldReply("Command catalogue (§32.6): Bug:, Task:")})
	r := runScenario(t, runS02, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "help_command" {
		t.Errorf("ClassificationSeen = %q, want help_command", r.ClassificationSeen)
	}
}

func TestS02_NoCatalogue_FAIL(t *testing.T) {
	// Reply lacks the "Command catalogue" substring → awaitReplyWithSubstring times out.
	h := newScenarioHarness(t, oneRow, "", []string{}, []messenger.Reply{pheraldReply("unrelated")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS02, h.env)
	mustFAIL(t, r, "")
}

func TestS03_PASS(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{fastPathLine("Status: r10 active — Herald")},
		[]messenger.Reply{pheraldReply("Status: r10 active")})
	r := runScenario(t, runS03, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "status_request" {
		t.Errorf("ClassificationSeen = %q, want status_request", r.ClassificationSeen)
	}
}

func TestS03_NoFastPathLine_FAIL(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{}, []messenger.Reply{pheraldReply("Status: x")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS03, h.env)
	mustFAIL(t, r, "await fast-path journal")
}

func TestS04_PASS(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{fastPathLine("# Continuation\n\nNext steps ...")},
		[]messenger.Reply{pheraldReply("# Continuation prose")})
	r := runScenario(t, runS04, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "continuation_request" {
		t.Errorf("ClassificationSeen = %q, want continuation_request", r.ClassificationSeen)
	}
}

func TestS04_NoFastPathLine_FAIL(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{}, []messenger.Reply{pheraldReply("x")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS04, h.env)
	mustFAIL(t, r, "await fast-path journal")
}

// ===================================================================
// S05/S06 — Bug:/Task: prefix → issue.open → Issues.md +1
// ===================================================================

func TestS05_PASS_IssuesPlusOne(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("bug")},
		[]messenger.Reply{pheraldReply("Opened HRD-002 for telemetry pipe")})
	// pherald appends HRD-002 to Issues.md during the scenario.
	h.mutateDocsAfter(t, twoRows, "")
	r := runScenario(t, runS05, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "bug" {
		t.Errorf("ClassificationSeen = %q, want bug", r.ClassificationSeen)
	}
	if h.env.LastOpenedHRD != "HRD-002" {
		t.Errorf("LastOpenedHRD = %q, want HRD-002 (S08/S10 depend on this)", h.env.LastOpenedHRD)
	}
	// §107: the issues-diff hunk MUST be cited.
	if !hasEvidenceKind(r, "issues-diff") {
		t.Errorf("S05 PASS missing issues-diff evidence fragment")
	}
}

func TestS05_NoIssuesMutation_FAIL(t *testing.T) {
	// Reply + classification are right, but Issues.md does NOT grow → FAIL.
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("bug")},
		[]messenger.Reply{pheraldReply("Opened HRD-002 for telemetry pipe")})
	// no mutateDocsAfter → Issues.md stays at 1 row, expected +1.
	r := runScenario(t, runS05, h.env)
	mustFAIL(t, r, "Issues.md delta")
}

func TestS06_PASS_IssuesPlusOne(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("task")},
		[]messenger.Reply{pheraldReply("Opened HRD-002 for channel registry refactor")})
	h.mutateDocsAfter(t, twoRows, "")
	r := runScenario(t, runS06, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "task" {
		t.Errorf("ClassificationSeen = %q, want task", r.ClassificationSeen)
	}
}

func TestS06_ClassificationMismatch_FAIL(t *testing.T) {
	// Transcript says "bug" but S06 expects "task".
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("bug")},
		[]messenger.Reply{pheraldReply("Opened HRD-002")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS06, h.env)
	mustFAIL(t, r, "await-classification (task)")
}

// ===================================================================
// S07 — Query: prefix → explicit query, no fs mutation
// ===================================================================

func TestS07_PASS(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("query")},
		[]messenger.Reply{pheraldReply("current tag is v0.5.0")})
	r := runScenario(t, runS07, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "query" {
		t.Errorf("ClassificationSeen = %q, want query", r.ClassificationSeen)
	}
}

func TestS07_IssuesMutatedUnexpectedly_FAIL(t *testing.T) {
	// A Query: must NOT mutate Issues.md; simulate an erroneous +1.
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("query")},
		[]messenger.Reply{pheraldReply("answer")})
	h.mutateDocsAfter(t, twoRows, "")
	r := runScenario(t, runS07, h.env)
	mustFAIL(t, r, "unexpectedly mutated")
}

// ===================================================================
// S08 — Done: → Issues -1, Fixed +1 (fs migration)
// ===================================================================

func TestS08_PASS_AtomicMigration(t *testing.T) {
	h := newScenarioHarness(t, twoRows, "", []string{ccLine("closure")},
		[]messenger.Reply{pheraldReply("Migrated HRD-002 to Fixed")})
	h.env.LastOpenedHRD = "HRD-002"
	// pherald removes HRD-002 from Issues, adds it to Fixed.
	h.mutateDocsAfter(t, oneRow, "| HRD-002 | task | fixed | low | y | 2026 | 2026 | x |\n")
	r := runScenario(t, runS08, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "closure" {
		t.Errorf("ClassificationSeen = %q, want closure", r.ClassificationSeen)
	}
	// §107: BOTH diff hunks MUST be cited.
	if !hasEvidenceKind(r, "issues-diff") || !hasEvidenceKind(r, "fixed-diff") {
		t.Errorf("S08 PASS missing issues-diff or fixed-diff evidence")
	}
}

func TestS08_NoFixedDelta_FAIL(t *testing.T) {
	h := newScenarioHarness(t, twoRows, "", []string{ccLine("closure")},
		[]messenger.Reply{pheraldReply("Migrated HRD-002 to Fixed")})
	h.env.LastOpenedHRD = "HRD-002"
	// Issues drops a row but Fixed does NOT grow → FAIL on the Fixed +1 delta.
	h.mutateDocsAfter(t, oneRow, "")
	r := runScenario(t, runS08, h.env)
	mustFAIL(t, r, "Fixed.md delta")
}

func TestS08_MissingPrereq_FAIL(t *testing.T) {
	h := newScenarioHarness(t, twoRows, "", []string{}, nil)
	h.env.LastOpenedHRD = "" // prerequisite missing
	r := runScenario(t, runS08, h.env)
	mustFAIL(t, r, "prerequisite missing")
}

// ===================================================================
// S09 — Done: from non-operator → rejection (and SKIP when no 2nd bot)
// ===================================================================

func TestS09_PASS_Rejection(t *testing.T) {
	nonOp := &fakeMessenger{}
	op := &fakeMessenger{}
	op.repliesQ = []messenger.Reply{
		{MessageID: "2002", SenderUsername: "pherald_bot",
			Text: "Forbidden — operator role required (HERALD_OPERATOR_IDS)"},
	}
	env := &Env{
		Msgr:           op,
		MsgrNonOp:      nonOp,
		PheraldBotUser: "pherald_bot",
		LastOpenedHRD:  "HRD-002",
		PerTimeout:     2 * time.Second,
	}
	r := runScenario(t, runS09, env)
	mustPASS(t, r)
	if r.ActionSeen != "reject (non-operator)" {
		t.Errorf("ActionSeen = %q, want reject (non-operator)", r.ActionSeen)
	}
}

func TestS09_NoRejectionReply_FAIL(t *testing.T) {
	nonOp := &fakeMessenger{}
	op := &fakeMessenger{} // no rejection reply queued
	env := &Env{
		Msgr:           op,
		MsgrNonOp:      nonOp,
		PheraldBotUser: "pherald_bot",
		LastOpenedHRD:  "HRD-002",
		PerTimeout:     300 * time.Millisecond,
	}
	r := runScenario(t, runS09, env)
	mustFAIL(t, r, "await rejection reply")
}

// S09 SKIP path is already covered by TestRunS09_SkipsWhenNonOpNil in
// lifecycle_test.go; assert it here too for completeness via the
// harness convention.
func TestS09_SkipsWhenNonOpNil(t *testing.T) {
	env := &Env{Msgr: &fakeMessenger{}, MsgrNonOp: nil, LastOpenedHRD: "HRD-002", PerTimeout: time.Second}
	r := runS09(context.Background(), env)
	if r.PASS {
		t.Fatal("S09 with nil MsgrNonOp must not PASS")
	}
	if !strings.HasPrefix(r.FailureReason, "SKIP:") {
		t.Errorf("FailureReason = %q, want SKIP: prefix", r.FailureReason)
	}
}

// ===================================================================
// S10 — Reopen: → Fixed -1, Issues +1
// ===================================================================

func TestS10_PASS_ReopenMigration(t *testing.T) {
	fixedBefore := "| HRD-002 | task | fixed | low | y | 2026 | 2026 | x |\n"
	h := newScenarioHarness(t, oneRow, fixedBefore, []string{ccLine("reopen")},
		[]messenger.Reply{pheraldReply("Migrated HRD-002 back to Issues")})
	h.env.LastOpenedHRD = "HRD-002"
	// pherald moves HRD-002 from Fixed back to Issues.
	h.mutateDocsAfter(t, twoRows, "")
	r := runScenario(t, runS10, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "reopen" {
		t.Errorf("ClassificationSeen = %q, want reopen", r.ClassificationSeen)
	}
}

func TestS10_NoIssuesDelta_FAIL(t *testing.T) {
	fixedBefore := "| HRD-002 | task | fixed | low | y | 2026 | 2026 | x |\n"
	h := newScenarioHarness(t, oneRow, fixedBefore, []string{ccLine("reopen")},
		[]messenger.Reply{pheraldReply("Migrated HRD-002")})
	h.env.LastOpenedHRD = "HRD-002"
	// Issues does NOT grow → FAIL on the +1 delta.
	r := runScenario(t, runS10, h.env)
	mustFAIL(t, r, "Issues.md delta")
}

// ===================================================================
// S11/S12/S13 — inbound attachment scenarios (mime evidence)
// ===================================================================

func TestS11_PASS_PhotoBugCaption(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("bug"), mimeLine("image/jpeg")},
		[]messenger.Reply{pheraldReply("Opened HRD-002 with attachment")})
	r := runScenario(t, runS11, h.env)
	mustPASS(t, r)
	if len(h.fm.sentPhotos) != 1 {
		t.Errorf("S11 should send exactly one photo, sent %d", len(h.fm.sentPhotos))
	}
	if !hasEvidenceKind(r, "attachment-mime-line") {
		t.Errorf("S11 PASS missing attachment-mime-line evidence")
	}
}

func TestS11_NoMimeEvidence_FAIL(t *testing.T) {
	// Classification arrives but no image/* mime line → FAIL.
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("bug")},
		[]messenger.Reply{pheraldReply("ok")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS11, h.env)
	mustFAIL(t, r, "image/")
}

func TestS12_PASS_DocumentTaskCaption(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("task"), mimeLine("application/pdf")},
		[]messenger.Reply{pheraldReply("Opened HRD-002 with attachment")})
	r := runScenario(t, runS12, h.env)
	mustPASS(t, r)
	if len(h.fm.sentDocs) != 1 {
		t.Errorf("S12 should send exactly one document, sent %d", len(h.fm.sentDocs))
	}
}

func TestS12_NoMimeEvidence_FAIL(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("task")},
		[]messenger.Reply{pheraldReply("ok")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS12, h.env)
	mustFAIL(t, r, "application/")
}

func TestS13_PASS_VoiceAudio(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{mimeLine("audio/ogg")},
		[]messenger.Reply{pheraldReply("got your voice note")})
	r := runScenario(t, runS13, h.env)
	mustPASS(t, r)
	if len(h.fm.sentVoice) != 1 {
		t.Errorf("S13 should send exactly one voice clip, sent %d", len(h.fm.sentVoice))
	}
}

func TestS13_NoAudioEvidence_FAIL(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{},
		[]messenger.Reply{pheraldReply("ok")})
	h.env.PerTimeout = 300 * time.Millisecond
	r := runScenario(t, runS13, h.env)
	mustFAIL(t, r, "audio/")
}

// ===================================================================
// S14 — outbound attachment fan-out (transcript evidence)
// ===================================================================

func TestS14_PASS_OutboundAttachment(t *testing.T) {
	line := `{"ts":"2026-05-23T13:00:00Z","direction":"out","kind":"tgram.send_reply","payload":{"text":"see attached","attachments":[{"mime":"image/png","sha256":"abc"}]}}`
	h := newScenarioHarness(t, oneRow, "", []string{line},
		[]messenger.Reply{pheraldReply("reply with attachment")})
	r := runScenario(t, runS14, h.env)
	mustPASS(t, r)
	if !hasEvidenceKind(r, "send-reply-line") {
		t.Errorf("S14 PASS missing send-reply-line evidence")
	}
}

func TestS14_SendReplyWithoutAttachments_FAIL(t *testing.T) {
	// tgram.send_reply present but NO attachments field → FAIL.
	line := `{"ts":"2026-05-23T13:00:00Z","direction":"out","kind":"tgram.send_reply","payload":{"text":"no attachment here"}}`
	h := newScenarioHarness(t, oneRow, "", []string{line},
		[]messenger.Reply{pheraldReply("reply")})
	r := runScenario(t, runS14, h.env)
	mustFAIL(t, r, "no attachments field")
}

// TestS14_OutboundSha256RoundTrip is the §107 anchor the dispatch calls
// out by name: upload a known file via the REAL TelegramClient, then
// download it back by file-id and assert the sha256 matches. A faked
// download (returning different bytes) would FAIL this test.
func TestS14_OutboundSha256RoundTrip(t *testing.T) {
	// Known payload — compute its sha256 up front.
	payload := []byte("HERALD-S14-OUTBOUND-ATTACHMENT-FIXTURE-PNG-BYTES")
	sum := sha256.Sum256(payload)
	wantSha := hex.EncodeToString(sum[:])

	const fileID = "FILE_ID_S14"
	const filePath = "photos/s14.png"

	var uploadHits int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "sendPhoto"):
			atomic.AddInt64(&uploadHits, 1)
			// Verify the uploaded multipart bytes match the payload.
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				http.Error(w, "bad multipart", 400)
				return
			}
			mr := multipart.NewReader(r.Body, params["boundary"])
			var got []byte
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				if p.FormName() == "photo" {
					got, _ = io.ReadAll(p)
				}
			}
			if !bytes.Equal(got, payload) {
				http.Error(w, "uploaded bytes mismatch", 400)
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":true,"result":{"message_id":7,"chat":{"id":1,"type":"group"},"photo":[{"file_id":%q,"file_size":%d}]}}`, fileID, len(payload))))
		case strings.Contains(r.URL.Path, "getFile"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":true,"result":{"file_id":%q,"file_path":%q}}`, fileID, filePath)))
		case strings.Contains(r.URL.Path, "/file/"):
			// File download endpoint — return the exact payload bytes.
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c, err := messenger.NewTelegramClient("TOK", 1, ts.URL)
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	// Upload.
	dir := t.TempDir()
	src := filepath.Join(dir, "s14.png")
	if err := os.WriteFile(src, payload, 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if _, err := c.SendPhoto(context.Background(), src, "round-trip"); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if atomic.LoadInt64(&uploadHits) != 1 {
		t.Fatalf("sendPhoto hit count = %d, want 1 (§107 wire-exercise anchor)", uploadHits)
	}

	// Download by file-id + assert sha256 round-trip.
	rc, err := c.Download(context.Background(), fileID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	gotBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	gotSum := sha256.Sum256(gotBytes)
	gotSha := hex.EncodeToString(gotSum[:])
	if gotSha != wantSha {
		t.Fatalf("sha256 round-trip mismatch: downloaded %s, uploaded %s — a faked download would land here", gotSha, wantSha)
	}
}

// ===================================================================
// S15 — natural-language + emoji → query fallthrough
// ===================================================================

func TestS15_PASS(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("query")},
		[]messenger.Reply{pheraldReply("sure, what's up")})
	r := runScenario(t, runS15, h.env)
	mustPASS(t, r)
	if r.ClassificationSeen != "query" {
		t.Errorf("ClassificationSeen = %q, want query", r.ClassificationSeen)
	}
}

func TestS15_IssuesMutated_FAIL(t *testing.T) {
	h := newScenarioHarness(t, oneRow, "", []string{ccLine("query")},
		[]messenger.Reply{pheraldReply("ans")})
	h.mutateDocsAfter(t, twoRows, "")
	r := runScenario(t, runS15, h.env)
	mustFAIL(t, r, "mutated on emoji fallthrough")
}

// hasEvidenceKind reports whether r carries an EvidenceFragment of the
// given Kind.
func hasEvidenceKind(r Result, kind string) bool {
	for _, e := range r.Evidence {
		if e.Kind == kind {
			return true
		}
	}
	return false
}
