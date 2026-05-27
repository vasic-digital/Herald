// Package stubs registers sherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every remaining §43 stub command targeted at sherald.
//
// The destructive-guard (HRD-033), constitution-pull (HRD-040), force-push-gate
// (HRD-046), and mem-budget-watch (HRD-056) commands are now LIVE (v1.0.0 Batch C
// cluster C2 — see sherald/cmd/sherald/sysops_cmds.go); they are registered via
// registerSysOps in main.go, not here.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("backup-snapshot", "HRD-034", "Hardlinked snapshot helper (§9.3)"))
}
