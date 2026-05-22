package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// runClaudeCode walks the operator through configuring the Claude Code
// dispatcher (HRD-012). Each input honors Opts → env → prompt order:
//   - bin:      opts.ClaudeBin     | HERALD_CLAUDE_BIN          | prompt (default "claude")
//   - project:  opts.ClaudeProject | HERALD_CLAUDE_PROJECT_NAME | prompt (default "Herald")
//   - session:  opts.ClaudeSession | HERALD_CLAUDE_SESSION_UUID | prompt (optional; skipped if empty)
//
// Per §107 every resolved value is verified BEFORE persistence
// (`claude --version` returns non-empty; project name passes the
// filesystem-unsafe-character check; session UUID matches RFC 4122).
// Env-supplied values are NOT re-persisted (the operator already has
// them in their shell file).
func runClaudeCode(ctx context.Context, r *bufio.Reader, out io.Writer, target ShellTarget, opts Opts) error {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "── Claude Code dispatcher (HRD-012) ────────────────────────────────")
	fmt.Fprintln(out, "")

	// Step 1: locate the claude CLI.
	bin, binFromEnv, err := resolveStringInput(r, out, opts.ClaudeBin, "HERALD_CLAUDE_BIN", "bin", "claude", opts.NonInteractive,
		"Path to the `claude` CLI [default=claude — looked up via PATH]: ")
	if err != nil {
		return fmt.Errorf("wizard.claude_code: %w", err)
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

	if !binFromEnv {
		if err := writeAndSummarize(out, target,
			"HERALD_CLAUDE_BIN", bin,
			"Herald — Claude Code (HRD-012) — claude CLI path",
			"claude_code",
			fmt.Sprintf("version: %s", verStr),
			r,
		); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "  (bin came from HERALD_CLAUDE_BIN env — not re-persisting)\n")
	}

	// Step 2: project name.
	projectName, projectFromEnv, err := resolveStringInput(r, out, opts.ClaudeProject, "HERALD_CLAUDE_PROJECT_NAME", "project name", "Herald", opts.NonInteractive,
		"Project name (filename-safe; typically your repo folder name) [default=Herald]: ")
	if err != nil {
		return fmt.Errorf("wizard.claude_code: %w", err)
	}
	if strings.ContainsAny(projectName, "/\\:*?\"<>|") {
		return fmt.Errorf("wizard.claude_code: project name %q contains filesystem-unsafe characters", projectName)
	}

	if !projectFromEnv {
		if err := writeAndSummarize(out, target,
			"HERALD_CLAUDE_PROJECT_NAME", projectName,
			"Herald — Claude Code (HRD-012) — project anchor name (per spec §33.2)",
			"claude_code",
			"used to resolve <workdir>/.herald/claude-code/sessions/<project>.session",
			r,
		); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "  (project came from HERALD_CLAUDE_PROJECT_NAME env — not re-persisting)\n")
	}

	// Step 3: optional session UUID.
	var sessionUUID string
	var sessionFromEnv bool
	if opts.ClaudeSession != "" {
		sessionUUID = strings.TrimSpace(opts.ClaudeSession)
		fmt.Fprintln(out, "  Session UUID: supplied via --claude-session-uuid flag.")
	} else if envSess := strings.TrimSpace(os.Getenv("HERALD_CLAUDE_SESSION_UUID")); envSess != "" {
		sessionUUID = envSess
		sessionFromEnv = true
		fmt.Fprintln(out, "  Session UUID: detected in HERALD_CLAUDE_SESSION_UUID env (will not re-persist).")
	} else if !opts.NonInteractive {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Optional: set HERALD_CLAUDE_SESSION_UUID to a specific session.")
		fmt.Fprintln(out, "  Leave empty (recommended) → Herald auto-bootstraps a new session on first Dispatch per §33.2.")
		fmt.Fprint(out, "  Session UUID (or Enter to skip): ")
		line, rerr := r.ReadString('\n')
		if rerr != nil {
			return fmt.Errorf("wizard.claude_code: read session uuid: %w", rerr)
		}
		sessionUUID = strings.TrimSpace(line)
	}

	if sessionUUID != "" {
		if !looksLikeUUID(sessionUUID) {
			return fmt.Errorf("wizard.claude_code: session UUID %q does not match the expected 8-4-4-4-12 hex shape", sessionUUID)
		}
		if !sessionFromEnv {
			if err := writeAndSummarize(out, target,
				"HERALD_CLAUDE_SESSION_UUID", sessionUUID,
				"Herald — Claude Code (HRD-012) — pre-bootstrapped session UUID",
				"claude_code",
				"operator-supplied; will be used by ResolveSession() instead of auto-bootstrap",
				r,
			); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintln(out, "  (no session UUID — Herald will auto-bootstrap on first Dispatch)")
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
