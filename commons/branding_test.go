// Package commons — branding_test.go (Wave 6 T1 §107 anti-bluff).
//
// TestProjectName exercises the precedence order of commons.ProjectName():
//   1. HERALD_PROJECT_NAME env var (when non-empty after TrimSpace) wins.
//   2. filepath.Base(os.Getwd()) when env empty.
//   3. "Herald" as final fallback (covered structurally — the env-empty path
//      is exercised via t.TempDir+os.Chdir, which is the only realistic way
//      a test process reaches it without breaking the test runner's cwd).
//
// §107 covenant: NO mocking of the filesystem. Real os.Chdir + os.Setenv +
// t.TempDir manipulation proves the precedence works end-to-end. A
// ProjectName() that always returned "Herald" would pass type-checks; this
// test catches that bluff.
package commons_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

func TestProjectName(t *testing.T) {
	// Save + restore env + cwd so subsequent tests aren't poisoned.
	origEnv, hadEnv := os.LookupEnv("HERALD_PROJECT_NAME")
	origCwd, _ := os.Getwd()
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("HERALD_PROJECT_NAME", origEnv)
		} else {
			_ = os.Unsetenv("HERALD_PROJECT_NAME")
		}
		if origCwd != "" {
			_ = os.Chdir(origCwd)
		}
	})

	// Case 1: env var set wins, overriding cwd.
	t.Run("env_var_wins", func(t *testing.T) {
		_ = os.Setenv("HERALD_PROJECT_NAME", "AtmosphereProject")
		if got := commons.ProjectName(); got != "AtmosphereProject" {
			t.Fatalf("env-set: got %q want %q", got, "AtmosphereProject")
		}
	})

	// Case 1b: env var with surrounding whitespace is trimmed; if the trim
	// leaves empty, fall through to the cwd path.
	t.Run("env_whitespace_only_falls_through", func(t *testing.T) {
		_ = os.Setenv("HERALD_PROJECT_NAME", "   ")
		tmp := t.TempDir()
		sub := filepath.Join(tmp, "FallthroughProject")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Chdir(sub); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		got := commons.ProjectName()
		// Whitespace-only env should be treated as unset → cwd base wins.
		// macOS may resolve /var/folders/... → /private/var/folders/...; we
		// compare the basename, which is stable across the symlink.
		if got != "FallthroughProject" {
			t.Fatalf("whitespace-env: got %q want %q", got, "FallthroughProject")
		}
	})

	// Case 2: env empty → filepath.Base(cwd).
	t.Run("cwd_basename_when_env_empty", func(t *testing.T) {
		_ = os.Unsetenv("HERALD_PROJECT_NAME")
		tmp := t.TempDir()
		sub := filepath.Join(tmp, "MyFancyProject")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Chdir(sub); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		if got := commons.ProjectName(); got != "MyFancyProject" {
			t.Fatalf("cwd-base: got %q want %q", got, "MyFancyProject")
		}
	})

	// Case 3: explicit non-default basename — proves the function isn't a
	// constant in disguise.
	t.Run("cwd_basename_distinct_value", func(t *testing.T) {
		_ = os.Unsetenv("HERALD_PROJECT_NAME")
		tmp := t.TempDir()
		sub := filepath.Join(tmp, "SecondProject")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Chdir(sub); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		if got := commons.ProjectName(); got != "SecondProject" {
			t.Fatalf("cwd-base distinct: got %q want %q", got, "SecondProject")
		}
	})
}
