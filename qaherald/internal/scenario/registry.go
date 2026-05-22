// Wave 5 Task 5 — scenario registry + shared assertion helpers +
// bidirectional-invariant validator.
//
// Scenarios self-register from init() functions in their own file (one
// scenario per file under qaherald/internal/scenario/). The T7 runner
// resolves scenarios via Get(name) / All(); the T8 live runner walks
// All() in alphabetical-by-name order for deterministic transcripts.
//
// §107 anti-bluff anchor: ValidateScenarioBidirectional re-reads the
// transcript JSONL from disk (NOT the in-memory writer state) and
// asserts each named scenario emitted ≥1 herald.* event AND ≥1 tg.*
// event. T10 mutation gate (a) plants a blank Writer.Append; the
// validator then sees zero events per scenario and returns the
// bluff-detected error.
package scenario

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// registry holds every Scenario the package has registered via init().
// It is keyed by Scenario.Name; Register panics on duplicate names so
// a typo at compile-time surfaces immediately rather than silently
// shadowing a real scenario.
var registry = map[string]Scenario{}

// Register installs s into the package-level registry. Panics on
// duplicate name — a typo in two scenarios' init() functions would
// otherwise silently lose one of them, which is a class of bluff this
// engine forbids.
func Register(s Scenario) {
	if _, ok := registry[s.Name]; ok {
		panic(fmt.Sprintf("scenario: duplicate registration for %q", s.Name))
	}
	registry[s.Name] = s
}

// Get returns the Scenario registered under name, plus an ok flag.
// Callers (T7 `qaherald run --scenario=<name>`) MUST surface a clear
// error when ok is false rather than silently no-op.
func Get(name string) (Scenario, bool) {
	s, ok := registry[name]
	return s, ok
}

// All returns every registered Scenario in deterministic
// name-alphabetical order. Used by `qaherald run --scenario=all`.
func All() []Scenario {
	out := make([]Scenario, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns every registered scenario name in alphabetical order.
// Convenience helper for the CLI `--scenario=` flag's completion list.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// assertStatus returns nil if got matches want, else a descriptive
// error pinpointing the where-string so scenario stack traces stay
// readable.
func assertStatus(want, got int, where string) error {
	if want != got {
		return fmt.Errorf("%s: expected HTTP %d, got %d", where, want, got)
	}
	return nil
}

// assertHeader returns nil if got matches want, else a descriptive
// error. Used by idempotency-replay for X-Herald-Replay assertions.
func assertHeader(want, got, where string) error {
	if want != got {
		return fmt.Errorf("%s: expected header %q, got %q", where, want, got)
	}
	return nil
}

// assertHeaderPresent returns nil if got is non-empty, else a
// descriptive error.
func assertHeaderPresent(got, where string) error {
	if got == "" {
		return fmt.Errorf("%s: expected non-empty header, got empty", where)
	}
	return nil
}

// ValidateScenarioBidirectional opens transcriptPath, walks every
// JSONL row, and asserts the named scenario emitted at least one
// Herald-side event AND at least one Telegram-side event. Returns nil
// when the invariant holds; returns a descriptive error otherwise.
//
// §107 anti-bluff hook: this is the post-run check the unit test +
// T10 mutation harness invoke. Mutation gate (a) blanks the
// transcript Writer's Append body — every scenario then ends up with
// zero events recorded; this validator reports `0 herald events`
// + `0 tg events` and returns an error, surfacing the bluff.
func ValidateScenarioBidirectional(transcriptPath, scenarioName string) error {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return fmt.Errorf("scenario %q: open transcript: %w", scenarioName, err)
	}
	defer f.Close()

	var heraldCount, tgCount int
	sc := bufio.NewScanner(f)
	// Scenarios may attach JSON payload blobs of arbitrary size (e.g.
	// the Receipt body). Bump the scanner's max line length so an
	// oversized payload does not silently truncate the transcript walk
	// and yield a false-negative invariant violation.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev transcript.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			// Skip malformed lines rather than aborting — a partial
			// transcript should still be analysable.
			continue
		}
		if ev.Scenario != scenarioName {
			continue
		}
		switch ev.Kind {
		case transcript.KindHeraldPost, transcript.KindHeraldGet, transcript.KindHeraldResponse:
			heraldCount++
		case transcript.KindTGSend, transcript.KindTGReceive, transcript.KindTGUpload, transcript.KindTGDownload:
			tgCount++
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scenario %q: scan transcript: %w", scenarioName, err)
	}
	if heraldCount == 0 || tgCount == 0 {
		return fmt.Errorf(
			"scenario %q: §107 bidirectional invariant violated — "+
				"herald events=%d tg events=%d (need ≥1 of each)",
			scenarioName, heraldCount, tgCount)
	}
	return nil
}

// containsCloudEventID returns true when m's Text or Caption contains
// eventID as a substring. Scenarios use this as the canonical
// WaitForMessage predicate for delivery cross-checks. Defined here so
// every scenario file shares the same matcher.
func containsCloudEventID(text, caption, eventID string) bool {
	if eventID == "" {
		return false
	}
	return strings.Contains(text, eventID) || strings.Contains(caption, eventID)
}
