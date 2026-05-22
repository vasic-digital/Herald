// Package inbound — issue_opener.go: concrete IssueOpener that mutates
// docs/Issues.md atomically per V3 §8.3 lifecycle.
//
// §107 anchor: a real file is mutated. M2 mutation plants /dev/null writer;
// unit test re-reads the file and catches the absence of the new row.
package inbound

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// DocsIssueOpener writes a new HRD-NNN row to docs/Issues.md and
// allocates the next free HRD number by scanning both Issues.md and
// Fixed.md.
//
// Concurrency: serialised via a sidecar lockfile created with
// os.OpenFile(O_CREATE|O_EXCL). Two concurrent OpenIssue calls in the
// same process will see one acquire the lock + the other spin-wait
// (10ms backoff, 30s ceiling). pherald listen is single-goroutine per
// channel today; the lockfile is the future-proofing for the
// multi-channel-fanout case (V3 §32.2).
type DocsIssueOpener struct {
	IssuesPath string        // canonical: "docs/Issues.md"
	FixedPath  string        // canonical: "docs/Fixed.md"
	LockPath   string        // canonical: "docs/.issues.lock"
	Clock      commons.Clock // for testable timestamps
}

// OpenIssue allocates the next HRD-NNN, builds the row, prepends it
// under the "## Open" heading of IssuesPath, and writes the file
// atomically via temp+rename.
func (o *DocsIssueOpener) OpenIssue(ctx context.Context, p IssuePayload) error {
	if o.IssuesPath == "" {
		return errors.New("DocsIssueOpener: IssuesPath empty")
	}
	if o.LockPath == "" {
		return errors.New("DocsIssueOpener: LockPath empty")
	}
	if o.Clock == nil {
		o.Clock = commons.RealClock{}
	}

	if err := o.acquireLock(ctx); err != nil {
		return fmt.Errorf("acquire issues lock: %w", err)
	}
	defer o.releaseLock()

	next, err := o.nextHRDNumber()
	if err != nil {
		return fmt.Errorf("allocate HRD-NNN: %w", err)
	}

	row := o.buildRow(next, p)

	if err := o.prependUnderOpen(row); err != nil {
		return fmt.Errorf("prepend row: %w", err)
	}
	return nil
}

// acquireLock spins up to 30s waiting for the lockfile to be free.
func (o *DocsIssueOpener) acquireLock(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		f, err := os.OpenFile(o.LockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
			_ = f.Close()
			return nil
		}
		if !os.IsExist(err) {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("DocsIssueOpener: lock held >30s — stale?")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (o *DocsIssueOpener) releaseLock() {
	_ = os.Remove(o.LockPath)
}

var hrdNumberRE = regexp.MustCompile(`HRD-(\d+)`)

// nextHRDNumber scans both IssuesPath and FixedPath for HRD-NNN tokens
// and returns max+1.
func (o *DocsIssueOpener) nextHRDNumber() (int, error) {
	max := 0
	for _, p := range []string{o.IssuesPath, o.FixedPath} {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		for _, m := range hrdNumberRE.FindAllStringSubmatch(string(data), -1) {
			n, _ := strconv.Atoi(m[1])
			if n > max {
				max = n
			}
		}
	}
	return max + 1, nil
}

// buildRow constructs a single Markdown table row matching the
// docs/Issues.md "Open" table schema:
//
//	| ID | Type | Status | Criticality | Title | Opened | Last update | References |
//
// 8 pipe-delimited columns (verified against docs/Issues.md as of 2026-05-22).
func (o *DocsIssueOpener) buildRow(n int, p IssuePayload) string {
	today := o.Clock.Now().UTC().Format("2006-01-02")
	title := strings.ReplaceAll(p.Title, "|", `\|`)
	return fmt.Sprintf("| HRD-%03d | %s | open | %s | %s | %s | %s | Catalogue-Check: TBD |\n",
		n, p.Type, p.Criticality, title, today, today)
}

// prependUnderOpen inserts row immediately after the "## Open" heading
// and the table-header / table-separator lines. Writes via
// temp+rename for atomicity.
func (o *DocsIssueOpener) prependUnderOpen(row string) error {
	data, err := os.ReadFile(o.IssuesPath)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")

	// Find "## Open" + skip the table header + separator (2 lines after).
	insertAt := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "## Open") {
			// Walk forward to the first table data row (skip blank lines,
			// header, separator, and any narrative "Per Universal" prose).
			for j := i + 1; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if t == "" {
					continue
				}
				if strings.HasPrefix(t, "|---") {
					continue
				}
				if strings.HasPrefix(t, "| ID") {
					continue
				}
				if strings.HasPrefix(t, "Per Universal") {
					continue
				}
				// Defensive: any other non-row markdown gets skipped only
				// if it is clearly not a data row (does not start with "|").
				if !strings.HasPrefix(t, "|") {
					continue
				}
				insertAt = j
				break
			}
			break
		}
	}
	if insertAt < 0 {
		return errors.New("docs/Issues.md: '## Open' heading not found or table empty")
	}

	var newLines []string
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, row)
	newLines = append(newLines, lines[insertAt:]...)

	dir := filepath.Dir(o.IssuesPath)
	tmp := filepath.Join(dir, filepath.Base(o.IssuesPath)+".tmp."+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.WriteFile(tmp, []byte(strings.Join(newLines, "")), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, o.IssuesPath)
}
