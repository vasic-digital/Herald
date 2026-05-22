// issue_opener_test.go — unit tests for DocsIssueOpener per Wave 6.5 T2.
//
// §107 anti-bluff: every assertion re-reads the file from disk and asserts
// on the raw bytes. A writer that returns nil but never touches the file
// would FAIL these tests because the canned fixture's max HRD is 100 — a
// "always returns HRD-001" stub would also FAIL because the appended row's
// HRD-NNN is checked literally.
package inbound

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

// fixtureIssues mirrors the docs/Issues.md schema verified 2026-05-22.
// 8 columns: ID | Type | Status | Criticality | Title | Opened | Last update | References
const fixtureIssues = `# Herald — Issues

## Open

Per Universal §11.4.74 every new row carries a Catalogue-Check.

| ID | Type | Status | Criticality | Title | Opened | Last update | References |
|---|---|---|---|---|---|---|---|
| HRD-099 | task | open | low | sentinel row | 2026-05-20 | 2026-05-20 | x |
`

const fixtureFixed = `# Herald — Fixed

| ID | Type | Status | Criticality | Title | Opened | Last update | References |
|---|---|---|---|---|---|---|---|
| HRD-100 | task | closed | low | sentinel row | 2026-05-20 | 2026-05-21 | x |
`

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestOpener(t *testing.T, dir string) *DocsIssueOpener {
	t.Helper()
	ip := writeFixture(t, dir, "Issues.md", fixtureIssues)
	fp := writeFixture(t, dir, "Fixed.md", fixtureFixed)
	return &DocsIssueOpener{
		IssuesPath: ip,
		FixedPath:  fp,
		LockPath:   filepath.Join(dir, ".issues.lock"),
		Clock:      commons.NewFakeClock(),
	}
}

func TestDocsIssueOpener_NextHRDNumber(t *testing.T) {
	dir := t.TempDir()
	o := newTestOpener(t, dir)
	n, err := o.nextHRDNumber()
	if err != nil {
		t.Fatal(err)
	}
	// fixtureIssues max = 99, fixtureFixed max = 100 → next = 101
	if n != 101 {
		t.Errorf("nextHRDNumber: got %d want 101", n)
	}
}

func TestDocsIssueOpener_NextHRDNumber_MissingFixed(t *testing.T) {
	dir := t.TempDir()
	ip := writeFixture(t, dir, "Issues.md", fixtureIssues)
	o := &DocsIssueOpener{
		IssuesPath: ip,
		FixedPath:  filepath.Join(dir, "Fixed.md.absent"),
		LockPath:   filepath.Join(dir, ".issues.lock"),
		Clock:      commons.NewFakeClock(),
	}
	n, err := o.nextHRDNumber()
	if err != nil {
		t.Fatal(err)
	}
	// Only Issues.md scanned → max = 99 → next = 100
	if n != 100 {
		t.Errorf("nextHRDNumber w/o Fixed.md: got %d want 100", n)
	}
}

