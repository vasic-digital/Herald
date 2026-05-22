// Package report renders the qaherald Markdown evidence report.
//
// Generate reads <runDir>/transcript.jsonl (one transcript.Event per
// line) plus <runDir>/results.json (the scenario.Result[] the T7 runner
// writes at scenario-suite end) and emits <runDir>/report.md with:
//
//   - A run-summary header (run ID inferred from the directory name,
//     start/end timestamps drawn from the first/last transcript event,
//     total duration).
//   - A PASS/FAIL summary table keyed off the actual Result[] entries —
//     §107 anti-bluff: no PASS-coercion, no FAIL-suppression. If the
//     orchestrator wrote 3 PASS + 5 FAIL into results.json, the table
//     shows 3 PASS + 5 FAIL. The aggregate row reports the actual
//     count, the p50/p99 HTTP latency derived from real
//     KindHeraldPost → KindHeraldResponse deltas (sort-then-index, no
//     interpolation), and any "(no scenarios run)" placeholder when the
//     Result[] is empty.
//   - A per-scenario timeline section: events sorted by TS, formatted
//     as `- [direction] kind ts payload-summary note` with the payload
//     summary truncated to ~80 chars.
//   - An attachment manifest listing every Attachment seen in any
//     event with its sha256, content-type, size, original filename, and
//     on-disk path. The report does NOT re-hash the attachment file on
//     disk (T10 paired mutation gate (c) covers sha256 tolerance) — it
//     only checks the file exists.
//
// §107 anchor: every report section reflects observed state. The
// scenario-timeline iteration is event-by-event; the summary table
// iterates Result[] without filtering. The Wave 5 mutation gate (T10
// (a)) blanks transcript.Writer.Append; with no events on disk, the
// report's timeline section degenerates to scenario-stub headers and
// the e2e bluff-hunt invariant (E63..E70) flags the missing
// bidirectional traffic.
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// scenarioResult mirrors qaherald/internal/scenario.Result for the
// purpose of decoding results.json. Defined locally so the report
// package does not import scenario (which would create a cycle once
// scenario imports report — even though it currently does not, the
// decoupling is the cleaner contract).
type scenarioResult struct {
	Scenario  string        `json:"scenario"`
	PASS      bool          `json:"pass"`
	Duration  time.Duration `json:"duration"`
	ErrorText string        `json:"error,omitempty"`
}

// Generate renders the Markdown evidence report.
//
//   - transcriptPath: path to <runDir>/transcript.jsonl. MUST exist;
//     errors propagate.
//   - resultsPath:    path to <runDir>/results.json. MAY be absent —
//     the report degrades to "(no scenarios run)" placeholder if so.
//   - outPath:        absolute or relative path the report.md is
//     written to. Parent directory MUST exist (qaherald run creates it
//     via transcript.NewWriter).
func Generate(transcriptPath, resultsPath, outPath string) error {
	events, err := readTranscript(transcriptPath)
	if err != nil {
		return fmt.Errorf("report: read transcript: %w", err)
	}
	results, err := readResults(resultsPath)
	if err != nil {
		return fmt.Errorf("report: read results: %w", err)
	}

	runDir := filepath.Dir(transcriptPath)
	runID := filepath.Base(runDir)
	attachDir := filepath.Join(runDir, "attachments")

	var buf strings.Builder
	writeHeader(&buf, runID, events)
	writeSummary(&buf, results, events)
	writeTimelines(&buf, events)
	writeAttachmentManifest(&buf, events, attachDir)

	if err := os.WriteFile(outPath, []byte(buf.String()), 0o644); err != nil {
		return fmt.Errorf("report: write %s: %w", outPath, err)
	}
	return nil
}

// readTranscript decodes <runDir>/transcript.jsonl into a slice of
// transcript.Event. Returns an empty slice (not error) when the file is
// absent — the §107 reporting contract demands a partial report rather
// than a hard failure on a half-built run.
func readTranscript(path string) ([]transcript.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []transcript.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev transcript.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("decode line: %w", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, err
	}
	return events, nil
}

// readResults decodes <runDir>/results.json. Returns an empty slice
// (not error) when the file is absent.
func readResults(path string) ([]scenarioResult, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var results []scenarioResult
	if err := json.NewDecoder(f).Decode(&results); err != nil {
		return nil, fmt.Errorf("decode results: %w", err)
	}
	return results, nil
}

