package commons_watch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// writeFile creates/overwrites a file with content, returning on error via t.Fatal.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// startWatcher spins up a Watcher in a goroutine and returns it plus a cancel func.
func startWatcher(t *testing.T, opts Options) (*Watcher, context.CancelFunc) {
	t.Helper()
	w, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = w.Start(ctx)
	}()
	// Give the watcher a moment to register fsnotify + poll goroutines.
	time.Sleep(100 * time.Millisecond)
	return w, cancel
}

// collectEvents drains the events channel for the given window, returning all events seen.
func collectEvents(w *Watcher, window time.Duration) []Event {
	var got []Event
	deadline := time.After(window)
	for {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-deadline:
			return got
		}
	}
}

// (a) Modifying a watched file emits an Event with the right path.
func TestWatch_EmitsOnModify(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ssot.db")
	writeFile(t, target, "v0")

	w, cancel := startWatcher(t, Options{Paths: []string{target}, Debounce: 50 * time.Millisecond})
	defer w.Close()
	defer cancel()

	writeFile(t, target, "v1")

	got := collectEvents(w, 1*time.Second)
	if len(got) == 0 {
		t.Fatalf("expected at least one event, got none")
	}
	found := false
	for _, ev := range got {
		if ev.Path == target {
			found = true
		}
	}
	if !found {
		t.Fatalf("no event for path %s; got %+v", target, got)
	}
}

// (b) Debounce coalesces 5 rapid writes into 1 event.
func TestWatch_DebounceCoalesces(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trackers.md")
	writeFile(t, target, "v0")

	w, cancel := startWatcher(t, Options{Paths: []string{target}, Debounce: 300 * time.Millisecond})
	defer w.Close()
	defer cancel()

	// 5 rapid writes well within the debounce window.
	for i := 0; i < 5; i++ {
		writeFile(t, target, "rapid")
		time.Sleep(20 * time.Millisecond)
	}

	// Wait long enough for the single debounced event to be emitted.
	got := collectEvents(w, 1*time.Second)
	count := 0
	for _, ev := range got {
		if ev.Path == target {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 coalesced event for %s, got %d (%+v)", target, count, got)
	}
}

// (c) PollInterval detects an mtime/size change on a path that is polled but NOT
// fsnotify-watched. This proves the WAL-poll fallback fires independently of fsnotify,
// simulating SQLite WAL-mode writes that update the -wal sidecar without a reliable
// fsnotify Write on the main .db file.
func TestWatch_PollFallbackDetectsSidecar(t *testing.T) {
	dir := t.TempDir()
	mainDB := filepath.Join(dir, "ssot.db")
	walSidecar := filepath.Join(dir, "ssot.db-wal")
	writeFile(t, mainDB, "v0")
	writeFile(t, walSidecar, "wal0")

	// Watch BOTH paths but rely on the poll loop to catch the sidecar change.
	w, cancel := startWatcher(t, Options{
		Paths:        []string{mainDB, walSidecar},
		Debounce:     50 * time.Millisecond,
		PollInterval: 100 * time.Millisecond,
	})
	defer w.Close()
	defer cancel()

	// Modify the sidecar and bump its mtime forward so the poll definitely sees it.
	writeFile(t, walSidecar, "wal1-bigger-content")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(walSidecar, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got := collectEvents(w, 1500*time.Millisecond)
	found := false
	for _, ev := range got {
		if ev.Path == walSidecar {
			found = true
		}
	}
	if !found {
		t.Fatalf("poll fallback did not emit event for sidecar %s; got %+v", walSidecar, got)
	}
}

// (d) ctx cancel stops Start cleanly + no goroutine leak.
func TestWatch_CancelNoGoroutineLeak(t *testing.T) {
	// Let any test-runtime goroutines settle first.
	time.Sleep(200 * time.Millisecond)
	before := runtime.NumGoroutine()

	dir := t.TempDir()
	target := filepath.Join(dir, "ssot.db")
	writeFile(t, target, "v0")

	w, err := New(Options{Paths: []string{target}, Debounce: 50 * time.Millisecond, PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Start(ctx)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Settle delay to let goroutines fully unwind.
	time.Sleep(300 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after-before > 1 {
		t.Fatalf("goroutine leak: before=%d after=%d (delta=%d)", before, after, after-before)
	}
}
