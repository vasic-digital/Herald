// Helpers shared across the 15 scenarios.
//
// Each helper is intentionally small + side-effect-free so scenario
// authors can compose them without surprise. The helpers fall into
// three buckets:
//
//  1. IO: awaitReplyWithSubstring + awaitClassification — both block
//     until a pherald-side event lands or ctx expires. They return
//     errors that wrap context.DeadlineExceeded so callers can
//     errors.Is against the sentinel.
//
//  2. FS state: snapshotIssuesFixed + assertIssuesDelta — capture
//     Issues.md / Fixed.md content for before/after diffing. The diff
//     hunk is the §107 anti-bluff anchor for fs-mutation scenarios.
//
//  3. Parsing: extractHRDID — pulls "HRD-NNN" out of pherald's reply
//     text so S08/S10 can target the row S05/S06 allocated.
//
// File-handle and lock policy: snapshotIssuesFixed serialises reads
// through issuesMu (package-scoped sync.Mutex) so concurrent scenario
// snapshots cannot interleave a partial pherald write. The orchestrator
// already runs scenarios sequentially, but the mutex is cheap and
// defends against future parallel orchestration.
package lifecycle

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// issuesMu serialises Issues.md / Fixed.md reads across scenarios.
// The orchestrator runs scenarios sequentially today, but this mutex
// is the load-bearing serialization for Wave 7's planned parallel
// orchestration. See package comment.
var issuesMu sync.Mutex

// awaitReplyWithSubstring posts text via env.Msgr.Send, then polls
// the messenger inbound stream for a Reply whose text or caption
// contains substr. The toMsgID returned by Send is propagated to the
// predicate so callers can choose strict reply-chain matching.
//
// substr=="" matches the first Reply from pherald-bot regardless of
// content — useful for S01 which only asserts that SOME reply arrives.
//
// The Predicate also enforces a sender filter: only replies from
// pherald-bot count. Echo replies from qa-bot itself are ignored —
// the messenger interface contract leaves that filtering to the
// caller because some scenarios (S09) may want to see them.
func awaitReplyWithSubstring(ctx context.Context, env *Env, text string, substr string) (messenger.Reply, messenger.MessageID, error) {
	inID, err := env.Msgr.Send(ctx, text)
	if err != nil {
		return messenger.Reply{}, "", fmt.Errorf("send: %w", err)
	}

	pred := func(r messenger.Reply) bool {
		// Filter to pherald-bot replies. SenderUsername comparison is
		// case-sensitive per Telegram's convention (usernames are
		// canonically lowercase, but exact-match is the safest contract).
		if env.PheraldBotUser != "" && r.SenderUsername != env.PheraldBotUser {
			return false
		}
		if substr == "" {
			return true
		}
		return strings.Contains(r.Text, substr) || strings.Contains(r.Caption, substr)
	}

	reply, werr := env.Msgr.WaitForReply(ctx, inID, pred, env.PerTimeout)
	if werr != nil {
		return messenger.Reply{}, inID, fmt.Errorf("await-reply (substr=%q): %w", substr, werr)
	}
	return reply, inID, nil
}

// awaitReplyOnly waits for a pherald-bot reply WITHOUT sending a
// fresh inbound. Used by attachment scenarios (S11/S12/S13) which
// have already sent the photo/document/voice via SendPhoto / etc.
// and only need to observe pherald's reply.
//
// substr=="" matches the first pherald-bot reply that arrives.
// Returns context.DeadlineExceeded on timeout.
func awaitReplyOnly(ctx context.Context, env *Env, substr string) (messenger.Reply, error) {
	pred := func(r messenger.Reply) bool {
		if env.PheraldBotUser != "" && r.SenderUsername != env.PheraldBotUser {
			return false
		}
		if substr == "" {
			return true
		}
		return strings.Contains(r.Text, substr) || strings.Contains(r.Caption, substr)
	}
	reply, err := env.Msgr.WaitForReply(ctx, "", pred, env.PerTimeout)
	if err != nil {
		return messenger.Reply{}, fmt.Errorf("await-reply-only (substr=%q): %w", substr, err)
	}
	return reply, nil
}

