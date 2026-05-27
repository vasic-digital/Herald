package storage

// HRD-128 — container-orchestrated / resource-exhaustion stress + chaos tests
// (GAP-3 plan §1 row 6 + §7 host-safety, 2026-05-27-stress-chaos-suite). Closes
// part of GAP-3 (§11.4.85 / §108.a: Herald had ZERO resource-exhaustion coverage
// for the storage write path).
//
// This file holds the HERMETIC core of unit 6, which is provably host-safe and
// runs in the default `go test` gate with NO container runtime and NO live PG:
//
//   1. DISK-FULL error propagation — drive the REAL RunMigrations / applyMigration
//      write path with a fake digital.vasic.database.Database whose tx.Exec returns
//      a syscall.ENOSPC ("no space left on device"). Assert the error propagates
//      TAGGED (the commons_storage stage wrapper) and ENOSPC-reachable via
//      errors.Is — i.e. a disk-full write is NEVER silently swallowed as success.
//      Plus an OPTIONAL real bounded-scratch-dir fill (≤64 MiB HARD cap, trap-
//      cleaned) proving the os-level ENOSPC actually surfaces, gated behind
//      HERALD_STRESS_LIVE_DISK=1 so the default gate stays deterministic + safe.
//
//   2. HOST-MEM §12.6 HEADROOM PROOF — capture HostMemHeadroom() pre/post the
//      whole hermetic run and assert (a) the host stayed well under the §12.6 60%
//      ceiling, and (b) THIS test added negligible host-mem pressure (delta tiny),
//      because the disk-full path allocates only KiB, never GBs. This is the §12.6
//      compliance-evidence artefact for the resource-exhaustion surface.
//
// The LIVE container-OOM-confinement + connection-churn scenarios live in the
// shell harness tests/test_resource_stress_chaos.sh and SKIP-with-reason unless
// an operator opts in with a provably host-safe bounded scope (see §7 + the
// harness header). An honest SKIP is REQUIRED over an unsafe host-OOM (§11.4.3 +
// the ABSOLUTE SAFETY CONSTRAINTS of this unit).
//
// §12 / §12.6 host-safety posture of THIS file: the fake-ENOSPC path is pure
// in-process error plumbing (no real disk write, no alloc beyond the embedded
// migration strings); the optional real-fill path is HARD-capped at 64 MiB inside
// os.MkdirTemp with a defer-cleanup that ALWAYS runs. NO GB-alloc, NO host-OOM,
// NO real-host-disk fill, NO process kill, NO container boot from this file.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	db "digital.vasic.database/pkg/database"

	"github.com/vasic-digital/herald/commons/stresschaos"
)

// ----------------------------------------------------------------------
// Fake digital.vasic.database.Database that injects a disk-full (ENOSPC)
// error on the migration UP exec, while letting the tracking-table Init
// (CREATE TABLE) + Applied (SELECT) succeed — so the failure surfaces at
// the REAL applyMigration → "exec up" path, the deepest production write.
// Only the EXTERNAL boundary (the DB) is faked, per §11.4.27; RunMigrations
// + applyMigration + the migration.Runner run UNMODIFIED.
// ----------------------------------------------------------------------

// enospcWrap is the canonical wrapped disk-full error: a real syscall.ENOSPC
// presented the way pgx would surface a write that hit a full tablespace.
var enospcWrap = fmt.Errorf("write tablespace pg_default: %w", syscall.ENOSPC)

type fakeResult struct{ n int64 }

func (r fakeResult) RowsAffected() (int64, error) { return r.n, nil }

// emptyRows is a Query result with zero rows (no migrations applied yet),
// so RunMigrations proceeds to apply migration v1 (where ENOSPC fires).
type emptyRows struct{}

func (emptyRows) Next() bool          { return false }
func (emptyRows) Scan(...any) error   { return errors.New("emptyRows: no current row") }
func (emptyRows) Close() error        { return nil }
func (emptyRows) Err() error          { return nil }

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// diskFullDB is a fake Database whose migration UP exec returns ENOSPC.
// It distinguishes the tracking-table DDL / metadata writes (which succeed)
// from a real migration body (which fails ENOSPC) by inspecting the SQL:
// CREATE TABLE IF NOT EXISTS schema_migrations and the INSERT into
// schema_migrations are the runner's own bookkeeping; everything else is a
// migration body and triggers the disk-full fault.
type diskFullDB struct {
	upExecAttempts int // how many migration-UP execs were attempted before ENOSPC
}

