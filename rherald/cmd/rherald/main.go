// rherald — Release Herald per spec V3 §18 + §43 (Wave 2 r1 scaffold).
//
// Single-binary CLI for the Release Herald flavor (tag mirroring +
// changelog + installable-asset evidence). CLI-only — no HTTP serve
// plane (DefaultPort=0 per branding); only §43 stubs are registered.
package main

import (
	"fmt"
	"os"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
	"github.com/vasic-digital/herald/rherald/internal/stubs"
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

	branding := commons.DefaultBranding("r", version)

	root := cli.NewRootCmd(branding)
	root.Version = version + " (" + commit + ")"

	root.AddCommand(cli.VersionCmd(branding))
	// §43 release-lifecycle commands register via registerReleaseOps (HRD-031
	// tag-mirror / HRD-032 changelog-generate / HRD-045 gate-retest); the
	// remaining stubs (currently none) via rherald/internal/stubs.
	registerReleaseOps(root)
	stubs.Register(root)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "rherald:", err)
		os.Exit(1)
	}
}
