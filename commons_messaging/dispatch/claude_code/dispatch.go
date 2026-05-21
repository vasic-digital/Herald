package claude_code

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

// Dispatch invokes `claude --resume <UUID> --print "<envelope>"` and parses
// the structured reply line prefixed with <<<HERALD-REPLY>>>.
//
// §107 (anti-bluff): a PASS requires (a) the claude binary exits 0, AND
// (b) stdout contains a well-formed JSON reply on a line carrying the
// <<<HERALD-REPLY>>> marker. A reply where Claude refused, errored, or
// never produced the marker is an explicit FAIL — we do not synthesise
// defaults.
//
// Spec §33.2 step 1 (session resolution): if ResolveSession returns
// uuid.Nil we MUST NOT pretend; instead we fail explicitly so the
// caller (HRD-012 step 7 persistence layer) can decide to spawn an
// initialising session and PersistSession before retrying.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResponse, error) {
	sessionUUID, anchor, err := d.ResolveSession()
	if err != nil {
		return DispatchResponse{}, fmt.Errorf("claude_code: dispatch resolve session: %w", err)
	}
	if sessionUUID == uuid.Nil {
		return DispatchResponse{}, fmt.Errorf("claude_code: dispatch: no anchored session at %s (HRD-012 step 7 will bootstrap)", anchor)
	}

	envelope := d.FormatEnvelope(req)
	cmd := exec.CommandContext(ctx, d.binaryPath,
		"--resume", sessionUUID.String(),
		"--print", envelope,
	)
	cmd.Dir = d.workingDir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return DispatchResponse{}, fmt.Errorf("claude_code: dispatch claude --resume %s: exit %d: %s",
				sessionUUID, ee.ExitCode(), strings.TrimSpace(string(ee.Stderr)))
		}
		return DispatchResponse{}, fmt.Errorf("claude_code: dispatch exec: %w", err)
	}

	resp, err := parseReply(out)
	if err != nil {
		return DispatchResponse{}, fmt.Errorf("claude_code: dispatch parse reply: %w", err)
	}
	// Stash session metadata for the persistence layer (HRD-012 step 7).
	resp.SessionUUID = sessionUUID
	resp.AnchorPath = anchor
	return resp, nil
}

// parseReply scans Claude Code's stdout for the <<<HERALD-REPLY>>> marker
// and decodes the first JSON object that follows it into a
// DispatchResponse. Returns an error if no marker is found or the JSON
// is malformed — explicit rejection preserves §107 anti-bluff.
//
// The marker may appear mid-line (e.g. prefixed by some Claude-side
// formatting) — we accept the JSON object that starts at the first '{'
// at-or-after the marker. This tolerates trailing prose without
// softening the strict "no marker = FAIL" rule.
func parseReply(stdout []byte) (DispatchResponse, error) {
	const marker = "<<<HERALD-REPLY>>>"
	s := string(stdout)
	idx := strings.Index(s, marker)
	if idx < 0 {
		return DispatchResponse{}, fmt.Errorf("no <<<HERALD-REPLY>>> marker found in claude stdout (§107 bluff guard); stdout=%q", truncate(s, 512))
	}
	after := s[idx+len(marker):]
	braceIdx := strings.Index(after, "{")
	if braceIdx < 0 {
		return DispatchResponse{}, fmt.Errorf("marker present but no JSON object follows it; after-marker=%q", truncate(after, 512))
	}
	dec := json.NewDecoder(strings.NewReader(after[braceIdx:]))
	var resp DispatchResponse
	if err := dec.Decode(&resp); err != nil {
		return DispatchResponse{}, fmt.Errorf("decode reply JSON: %w (raw: %q)", err, truncate(after[braceIdx:], 512))
	}
	return resp, nil
}

// truncate keeps error messages readable when claude emits multi-kilobyte
// stdout.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
