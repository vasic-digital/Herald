// sherald — System Herald per spec V3 §18 + §43 (Wave 3a live).
//
// Wave 3a wires:
//   - commons_auth.GinMiddleware for JWT (HMAC or JWKS per env)
//   - safety.Aggregator + safety.StartMemSampler background goroutine
//     (lifecycle bound to a SIGTERM-cancelled context)
//   - sherald/internal/safety.Handler serving GET /v1/safety_state
//
// §43 stub commands still register via sherald/internal/stubs.
//
// §107 anti-bluff posture: the serve plane refuses to start without a
// usable JWT verifier (HERALD_AUTH_MODE + the matching secret/URL must be
// set). The mem-sampler context is bound to os.Interrupt + syscall.SIGTERM
// so the background goroutine cleanly exits on shutdown rather than leaking
// — silently leaking a goroutine on shutdown would be a §107 PASS-bluff.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	flavhttp "github.com/vasic-digital/herald/sherald/internal/http"
	"github.com/vasic-digital/herald/sherald/internal/safety"
	"github.com/vasic-digital/herald/sherald/internal/stubs"
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

	branding := commons.DefaultBranding("s", version)

	// Process-global Aggregator + sampler goroutine. Lifecycle bound to a
	// top-level context so SIGTERM cleanly terminates the sampler. The
	// cli.ServeCmd cobra command has its own server-side signal trap; the
	// sampler runs outside that lifecycle so it needs its own ctx.
	agg := safety.NewAggregator()
	samplerCtx, stopSampler := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSampler()
	safety.StartMemSampler(samplerCtx, agg)

	// Build the auth verifier. Redis is optional — JWKS-mode caches keys
	// in Redis when available; HMAC mode ignores rdb entirely.
	var rdb redis.Cmdable
	if url := os.Getenv("HERALD_REDIS_URL"); url != "" {
		opts, err := redis.ParseURL(url)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sherald: parse HERALD_REDIS_URL:", err)
			os.Exit(1)
		}
		rdb = redis.NewClient(opts)
	}
	verifier, err := commons_auth.NewVerifierFromEnv(rdb)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sherald: build verifier:", err)
		os.Exit(1)
	}

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(cli.ServeCmd(cli.ServeOpts{
		Branding:   branding,
		Routes:     flavhttp.Routes(agg),
		Middleware: []gin.HandlerFunc{commons_auth.GinMiddleware(verifier)},
	}))
	stubs.Register(root)

	if rerr := root.Execute(); rerr != nil {
		fmt.Fprintln(os.Stderr, "sherald:", rerr)
		os.Exit(1)
	}
}
