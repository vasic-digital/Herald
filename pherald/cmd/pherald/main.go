// pherald — Project Herald CLI per spec V3 §3 + §18.2 (Wave 2 r1).
//
// Refactored 2026-05-21 to consume the shared commons/cli/ scaffold
// (Wave 2 design §3): NewRootCmd + VersionCmd come from there; pherald
// owns only the flavor-specific subcommands (serve, migrate, wizard) +
// the §43 GitOps stubs registered via registerStubs.
package main

import (
	"fmt"
	"os"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
)

// version is overridden at build time:
//   go build -ldflags "-X main.version=$(git describe --tags)"
var version = "0.0.0-dev"

// commit is overridden at build time:
//   go build -ldflags "-X main.commit=$(git rev-parse --short HEAD)"
var commit = "unknown"

func main() {
	branding := commons.DefaultBranding("p", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	root.AddCommand(newServeCmd())   // preserved real impl (Task 6 will refactor to cli.ServeCmd)
	root.AddCommand(newMigrateCmd()) // real impl (HRD-010) — unchanged
	root.AddCommand(newWizardCmd())  // real impl (HRD-011/012 setup) — unchanged
	registerStubs(root)              // §43 GitOps stubs via cli.StubCmd

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pherald:", err)
		os.Exit(1)
	}
}
