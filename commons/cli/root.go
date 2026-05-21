package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
)

// RootOpt is a functional option for NewRootCmd.
type RootOpt func(*cobra.Command)

// NewRootCmd returns the top-level Cobra command for a flavor. Branding
// drives Use / Short / Long / Version output.
func NewRootCmd(br commons.Branding, opts ...RootOpt) *cobra.Command {
	cmd := &cobra.Command{
		Use:   br.Flavor,
		Short: fmt.Sprintf("%s — %s", br.DisplayName, br.Mission),
		Long: fmt.Sprintf(
			"%s (%s).\n\n%s\n\nFlavor prefix: %s — see Helix Universal Constitution §8.2.",
			br.DisplayName, br.Flavor, br.Mission, br.Prefix,
		),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	for _, opt := range opts {
		opt(cmd)
	}
	return cmd
}
