// bherald — Build Herald per spec V3 §18 + §43 (Wave 2 r1 scaffold).
//
// Single-binary CLI for the Build Herald flavor (CI/test bindings +
// test-tier verifier + evidence capture). CLI-only — no HTTP serve
// plane (DefaultPort=0 per branding); only §43 stubs are registered.
package main

import (
	"fmt"
	"os"

	"github.com/vasic-digital/herald/bherald/internal/stubs"
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
	cli.BuildVersion = version
	cli.BuildCommit = commit

	branding := commons.DefaultBranding("b", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	stubs.Register(root)
	registerBuildOps(root)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bherald:", err)
		os.Exit(1)
	}
}
