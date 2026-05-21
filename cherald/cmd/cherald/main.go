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

	flavhttp "github.com/vasic-digital/herald/cherald/internal/http"
	"github.com/vasic-digital/herald/cherald/internal/stubs"
	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	constitution "github.com/vasic-digital/herald/commons_constitution"
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

	// Build the ConstitutionStore. Postgres if HERALD_PG_DSN is present
	// (production path), Memory otherwise (dev/CI quickstart path).
	// Both back-ends implement constitution.ConstitutionStore identically;
	// the cherald/internal/compliance.Handler doesn't care which is wired.
	store, err := buildStore(context.Background())
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
		Routes:     flavhttp.Routes(store),
		Middleware: []gin.HandlerFunc{commons_auth.GinMiddleware(verifier)},
	}))
	stubs.Register(root)

	if rerr := root.Execute(); rerr != nil {
		fmt.Fprintln(os.Stderr, "cherald:", rerr)
		os.Exit(1)
	}
}

// buildStore returns the ConstitutionStore selected by env. Postgres wins
// when HERALD_PG_DSN is present (the value is parsed via
// storage.ParseDSN — same path used by `pherald migrate`); otherwise the
// in-memory backend is used so dev/CI runs without a database.
//
// commons_storage.Open already returns a digital.vasic.database.Database,
// which is what state.NewPostgres expects — so no adapter shim is needed.
func buildStore(ctx context.Context) (constitution.ConstitutionStore, error) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		return state.NewMemory(), nil
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse HERALD_PG_DSN: %w", err)
	}
	database, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pg pool: %w", err)
	}
	return state.NewPostgres(database), nil
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