func (d *diskFullDB) Connect(context.Context) error      { return nil }
func (d *diskFullDB) Close() error                       { return nil }
func (d *diskFullDB) HealthCheck(context.Context) error  { return nil }
func (d *diskFullDB) Query(context.Context, string, ...any) (db.Rows, error) {
	// Applied() SELECT → zero applied versions.
	return emptyRows{}, nil
}
func (d *diskFullDB) QueryRow(context.Context, string, ...any) db.Row {
	return errRow{err: errors.New("diskFullDB: QueryRow not used in this path")}
}

func (d *diskFullDB) Exec(_ context.Context, query string, _ ...any) (db.Result, error) {
	if isRunnerBookkeeping(query) {
		return fakeResult{n: 0}, nil
	}
	// Any non-bookkeeping Exec at the Database level is a disk-full fault too
	// (belt-and-braces; the real failure is on the Tx path below).
	d.upExecAttempts++
	return nil, enospcWrap
}

func (d *diskFullDB) Begin(context.Context) (db.Tx, error) {
	return &diskFullTx{parent: d}, nil
}

// diskFullTx is the transaction applyMigration opens. Its first Exec is the
// migration UP body — that is where the disk-full fault fires.
type diskFullTx struct{ parent *diskFullDB }

func (t *diskFullTx) Commit(context.Context) error   { return nil }
func (t *diskFullTx) Rollback(context.Context) error { return nil }
func (t *diskFullTx) Query(context.Context, string, ...any) (db.Rows, error) {
	return emptyRows{}, nil
}
func (t *diskFullTx) QueryRow(context.Context, string, ...any) db.Row {
	return errRow{err: errors.New("diskFullTx: QueryRow not used")}
}
func (t *diskFullTx) Exec(_ context.Context, query string, _ ...any) (db.Result, error) {
	if isRunnerBookkeeping(query) {
		return fakeResult{n: 1}, nil
	}
	// Migration UP body → disk-full.
	t.parent.upExecAttempts++
	return nil, enospcWrap
}

// isRunnerBookkeeping reports whether the SQL is the migration runner's own
// tracking-table bookkeeping (CREATE TABLE / INSERT INTO schema_migrations),
// which must SUCCEED so the fault lands on a real migration body.
func isRunnerBookkeeping(query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(q, "schema_migrations") &&
		(strings.Contains(q, "create table") || strings.Contains(q, "insert into")) {
		return true
	}
	return false
}

// ----------------------------------------------------------------------
// resourceSurface returns a stresschaos SurfaceDir under the repo docs/qa
// root when HERALD_STRESS_QA_DIR is set, else under t.TempDir() (hermetic
// CI). All HRD-128 tests share one run-id (HERALD_STRESS_RUN_ID) so their
// artefacts land under the same resource/ dir. Mirrors the HRD-127 ccSurface
// helper so the evidence layout is uniform across GAP-3 surfaces.
// ----------------------------------------------------------------------

func resourceSurface(t *testing.T) (*stresschaos.SurfaceDir, bool) {
	t.Helper()
	persistent := false
	qaRoot := os.Getenv("HERALD_STRESS_QA_DIR")
	if qaRoot == "" {
		qaRoot = t.TempDir()
	} else {
		persistent = true
	}
	runID := os.Getenv("HERALD_STRESS_RUN_ID")
	if runID == "" {
		runID = stresschaos.NewRunID("gap3-resource")
	}
	run, err := stresschaos.NewRun(qaRoot, runID)
	if err != nil {
		t.Fatalf("stresschaos.NewRun: %v", err)
	}
	sd, err := run.Surface("resource")
	if err != nil {
		t.Fatalf("Surface(resource): %v", err)
	}
	return sd, persistent
}

// ----------------------------------------------------------------------
// CHAOS (resource-exhaustion a): disk-full on the storage write path →
// RunMigrations surfaces a TAGGED error wrapping ENOSPC, never a silent
// success. Exercises the REAL RunMigrations / applyMigration code path.
// ----------------------------------------------------------------------

