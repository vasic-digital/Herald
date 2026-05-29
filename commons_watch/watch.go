// Package commons_watch watches a set of files (a SQLite .db single-source-of-truth
// plus its Markdown trackers) and emits coalesced change events.
//
// It combines two detection mechanisms:
//
//  1. fsnotify — kernel-level inotify/kqueue/FSEvents notifications on the watched
//     files (and their parent directories, so atomic rename-replace writes are seen).
//
//  2. A WAL-poll fallback (PollInterval > 0) — SQLite operating in WAL journal mode
//     writes go to the "<db>-wal" sidecar file and are only periodically checkpointed
//     back into the main ".db" file. As a result a logical DB mutation may produce NO
//     reliable fsnotify Write event on the main ".db" inode within a useful window.
//     To catch those, the poll loop stats each watched path's (mtime, size) at the
//     configured interval and synthesizes an Event when either changes. The two paths
//     are de-duplicated by (path + debounce-window) so a change seen by BOTH fsnotify
//     and the poll loop is emitted only once.
package commons_watch

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultDebounce = 200 * time.Millisecond
)

// Event is a coalesced change notification for a single watched path.
type Event struct {
	// Path is the watched file path that changed.
	Path string
	// Op is a short description of the originating signal ("fsnotify" or "poll").
	Op string
}

// Options configures a Watcher.
type Options struct {
	// Paths is the set of file paths to watch.
	Paths []string
	// Debounce is the window within which rapid successive changes on the same path
	// are coalesced into a single emitted Event. Defaults to 200ms when zero.
	Debounce time.Duration
	// PollInterval, when > 0, enables the WAL-poll fallback: each watched path's
	// (mtime, size) is sampled at this interval and a synthetic Event is emitted on
	// change. Zero disables polling (fsnotify only).
	PollInterval time.Duration
}

// fileStat is a lightweight snapshot of a file's mtime + size for poll comparison.
type fileStat struct {
	mtime time.Time
	size  int64
	known bool
}

// Watcher watches a set of file paths and emits coalesced Events.
type Watcher struct {
	opts   Options
	fsw    *fsnotify.Watcher
	events chan Event

	mu       sync.Mutex
	lastEmit map[string]time.Time // path -> last time an Event was emitted (debounce/dedup key)
	pending  map[string]*time.Timer

	closeOnce sync.Once
	closed    chan struct{}
}

// New constructs a Watcher watching opts.Paths. It registers each path AND its parent
// directory with fsnotify (so editor / SQLite atomic rename-replace writes are observed).
func New(opts Options) (*Watcher, error) {
	if opts.Debounce <= 0 {
		opts.Debounce = defaultDebounce
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		opts:     opts,
		fsw:      fsw,
		events:   make(chan Event, 64),
		lastEmit: make(map[string]time.Time),
		pending:  make(map[string]*time.Timer),
		closed:   make(chan struct{}),
	}

	// Track which directories we've added to avoid duplicate adds.
	seenDir := make(map[string]bool)
	for _, p := range opts.Paths {
		// Add the file itself (best-effort: it may not exist yet).
		_ = fsw.Add(p)
		dir := dirOf(p)
		if dir != "" && !seenDir[dir] {
			if err := fsw.Add(dir); err != nil {
				_ = fsw.Close()
				return nil, err
			}
			seenDir[dir] = true
		}
	}

	return w, nil
}

// dirOf returns the parent directory of a path, or "" if it has none.
func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == os.PathSeparator || p[i] == '/' {
			if i == 0 {
				return string(p[0])
			}
			return p[:i]
		}
	}
	return ""
}

// Events returns the read-only channel on which coalesced Events are delivered.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// watchedSet returns the set of paths this Watcher cares about for fast membership tests.
func (w *Watcher) watchedSet() map[string]bool {
	m := make(map[string]bool, len(w.opts.Paths))
	for _, p := range w.opts.Paths {
		m[p] = true
	}
	return m
}

// Start runs the watch loops until ctx is cancelled. It returns ctx.Err() (or nil)
// when it stops. fsnotify and (optionally) the poll loop run as child goroutines that
// are torn down on ctx cancel.
func (w *Watcher) Start(ctx context.Context) error {
	watched := w.watchedSet()

	var wg sync.WaitGroup

	// Poll loop (WAL fallback) — only when enabled.
	if w.opts.PollInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.pollLoop(ctx)
		}()
	}

	// fsnotify event loop.
	for {
		select {
		case <-ctx.Done():
			w.cancelPending()
			wg.Wait()
			return ctx.Err()
		case ev, ok := <-w.fsw.Events:
			if !ok {
				w.cancelPending()
				wg.Wait()
				return nil
			}
			if watched[ev.Name] {
				w.schedule(ctx, ev.Name, "fsnotify")
			}
		case _, ok := <-w.fsw.Errors:
			if !ok {
				w.cancelPending()
				wg.Wait()
				return nil
			}
			// fsnotify errors are non-fatal here; the poll loop (if any) still covers us.
		}
	}
}

// pollLoop samples each watched path's (mtime, size) at PollInterval and emits a
// synthetic Event when a change is detected. See the package doc for the WAL rationale.
func (w *Watcher) pollLoop(ctx context.Context) {
	prev := make(map[string]fileStat, len(w.opts.Paths))
	// Seed the baseline so the first observed change (not the initial state) fires.
	for _, p := range w.opts.Paths {
		prev[p] = statOf(p)
	}

	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, p := range w.opts.Paths {
				cur := statOf(p)
				old := prev[p]
				if changed(old, cur) {
					prev[p] = cur
					w.schedule(ctx, p, "poll")
				}
			}
		}
	}
}

// statOf snapshots a path's mtime + size. A missing file yields known=false.
func statOf(p string) fileStat {
	fi, err := os.Stat(p)
	if err != nil {
		return fileStat{known: false}
	}
	return fileStat{mtime: fi.ModTime(), size: fi.Size(), known: true}
}

// changed reports whether the (mtime, size) snapshot differs in a way that signals a write.
func changed(old, cur fileStat) bool {
	if old.known != cur.known {
		return true
	}
	if !cur.known {
		return false
	}
	return !old.mtime.Equal(cur.mtime) || old.size != cur.size
}

// schedule coalesces signals for a path within the debounce window into a single Event.
// Both the fsnotify and poll paths funnel through here, so a change observed by both is
// emitted at most once per debounce window (dedup by path + window).
func (w *Watcher) schedule(ctx context.Context, path, op string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Dedup: if we already emitted for this path within the debounce window, drop.
	if last, ok := w.lastEmit[path]; ok && time.Since(last) < w.opts.Debounce {
		return
	}

	// If a timer is already pending for this path, the rapid follow-up is coalesced
	// into the existing pending emit — do nothing.
	if _, pending := w.pending[path]; pending {
		return
	}

	op0 := op
	t := time.AfterFunc(w.opts.Debounce, func() {
		w.mu.Lock()
		delete(w.pending, path)
		w.lastEmit[path] = time.Now()
		w.mu.Unlock()

		select {
		case w.events <- Event{Path: path, Op: op0}:
		case <-ctx.Done():
		case <-w.closed:
		}
	})
	w.pending[path] = t
}

// cancelPending stops any in-flight debounce timers (called on shutdown).
func (w *Watcher) cancelPending() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for p, t := range w.pending {
		t.Stop()
		delete(w.pending, p)
	}
}

// Close releases the underlying fsnotify watcher. It is safe to call multiple times.
func (w *Watcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.closed)
		err = w.fsw.Close()
	})
	return err
}
