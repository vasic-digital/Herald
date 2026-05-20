// Package infra provides Herald's on-demand container-infrastructure layer.
//
// Per Universal Constitution §11.4.76 (containers-submodule mandate) and
// Herald V3 spec §44, Foundation tests + the pherald doctor subcommand
// MUST boot Postgres / Redis / OTel via the canonical
// `digital.vasic.containers` Submodule — never via ad-hoc `docker compose`
// shellouts. This package is the thin Herald-side facade that wraps the
// Submodule's `pkg/compose` orchestrator with Herald-flavor defaults:
//
//   - Project name "herald-quickstart" (matches the §26.5 compose's
//     declared project name).
//   - Compose-file path defaults to <repo-root>/quickstart/
//     docker-compose.quickstart.yml.
//   - Wait-for-healthy enabled by default so tests don't race the
//     healthchecks declared in the compose file.
//
// Usage from a test:
//
//	func TestMain(m *testing.M) {
//	    ctx := context.Background()
//	    boot, err := infra.NewQuickstartBoot()
//	    if err != nil { log.Fatal(err) }
//	    if err := boot.Up(ctx); err != nil { log.Fatal(err) }
//	    code := m.Run()
//	    _ = boot.Down(context.Background())
//	    os.Exit(code)
//	}
//
// The boot is anti-bluff per §11.4.76: a test that imports this package
// and calls Up but skips verification that the services are actually
// reachable is still a bluff. Callers MUST follow Up with a healthcheck
// (e.g. `boot.WaitHealthy(ctx, "postgres")`) before trusting the infra.
package infra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"digital.vasic.cache/pkg/redis"
	"digital.vasic.containers/pkg/compose"
	"digital.vasic.containers/pkg/logging"
	"digital.vasic.database/pkg/database"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// DefaultProjectName is the compose --project-name Herald uses by convention.
// Matches the project_name declared in quickstart/docker-compose.quickstart.yml.
const DefaultProjectName = "herald-quickstart"

// QuickstartBoot wraps a compose.Orchestrator with Herald-specific defaults.
//
// HRD-010 wiring fields (added Task 1, populated by Tasks 2/4/5):
//   - pool: pgx-backed database.Database; nil until Up() opens it.
//   - queue: task queue; nil until Up() wires it.
//   - redis: cache client; nil until Up() opens it.
//
// All three are exposed via Pool/Queue/Redis getters in clients.go.
// Getters return ErrNotBooted when the field is nil — see clients.go
// for the anti-bluff contract.
type QuickstartBoot struct {
	orch    compose.ComposeOrchestrator
	project compose.ComposeProject

	// HRD-010 client fields. Populated by Up() in later tasks; remain
	// nil until then. Pool()/Queue()/Redis() return ErrNotBooted on nil.
	pool  database.Database
	queue TaskQueue
	redis *redis.Client
}

// Config configures NewQuickstartBoot. Zero values pick Herald defaults.
type Config struct {
	// ComposeFile is the path to docker-compose.quickstart.yml. If empty,
	// resolves to <repo-root>/quickstart/docker-compose.quickstart.yml by
	// walking up from the test's working dir until a `quickstart/` dir
	// containing the file is found.
	ComposeFile string

	// ProjectName overrides DefaultProjectName.
	ProjectName string

	// Services limits the boot to a subset of services. Empty = all.
	Services []string

	// Logger is passed to the underlying compose orchestrator. nil = NopLogger.
	Logger logging.Logger
}

// NewQuickstartBoot constructs a QuickstartBoot using the default
// auto-detected compose runtime (docker / podman / podman-compose).
//
// Returns an error if no compose runtime is available on the host (in
// which case the operator MUST install podman or docker; the §11.4.76
// on-demand-infra invariant assumes one is present — the boot is the
// FIRST step of the test entry point, not "operator already started
// podman manually").
func NewQuickstartBoot(cfg Config) (*QuickstartBoot, error) {
	if cfg.ComposeFile == "" {
		path, err := findQuickstartCompose()
		if err != nil {
			return nil, fmt.Errorf("infra: locate compose file: %w", err)
		}
		cfg.ComposeFile = path
	}
	if cfg.ProjectName == "" {
		cfg.ProjectName = DefaultProjectName
	}

	// The orchestrator workDir is the directory of the compose file so
	// any relative paths in it (build contexts, env-file refs) resolve.
	workDir := filepath.Dir(cfg.ComposeFile)
	orch, err := compose.NewDefaultOrchestrator(workDir, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("infra: orchestrator: %w", err)
	}

	return &QuickstartBoot{
		orch: orch,
		project: compose.ComposeProject{
			Name:     cfg.ProjectName,
			File:     cfg.ComposeFile,
			Services: cfg.Services,
		},
	}, nil
}

