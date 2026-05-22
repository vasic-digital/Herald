// qaherald — QA Herald per spec V3 §47 + Herald Constitution §107.x
// (operator-locked, 2026-05-22).
//
// qaherald is Herald's QA flavor binary — the 8th flavor and the 16th
// workspace module. It impersonates a Telegram client, posts CloudEvents
// to pherald via HTTPS+JWT (with TOON content-negotiation, Brotli, and
// HTTP/3 ALPN preference per Waves 4a + 4b), records bidirectional
// transcripts + sha256-checked attachments under docs/qa/<run-id>/, and
// emits a Markdown report.
//
// Wave 5 Task 1 lands the skeleton: Cobra root + `qaherald version`
// (via the shared commons/cli scaffold). Internal packages (transcript,
// tgram, herald, scenario, report) and the `qaherald run` subcommand
// land in Tasks 2..7.
//
// §107 anti-bluff posture: build-time -ldflags injection of `version`
// + `commit` MUST surface verbatim in the `version` subcommand output
// (see qaherald/cmd/qaherald/main_test.go). T10 mutation gate plants
// an always-"0.0.0" bluff in this file; the test detects it.
//
// CLI-only flavor (DefaultPort=0 per branding); qaherald drives external
// services and does not serve HTTP itself.
package main

import (
	"fmt"
	"os"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
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
	// Propagate ldflags-injected build info into commons/cli so
	// VersionCmd's human + JSON output surface the real values (not the
	// defaults). Mirrors pherald/cherald/sherald/rherald wiring.
	cli.BuildVersion = version
	cli.BuildCommit = commit

	branding := commons.DefaultBranding("qa", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	// `qaherald run` lands in Wave 5 Task 7 once the scenario engine
	// (T5) + report generator (T6) are wired. Until then, `qaherald
	// version` is the only working subcommand.

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "qaherald:", err)
		os.Exit(1)
	}
}