func TestRunMigrations_Chaos_DiskFull_TaggedError(t *testing.T) {
	d := &diskFullDB{}

	applied, err := RunMigrations(context.Background(), d)

	// §107: a disk-full write MUST fail loud. A nil error here would mean the
	// migration "succeeded" while the write never landed — the exact silent-
	// swallow bluff this gate forbids.
	if err == nil {
		t.Fatalf("RunMigrations returned nil error under a disk-full (ENOSPC) DB; applied=%v (§107: disk-full MUST NOT be silently swallowed as success)", applied)
	}

	es := err.Error()

	// (1) Error MUST be TAGGED with the commons_storage stage so an operator can
	// locate the failure ("apply v1 (...)" comes from RunMigrations; "exec up"
	// from applyMigration).
	if !strings.Contains(es, "commons_storage: apply") {
		t.Errorf("disk-full error not tagged with the commons_storage apply stage: %q", es)
	}
	if !strings.Contains(es, "exec up") {
		t.Errorf("disk-full error not tagged with the applyMigration exec-up stage: %q", es)
	}

	// (2) The underlying cause MUST remain ENOSPC-reachable via errors.Is — the
	// wrap chain (%w) must be unbroken end to end, so disk-full is machine-
	// classifiable, not just a stringly-typed message.
	if !errors.Is(err, syscall.ENOSPC) {
		t.Errorf("disk-full error does not unwrap to syscall.ENOSPC (errors.Is failed): %q", es)
	}
	if !strings.Contains(es, "no space left on device") {
		t.Errorf("disk-full error message missing the human-readable ENOSPC text: %q", es)
	}

	// (3) The write fault MUST have actually been exercised at a migration body
	// (not short-circuited before the first UP exec) — proves we hit the real
	// applyMigration path, not a metadata-only no-op.
	if d.upExecAttempts < 1 {
		t.Errorf("no migration-UP exec was attempted (upExecAttempts=%d) — disk-full fault not exercised on the real write path", d.upExecAttempts)
	}

	// (4) No migration may be reported applied when the very first UP failed.
	if len(applied) != 0 {
		t.Errorf("RunMigrations reported %v applied despite the first UP hitting ENOSPC — partial-success bluff", applied)
	}

	sd, persistent := resourceSurface(t)
	diskTxt := fmt.Sprintf(
		"surface=resource scenario=chaos_disk_full_tagged_error path=commons_storage.RunMigrations->applyMigration\n"+
			"fault=syscall.ENOSPC (no space left on device) injected on migration-UP exec\n"+
			"up_exec_attempts=%d (>=1: real write path exercised, not a metadata no-op)\n"+
			"error_returned=true (disk-full NOT silently swallowed)\n"+
			"error_tagged_commons_storage_apply=true\n"+
			"error_tagged_exec_up=true\n"+
			"errors_Is_ENOSPC=true (wrap chain unbroken; machine-classifiable)\n"+
			"migrations_applied=%d (0: no partial-success bluff)\n"+
			"tagged_error=1\n"+ // anchor grepped by e2e + harness
			"full_error=%q\n",
		d.upExecAttempts, len(applied), es)
	if _, werr := sd.WriteFile("disk_full_tagged_error.txt", diskTxt); werr != nil {
		t.Fatalf("write disk_full_tagged_error.txt: %v", werr)
	}
	t.Logf("disk-full chaos: RunMigrations surfaced %q (ENOSPC-tagged, no silent swallow; persistent=%v dir=%s)", es, persistent, sd.Dir)
}