// Up brings the compose project up in detached mode.
//
// NOTE: `compose.WithWait(true)` is intentionally NOT passed because
// `podman-compose` (the canonical Herald-dev runtime) does not recognize
// the `--wait` flag — see HRD-081 (extend `digital.vasic.containers/pkg/
// compose` with a Detect runtime → emit-flag-or-fallback split). For now
// callers MUST follow Up with their own healthcheck poll (Status() loop
// or service-specific TCP probe) before trusting the infra.
//
// Idempotent: running Up against an already-running project is a no-op-ish
// nudge that compose handles.
func (b *QuickstartBoot) Up(ctx context.Context) error {
	if err := b.orch.Up(
		ctx, b.project,
		compose.WithUpDetach(true),
		compose.WithUpTimeout(120),
	); err != nil {
		return fmt.Errorf("infra: compose up: %w", err)
	}

	// HRD-010 Task 4 lifecycle fix: idempotency guard. If Up() is called
	// twice in the same boot lifecycle and we already opened clients,
	// short-circuit — unconditionally re-opening leaks pgx connections
	// (no Close on the previous pool) and goroutines, AND can race the
	// Redis healthcheck against an already-good client.
	if b.pool != nil || b.redis != nil {
		return nil
	}

	// Selective client opens: when the caller limited Services to a
	// subset, only open clients for the services actually requested.
	// Empty list = all services per Config.Services contract.
	openPG := b.serviceRequested("postgres")
	openRedis := b.serviceRequested("redis")

	// HRD-010 Task 2 (Task 4 cleanup): open the pgx pool against the
	// booted Postgres container and apply migrations so the schema is
	// live before tests use the pool.
	//
	// Host port default (24100) matches the host-side mapping declared
	// in quickstart/docker-compose.quickstart.yml ("24100:5432" — spec
	// §9.4 reserved range). User/password/db default to the Herald
	// development credentials (the compose container is bootstrapped
	// with POSTGRES_USER=herald, POSTGRES_DB=herald, and the password
	// is supplied via the ${HERALD_DB_PASSWORD} env var the compose
	// file requires).
	//
	// Canonical env vars (aligned with quickstart/docker-compose.quickstart.yml
	// — Task 4 standardized on these names to remove the HERALD_PG_PASSWORD
	// chained drift Task 2 carried):
	//   HERALD_DB_HOST, HERALD_DB_PORT, HERALD_DB_USER,
	//   HERALD_DB_PASSWORD, HERALD_DB_NAME — Postgres.
	//   HERALD_REDIS_ADDR, HERALD_REDIS_PASSWORD, HERALD_REDIS_DB — Redis.
	var pool database.Database
	if openPG {
		cfg := storage.ConfigForHerald(
			envOr("HERALD_DB_HOST", "127.0.0.1"),
			envOrInt("HERALD_DB_PORT", 24100),
			envOr("HERALD_DB_USER", "herald"),
			envOr("HERALD_DB_PASSWORD", "herald_dev"),
			envOr("HERALD_DB_NAME", "herald"),
		)
		// podman-compose does not honor compose.WithWait — Postgres can be
		// "container Up" but still mid-`pg_ctl start` (SQLSTATE 57P03,
		// "the database system is starting up"). Retry the connect for up
		// to 30s before declaring the boot failed. This is post-compose-up
		// readiness polling, not arbitrary infinite-retry — bounded by the
		// loop budget and the caller's ctx.
		var err error
		const maxBootWait = 30 * time.Second
		const pollInterval = 500 * time.Millisecond
		deadline := time.Now().Add(maxBootWait)
		for {
			pool, err = storage.Open(ctx, cfg)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("commons_infra.Up: open pgx pool: %w", err)
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("commons_infra.Up: open pgx pool: %w", ctx.Err())
			case <-time.After(pollInterval):
			}
		}
		b.pool = pool

		// Apply migrations so the schema is live before tests use the pool.
		applied, err := storage.RunMigrations(ctx, pool)
		if err != nil {
			_ = pool.Close()
			b.pool = nil
			return fmt.Errorf("commons_infra.Up: run migrations: %w", err)
		}
		_ = applied // logged when a logger is wired
	}

	// HRD-010 Task 4: open the Redis client against the booted Redis
	// container. Host port default (24200) matches the host-side
	// mapping declared in quickstart/docker-compose.quickstart.yml
	// ("24200:6379" — spec §9.4 reserved range, see also redis service
	// command line which requires --requirepass). Password is supplied
	// via the required HERALD_REDIS_PASSWORD env var the compose file
	// declares.
	if openRedis {
		redisCfg := &redis.Config{
			Addr:     envOr("HERALD_REDIS_ADDR", "127.0.0.1:24200"),
			Password: envOr("HERALD_REDIS_PASSWORD", ""),
			DB:       envOrInt("HERALD_REDIS_DB", 0),
		}
		rc := redis.New(redisCfg)
		if err := rc.HealthCheck(ctx); err != nil {
			// Rollback the pool we just opened so b.pool isn't left
			// dangling on a partial-boot. Anti-bluff §107: leaking the
			// pool here + returning an error would let a retry's
			// idempotency guard short-circuit on a half-booted lifecycle.
			_ = rc.Close()
			if pool != nil {
				_ = pool.Close()
				b.pool = nil
			}
			return fmt.Errorf("commons_infra.Up: redis healthcheck: %w", err)
		}
		b.redis = rc
	}

	return nil
}

