// pherald watch — HRD-153 (ATMOSphere integration WS-2/WS-7).
//
// Long-running subcommand that composes the workable-items SSoT
// watcher → diff → notify outbound flow end-to-end:
//
//   - commons_workable.Open(db) + NewRepo — the shared ATMOSphere/Herald
//     SQLite single-source-of-truth.
//   - commons_watch.Watcher — fsnotify + WAL-poll fallback on the .db file
//     and the Markdown trackers (Issues.md / Fixed.md).
//   - commons_workable.Diff(prev, curr) — per-property change set between
//     consecutive snapshots.
//   - pherald/internal/workflow.Notifier — renders each Change and fans it
//     out through the REAL runner.ChannelDispatcher (Stage-6 outbound).
//
// §107 anti-bluff posture: the watch command's PASS is NOT "the process
// boots + the watcher starts". It is "a real DB mutation produces a real
// rendered diff message dispatched through the real fan-out to a real
// channel". watch_test proves exactly that against a recording channel +
// temp SQLite DB + the real fsnotify watcher — no mock of the pipeline.
//
// The runWatch helper is the testable seam: production RunE constructs the
// real Repo + Watcher paths + Notifier (the real ChannelDispatcher over the
// configured channels) and calls runWatch; the test constructs the same
// real components over a recording channel + temp DB and calls runWatch
// directly (no binary spawn — fast + deterministic).
//
// Env / flags:
//
//	--db <path>          DB path. Default $HERALD_WORKABLE_DB or
//	                     "docs/workable_items.db".
//	--issues <path>      Issues.md tracker path (watched). Default
//	                     "docs/Issues.md".
//	--fixed <path>       Fixed.md tracker path (watched). Default
//	                     "docs/Fixed.md".
//	--poll <duration>    WAL-poll fallback interval. Default 1s.
//	plus the channel/recipient config — reused from listen.go's
//	loadListenConfigFromEnv channel-setup helpers (HERALD_CHANNELS +
//	per-channel namespaced env), which is the production fan-out path.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_watch"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/runner"
	"github.com/vasic-digital/herald/pherald/internal/workflow"
)

// watchDeps carries the resolved, fully-real dependencies for runWatch.
// Production RunE populates these from flags/env (the real Repo + the real
// ChannelDispatcher wrapped in a Notifier); watch_test populates the same
// real types over a recording channel + temp DB so the pipeline is driven
// hermetically with zero pipeline mocks.
type watchDeps struct {
	// Repo is the open commons_workable CRUD repo over the SQLite SSoT.
	Repo *workable.Repo
	// Locations is the set of current_location values to snapshot + diff
	// (production: {"Issues","Fixed"}).
	Locations []string
	// Paths are the filesystem paths to watch (the .db file + the MD
	// trackers). A change on ANY of them triggers a re-list + diff.
	Paths []string
	// Notifier is the real workflow.Notifier over the real
	// runner.ChannelDispatcher — the production outbound fan-out.
	Notifier *workflow.Notifier
	// PollInterval enables the commons_watch WAL-poll fallback. Required for
	// reliable SQLite-WAL change detection (writes land in the -wal sidecar,
	// so the main .db inode may not emit a timely fsnotify Write).
	PollInterval time.Duration
	// Debounce coalesces rapid successive changes per path. Zero → watcher
	// default (200ms).
	Debounce time.Duration
	// Ready, when non-nil, is closed exactly once by runWatch immediately
	// after the initial baseline snapshot is taken AND the watcher is started
	// — i.e. when runWatch is guaranteed to observe any subsequent mutation.
	// Production leaves it nil. The e2e test sets it and waits on it before
	// issuing the first DB mutation, eliminating the boot scheduling race
	// deterministically (no sleeps). It is a startup-ordering signal only,
	// NOT a pipeline mock — the watcher→diff→notify path is fully real.
	Ready chan struct{}
}

