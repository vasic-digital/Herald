package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
)

// RootOpt is a functional option for NewRootCmd.
type RootOpt func(*cobra.Command)

// NewRootCmd returns the top-level Cobra command for a flavor. Branding
// drives Use / Short / Long / Version output. Use is the binary name
// (e.g. "pherald") since Cobra renders it as the invocation command in
// --help; Flavor (single-letter key, e.g. "p") and Prefix (3-letter,
// e.g. "PHR") appear in the long description for §8.2 context.
//
// Falls back to br.Flavor if BinaryName is unset so callers that only
// populate Flavor (older tests / synthetic Branding) still produce a
// usable Cobra command.
func NewRootCmd(br commons.Branding, opts ...RootOpt) *cobra.Command {
	use := br.BinaryName
	if use == "" {
		use = br.Flavor
	}
	cmd := &cobra.Command{
		Use:   use,
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
