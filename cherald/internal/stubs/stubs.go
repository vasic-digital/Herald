// Package stubs registers cherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"
)

// Register attaches the remaining §43 stub commands targeted at cherald. Both
// the C3a docs-pipeline commands (docs-sync HRD-037, fixed-align HRD-039,
// fixed-summary-sync HRD-048, readme-sync HRD-050, export HRD-052 — registered
// via registerDocsOps) AND the C3b verify/check commands (creds-scan HRD-036,
// script-docs-check HRD-038, submanifest-verify HRD-042, composite-gate
// HRD-051, spec-version-check HRD-054, catalogue-check HRD-055 — registered via
// registerCheckOps) are now LIVE command bodies and are no longer stubbed here.
// Register is kept (and called from main.go) as the seam for any FUTURE
// cherald-owned §43 stub that has not yet landed its implementation.
func Register(_ *cobra.Command) {}
