package infra

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindQuickstartCompose is an anti-bluff unit test that proves
// findQuickstartCompose actually walks the filesystem and returns the
// correct path (rather than silently succeeding on a hardcoded value).
func TestFindQuickstartCompose(t *testing.T) {
	// Walking up from the package dir, we expect to find
	// <repo-root>/quickstart/docker-compose.quickstart.yml.
	path, err := findQuickstartCompose()
	if err != nil {
		t.Fatalf("findQuickstartCompose: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path; got %q", path)
	}
	if filepath.Base(path) != "docker-compose.quickstart.yml" {
		t.Errorf("filename = %q; want docker-compose.quickstart.yml", filepath.Base(path))
	}
	// Sanity: the file must actually exist (anti-bluff against a function
	// that returns a path it never verified).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat resolved path: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("resolved compose file is empty — would be a silent bluff if Up() proceeded")
	}
}

func TestFindQuickstartCompose_HonoursWorkingDir(t *testing.T) {
	// Move into a deep subdir; walk-up must still find the compose file.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	// Walk up to repo root + into the deepest nested test dir we know exists.
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.Chdir("commons_constitution/ladder"); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	path, err := findQuickstartCompose()
	if err != nil {
		t.Fatalf("findQuickstartCompose from deep subdir: %v", err)
	}
	if filepath.Base(path) != "docker-compose.quickstart.yml" {
		t.Errorf("filename = %q; want docker-compose.quickstart.yml", filepath.Base(path))
	}
}

func TestNewQuickstartBoot_DefaultsApply(t *testing.T) {
	b, err := NewQuickstartBoot(Config{})
	if err != nil {
		// Allowed to fail with "no compose runtime" on machines without
		// docker/podman installed. SKIP per §11.4.69 closed-set reason.
		t.Skipf("compose runtime not available: %v (closed-set reason: hardware_not_present)", err)
	}
	if b.project.Name != DefaultProjectName {
		t.Errorf("project name = %q; want %q", b.project.Name, DefaultProjectName)
	}
	if filepath.Base(b.project.File) != "docker-compose.quickstart.yml" {
		t.Errorf("compose file basename = %q; want docker-compose.quickstart.yml", filepath.Base(b.project.File))
	}
}
