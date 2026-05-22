package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Opts carries pre-supplied values that let the wizard skip prompts.
// When a field is empty, the wizard falls back to the corresponding
// HERALD_* env var (where one exists); when env is also empty it
// prompts the operator (interactive mode) or errors out (non-interactive).
//
// Per §11.4.10 + §107: Opts is the way to drive the wizard from scripts /
// CI / pipelines without leaking secrets through interactive stdin. The
// caller (pherald cmd) reads CLI flags into Opts; the wizard internals
// honor them.
type Opts struct {
	// Telegram (HRD-011):
	BotToken string // --bot-token (fallback: HERALD_TGRAM_BOT_TOKEN)
	ChatID   string // --chat-id   (fallback: HERALD_TGRAM_CHAT_ID; skips getUpdates polling)

	// Claude Code (HRD-012):
	ClaudeBin     string // --claude-bin           (fallback: HERALD_CLAUDE_BIN)
	ClaudeProject string // --claude-project       (fallback: HERALD_CLAUDE_PROJECT_NAME)
	ClaudeSession string // --claude-session-uuid  (fallback: HERALD_CLAUDE_SESSION_UUID)

	// Behaviour:
	NonInteractive bool   // --non-interactive — fail loud if a required value is missing
	ShellTarget    string // --shell-target — override the shell-startup file picker (zshrc|bashrc|profile)
}

// Run dispatches the wizard for the given service identifier. Empty
// string => interactive menu. Supported identifiers: "telegram",
// "claude-code" (or "claude_code"), "all".
//
// Reads input from `in`, writes prompts/results to `out`. Per §11.4.10
// the wizard NEVER prints raw secret values back to `out` — credentials
// are masked via MaskValue. Per §107 every step that claims success
// observes positive evidence (token validates via getMe; export line
// is re-read from disk).
//
// `opts` carries pre-supplied values; an empty Opts{} preserves the
// classic fully-interactive flow.
func Run(ctx context.Context, in io.Reader, out io.Writer, service string, opts Opts) error {
	r := bufio.NewReader(in)
	target, err := resolveShellTarget(r, out, opts)
	if err != nil {
		return err
	}

	if service == "" {
		if opts.NonInteractive {
			return fmt.Errorf("wizard: --non-interactive set but no service supplied (pass telegram|claude-code|all)")
		}
		service, err = promptService(r, out)
		if err != nil {
			return err
		}
	}

	switch strings.ToLower(strings.ReplaceAll(service, "_", "-")) {
	case "telegram":
		return runTelegram(ctx, r, out, target, opts)
	case "claude-code":
		return runClaudeCode(ctx, r, out, target, opts)
	case "all":
		if err := runTelegram(ctx, r, out, target, opts); err != nil {
			return err
		}
		return runClaudeCode(ctx, r, out, target, opts)
	default:
		return fmt.Errorf("wizard: unknown service %q; expected one of: telegram, claude-code, all", service)
	}
}

// resolveShellTarget honors --shell-target when set; otherwise prompts
// (interactive) or picks the default first detected target (non-interactive).
func resolveShellTarget(r *bufio.Reader, out io.Writer, opts Opts) (ShellTarget, error) {
	targets, err := DetectShellTargets()
	if err != nil {
		return ShellTarget{}, err
	}
	if opts.ShellTarget != "" {
		want := strings.ToLower(opts.ShellTarget)
		// Normalize: accept both the shell-kind vocabulary ("zsh", "bash")
		// AND the file-suffix vocabulary ("zshrc", "bashrc"). "profile" is
		// the same in both. The CLI help docs both forms; the picker
		// internally only knows ShellKind.
		want = strings.TrimPrefix(want, ".")     // ".zshrc" → "zshrc"
		want = strings.TrimPrefix(want, "~/")    // "~/.zshrc" → "zshrc"
		want = strings.TrimPrefix(want, ".")     // strip again if both prefixes were present
		// "zshrc"/"bashrc" → "zsh"/"bash"; "profile" stays
		if strings.HasSuffix(want, "rc") && want != "profile" {
			want = strings.TrimSuffix(want, "rc")
		}
		for _, t := range targets {
			if strings.ToLower(t.ShellKind) == want {
				return t, nil
			}
		}
		return ShellTarget{}, fmt.Errorf("wizard: --shell-target=%q did not match any detected target (have: %v; accepts: zsh/zshrc, bash/bashrc, profile)", opts.ShellTarget, shellNames(targets))
	}
	if opts.NonInteractive {
		// Fail loud rather than silently picking targets[0] when the operator
		// asked for non-interactive — they probably want to control the target.
		return ShellTarget{}, fmt.Errorf("wizard: --non-interactive set but --shell-target not provided (have: %v)", shellNames(targets))
	}
	return promptShellTargetFromList(r, out, targets)
}