func writeHeader(buf *strings.Builder, runID string, events []transcript.Event) {
	var start, end time.Time
	if len(events) > 0 {
		start = events[0].TS
		end = events[len(events)-1].TS
		for _, ev := range events {
			if ev.TS.Before(start) {
				start = ev.TS
			}
			if ev.TS.After(end) {
				end = ev.TS
			}
		}
	}
	dur := end.Sub(start)
	startStr := "(no events)"
	endStr := "(no events)"
	durStr := "0s"
	if !start.IsZero() {
		startStr = start.Format(time.RFC3339Nano)
		endStr = end.Format(time.RFC3339Nano)
		durStr = dur.String()
	}
	fmt.Fprintf(buf, "# qaherald run report\n\n")
	fmt.Fprintf(buf, "| Run ID | Start | End | Duration |\n")
	fmt.Fprintf(buf, "|---|---|---|---|\n")
	fmt.Fprintf(buf, "| %s | %s | %s | %s |\n\n", runID, startStr, endStr, durStr)
}

func writeSummary(buf *strings.Builder, results []scenarioResult, events []transcript.Event) {
	fmt.Fprintf(buf, "## Summary\n\n")
	fmt.Fprintf(buf, "| Scenario | Result | Duration | Error |\n")
	fmt.Fprintf(buf, "|---|---|---|---|\n")
	if len(results) == 0 {
		fmt.Fprintf(buf, "| (no scenarios run) | - | - | - |\n\n")
	} else {
		for _, r := range results {
			verdict := "FAIL"
			if r.PASS {
				verdict = "PASS"
			}
			errCell := r.ErrorText
			if errCell == "" {
				errCell = "-"
			}
			errCell = sanitizeCell(errCell)
			fmt.Fprintf(buf, "| %s | %s | %s | %s |\n",
				sanitizeCell(r.Scenario), verdict, r.Duration, errCell)
		}
		fmt.Fprintln(buf)
	}

	// Aggregates.
	passN, failN := 0, 0
	for _, r := range results {
		if r.PASS {
			passN++
		} else {
			failN++
		}
	}
	p50, p99 := computeLatency(events)
	fmt.Fprintf(buf, "| Aggregate | Value |\n")
	fmt.Fprintf(buf, "|---|---|\n")
	fmt.Fprintf(buf, "| PASS | %d |\n", passN)
	fmt.Fprintf(buf, "| FAIL | %d |\n", failN)
	fmt.Fprintf(buf, "| Total scenarios | %d |\n", len(results))
	fmt.Fprintf(buf, "| HTTP p50 latency | %s |\n", formatLatency(p50))
	fmt.Fprintf(buf, "| HTTP p99 latency | %s |\n\n", formatLatency(p99))
}

// computeLatency pairs each KindHeraldPost event with the next
// KindHeraldResponse event sharing the same scenario string. The
// delta is the per-request latency. Pairing rule: scan events in TS
// order; maintain a per-scenario stack of "pending post" timestamps;
// on response, pop the oldest pending for that scenario. Multiple
// post/response cycles per scenario are supported.
//
// Returns zero-duration sentinels when no deltas exist.
func computeLatency(events []transcript.Event) (p50, p99 time.Duration) {
	sorted := make([]transcript.Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].TS.Before(sorted[j].TS)
	})
	pending := map[string][]time.Time{}
	var deltas []time.Duration
	for _, ev := range sorted {
		switch ev.Kind {
		case transcript.KindHeraldPost, transcript.KindHeraldGet:
			pending[ev.Scenario] = append(pending[ev.Scenario], ev.TS)
		case transcript.KindHeraldResponse:
			q := pending[ev.Scenario]
			if len(q) == 0 {
				continue
			}
			postTS := q[0]
			pending[ev.Scenario] = q[1:]
			d := ev.TS.Sub(postTS)
			if d < 0 {
				d = 0
			}
			deltas = append(deltas, d)
		}
	}
	if len(deltas) == 0 {
		return 0, 0
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i] < deltas[j] })
	return percentile(deltas, 50), percentile(deltas, 99)
}

// percentile picks the value at index ⌈p/100 · N⌉ - 1 (clamped to
// [0, N-1]). Sort-then-index, no interpolation — the bounded-input
// guarantee the operator locked at plan-time.
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

func formatLatency(d time.Duration) string {
	if d == 0 {
		return "(no data)"
	}
	return d.String()
}

