// Package stubs registers bherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every §43 stub command targeted at bherald.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("evidence-capture", "HRD-035", "CI evidence-bundle capture per §11.4.27"))
	root.AddCommand(cli.StubCmd("test-tier-verify", "HRD-041", "Tier-1/2/3 test promotion gate per §11.4.34"))
	root.AddCommand(cli.StubCmd("gate-retest", "HRD-045", "Re-run composite gate post-fix per §11.4.38 (alias shared with rherald)"))
}