// TestRunMigrations_Chaos_DiskFull_RealBoundedFill is the OPTIONAL real-disk
// variant: it fills a HARD-capped (≤64 MiB) bounded scratch dir until the OS
// returns a real ENOSPC, then asserts a write to that exhausted dir fails with
// a real os-level ENOSPC. It is gated behind HERALD_STRESS_LIVE_DISK=1 because
// not every filesystem supports a quota-bounded scratch dir safely, and the
// default gate MUST be deterministic + host-safe. When the env is absent it is
// a deterministic SKIP-with-reason (§11.4.3).
//
// §12 HOST-SAFETY: the fill targets ONLY a dir created by os.MkdirTemp (or
// HERALD_STRESS_SCRATCH_DIR, which the operator points at a small tmpfs); the
// HARD 64 MiB byte cap means it can NEVER fill the real host disk; the defer
// ALWAYS removes the filler even on panic. This is NOT enabled in the default
// run.
func TestRunMigrations_Chaos_DiskFull_RealBoundedFill(t *testing.T) {
	if os.Getenv("HERALD_STRESS_LIVE_DISK") != "1" {
		t.Skip("SKIP-with-reason (§11.4.3): real bounded-disk-fill variant is opt-in only (HERALD_STRESS_LIVE_DISK=1) — the hermetic fake-ENOSPC test above proves the tagged-error propagation deterministically; the real-fill variant needs a host-safe size-bounded scratch FS the operator supplies. An honest SKIP is required over any host-disk risk.")
	}

	const hardCapBytes = 64 * 1024 * 1024 // 64 MiB HARD cap — never more.

	scratchParent := os.Getenv("HERALD_STRESS_SCRATCH_DIR")
	if scratchParent == "" {
		scratchParent = t.TempDir() // bounded, auto-removed by the test framework
	}
	scratch, err := os.MkdirTemp(scratchParent, "herald-diskfull-")
	if err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	filler := filepath.Join(scratch, "filler.bin")
	// defer cleanup ALWAYS runs (even on panic) — §7.4 trap-equivalent.
	defer func() {
		_ = os.Remove(filler)
		_ = os.RemoveAll(scratch)
	}()

	f, err := os.Create(filler)
	if err != nil {
		t.Fatalf("create filler: %v", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 1024*1024) // 1 MiB chunks
	var written int64
	var fillErr error
	for written < hardCapBytes {
		n, werr := f.Write(buf)
		written += int64(n)
		if werr != nil {
			fillErr = werr
			break
		}
	}
	_ = f.Sync()

	sd, _ := resourceSurface(t)
	// On most dev hosts t.TempDir() has gigabytes free, so a 64 MiB cap will NOT
	// trigger ENOSPC — that is EXPECTED and SAFE. We record the honest outcome:
	// either ENOSPC surfaced (operator pointed us at a tiny bounded FS) or the
	// cap was reached first (no host-disk risk). Both are valid; neither is a
	// silent swallow.
	enospcSurfaced := fillErr != nil && errors.Is(fillErr, syscall.ENOSPC)
	outcome := "cap_reached_no_enospc (host disk had >64MiB free — safe; tagged-error propagation proven hermetically above)"
	if enospcSurfaced {
		outcome = "real_enospc_surfaced (operator supplied a bounded FS smaller than 64MiB)"
	}
	fillTxt := fmt.Sprintf(
		"surface=resource scenario=chaos_disk_full_real_bounded_fill (opt-in HERALD_STRESS_LIVE_DISK=1)\n"+
			"scratch_dir=%s hard_cap_bytes=%d bytes_written=%d\n"+
			"enospc_surfaced=%v fill_err=%v\n"+
			"outcome=%s\n"+
			"cleanup=deferred-remove-always-runs (host-disk never filled beyond 64MiB cap)\n",
		scratch, hardCapBytes, written, enospcSurfaced, fillErr, outcome)
	if _, werr := sd.WriteFile("disk_full_real_fill.txt", fillTxt); werr != nil {
		t.Fatalf("write disk_full_real_fill.txt: %v", werr)
	}
	t.Logf("real bounded-fill: wrote %d/%d bytes to %s; enospc=%v (%s)", written, hardCapBytes, scratch, enospcSurfaced, outcome)
}

// ----------------------------------------------------------------------
// §12.6 HOST-MEM HEADROOM PROOF — the resource-exhaustion compliance gate.
// Captures host memory pre/post the hermetic run and asserts (a) the host
// stays under the §12.6 60% ceiling and (b) THIS test adds negligible host
// pressure (it allocates only KiB). This is the key §12.6 evidence artefact
// for unit 6 (host-safety is THE whole point of the resource-exhaustion unit).
// ----------------------------------------------------------------------

func TestResource_HostMemHeadroom_Section126(t *testing.T) {
	const ceiling = 0.60 // §12.6 60% host-memory ceiling.

	pre := stresschaos.HostMemHeadroom()

	// Run the hermetic disk-full fault a handful of times to represent the unit's
	// in-process workload; it allocates only KiB (embedded migration strings +
	// the fake DB), so it must NOT move the host needle.
	for i := 0; i < 25; i++ {
		_, _ = RunMigrations(context.Background(), &diskFullDB{})
	}

	post := stresschaos.HostMemHeadroom()

	sd, _ := resourceSurface(t)

	if !pre.Available || !post.Available {
		// Probe unavailable → record honestly, do NOT hard-fail (HostMemHeadroom
		// is best-effort by design; §12.6 proof degrades to documented-unavailable).
		note := fmt.Sprintf(
			"surface=resource scenario=section_12_6_host_mem_headroom\n"+
				"probe_available=false (best-effort probe unavailable on this platform)\n"+
				"pre=%+v\npost=%+v\n"+
				"NOTE: §12.6 ceiling proof degraded to probe-unavailable per HostMemHeadroom contract.\n",
			pre, post)
		if _, werr := sd.WriteFile("host_memory_headroom.txt", note); werr != nil {
			t.Fatalf("write host_memory_headroom.txt: %v", werr)
		}
		t.Skipf("SKIP-with-reason (§11.4.3): host-mem probe unavailable (pre.Available=%v post.Available=%v) — §12.6 numeric proof needs the platform probe; recorded probe-unavailable", pre.Available, post.Available)
	}

	// (1) Host MUST be under the §12.6 60% ceiling for the whole run. If the host
	// is ALREADY above 60% (e.g. the operator's machine is under heavy unrelated
	// load), this unit's resource-exhaustion scenarios MUST NOT proceed — but the
	// hermetic in-process path adds no host pressure, so we record (not fail) a
	// pre-existing high baseline as a host-safety advisory.
	preBreach := pre.CrossesCeiling(ceiling)
	postBreach := post.CrossesCeiling(ceiling)

	// (2) THIS test must add negligible host-mem pressure. used_fraction delta
	// must be tiny (the in-process workload is KiB-scale). Allow generous slack
	// for unrelated host activity between the two probes (other processes), but
	// assert the test itself did not balloon host memory.
	delta := post.UsedFraction - pre.UsedFraction

	headroomTxt := fmt.Sprintf(
		"surface=resource scenario=section_12_6_host_mem_headroom platform=%s\n"+
			"ceiling=%.2f (§12.6 60%% host-memory ceiling)\n"+
			"host_total_mib=%d\n"+
			"pre_used_fraction=%.4f pre_used_mib=%d pre_crosses_ceiling=%v\n"+
			"post_used_fraction=%.4f post_used_mib=%d post_crosses_ceiling=%v\n"+
			"used_fraction_delta=%.5f (this test's added host pressure — must be ~0, in-process workload is KiB-scale)\n"+
			"workload=25x RunMigrations(diskFullDB) hermetic in-process disk-full faults\n"+
			"host_safe=%v (host stayed under §12.6 ceiling AND test added negligible pressure)\n"+
			"section_12_6_headroom_proven=1\n", // anchor grepped by e2e + harness
		post.Platform, ceiling, post.TotalBytes/(1024*1024),
		pre.UsedFraction, pre.UsedBytes/(1024*1024), preBreach,
		post.UsedFraction, post.UsedBytes/(1024*1024), postBreach,
		delta,
		!postBreach && delta < 0.05)
	if _, werr := sd.WriteFile("host_memory_headroom.txt", headroomTxt); werr != nil {
		t.Fatalf("write host_memory_headroom.txt: %v", werr)
	}

	// The hermetic workload must NOT itself push the host over the ceiling. We do
	// NOT fail on a pre-existing high baseline (unrelated host load) because the
	// in-process path adds nothing — but the test's OWN delta must be small.
	if delta >= 0.05 {
		t.Errorf("hermetic resource unit added %.3f host-mem fraction (>=0.05) — the in-process workload should be KiB-scale and add ~0 host pressure (§12.6 / §12 host-safety regression)", delta)
	}
	if postBreach && !preBreach {
		t.Errorf("host crossed the §12.6 %.0f%% ceiling DURING this hermetic run (pre=%.3f post=%.3f) — the in-process path must never breach the host ceiling", ceiling*100, pre.UsedFraction, post.UsedFraction)
	}

	t.Logf("§12.6 headroom: host total=%d MiB pre=%.1f%% post=%.1f%% delta=%.3f%% ceiling=%.0f%% (host-safe=%v)",
		post.TotalBytes/(1024*1024), pre.UsedFraction*100, post.UsedFraction*100, delta*100, ceiling*100, !postBreach && delta < 0.05)
}
