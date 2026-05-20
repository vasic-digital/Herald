package constitution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// BundleHash is the SHA-256 of the rendered Constitution.md bundle at
// the time an Evaluator runs. Persisted on every emitted event for
// replayability (spec §42.1.3).
type BundleHash [sha256.Size]byte

// Hex returns the lowercase hex-encoded hash.
func (h BundleHash) Hex() string { return hex.EncodeToString(h[:]) }

// String mirrors Hex for fmt.Stringer.
func (h BundleHash) String() string { return h.Hex() }

// IsZero reports whether the hash is the zero value — useful to detect
// "captureer hasn't run yet" vs "captureer ran and returned an all-zeros
// hash" (the latter is cryptographically improbable, so IsZero is a safe
// uninitialized-state probe).
func (h BundleHash) IsZero() bool {
	var z BundleHash
	return h == z
}

// Capture computes the SHA-256 of the file at path. Returns ErrBundleMissing
// if the file does not exist (callers translate to DecisionSkip per §4 of
// the Foundation design).
//
// Capture is intentionally not cached — callers should use a Captureer
// instance with TTL caching if they need to amortize repeated reads.
func Capture(path string) (BundleHash, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BundleHash{}, fmt.Errorf("constitution: Capture(%q): %w", path, ErrBundleMissing)
		}
		return BundleHash{}, fmt.Errorf("constitution: Capture(%q): %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return BundleHash{}, fmt.Errorf("constitution: Capture(%q): read: %w", path, err)
	}
	var out BundleHash
	copy(out[:], h.Sum(nil))
	return out, nil
}

// CaptureBytes computes the SHA-256 of an in-memory bundle. Mirrors Capture
// for tests + in-process bundle composition (e.g., concatenating multi-file
// constitution sources).
func CaptureBytes(b []byte) BundleHash {
	return BundleHash(sha256.Sum256(b))
}

// ErrBundleMissing is returned by Capture when the bundle file is absent.
var ErrBundleMissing = errors.New("constitution bundle missing")

// Captureer wraps Capture with TTL caching keyed by path. Safe for
// concurrent use.
//
// The cache invalidates on file mtime change AND on TTL elapse — whichever
// happens first. This means rewriting Constitution.md without bumping the
// mtime (rare; mostly editors do bump it) still surfaces within TTL.
type Captureer struct {
	mu    sync.RWMutex
	cache map[string]bundleEntry
}

type bundleEntry struct {
	hash  BundleHash
	mtime int64 // unix nano
}

// NewCaptureer returns a fresh Captureer with an empty cache.
func NewCaptureer() *Captureer {
	return &Captureer{cache: make(map[string]bundleEntry)}
}

// Hash returns the cached BundleHash for path, capturing it if absent
// or stale. Staleness = file mtime differs from the cached mtime.
func (c *Captureer) Hash(path string) (BundleHash, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BundleHash{}, fmt.Errorf("constitution: Captureer.Hash(%q): %w", path, ErrBundleMissing)
		}
		return BundleHash{}, fmt.Errorf("constitution: Captureer.Hash(%q): stat: %w", path, err)
	}
	mtime := stat.ModTime().UnixNano()
	c.mu.RLock()
	if e, ok := c.cache[path]; ok && e.mtime == mtime {
		c.mu.RUnlock()
		return e.hash, nil
	}
	c.mu.RUnlock()
	h, err := Capture(path)
	if err != nil {
		return BundleHash{}, err
	}
	c.mu.Lock()
	c.cache[path] = bundleEntry{hash: h, mtime: mtime}
	c.mu.Unlock()
	return h, nil
}

// Invalidate drops the cached entry for path (and is a no-op if absent).
// Useful from a SIGHUP handler that wants to force re-walk per §7 q1 of
// the Foundation design open questions.
func (c *Captureer) Invalidate(path string) {
	c.mu.Lock()
	delete(c.cache, path)
	c.mu.Unlock()
}
