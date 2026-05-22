// qaherald/cmd/qaherald/main_test.go — §107 anti-bluff anchor for Wave
// 5 Task 1.
//
// T10 (Wave 5 mutation gate) plants an always-"0.0.0-dev" bluff in this
// file's `version` package variable. This test builds qaherald with
// `-ldflags "-X main.version=<canary>"` to a t.TempDir(), executes
// `qaherald version`, and asserts the canary surfaces verbatim in the
// output. If the build-time injection is broken (e.g. the variable was
// renamed, the cli.BuildVersion propagation was removed, or the
// mutation gate planted a `const version = "0.0.0-dev"` shortcut) the
// canary won't appear and this test will FAIL.
//
// The build-and-exec pattern mirrors pherald/cmd/pherald/migrate_test.go's
// shell-out integration shape (canonical for Cobra-driven CLIs in this
// repo). We deliberately do NOT call main.main() in-process — that
// would not exercise the build-time -ldflags path which is the very
// thing the §107 anchor guards.
package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionSubcommand_LDFlagsInjection builds the qaherald binary
// with `-ldflags "-X main.version=<canary>"` and asserts the canary
// appears verbatim in the `version` subcommand output. Anti-bluff
// anchor for T10 mutation gate (b) — the always-"0.0.0" bluff.
func TestVersionSubcommand_LDFlagsInjection(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH — skipping build-driven test")
	}

	const canaryVersion = "test-1.2.3-qaherald-ldflags-canary"
	const canaryCommit = "abc1234"

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "qaherald")

	// Build: -ldflags injects the canary into main.version + main.commit.
	// We target the package containing this test (".") rather than the
	// module-qualified path so the build runs in the same module context
	// as the test (workspace-aware via go.work).
	build := exec.Command("go", "build",
		"-ldflags", "-X main.version="+canaryVersion+" -X main.commit="+canaryCommit,
		"-o", binPath, ".")
	var bo bytes.Buffer
	build.Stdout = &bo
	build.Stderr = &bo
	if err := build.Run(); err != nil {
		t.Fatalf("go build failed: %v\noutput:\n%s", err, bo.String())
	}

	// Execute: `qaherald version` — expect the canary verbatim.
	run := exec.Command(binPath, "version")
	var ro bytes.Buffer
	run.Stdout = &ro
	run.Stderr = &ro
	if err := run.Run(); err != nil {
		t.Fatalf("%s version failed: %v\noutput:\n%s", binPath, err, ro.String())
	}
	out := ro.String()
	if !strings.Contains(out, canaryVersion) {
		t.Fatalf("expected version output to contain %q (proves -ldflags injection lives), got:\n%s",
			canaryVersion, out)
	}
	if !strings.Contains(out, canaryCommit) {
		t.Fatalf("expected version output to contain commit %q (proves -ldflags injection of main.commit lives), got:\n%s",
			canaryCommit, out)
	}
	// Anti-bluff: also assert the FLAVOR is correct ("qa" — not a stray
	// "p" or "s" copy-paste from the pherald/sherald template).
	if !strings.Contains(out, "flavor:     qa") && !strings.Contains(out, `"flavor":"qa"`) {
		t.Fatalf("expected version output to identify flavor as 'qa', got:\n%s", out)
	}
}

// TestVersionSubcommand_JSONFlag asserts the `--json` flag emits the
// canonical JSON shape (binary/flavor/version/go_version/os/arch all
// non-empty) — the same shape e2e_bluff_hunt E2/E19-E24 asserts across
// the other flavor binaries. Anti-bluff: a mutation that emits empty
// strings here would silently pass a grep-only PASS but be caught by
// the field-presence asserts below.
func TestVersionSubcommand_JSONFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH — skipping build-driven test")
	}

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "qaherald")

	build := exec.Command("go", "build", "-o", binPath, ".")
	var bo bytes.Buffer
	build.Stdout = &bo
	build.Stderr = &bo
	if err := build.Run(); err != nil {
		t.Fatalf("go build failed: %v\noutput:\n%s", err, bo.String())
	}

	run := exec.Command(binPath, "version", "--json")
	var ro bytes.Buffer
	run.Stdout = &ro
	run.Stderr = &ro
	if err := run.Run(); err != nil {
		t.Fatalf("%s version --json failed: %v\noutput:\n%s", binPath, err, ro.String())
	}
	out := ro.String()
	// Required keys per cli.VersionCmd contract. Strings are deliberately
	// asserted via substring — full JSON-decode would couple this test
	// to encoding/json's key ordering.
	for _, key := range []string{
		`"binary":"qaherald"`,
		`"flavor":"qa"`,
		`"version":"`,
		`"go_version":"go`,
		`"os":"`,
		`"arch":"`,
	} {
		if !strings.Contains(out, key) {
			t.Fatalf("expected JSON output to contain %q, got:\n%s", key, out)
		}
	}
}
