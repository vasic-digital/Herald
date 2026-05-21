// Package stubs registers sherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every §43 stub command targeted at sherald.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("destructive-guard", "HRD-033", "Wrap rm/git-reset/git-push-force with prerequisite checks (§9.1)"))
	root.AddCommand(cli.StubCmd("backup-snapshot", "HRD-034", "Hardlinked snapshot helper (§9.3)"))
	root.AddCommand(cli.StubCmd("constitution-pull", "HRD-040", "Wrap fetch + rebase + validation gate (§11.4.26)"))
	root.AddCommand(cli.StubCmd("force-push-gate", "HRD-046", "Merge-first + per-session-auth enforcement (§11.4.41)"))
	root.AddCommand(cli.StubCmd("mem-budget-watch", "HRD-056", "Daemon-mode 60% threshold watcher (§12.6)"))
}