func shellNames(ts []ShellTarget) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ShellKind)
	}
	return out
}

func promptShellTargetFromList(r *bufio.Reader, out io.Writer, targets []ShellTarget) (ShellTarget, error) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Which shell-startup file should receive the `export` lines?")
	for i, t := range targets {
		marker := " "
		if i == 0 {
			marker = "*"
		}
		fmt.Fprintf(out, "  %s [%d] %s (%s)\n", marker, i+1, t.DisplayName, t.ShellKind)
	}
	fmt.Fprint(out, "Pick a number [default=1]: ")
	line, err := r.ReadString('\n')
	if err != nil {
		return ShellTarget{}, fmt.Errorf("wizard: read shell choice: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return targets[0], nil
	}
	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err != nil || idx < 1 || idx > len(targets) {
		return ShellTarget{}, fmt.Errorf("wizard: invalid shell choice %q (pick 1..%d)", line, len(targets))
	}
	return targets[idx-1], nil
}

func promptService(r *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Which Herald integration?")
	fmt.Fprintln(out, "  * [1] telegram     — HRD-011 Telegram bot token + chat ID")
	fmt.Fprintln(out, "    [2] claude-code  — HRD-012 Claude Code project name + (auto-bootstrapped) session")
	fmt.Fprintln(out, "    [3] all          — both, in order")
	fmt.Fprint(out, "Pick a number [default=1]: ")
	line, err := r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("wizard: read service choice: %w", err)
	}
	switch strings.TrimSpace(line) {
	case "", "1":
		return "telegram", nil
	case "2":
		return "claude-code", nil
	case "3":
		return "all", nil
	}
	return "", fmt.Errorf("wizard: invalid service choice %q (pick 1..3)", strings.TrimSpace(line))
}

// resolveStringInput picks a string value in opts → env → prompt order.
// The bool return is true iff the value came from env (so the caller
// knows to skip re-persisting). A `defaultVal` is used only when the
// operator gives an empty answer to the interactive prompt.
//
// When --non-interactive is set and neither flag nor env provides a
// value, returns an error naming the env var so the operator knows
// what to set.
func resolveStringInput(r *bufio.Reader, out io.Writer, optsVal, envVar, label, defaultVal string, nonInteractive bool, promptText string) (string, bool, error) {
	if optsVal != "" {
		v := strings.TrimSpace(optsVal)
		if v == "" {
			return "", false, fmt.Errorf("%s: opts value was whitespace-only", label)
		}
		fmt.Fprintf(out, "  %s: supplied via flag.\n", label)
		return v, false, nil
	}
	if envVar != "" {
		if envVal := strings.TrimSpace(os.Getenv(envVar)); envVal != "" {
			fmt.Fprintf(out, "  %s: detected in %s env (will not re-persist).\n", label, envVar)
			return envVal, true, nil
		}
	}
	if nonInteractive {
		return "", false, fmt.Errorf("%s: --non-interactive set but no value (provide via flag or %s env)", label, envVar)
	}
	fmt.Fprint(out, promptText)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", false, fmt.Errorf("%s: read input: %w", label, err)
	}
	v := strings.TrimSpace(line)
	if v == "" {
		v = defaultVal
	}
	return v, false, nil
}

// writeAndSummarize is the canonical persistence helper: appends the
// export line to the shell file (or replaces if the operator opted in)
// then records the (masked) entry in ~/.herald/credentials.md.
// Per §107: AppendExport + AppendSummary each re-read their target
// files to confirm the write took. A silent failure mode is impossible.
func writeAndSummarize(out io.Writer, target ShellTarget, varName, value, comment, service, notes string, r *bufio.Reader) error {
	err := AppendExport(target.Path, varName, value, comment)
	if err == ErrAlreadyExported {
		fmt.Fprintf(out, "  %s is already exported in %s.\n", varName, target.DisplayName)
		fmt.Fprint(out, "  Replace with the new value? [y/N]: ")
		line, _ := r.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			if err := ReplaceExport(target.Path, varName, value, comment); err != nil {
				return err
			}
			fmt.Fprintf(out, "  ✓ %s replaced in %s\n", varName, target.DisplayName)
		} else {
			fmt.Fprintf(out, "  (skipped — existing value kept)\n")
			return nil
		}
	} else if err != nil {
		return err
	} else {
		fmt.Fprintf(out, "  ✓ %s appended to %s\n", varName, target.DisplayName)
	}

	if err := AppendSummary(SummaryEntry{
		Service:     service,
		VarName:     varName,
		MaskedValue: MaskValue(value),
		ShellFile:   target.Path,
		Timestamp:   time.Now(),
		Notes:       notes,
	}); err != nil {
		return err
	}
	return nil
}
