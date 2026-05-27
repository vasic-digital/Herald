package main

// §11.4.85 stress test for the §43 project-lifecycle command bodies (cluster C1).
//
// HERMETIC + concurrent. The load-bearing concurrency surfaces here are:
//
//   - the §2 commit-lock (O_CREATE|O_EXCL) under contention — N goroutines race
//     to acquire the same lockfile; EXACTLY ONE may hold it at a time, and the
//     winner must release it so the next can proceed. A double-hold (two
//     goroutines both seeing "freshly held") would be a §2 correctness defect.
//   - the HRD-023 classify+emit path under sustained concurrent invocation — the
//     in-memory pipeline must not data-race (run under -race) across many
//     concurrent EvaluateSubject calls from independent command runs.
//   - install-upstreams --apply idempotency under repeat invocation — re-running
//     converges to the same configured-remote set (no duplicate/garbled remotes).
//
// Failure-injection (chaos) is exercised by the FAIL-path tests in
// gitops_cmds_test.go (push to a missing remote, behind-upstream, drifted
// submodule pin, non-HRD reopen arg) — each injects a real fault and asserts the
// honest error / breach verdict rather than a silent PASS.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// TestStress_CommitLockMutualExclusion races N goroutines to acquire the same
// §2 commit-lock. The invariant: the number of concurrently-held locks never
// exceeds 1. Each winner releases before the next can win.
func TestStress_CommitLockMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".commit_all.lock")
	ctx := context.Background()

	const workers = 16
	var (
		held      int32 // currently-held count — must never exceed 1
		maxHeld   int32
		successes int32
		wg        sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := acquireCommitLock(ctx, lockPath)
			if err != nil || !ok {
				return
			}
			atomic.AddInt32(&successes, 1)
			cur := atomic.AddInt32(&held, 1)
			for {
				m := atomic.LoadInt32(&maxHeld)
				if cur <= m || atomic.CompareAndSwapInt32(&maxHeld, m, cur) {
					break
				}
			}
			// Hold briefly, then release (the §11.4.88 "release the instant work is
			// durable" contract — here the held window is the critical section).
			atomic.AddInt32(&held, -1)
			_ = os.Remove(lockPath)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxHeld); got > 1 {
		t.Fatalf("commit-lock allowed %d concurrent holders; §2 single-entrypoint requires exactly 1", got)
	}
	if atomic.LoadInt32(&successes) == 0 {
		t.Fatal("no goroutine ever acquired the commit-lock")
	}
	// REAL side-effect: lock released at the end (no residue).
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("commit-lock residue after all workers released")
	}
}

// TestStress_ConcurrentClassifyEmit drives many concurrent classify+emit runs
// across all six rules. Under -race this proves the in-memory pipeline + binding
// catalogue are safe for concurrent command invocation (no shared mutable state
// leaks across runs). Each run builds its own pipeline (the per-command seam).
func TestStress_ConcurrentClassifyEmit(t *testing.T) {
	work := newWorkRepo(t)
	const runs = 24
	var wg sync.WaitGroup
	errs := make(chan error, runs)
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := newFetchGuardCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			// No --fetch + no upstream ⇒ "treating as rebased" PASS path; no network.
			cmd.SetArgs([]string{"--repo", work, "--emit"})
			if err := cmd.Execute(); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent fetch-guard classify+emit failed: %v", err)
	}
}

// TestStress_InstallUpstreamsIdempotentRepeat re-runs --apply many times and
// asserts the configured-remote set converges (no duplicates, stable URLs).
func TestStress_InstallUpstreamsIdempotentRepeat(t *testing.T) {
	work := newWorkRepo(t)
	upDir := filepath.Join(work, "upstreams")
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"GitHub.sh", "GitLab.sh"} {
		if err := os.WriteFile(filepath.Join(upDir, n),
			[]byte("export UPSTREAMABLE_REPOSITORY=\"file:///tmp/"+n+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		cmd := newInstallUpstreamsCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--repo", work, "--apply"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("install-upstreams repeat #%d: %v\n%s", i, err, buf.String())
		}
	}
	// REAL side-effect: exactly two remotes, no duplicates.
	out := tgit(t, work, "remote")
	count := 0
	for _, ln := range bytes.Fields([]byte(out)) {
		switch string(ln) {
		case "github", "gitlab":
			count++
		default:
			t.Fatalf("unexpected remote %q after idempotent repeat", ln)
		}
	}
	if count != 2 {
		t.Fatalf("after 5 idempotent --apply runs: %d remotes, want 2", count)
	}
}