func writeTimelines(buf *strings.Builder, events []transcript.Event) {
	fmt.Fprintf(buf, "## Scenario timelines\n\n")
	if len(events) == 0 {
		fmt.Fprintf(buf, "(no events recorded)\n\n")
		return
	}
	// Group events by scenario; preserve internal/non-scenario events
	// under a synthetic "(internal)" bucket so they still surface.
	groups := map[string][]transcript.Event{}
	order := []string{}
	for _, ev := range events {
		key := ev.Scenario
		if key == "" {
			key = "(internal)"
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], ev)
	}
	for _, name := range order {
		group := groups[name]
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].TS.Before(group[j].TS)
		})
		fmt.Fprintf(buf, "### %s\n\n", name)
		for _, ev := range group {
			fmt.Fprintf(buf, "- [%s] %s %s %s%s\n",
				ev.Direction,
				ev.Kind,
				ev.TS.Format(time.RFC3339Nano),
				summarizePayload(ev.Payload),
				formatNote(ev.Note),
			)
		}
		fmt.Fprintln(buf)
	}
}

// summarizePayload renders the payload JSON as a single line truncated
// to ~80 chars with a `…` suffix on overflow. Empty payloads produce
// the empty string.
func summarizePayload(p []byte) string {
	if len(p) == 0 {
		return ""
	}
	s := string(p)
	// Collapse whitespace to single spaces so multi-line JSON renders
	// on one line.
	s = strings.Join(strings.Fields(s), " ")
	const max = 80
	if len(s) > max {
		// Truncate by bytes (payloads are typically ASCII JSON; this is
		// safe enough for a summary).
		s = s[:max] + "…"
	}
	return s
}

func formatNote(note string) string {
	if note == "" {
		return ""
	}
	return " — " + sanitizeCell(note)
}

func writeAttachmentManifest(buf *strings.Builder, events []transcript.Event, attachDir string) {
	fmt.Fprintf(buf, "## Attachment manifest\n\n")
	type row struct {
		sha     string
		ctype   string
		size    int64
		orig    string
		path    string
		present bool
	}
	seen := map[string]bool{}
	var rows []row
	for _, ev := range events {
		for _, a := range ev.Attachments {
			if seen[a.Sha256] {
				continue
			}
			seen[a.Sha256] = true
			ext := pickExt(a.ContentType, a.OriginalFilename)
			path := filepath.Join(attachDir, a.Sha256+ext)
			_, err := os.Stat(path)
			rows = append(rows, row{
				sha:     a.Sha256,
				ctype:   a.ContentType,
				size:    a.SizeBytes,
				orig:    a.OriginalFilename,
				path:    filepath.Join("attachments", a.Sha256+ext),
				present: err == nil,
			})
		}
	}
	fmt.Fprintf(buf, "| sha256 | content-type | size | original filename | path | on-disk |\n")
	fmt.Fprintf(buf, "|---|---|---|---|---|---|\n")
	if len(rows) == 0 {
		fmt.Fprintf(buf, "| (none) | - | - | - | - | - |\n\n")
		return
	}
	for _, r := range rows {
		orig := r.orig
		if orig == "" {
			orig = "-"
		}
		present := "yes"
		if !r.present {
			present = "MISSING"
		}
		fmt.Fprintf(buf, "| %s | %s | %d B | %s | %s | %s |\n",
			r.sha,
			sanitizeCell(r.ctype),
			r.size,
			sanitizeCell(orig),
			sanitizeCell(r.path),
			present,
		)
	}
	fmt.Fprintln(buf)
}

// pickExt mirrors transcript.pickExt: returns the extension that
// AttachFile would have chosen for the stored file. The function in
// transcript is package-private, so we re-derive the same logic here.
// (Adjusting transcript to export it would widen the public surface
// without need.)
func pickExt(contentType, origName string) string {
	if origName != "" {
		if e := filepath.Ext(origName); e != "" {
			return e
		}
	}
	// Common message-channel content types — mirrors the mime package
	// fallback set used in transcript.pickExt. For unknown types the
	// .bin fallback matches transcript.
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "application/pdf":
		return ".pdf"
	case "text/plain", "text/plain; charset=utf-8":
		return ".txt"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "application/json":
		return ".json"
	case "application/octet-stream":
		return ".bin"
	}
	return ".bin"
}

// sanitizeCell escapes pipe + newline chars that would break GitHub
// Flavored Markdown table rendering.
func sanitizeCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
