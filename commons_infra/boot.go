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

	"digital.vasic.cache/pkg/redis"
	"digital.vasic.containers/pkg/compose"
	"digital.vasic.containers/pkg/logging"
	"digital.vasic.database/pkg/database"
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
	return nil
}

// Down stops + removes the compose project's containers + networks.
// Idempotent: calling Down on an already-down project is a no-op.
func (b *QuickstartBoot) Down(ctx context.Context) error {
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
