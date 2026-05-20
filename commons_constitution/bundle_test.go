package constitution

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureBytes_Determinism(t *testing.T) {
	in := []byte("Helix Universal Constitution §11.4.74 — catalogue-first.")
	a := CaptureBytes(in)
	b := CaptureBytes(in)
	if a != b {
		t.Errorf("CaptureBytes is non-deterministic: %s vs %s", a.Hex(), b.Hex())
	}
	want := sha256.Sum256(in)
	if a != BundleHash(want) {
		t.Errorf("CaptureBytes diverges from canonical SHA-256: got %x want %x", a, want)
	}
}

func TestCaptureBytes_OneByteMutationDrift(t *testing.T) {
	a := CaptureBytes([]byte("Constitution.md"))
	b := CaptureBytes([]byte("constitution.md")) // single-char case change
	if a == b {
		t.Errorf("CaptureBytes hash collision on one-byte mutation; this should be cryptographically impossible")
	}
}

func TestCapture_FileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.md")
	body := []byte("# Helix Constitution\n\n…rules go here…\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := Capture(path)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	want := CaptureBytes(body)
	if got != want {
		t.Errorf("Capture(file) = %s; want %s (CaptureBytes of same body)", got, want)
	}
}

func TestCapture_MissingFile(t *testing.T) {
	_, err := Capture(filepath.Join(t.TempDir(), "absent.md"))
	if !errors.Is(err, ErrBundleMissing) {
		t.Errorf("Capture(missing) returned %v; want errors.Is(ErrBundleMissing) == true", err)
	}
}

func TestBundleHash_Hex(t *testing.T) {
	h := CaptureBytes([]byte("x"))
	hex := h.Hex()
	if len(hex) != 64 {
		t.Errorf("Hex() length = %d; want 64 chars (32 bytes × 2)", len(hex))
	}
	// Idempotent.
	if h.Hex() != h.String() {
		t.Errorf("Hex() and String() diverged: %q vs %q", h.Hex(), h.String())
	}
}

func TestBundleHash_IsZero(t *testing.T) {
	var z BundleHash
	if !z.IsZero() {
		t.Error("zero-value BundleHash should report IsZero() = true")
	}
	if CaptureBytes([]byte("x")).IsZero() {
		t.Error("non-zero hash should report IsZero() = false")
	}
}

func TestCaptureer_CachesUntilMTimeChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cached.md")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	c := NewCaptureer()
	h1, err := c.Hash(path)
	if err != nil {
		t.Fatalf("Hash v1: %v", err)
	}
	// Same call — must hit cache (we'll verify drift below by mutating).
	h2, err := c.Hash(path)
	if err != nil {
		t.Fatalf("Hash v1 second call: %v", err)
	}
	if h1 != h2 {
		t.Errorf("Captureer.Hash returned different values for unchanged file: %s vs %s", h1, h2)
	}

	// Force mtime change by writing different content with future mtime.
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	h3, err := c.Hash(path)
	if err != nil {
		t.Fatalf("Hash v2: %v", err)
	}
	if h3 == h1 {
		t.Errorf("Captureer.Hash returned stale value after file mutation: %s == %s", h3, h1)
	}
}

func TestCaptureer_Invalidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.md")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := NewCaptureer()
	if _, err := c.Hash(path); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	c.Invalidate(path) // must not panic on present-and-cached
	c.Invalidate(path) // idempotent
	c.Invalidate(filepath.Join(dir, "missing.md")) // idempotent on absent

	// After Invalidate + content change WITHOUT mtime change, the next Hash
	// MUST recompute. Most filesystems update mtime on write, so we
	// additionally explicitly set mtime to the original to test the path.
	st, _ := os.Stat(path)
	if err := os.WriteFile(path, []byte("ab"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	_ = os.Chtimes(path, st.ModTime(), st.ModTime())

	h, err := c.Hash(path)
	if err != nil {
		t.Fatalf("Hash after invalidate: %v", err)
	}
	want := CaptureBytes([]byte("ab"))
	if h != want {
		t.Errorf("after Invalidate, Hash returned stale %s; want %s", h, want)
	}
}