// awaitClassification reads pherald's transcript.jsonl (the file
// pherald listen writes under --qa-out-dir) and blocks until a line
// appears with cc.dispatch + classification.Type == expectedType, OR
// kind == tgram.send_reply (for fast-path command types) referencing
// the qa-bot's inbound message_id.
//
// Polls at 500ms cadence; returns context.DeadlineExceeded on timeout.
//
// §107: the returned bytes are the verbatim JSONL line — the report
// quotes them as the per-PASS evidence anchor.
func awaitClassification(ctx context.Context, env *Env, _ messenger.MessageID, expectedType string) ([]byte, error) {
	if env.PheraldQADir == "" {
		return nil, errors.New("awaitClassification: env.PheraldQADir is empty (pherald --qa-out-dir not wired)")
	}
	path := filepath.Join(env.PheraldQADir, "transcript.jsonl")
	deadline := time.Now().Add(env.PerTimeout)

	// Use the inbound message_id-agnostic match for now — pherald's
	// transcript records `Classification.Type` per scenario but does
	// not echo the qa-bot's message_id in the same line. We match on
	// (kind == cc.dispatch AND classification.Type == expectedType)
	// observed AFTER scenario start. The orchestrator's start-of-
	// scenario file offset (taken on entry) bounds this match to
	// events emitted DURING this scenario.

	offset := int64(0)
	if startOff, ok := scenarioStartOffset.Load(env); ok {
		offset = startOff.(int64)
	}

	for {
		if time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		hit, err := scanTranscriptFromOffset(path, offset, expectedType)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("scan transcript: %w", err)
		}
		if hit != nil {
			return hit, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// scenarioStartOffset is set by the orchestrator before each
// scenario.Run() call and consumed by awaitClassification to bound
// the JSONL scan. Map key is the *Env pointer (single Env per run).
var scenarioStartOffset sync.Map

// scanTranscriptFromOffset opens path, seeks past `start` bytes, and
// scans for a JSONL line where kind == "cc.dispatch" AND
// payload.classification.Type == expected, OR (for fast-path types)
// kind matches the fast-path command kind.
//
// Returns the raw matching JSONL bytes on hit, nil + nil on no-match,
// (nil, err) on IO error. os.ErrNotExist is propagated so the caller
// can interpret "file not yet created by pherald listen".
func scanTranscriptFromOffset(path string, start int64, expectedType string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if start > 0 {
		if _, err := f.Seek(start, 0); err != nil {
			return nil, err
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if matchesClassification(line, expectedType) {
			// Defensive copy — scanner reuses its buffer.
			out := make([]byte, len(line))
			copy(out, line)
			return out, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// classificationProbe is the minimal shape we decode out of the
// pherald journal record to match Classification.Type. Capitalized
// JSON field names match pherald's Classification struct (no
// json tags, so encoder uses Go field names verbatim).
type classificationProbe struct {
	Kind    string `json:"kind"`
	Payload struct {
		Classification *struct {
			Type        string  `json:"Type"`
			Criticality string  `json:"Criticality"`
			Confidence  float64 `json:"Confidence"`
		} `json:"classification,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"payload"`
}

// matchesClassification returns true when line decodes to a
// classificationProbe whose kind+type satisfy expected. Matching is
// permissive — for fast-path types (help_command/status_request/
// continuation_request/closure/reopen) the dispatcher emits a
// tgram.send_reply (no cc.dispatch); we accept that kind too.
//
// For non-fast-path types (query/bug/task/investigation) we require
// kind == "cc.dispatch" AND classification.Type == expectedType.
//
// Note: pherald's Wave 6.5 dispatcher currently does NOT emit a
// classification.Type field for fast-path replies — only the
// `tgram.send_reply` kind. We match expected{help|status|continuation}_
// command_type by observing a tgram.send_reply whose payload.text
// contains the expected catalogue/prose substring instead. Callers
// pass expectedType containing the substring (e.g. "Command catalogue"
// for S02) when matching fast-path replies.
func matchesClassification(line []byte, expectedType string) bool {
	if !json.Valid(line) {
		return false
	}
	var probe classificationProbe
	if err := json.Unmarshal(line, &probe); err != nil {
		return false
	}
	// Direct classification match (CC path).
	if probe.Kind == "cc.dispatch" && probe.Payload.Classification != nil &&
		probe.Payload.Classification.Type == expectedType {
		return true
	}
	// Fast-path catalogue substring match (Help/Status/Continue).
	if probe.Kind == "tgram.send_reply" && expectedType != "" &&
		strings.Contains(probe.Payload.Text, expectedType) {
		return true
	}
	return false
}

// snapshotIssuesFixed reads docs/Issues.md + docs/Fixed.md under the
// issuesMu lock and returns their contents for before/after diffing.
//
// Returns (issues, fixed, nil) on success. Missing files are not
// errors — empty bytes is the canonical "empty file" representation,
// and the orchestrator should never run against a docs/ tree missing
// those files (validateLifecycleRequired catches it earlier).
func snapshotIssuesFixed(env *Env) ([]byte, []byte, error) {
	issuesMu.Lock()
	defer issuesMu.Unlock()

	docs := env.DocsDir
	if docs == "" {
		docs = "docs"
	}
	issues, err := os.ReadFile(filepath.Join(docs, "Issues.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read Issues.md: %w", err)
	}
	fixed, err := os.ReadFile(filepath.Join(docs, "Fixed.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read Fixed.md: %w", err)
	}
	return issues, fixed, nil
}

// assertIssuesDelta computes the count of HRD-NNN-prefixed table rows
// in before vs after and asserts after-before == expectedDelta.
//
// Returns (hunk, nil) on success; hunk is a unified-diff-ish slice of
// the rows that differ — the report quotes this verbatim per §107.
//
// Returns (hunk, error) on mismatch; the hunk is included so the
// failure report still cites concrete evidence.
//
// expectedDelta sign convention: +N means N rows added (S5/S6/S10);
// -N means N rows removed (S8); 0 means unchanged (S1/S2/S3/S7/S15).
func assertIssuesDelta(before, after []byte, expectedDelta int) (string, error) {
	bRows := countHRDRows(before)
	aRows := countHRDRows(after)
	actualDelta := aRows - bRows
	hunk := buildHRDHunk(before, after)
	if actualDelta != expectedDelta {
		return hunk, fmt.Errorf("HRD-row delta: expected %+d, got %+d (before=%d after=%d)",
			expectedDelta, actualDelta, bRows, aRows)
	}
	return hunk, nil
}

// hrdRowPattern matches the leading `| HRD-NNN |` cell of a table row.
// Anchored at line start. Used by countHRDRows.
var hrdRowPattern = regexp.MustCompile(`(?m)^\|\s*HRD-\d+\s*\|`)

// hrdLinePattern is a stricter variant used to pull whole rows for
// the hunk render.
var hrdLinePattern = regexp.MustCompile(`(?m)^\|\s*HRD-\d+\s*\|.*$`)

func countHRDRows(b []byte) int {
	return len(hrdRowPattern.FindAllIndex(b, -1))
}

// buildHRDHunk renders a human-readable hunk citing the rows present
// in `after` but not `before` (prefixed +) and rows present in
// `before` but not `after` (prefixed -). Order is "+ rows first, then
// - rows" so an added-only scenario reads naturally top-to-bottom.
//
// The hunk is bounded at 4096 bytes to keep reports readable; if the
// raw delta exceeds that, a tail "... (truncated)" marker is appended.
func buildHRDHunk(before, after []byte) string {
	beforeSet := map[string]bool{}
	for _, row := range hrdLinePattern.FindAllString(string(before), -1) {
		beforeSet[row] = true
	}
	afterSet := map[string]bool{}
	for _, row := range hrdLinePattern.FindAllString(string(after), -1) {
		afterSet[row] = true
	}

	var sb strings.Builder
	for row := range afterSet {
		if !beforeSet[row] {
			sb.WriteString("+ ")
			sb.WriteString(row)
			sb.WriteByte('\n')
		}
	}
	for row := range beforeSet {
		if !afterSet[row] {
			sb.WriteString("- ")
			sb.WriteString(row)
			sb.WriteByte('\n')
		}
	}
	out := sb.String()
	const maxHunk = 4096
	if len(out) > maxHunk {
		out = out[:maxHunk] + "\n... (truncated)"
	}
	return out
}

// extractHRDID parses an HRD-NNN identifier from pherald's reply.
// pherald's IssueOpener emits replies containing either
// "Opened HRD-NNN" or just "HRD-NNN" — the regex below matches both.
//
// Returns ("HRD-NNN", nil) on a single unique match. Returns
// ("", error) on no-match or ambiguous-match (multiple distinct
// HRD-NNN ids in the same reply). Ambiguity is treated as a §107
// bluff — pherald should reply with exactly one allocated ticket.
func extractHRDID(replyText string) (string, error) {
	re := regexp.MustCompile(`HRD-\d+`)
	matches := re.FindAllString(replyText, -1)
	if len(matches) == 0 {
		return "", errors.New("no HRD-NNN found in reply")
	}
	// De-dupe.
	seen := map[string]bool{}
	for _, m := range matches {
		seen[m] = true
	}
	if len(seen) > 1 {
		// Multiple distinct HRDs — ambiguous. Return the first but
		// surface a structured error so the scenario marks FAIL.
		var dedup []string
		for k := range seen {
			dedup = append(dedup, k)
		}
		return matches[0], fmt.Errorf("ambiguous HRD-NNN in reply: %v", dedup)
	}
	return matches[0], nil
}

// scanTranscriptForSubstr opens pherald's transcript.jsonl, seeks
// past the scenario-start offset, and returns the first JSONL line
// containing the given substring (raw bytes). Polls at 500ms cadence
// up to env.PerTimeout. Returns (nil, error) on IO error or timeout.
//
// Used by attachment scenarios (S11/S12/S13) to assert that pherald
// journaled an `attachments` entry referencing the mime substring
// ("image/", "application/", "audio/").
func scanTranscriptForSubstr(env *Env, substr string) ([]byte, error) {
	if env.PheraldQADir == "" {
		return nil, errors.New("scanTranscriptForSubstr: env.PheraldQADir empty")
	}
	path := filepath.Join(env.PheraldQADir, "transcript.jsonl")
	offset := int64(0)
	if startOff, ok := scenarioStartOffset.Load(env); ok {
		offset = startOff.(int64)
	}
	deadline := time.Now().Add(env.PerTimeout)
	for {
		if time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		hit, err := scanLinesForSubstr(path, offset, substr)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if hit != nil {
			return hit, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// scanLinesForSubstr seeks past start bytes and returns the first
// JSONL line whose RAW BYTES contain substr. Cheap substring scan
// over an already-fsync'd file. Returns (nil, nil) on no-match.
func scanLinesForSubstr(path string, start int64, substr string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if start > 0 {
		if _, err := f.Seek(start, 0); err != nil {
			return nil, err
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if strings.Contains(string(line), substr) {
			out := make([]byte, len(line))
			copy(out, line)
			return out, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// transcriptFileOffset returns the current byte length of pherald's
// transcript.jsonl. Used by the orchestrator to bound per-scenario
// scans (events from earlier scenarios are skipped).
//
// Returns 0 if the file does not yet exist (pherald listen has not
// emitted any events).
func transcriptFileOffset(env *Env) int64 {
	if env.PheraldQADir == "" {
		return 0
	}
	st, err := os.Stat(filepath.Join(env.PheraldQADir, "transcript.jsonl"))
	if err != nil {
		return 0
	}
	return st.Size()
}
