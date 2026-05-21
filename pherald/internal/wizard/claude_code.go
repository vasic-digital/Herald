package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// runClaudeCode walks the operator through configuring the Claude Code
// dispatcher (HRD-012). Validates that `claude` is on PATH before
// persisting HERALD_CLAUDE_BIN; collects project name; offers to seed
// HERALD_CLAUDE_SESSION_UUID (optional — Herald auto-bootstraps when
// unset per spec §33.2). Per §107: every value is verified before save.
func runClaudeCode(ctx context.Context, r *bufio.Reader, out io.Writer, target ShellTarget) error {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "── Claude Code dispatcher (HRD-012) ────────────────────────────────")
	fmt.Fprintln(out, "")

	// Step 1: locate the claude CLI.
	fmt.Fprint(out, "Path to the `claude` CLI [default=claude — looked up via PATH]: ")
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("wizard.claude_code: read bin: %w", err)
	}
	bin := strings.TrimSpace(line)
	if bin == "" {
		bin = "claude"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("wizard.claude_code: `%s` not on PATH or not executable: %w (see docs/guides/dispatchers/CLAUDE_CODE.md §Step 1 for install)", bin, err)
	}
	// §107: confirm `claude --version` returns something.
	verOut, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return fmt.Errorf("wizard.claude_code: `%s --version` failed: %w", bin, err)
	}
	verStr := strings.TrimSpace(string(verOut))
	if verStr == "" {
		return fmt.Errorf("wizard.claude_code: `%s --version` returned empty output (§107 bluff guard)", bin)
	}
	fmt.Fprintf(out, "  ✓ %s --version: %s\n", bin, verStr)

	if err := writeAndSummarize(out, target,
		"HERALD_CLAUDE_BIN", bin,
		"Herald — Claude Code (HRD-012) — claude CLI path",
		"claude_code",
		fmt.Sprintf("version: %s", verStr),
		r,
	); err != nil {
		return err
	}

	// Step 2: project name.
	fmt.Fprint(out, "Project name (filename-safe; typically your repo folder name) [default=Herald]: ")
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("wizard.claude_code: read project name: %w", err)
	}
	projectName := strings.TrimSpace(line)
	if projectName == "" {
		projectName = "Herald"
	}
	if strings.ContainsAny(projectName, "/\\:*?\"<>|") {
		return fmt.Errorf("wizard.claude_code: project name %q contains filesystem-unsafe characters", projectName)
	}

	if err := writeAndSummarize(out, target,
		"HERALD_CLAUDE_PROJECT_NAME", projectName,
		"Herald — Claude Code (HRD-012) — project anchor name (per spec §33.2)",
		"claude_code",
		"used to resolve <workdir>/.herald/claude-code/sessions/<project>.session",
		r,
	); err != nil {
		return err
	}

	// Step 3: optional session UUID.
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Optional: set HERALD_CLAUDE_SESSION_UUID to a specific session.")
	fmt.Fprintln(out, "  Leave empty (recommended) → Herald auto-bootstraps a new session on first Dispatch per §33.2.")
	fmt.Fprint(out, "  Session UUID (or Enter to skip): ")
	line, err = r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("wizard.claude_code: read session uuid: %w", err)
	}
	sessionUUID := strings.TrimSpace(line)
	if sessionUUID != "" {
		if !looksLikeUUID(sessionUUID) {
			return fmt.Errorf("wizard.claude_code: session UUID %q does not match the expected 8-4-4-4-12 hex shape", sessionUUID)
		}
		if err := writeAndSummarize(out, target,
			"HERALD_CLAUDE_SESSION_UUID", sessionUUID,
			"Herald — Claude Code (HRD-012) — pre-bootstrapped session UUID",
			"claude_code",
			"operator-supplied; will be used by ResolveSession() instead of auto-bootstrap",
			r,
		); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(out, "  (skipped — Herald will auto-bootstrap on first Dispatch)")
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Claude Code setup complete. To verify end-to-end (HRD-012 E18):")
	fmt.Fprintln(out, "    source ~/.zshrc   # or your chosen shell file")
	fmt.Fprintln(out, "    go test ./commons_messaging/dispatch/claude_code/ -tags=integration -run TestDispatch_LiveClaudeInvocation -count=1 -timeout=300s")
	return nil
}

// looksLikeUUID returns true if s matches the canonical 8-4-4-4-12 hex
// shape (RFC 4122 string form). Permissive — accepts upper/lower hex.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
