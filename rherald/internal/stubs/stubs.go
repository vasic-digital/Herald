// Package stubs registers rherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every §43 stub command targeted at rherald.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("tag-mirror", "HRD-031", "Multi-mirror tag fan-out per §11.4.25"))
	root.AddCommand(cli.StubCmd("changelog-generate", "HRD-032", "CHANGELOG generation from commit graph per §11.4.61"))
	root.AddCommand(cli.StubCmd("gate-retest", "HRD-045", "Re-run composite gate post-fix per §11.4.38 (canonical owner)"))
}