// serviceRequested reports whether the named service is included in the
// project's boot set. The empty Services list is the "all services" sentinel
// per Config.Services — every name returns true. Otherwise the name must
// appear verbatim in the configured list.
func (b *QuickstartBoot) serviceRequested(name string) bool {
	if len(b.project.Services) == 0 {
		return true
	}
	for _, s := range b.project.Services {
		if s == name {
			return true
		}
	}
	return false
}

// Down closes any wired clients (pool, redis, queue) FIRST, then stops +
// removes the compose project's containers + networks. Idempotent:
// calling Down on an already-down project is a no-op.
//
// HRD-010 Task 4 lifecycle fix: previously Down() tore down compose but
// left b.pool / b.redis non-nil and holding dead TCP sockets. Anti-bluff
// §107 — a subsequent boot.Pool() in the same Go process would have
// returned the stale (now-disconnected) handle and PASSed.
//
// Order: close clients FIRST (gracefully — they may still talk to the
// containers), THEN compose-down (so pgx isn't trying to flush against
// an already-killed Postgres at shutdown).
func (b *QuickstartBoot) Down(ctx context.Context) error {
	if b.pool != nil {
		_ = b.pool.Close()
		b.pool = nil
	}
	if b.redis != nil {
		_ = b.redis.Close()
		b.redis = nil
	}
	if b.queue != nil {
		// Queue is just an interface today (queue.go). Task 5 will add
		// real construction with a documented Close() contract; for now
		// just clear the field so a follow-up Up() doesn't see a stale
		// interface value.
		b.queue = nil
	}

	if err := b.orch.Down(ctx, b.project); err != nil {
		return fmt.Errorf("infra: compose down: %w", err)
	}
	return nil
}

// Status returns per-service status as reported by `compose ps`. Used by
// `pherald doctor` and by integration tests that need to assert specific
// services reached "running"/"healthy" before proceeding.
func (b *QuickstartBoot) Status(ctx context.Context) ([]compose.ServiceStatus, error) {
	out, err := b.orch.Status(ctx, b.project)
	if err != nil {
		return nil, fmt.Errorf("infra: compose status: %w", err)
	}
	return out, nil
}

// findQuickstartCompose walks up from os.Getwd to locate
// <repo-root>/quickstart/docker-compose.quickstart.yml.
func findQuickstartCompose() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	cur := cwd
	for i := 0; i < 16; i++ {
		candidate := filepath.Join(cur, "quickstart", "docker-compose.quickstart.yml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", errors.New("infra: docker-compose.quickstart.yml not found within 16 parents of " + cwd)
}

// envOr returns the value of env var `key`, falling back to `fallback`
// when the var is unset or empty. Used by Up() to let operators override
// the pgx connection defaults without code changes.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envOrInt is the integer counterpart of envOr. Falls back when the env
// var is unset, empty, or not parseable as a decimal integer.
func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return fallback
}