// newWatchCmd wires the `pherald watch` Cobra subcommand. The production
// path builds the real Repo + Notifier over the configured channels (reusing
// listen.go's channel-setup helpers) and delegates the loop to runWatch.
func newWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch the workable-items SSoT and notify channels on every change",
		Long: `Long-running. Composes the workable-items single-source-of-truth
watcher → diff → notify outbound flow (HRD-153, ATMOSphere integration
WS-2/WS-7):

  1. Opens the shared workable-items SQLite DB (--db) and snapshots every
     item at the watched locations (Issues + Fixed).
  2. Starts a commons_watch.Watcher (fsnotify + WAL-poll) on the DB file
     and the Markdown trackers (--issues / --fixed).
  3. On every change: re-lists the current items, diffs against the prior
     snapshot, and — if anything changed — renders each per-property delta
     and fans it out through the production ChannelDispatcher to every
     configured recipient's channel.

Channel/recipient config reuses the same HERALD_CHANNELS + per-channel
namespaced env as ` + "`pherald listen`" + ` (the production fan-out path).

Signal handling: SIGINT/SIGTERM cancels the watch loop cleanly via
signal.NotifyContext.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			if dbPath == "" {
				if env := os.Getenv("HERALD_WORKABLE_DB"); env != "" {
					dbPath = env
				} else {
					dbPath = "docs/workable_items.db"
				}
			}
			issuesPath, _ := cmd.Flags().GetString("issues")
			fixedPath, _ := cmd.Flags().GetString("fixed")
			poll, _ := cmd.Flags().GetDuration("poll")

			store, err := workable.Open(dbPath)
			if err != nil {
				return fmt.Errorf("pherald watch: open workable DB %q: %w", dbPath, err)
			}
			defer store.Close()
			repo := workable.NewRepo(store)

			notifier, err := buildWatchNotifier()
			if err != nil {
				return err
			}

			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			fmt.Fprintf(cmd.OutOrStdout(),
				"pherald watch: watching %s (+ %s, %s); poll=%s\n",
				dbPath, issuesPath, fixedPath, poll)
			return runWatch(ctx, watchDeps{
				Repo:         repo,
				Locations:    []string{"Issues", "Fixed"},
				Paths:        []string{dbPath, issuesPath, fixedPath},
				Notifier:     notifier,
				PollInterval: poll,
			})
		},
	}
	cmd.Flags().String("db", "",
		"Workable-items SQLite DB path (default $HERALD_WORKABLE_DB or docs/workable_items.db)")
	cmd.Flags().String("issues", "docs/Issues.md", "Issues.md tracker path (watched)")
	cmd.Flags().String("fixed", "docs/Fixed.md", "Fixed.md tracker path (watched)")
	cmd.Flags().Duration("poll", time.Second, "WAL-poll fallback interval (0 disables, fsnotify only)")
	return cmd
}

// buildWatchNotifier constructs the production outbound fan-out: the real
// runner.ChannelDispatcher over the configured channels (reusing listen.go's
// loadEnabledChannels + perChannelConfig helpers) plus the resolved recipient
// set, wrapped in a workflow.Notifier.
//
// Wave-scope caveat: the recipient set is derived from the per-channel Target
// (the configured chat/channel id) so the watch command notifies the operator
// channel directly. Full PG-backed subscriber resolution is HRD-156 (WS-5);
// until then watch fans out to the configured channel targets, mirroring the
// explicit-recipient bypass workflow.NewNotifier already documents.
func buildWatchNotifier() (*workflow.Notifier, error) {
	enabled := loadEnabledChannels()
	channelMap := map[commons.ChannelID]commons.Channel{}
	var recipients []commons.Recipient
	for _, name := range enabled {
		ccfg, err := perChannelConfig(name)
		if err != nil {
			return nil, err
		}
		ch, err := channels.New(name, ccfg)
		if err != nil {
			return nil, fmt.Errorf("pherald watch: build %q channel: %w", name, err)
		}
		channelMap[commons.ChannelID(name)] = ch
		recipients = append(recipients, commons.Recipient{
			Channel:       name,
			ChannelUserID: ccfg.Target,
		})
	}
	if len(channelMap) == 0 {
		return nil, fmt.Errorf("pherald watch: no channels enabled (HERALD_CHANNELS resolved empty)")
	}
	dispatcher := &runner.ChannelDispatcher{Channels: channelMap}
	return workflow.NewNotifier(dispatcher, recipients), nil
}

// runWatch composes the watcher → diff → notify loop and blocks until ctx is
// cancelled. Extracted from RunE so watch_test can drive it hermetically with
// real components (real DB, real fsnotify watcher, real Diff, real Notifier
// over a recording channel) without a binary spawn.
//
// Logic: snapshot prev := List(every location); start a commons_watch.Watcher
// on the configured paths; on every Event re-list curr, Diff(prev, curr), and
// — if the change set is non-empty — Notify; then advance prev := curr. On
// ctx cancel: Close the watcher, drain, and return cleanly (no goroutine
// leak — the watcher's Start goroutine + the poll goroutine both unwind on
// ctx.Done()).
func runWatch(ctx context.Context, deps watchDeps) error {
	if deps.Repo == nil {
		return fmt.Errorf("pherald watch: nil Repo")
	}
	if deps.Notifier == nil {
		return fmt.Errorf("pherald watch: nil Notifier")
	}

	prev, err := snapshot(ctx, deps.Repo, deps.Locations)
	if err != nil {
		return fmt.Errorf("pherald watch: initial snapshot: %w", err)
	}

	// SQLite in WAL mode writes to the "<db>-wal" sidecar; the main ".db"
	// inode's (mtime,size) may not change on an INSERT/UPDATE until a
	// checkpoint. To detect logical mutations promptly we ALSO watch the
	// -wal/-shm sidecars of every watched path that looks like a DB file.
	// A change on the sidecar fires a re-list + Diff, which reads the
	// committed rows (the Store's single pinned connection sees its own WAL
	// writes). Sidecars that never exist are simply never-changing entries.
	watchPaths := withWALSidecars(deps.Paths)

	w, err := commons_watch.New(commons_watch.Options{
		Paths:        watchPaths,
		Debounce:     deps.Debounce,
		PollInterval: deps.PollInterval,
	})
	if err != nil {
		return fmt.Errorf("pherald watch: new watcher: %w", err)
	}

	// Run the watcher's detection loops in a child goroutine; Start returns
	// ctx.Err() when ctx is cancelled.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = w.Start(ctx)
	}()

	// Ensure clean teardown on every exit path: close the watcher and wait
	// for its goroutines to unwind before returning.
	defer func() {
		_ = w.Close()
		wg.Wait()
	}()

	// reconcile re-lists the current items, diffs against prev, notifies on any
	// change, and advances prev. Returns false only on ctx cancellation (the
	// caller then returns ctx.Err()). Transient list/notify errors are logged
	// but never kill the loop (a single bad poll must not silence the watcher).
	reconcile := func() bool {
		curr, err := snapshot(ctx, deps.Repo, deps.Locations)
		if err != nil {
			if ctx.Err() != nil {
				return false
			}
			fmt.Fprintf(os.Stderr, "pherald watch: re-list: %v\n", err)
			return true
		}
		changes := workable.Diff(prev, curr)
		if len(changes) > 0 {
			if err := deps.Notifier.Notify(ctx, changes); err != nil {
				if ctx.Err() != nil {
					return false
				}
				fmt.Fprintf(os.Stderr, "pherald watch: notify: %v\n", err)
			}
		}
		prev = curr
		return true
	}

	// Safety-net reconcile ticker. SQLite-WAL writes land in the -wal sidecar
	// and may not produce a timely fsnotify event on the main inode; the
	// watcher's own poll fallback covers most of that, but a periodic reconcile
	// here ALSO closes the startup race (a mutation landing between New and the
	// watcher goroutine's first poll-baseline seeding) deterministically and
	// guarantees eventual delivery regardless of filesystem-event fidelity.
	// Cadence tracks the configured PollInterval (default 1s) so it never lags
	// the watcher's own poll. Zero PollInterval → fall back to 1s so the
	// safety net still runs.
	reconcileEvery := deps.PollInterval
	if reconcileEvery <= 0 {
		reconcileEvery = time.Second
	}
	ticker := time.NewTicker(reconcileEvery)
	defer ticker.Stop()

	// Signal startup completion: the baseline prev is set, the watcher is
	// started, and the reconcile loop is about to run. Any mutation after this
	// point is guaranteed to be observed (by fsnotify, the watcher poll, or the
	// safety-net reconcile tick). See watchDeps.Ready.
	if deps.Ready != nil {
		close(deps.Ready)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if !reconcile() {
				return ctx.Err()
			}
		case _, ok := <-w.Events():
			if !ok {
				return nil
			}
			if !reconcile() {
				return ctx.Err()
			}
		}
	}
}

// withWALSidecars returns paths plus the SQLite WAL/SHM sidecars ("<p>-wal",
// "<p>-shm") for every entry whose name ends in ".db" — so WAL-mode writes
// (which land in the sidecar, not the main inode) are observed. Non-DB paths
// (the Markdown trackers) are passed through unchanged. The result is
// de-duplicated, preserving order.
func withWALSidecars(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range paths {
		add(p)
		if len(p) >= 3 && p[len(p)-3:] == ".db" {
			add(p + "-wal")
			add(p + "-shm")
		}
	}
	return out
}

// snapshot lists every item across the given locations into a single slice
// (the unit Diff consumes). Locations default to {"Issues","Fixed"} when nil.
func snapshot(ctx context.Context, repo *workable.Repo, locations []string) ([]workable.Item, error) {
	if len(locations) == 0 {
		locations = []string{"Issues", "Fixed"}
	}
	var all []workable.Item
	for _, loc := range locations {
		items, err := repo.List(ctx, loc)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}
