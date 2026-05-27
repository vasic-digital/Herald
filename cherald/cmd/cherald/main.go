// cherald — Constitution Herald per spec V3 §18 + §43 (Wave 3a live).
//
// Wave 3a wires:
//   - commons_auth.GinMiddleware for JWT (HMAC or JWKS per env)
//   - commons_constitution.ConstitutionStore (Postgres via state.NewPostgres
//     when HERALD_PG_DSN is set, Memory fallback otherwise)
//   - cherald/internal/compliance.Handler serving GET /v1/compliance
//
// §43 stub commands still register via cherald/internal/stubs.
//
// §107 anti-bluff posture: the serve plane refuses to start without a
// usable JWT verifier (HERALD_AUTH_MODE + the matching secret/URL must be
// set). Silently running an unauthenticated /v1/compliance would be a
// PASS-bluff — better to fail loudly at startup.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/vasic-digital/herald/cherald/internal/bindings"
	flavhttp "github.com/vasic-digital/herald/cherald/internal/http"
	"github.com/vasic-digital/herald/cherald/internal/stubs"
	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
	"github.com/vasic-digital/herald/commons_constitution/ladder"
	"github.com/vasic-digital/herald/commons_constitution/state"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags)"
var version = "0.0.0-dev"

// commit is overridden at build time:
//
//	go build -ldflags "-X main.commit=$(git rev-parse --short HEAD)"
var commit = "unknown"

func main() {
	cli.BuildVersion = version
	cli.BuildCommit = commit

	branding := commons.DefaultBranding("c", version)

	// Build the ConstitutionStore + ModeLadder. Postgres if HERALD_PG_DSN is
	// present (production path), Memory otherwise (dev/CI quickstart path).
	// Both back-ends implement their interfaces identically; the compliance +
	// modes handlers don't care which is wired. A single pool backs both.
	store, modeLadder, err := buildStoreAndLadder(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "cherald:", err)
		os.Exit(1)
	}

	// Build the HRD-019 bindings pipeline: it registers cherald's §42.3 rule
	// catalogue and drives the detect→emit→persist→audit flow that backs
	// POST /v1/compliance/evaluate. The emitter publishes onto an in-process
	// EventBus (the M1 substrate; the M2 swap to digital.vasic.eventbus is a
	// drop-in per the Foundation Catalogue-Check). Audit is shared with the
	// store backend selection: PostgresAudit when HERALD_PG_DSN is set,
	// MemoryAudit otherwise. A bundle-hash is captured from the discovered
	// Constitution.md when present (replayability per §42.1.3).
	pipeline, err := buildPipeline(modeLadder, store)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cherald:", err)
		os.Exit(1)
	}

	// Build the auth verifier. Redis is optional — JWKS-mode caches keys
	// in Redis when available; HMAC mode ignores rdb entirely.
	rdb, err := buildRedis()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cherald:", err)
		os.Exit(1)
	}
	verifier, err := commons_auth.NewVerifierFromEnv(rdb)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cherald: build verifier:", err)
		os.Exit(1)
	}

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(cli.ServeCmd(cli.ServeOpts{
		Branding:   branding,
		Routes:     flavhttp.Routes(store, modeLadder, pipeline),
		Middleware: []gin.HandlerFunc{commons_auth.GinMiddleware(verifier)},
	}))
	stubs.Register(root)

	if rerr := root.Execute(); rerr != nil {
		fmt.Fprintln(os.Stderr, "cherald:", rerr)
		os.Exit(1)
	}
}

// buildStoreAndLadder returns the ConstitutionStore + ModeLadder selected by
// env, both backed by ONE shared pool. Postgres wins when HERALD_PG_DSN is
// present (the value is parsed via storage.ParseDSN — same path used by
// `pherald migrate`); otherwise the in-memory backends are used so dev/CI
// runs without a database.
//
// commons_storage.Open returns a digital.vasic.database.Database, which is
// what both state.NewPostgres + ladder.NewPostgres expect — so no adapter
// shim is needed and the single pool serves /v1/compliance (state) and
// /v1/compliance/modes (ladder) alike.
func buildStoreAndLadder(ctx context.Context) (constitution.ConstitutionStore, constitution.ModeLadder, error) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		return state.NewMemory(), ladder.NewMemory(), nil
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse HERALD_PG_DSN: %w", err)
	}
	database, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open pg pool: %w", err)
	}
	return state.NewPostgres(database), ladder.NewPostgres(database), nil
}

// buildPipeline assembles the HRD-019 bindings.Pipeline. The emitter publishes
// onto a fresh in-process MemoryBus (M1 substrate). The AuditStore mirrors the
// store backend: PostgresAudit over the HERALD_PG_DSN pool when set, MemoryAudit
// otherwise. The bundle hash is captured from the discovered Constitution.md
// (HELIX_CONSTITUTION_PATH or the conventional sibling path) when readable; a
// missing bundle degrades to the zero hash (a valid "no bundle" sentinel) so
// dev runs without a constitution checkout still serve the surface.
func buildPipeline(ladderImpl constitution.ModeLadder, store constitution.ConstitutionStore) (*bindings.Pipeline, error) {
	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{})
	emitter, err := constitution.NewEmitter(bus, constitution.EmitterConfig{
		Source: "digital.vasic.herald/cherald",
	})
	if err != nil {
		return nil, fmt.Errorf("build emitter: %w", err)
	}

	audit, err := buildAudit(context.Background())
	if err != nil {
		return nil, fmt.Errorf("build audit: %w", err)
	}

	var bundle constitution.BundleHash
	if path := os.Getenv("HELIX_CONSTITUTION_PATH"); path != "" {
		if h, cerr := constitution.Capture(path); cerr == nil {
			bundle = h
		}
	}

	return bindings.NewPipeline(bindings.Config{
		Ladder:  ladderImpl,
		Store:   store,
		Emitter: emitter,
		Audit:   audit,
		Bundle:  bundle,
	})
}

// buildAudit returns the AuditStore matching the store backend selection:
// PostgresAudit over the HERALD_PG_DSN pool when set, MemoryAudit otherwise.
func buildAudit(ctx context.Context) (constitution.AuditStore, error) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		return state.NewMemoryAudit(), nil
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse HERALD_PG_DSN: %w", err)
	}
	database, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pg pool (audit): %w", err)
	}
	return state.NewPostgresAudit(database), nil
}

// buildRedis returns a Redis client when HERALD_REDIS_URL is set, or
// (nil, nil) when it isn't. JWKS-mode verifier accepts a nil rdb and
// falls back to in-memory key caching; HMAC mode ignores rdb entirely.
func buildRedis() (redis.Cmdable, error) {
	url := os.Getenv("HERALD_REDIS_URL")
	if url == "" {
		return nil, nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse HERALD_REDIS_URL: %w", err)
	}
	return redis.NewClient(opts), nil
}