func TestDocsIssueOpener_OpenIssueAppends(t *testing.T) {
	dir := t.TempDir()
	o := newTestOpener(t, dir)

	err := o.OpenIssue(context.Background(), IssuePayload{
		Type:        "bug",
		Criticality: "high",
		Title:       "sample bug",
		Body:        "Detailed description.",
		Labels:      []string{"telemetry"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// §107: re-read file from disk + assert on raw bytes.
	data, err := os.ReadFile(o.IssuesPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	// HRD-101 row must be present verbatim (max=100 in Fixed → 101).
	want := "| HRD-101 | bug | open | high | sample bug |"
	if !strings.Contains(s, want) {
		t.Errorf("expected row %q in file:\n%s", want, s)
	}

	// Existing sentinel row must still be present (no loss).
	if !strings.Contains(s, "| HRD-099 | task | open | low | sentinel row |") {
		t.Errorf("sentinel HRD-099 row lost; file:\n%s", s)
	}

	// File ends with a newline.
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Errorf("expected trailing newline; file ends: %q", s[max(0, len(s)-32):])
	}

	// Lockfile cleaned up after the call.
	if _, err := os.Stat(o.LockPath); !os.IsNotExist(err) {
		t.Errorf("expected lockfile removed; stat err = %v", err)
	}

	// New row appears BEFORE the existing HRD-099 row (prepend semantics).
	idx101 := strings.Index(s, "HRD-101")
	idx099 := strings.Index(s, "HRD-099")
	if idx101 < 0 || idx099 < 0 || idx101 >= idx099 {
		t.Errorf("expected HRD-101 to precede HRD-099; idx101=%d idx099=%d", idx101, idx099)
	}
}

func TestDocsIssueOpener_TitleWithPipeEscaped(t *testing.T) {
	dir := t.TempDir()
	o := newTestOpener(t, dir)

	err := o.OpenIssue(context.Background(), IssuePayload{
		Type:        "bug",
		Criticality: "middle",
		Title:       "pipe | in | title",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(o.IssuesPath)
	s := string(data)
	if !strings.Contains(s, `pipe \| in \| title`) {
		t.Errorf("expected pipes in title to be escaped; file:\n%s", s)
	}
}

func TestDocsIssueOpener_MissingOpenHeading(t *testing.T) {
	dir := t.TempDir()
	bad := writeFixture(t, dir, "Issues.md", "# Herald — Issues\n\n## Closed\n\n(none)\n")
	o := &DocsIssueOpener{
		IssuesPath: bad,
		FixedPath:  "",
		LockPath:   filepath.Join(dir, ".issues.lock"),
		Clock:      commons.NewFakeClock(),
	}
	err := o.OpenIssue(context.Background(), IssuePayload{Type: "bug", Criticality: "low", Title: "x"})
	if err == nil {
		t.Fatal("expected error on missing '## Open' heading; got nil")
	}
	if !strings.Contains(err.Error(), "## Open") {
		t.Errorf("expected error to mention '## Open'; got: %v", err)
	}
}

func TestDocsIssueOpener_ConcurrentAllocatesDistinct(t *testing.T) {
	dir := t.TempDir()
	o := newTestOpener(t, dir)

	const N = 2
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = o.OpenIssue(context.Background(), IssuePayload{
				Type:        "task",
				Criticality: "middle",
				Title:       "concurrent",
			})
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("goroutine %d: %v", i, e)
		}
	}

	// §107: re-read file from disk; assert two distinct HRD-NNN rows
	// allocated, no race-induced row loss, no file corruption.
	data, err := os.ReadFile(o.IssuesPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	// Both new "concurrent" rows present.
	gotConcurrent := strings.Count(s, "| open | middle | concurrent |")
	if gotConcurrent != 2 {
		t.Errorf("expected 2 concurrent rows; got %d in:\n%s", gotConcurrent, s)
	}

	// Two distinct HRD numbers (101 + 102).
	hrdRowRE := regexp.MustCompile(`HRD-(\d+) \| task \| open \| middle \| concurrent`)
	matches := hrdRowRE.FindAllStringSubmatch(s, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 HRD-NNN concurrent rows; got %d matches in:\n%s", len(matches), s)
	}
	if matches[0][1] == matches[1][1] {
		t.Errorf("expected distinct HRD numbers; both = %s", matches[0][1])
	}
	if !((matches[0][1] == "101" && matches[1][1] == "102") ||
		(matches[0][1] == "102" && matches[1][1] == "101")) {
		t.Errorf("expected HRD numbers {101,102}; got {%s,%s}", matches[0][1], matches[1][1])
	}

	// Existing sentinel row preserved.
	if !strings.Contains(s, "HRD-099") {
		t.Errorf("HRD-099 sentinel lost under concurrent writes; file:\n%s", s)
	}

	// File ends with a newline (no corruption from interleaving).
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Errorf("expected trailing newline after concurrent writes")
	}

	// Lockfile cleaned up.
	if _, err := os.Stat(o.LockPath); !os.IsNotExist(err) {
		t.Errorf("expected lockfile removed after concurrent calls; stat err = %v", err)
	}
}
