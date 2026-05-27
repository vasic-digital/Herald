// Package stubs registers sherald's §43 stub commands. Each entry was a
// cli.StubCmd that returned a 501-style error with an HRD pointer until the
// corresponding implementation landed. All sherald §43 commands are now LIVE
// (registered via registerSysOps in main.go), so Register is a no-op retained
// for the main() call-site contract.
package stubs

import (
	"github.com/spf13/cobra"
)

// Register attaches every remaining §43 stub command targeted at sherald.
//
// The destructive-guard (HRD-033), constitution-pull (HRD-040), force-push-gate
// (HRD-046), mem-budget-watch (HRD-056), and backup-snapshot (HRD-034) commands
// are now LIVE (v1.0.0 Batch C cluster C2 + §43 straggler — see
// sherald/cmd/sherald/sysops_cmds.go); they are registered via registerSysOps in
// main.go, not here. No sherald §43 stubs remain.
func Register(_ *cobra.Command) {}
