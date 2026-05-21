// Package stubs registers scherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every §43 stub command targeted at scherald.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("status-digest", "HRD-047", "Periodic Status.md digest emitter per §11.4.55"))
}
