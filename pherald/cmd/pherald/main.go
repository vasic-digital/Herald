// pherald — Project Herald CLI per spec V3 §3 + §18.2 (Wave 3b live).
//
// Refactored 2026-05-21 (Wave 2 Task 6) to consume the shared commons/cli/
// scaffold (Wave 2 design §3): NewRootCmd + VersionCmd come from there;
// pherald owns only the flavor-specific subcommands (serve, migrate,
// wizard) + the §43 GitOps stubs registered via registerStubs.
//
// Refactored 2026-05-22 (Wave 3b Task 10) to wire the live §32 Runner
// pipeline into POST /v1/events. The serve subcommand now requires
// HERALD_PG_DSN (no PG-less serve mode for Wave 3b) AND HERALD_AUTH_MODE
// (no anonymous serve plane). Optional: HERALD_REDIS_URL (used by both
// the Runner idempotency stage and the JWKS-mode auth verifier cache),
// HERALD_TGRAM_BOT_TOKEN (registers the Telegram channel; null:// is
// always registered).
//
// §107 anti-bluff posture: the serve plane refuses to start without a
// usable JWT verifier AND a usable PG pool. Silently running an
// unauthenticated /v1/events would be a PASS-bluff (E37/E39); running it
// without PG would mean idempotency + archiving silently drop on the
// floor (E38/E42). Better to fail loudly at startup with an HRD pointer.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/commons_auth"
	"github.com/vasic-digital/herald/commons_messaging/channels/null"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
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
	// Wave 2 Task 6: propagate ldflags-injected build info into
	// commons/cli so VersionCmd's JSON output and MetricsHandler's
	// build_info gauge surface the real values (not the defaults).
	cli.BuildVersion = version
	cli.BuildCommit = commit

	branding := commons.DefaultBranding("p", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(newServeCmd(branding)) // Wave 3b: builds Runner + verifier lazily inside serve
	root.AddCommand(newMigrateCmd())       // real impl (HRD-010) — unchanged
	root.AddCommand(newWizardCmd())        // real impl (HRD-011/012 setup) — unchanged
	root.AddCommand(newListenCmd())        // Wave 6 T6: inbound getUpdates long-poll + CC dispatch
	registerStubs(root)                    // §43 GitOps stubs via cli.StubCmd

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pherald:", err)
		os.Exit(1)
	}
}

// buildRedisClient returns a Redis client when HERALD_REDIS_URL is set, or
// nil otherwise. The Runner's IdempotencyChecker tolerates a nil client
// (the SetNX adapter short-circuits to "not seen" and the PG archive table
// becomes the sole duplicate detector — degraded but functional). The
// JWKS-mode verifier also tolerates nil (in-memory key cache fallback).
func buildRedisClient() redis.Cmdable {
	url := os.Getenv("HERALD_REDIS_URL")
	if url == "" {
		return nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: parse HERALD_REDIS_URL:", err)
		os.Exit(1)
	}
	return redis.NewClient(opts)
}

// mustOpenPGPool parses HERALD_PG_DSN and returns the underlying
// *pgxpool.Pool plus a closer for graceful shutdown. Wave 3b explicitly
// has NO PG-less serve mode — the Runner's idempotency archive +
// subscribers list + evidence writes all require PG. Failing loud here
// beats silently dropping events on the floor at request time.
func mustOpenPGPool(ctx context.Context) (*pgxpool.Pool, func()) {
	dsn := os.Getenv("HERALD_PG_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "pherald: HERALD_PG_DSN required (no PG-less serve mode for Wave 3b; see docs/Issues.md HRD-016 follow-ups)")
		os.Exit(1)
	}
	cfg, err := storage.ParseDSN(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: parse HERALD_PG_DSN:", err)
		os.Exit(1)
	}
	database, pool, err := storage.OpenWithPool(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: open PG:", err)
		os.Exit(1)
	}
	return pool, func() { _ = database.Close() }
}

// buildChannelRegistry constructs the map of channel adapters available to
// the Runner. null:// is always registered (sandbox); Telegram is added
// only when HERALD_TGRAM_BOT_TOKEN is set so dev/CI runs without real
// credentials don't pull the Bot API into the boot path.
func buildChannelRegistry() map[commons.ChannelID]commons.Channel {
	reg := map[commons.ChannelID]commons.Channel{}
	// null:// always available (sandbox/test).
	nullAdapter, err := null.New("null://herald-sandbox?ceiling=routed")
	if err != nil {
		// null:// adapter constructor is failsafe; this should never
		// happen. Fail loud so we notice if the contract changes.
		fmt.Fprintln(os.Stderr, "pherald: build null adapter:", err)
		os.Exit(1)
	}
	reg[commons.ChannelNull] = nullAdapter
	// Telegram if creds present. The Wave 3b dispatcher routes per-recipient
	// via the subscriber's alias rather than a fixed chat_id, so use a
	// placeholder chat_id of "0" here — the constructor only needs a
	// well-formed URL.
	if tok := os.Getenv("HERALD_TGRAM_BOT_TOKEN"); tok != "" {
		tgramAdapter, err := tgram.New("tgram://" + tok + "/0")
		if err != nil {
			fmt.Fprintln(os.Stderr, "pherald: build tgram adapter:", err)
			os.Exit(1)
		}
		reg[commons.ChannelTelegram] = tgramAdapter
	}
	return reg
}

// buildVerifier constructs the JWT verifier from env. Refuses to start
// without HERALD_AUTH_MODE — anonymous serve plane is a §107 PASS-bluff.
func buildVerifier() commons_auth.Verifier {
	v, err := commons_auth.NewVerifierFromEnv(buildRedisClient())
	if err != nil {
		fmt.Fprintln(os.Stderr, "pherald: build verifier:", err)
		os.Exit(1)
	}
	return v
}
