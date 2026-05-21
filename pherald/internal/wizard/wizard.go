package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// Run dispatches the wizard for the given service identifier. Empty
// string => interactive menu. Supported identifiers: "telegram",
// "claude-code" (or "claude_code"), "all".
//
// Reads input from `in`, writes prompts/results to `out`. Per §11.4.10
// the wizard NEVER prints raw secret values back to `out` — credentials
// are masked via MaskValue. Per §107 every step that claims success
// observes positive evidence (token validates via getMe; export line
// is re-read from disk).
func Run(ctx context.Context, in io.Reader, out io.Writer, service string) error {
	r := bufio.NewReader(in)
	target, err := promptShellTarget(r, out)
	if err != nil {
		return err
	}

	if service == "" {
		service, err = promptService(r, out)
		if err != nil {
			return err
		}
	}

	switch strings.ToLower(strings.ReplaceAll(service, "_", "-")) {
	case "telegram":
		return runTelegram(ctx, r, out, target)
	case "claude-code":
		return runClaudeCode(ctx, r, out, target)
	case "all":
		if err := runTelegram(ctx, r, out, target); err != nil {
			return err
		}
		return runClaudeCode(ctx, r, out, target)
	default:
		return fmt.Errorf("wizard: unknown service %q; expected one of: telegram, claude-code, all", service)
	}
}

func promptShellTarget(r *bufio.Reader, out io.Writer) (ShellTarget, error) {
	targets, err := DetectShellTargets()
	if err != nil {
		return ShellTarget{}, err
	}
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
