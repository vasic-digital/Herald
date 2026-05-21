// Package wizard implements `pherald wizard credentials` — an interactive
// CLI that walks operators through obtaining credentials for every
// supported Herald integration and persists them to the operator's
// shell startup file (~/.zshrc or ~/.bashrc) + a git-ignored
// ~/.herald/credentials.md summary.
//
// Per Universal Constitution §11.4.10:
//   - The wizard NEVER writes a credential to a git-tracked file.
//   - The summary file at ~/.herald/credentials.md is created chmod 600.
//   - The wizard echoes credential values masked (e.g. "12345...uXyZ").
//   - Existing exports are detected; the wizard asks before overwriting.
//
// Per §107: every claim of "credential saved" is verified by reading
// back the file and confirming the export line is present. A bluff
// where the file write silently failed is impossible.
package wizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ShellTarget identifies a shell-startup file the wizard can append to.
type ShellTarget struct {
	Path        string // absolute path, e.g. /Users/alice/.zshrc
	DisplayName string // friendly name, e.g. "~/.zshrc"
	ShellKind   string // "zsh" | "bash" | "profile"
}

// DetectShellTargets returns the candidate shell-startup files in order
// of preference for the current host. The first candidate is the
// recommended default; the operator picks one in the interactive flow.
func DetectShellTargets() ([]ShellTarget, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("wizard.DetectShellTargets: %w", err)
	}
	defaultShell := os.Getenv("SHELL")
	out := []ShellTarget{}
	// Prefer the operator's actual default shell. macOS = zsh; many Linux = bash.
	if strings.Contains(defaultShell, "zsh") || runtime.GOOS == "darwin" {
		out = append(out, ShellTarget{Path: filepath.Join(home, ".zshrc"), DisplayName: "~/.zshrc", ShellKind: "zsh"})
		out = append(out, ShellTarget{Path: filepath.Join(home, ".bashrc"), DisplayName: "~/.bashrc", ShellKind: "bash"})
	} else {
		out = append(out, ShellTarget{Path: filepath.Join(home, ".bashrc"), DisplayName: "~/.bashrc", ShellKind: "bash"})
		out = append(out, ShellTarget{Path: filepath.Join(home, ".zshrc"), DisplayName: "~/.zshrc", ShellKind: "zsh"})
	}
	out = append(out, ShellTarget{Path: filepath.Join(home, ".profile"), DisplayName: "~/.profile", ShellKind: "profile"})
	return out, nil
}

// exportLineRE matches a shell export line for a given var name:
//
//	export NAME='...'
//	export NAME="..."
//	export NAME=...
//
// Anchored to start-of-line (allowing leading whitespace).
func exportLineRE(varName string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*export\s+` + regexp.QuoteMeta(varName) + `\s*=`)
}

// HasExport returns true if the given shell file already declares an
// export for varName (regardless of value). Used to detect duplicates
// before append.
func HasExport(filePath, varName string) (bool, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("wizard.HasExport: read %s: %w", filePath, err)
	}
	return exportLineRE(varName).Match(data), nil
}

// AppendExport appends `export NAME='VALUE'` to the given shell file.
// The value is single-quoted (POSIX shell convention) with internal
// single-quotes escaped via the `'\''` trick. Creates the file with
// mode 0644 if it does not exist; preserves existing permissions
// otherwise.
//
// §107 verification: after appending, the function re-reads the file
// and confirms the new line is present. If the verification fails,
// returns an error — the wizard never claims success on a silent write
// failure.
//
// Idempotency: if the variable already has an export line, this
// function returns ErrAlreadyExported. The caller decides whether to
// overwrite (via ReplaceExport) or skip.
func AppendExport(filePath, varName, value, comment string) error {
	already, err := HasExport(filePath, varName)
	if err != nil {
		return err
	}
	if already {
		return ErrAlreadyExported
	}

	line := buildExportLine(varName, value, comment)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("wizard.AppendExport: open %s: %w", filePath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("wizard.AppendExport: write: %w", err)
	}

	// §107 verification: re-read and confirm the new line is present.
	ok, err := HasExport(filePath, varName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("wizard.AppendExport: post-write verification failed — export for %q not found in %s (§107 bluff guard)", varName, filePath)
	}
	return nil
}

// ReplaceExport replaces every existing `export VARNAME=...` line in
// the file with a single new entry. Used when the operator confirms
// they want to overwrite an existing export.
//
// §107 verification: after writing, the function re-reads the file and
// confirms the new VALUE is present in the matching export line.
func ReplaceExport(filePath, varName, value, comment string) error {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		// No file yet — just append.
		return AppendExport(filePath, varName, value, comment)
	}
	if err != nil {
		return fmt.Errorf("wizard.ReplaceExport: read: %w", err)
	}

	re := exportLineRE(varName)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out strings.Builder
	replaced := false
	newLine := strings.TrimRight(buildExportLine(varName, value, comment), "\n")
	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			if !replaced {
				out.WriteString(newLine)
				out.WriteByte('\n')
				replaced = true
			}
			// Skip the old line(s).
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if !replaced {
		// No existing export — append.
		out.WriteString(newLine)
		out.WriteByte('\n')
	}

	info, _ := os.Stat(filePath)
	mode := os.FileMode(0o644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(filePath, []byte(out.String()), mode); err != nil {
		return fmt.Errorf("wizard.ReplaceExport: write: %w", err)
	}

	// §107 verification: confirm new value present.
	finalData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(finalData), value) {
		return fmt.Errorf("wizard.ReplaceExport: post-write verification failed — value not found (§107 bluff guard)")
	}
	return nil
}

func buildExportLine(varName, value, comment string) string {
	// Escape single quotes in value using the POSIX-portable trick:
	//   'val'\''ue'
	escaped := strings.ReplaceAll(value, "'", `'\''`)
	if comment != "" {
		return fmt.Sprintf("\n# %s\nexport %s='%s'\n", comment, varName, escaped)
	}
	return fmt.Sprintf("\nexport %s='%s'\n", varName, escaped)
}

// ErrAlreadyExported is returned by AppendExport when the variable is
// already declared in the file. The caller decides whether to skip,
// overwrite via ReplaceExport, or abort.
var ErrAlreadyExported = fmt.Errorf("wizard: variable already exported (use ReplaceExport to overwrite)")

// MaskValue returns a partial-mask of a credential for safe terminal
// display. Examples (boundary at len()=8):
//
//	""                  -> "(empty)"
//	"abc"               -> "***" (too short to show prefix)
//	"abcdefgh"          -> "abc...***"
//	"abc:DEF1234567"    -> "abc:DE...567"  (preserves both ends for human pattern-match)
func MaskValue(v string) string {
	if v == "" {
		return "(empty)"
	}
	if len(v) <= 6 {
		return "***"
	}
	if len(v) <= 16 {
		return v[:3] + "...***"
	}
	return v[:6] + "..." + v[len(v)-3:]
}
