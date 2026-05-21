// iherald — Incident Herald per spec V3 §18 + §43 (Wave 2 r1 scaffold).
//
// Single-binary CLI for the Incident Herald flavor (credential-leak
// page-out + operator-blocked escalation). HRD-024's paging surface
// lives only on the HTTP plane, so this flavor ships ZERO §43 stub
// subcommands by design — the stubs package is intentionally omitted
// (no internal/stubs/ directory) to keep main.go minimal. The HTTP
// serve plane exposes /v1/webhooks/page as a 501 stub (→ HRD-024) and
// the shared healthz/readyz/metrics from commons/cli/.
package main

import (
	"fmt"
	"os"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	flavhttp "github.com/vasic-digital/herald/iherald/internal/http"
)

// version is overridden at build time:
//   go build -ldflags "-X main.version=$(git describe --tags)"
var version = "0.0.0-dev"

// commit is overridden at build time:
//   go build -ldflags "-X main.commit=$(git rev-parse --short HEAD)"
var commit = "unknown"

func main() {
	cli.BuildVersion = version
	cli.BuildCommit = commit

	branding := commons.DefaultBranding("i", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(cli.ServeCmd(cli.ServeOpts{
		Branding: branding,
		Routes:   flavhttp.Routes(),
	}))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "iherald:", err)
		os.Exit(1)
	}
}
