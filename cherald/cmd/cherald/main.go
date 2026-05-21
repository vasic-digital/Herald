// cherald — Constitution Herald per spec V3 §18 + §43 (Wave 2 r1 scaffold).
//
// Single-binary CLI for the Constitution Herald flavor (policy evaluator
// + creds scan + docs sync + composite gate). Stubs every §43 command
// targeted at this flavor as a 501-style cli.StubCmd until the
// corresponding HRD implementation lands; the HTTP serve plane exposes
// /v1/compliance as a 501 stub (→ HRD-028) and the shared
// healthz/readyz/metrics from commons/cli/.
package main

import (
	"fmt"
	"os"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	flavhttp "github.com/vasic-digital/herald/cherald/internal/http"
	"github.com/vasic-digital/herald/cherald/internal/stubs"
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

	branding := commons.DefaultBranding("c", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(cli.ServeCmd(cli.ServeOpts{
		Branding: branding,
		Routes:   flavhttp.Routes(),
	}))
	stubs.Register(root)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "cherald:", err)
		os.Exit(1)
	}
}
