// Package cli is the shared CLI scaffold for every Herald flavor binary.
// Per Universal §11.4.74 catalogue-check: vendored as Herald-internal
// (no-match against vasic-digital + HelixDevelopment).
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// StubCmd returns a Cobra subcommand that always fails with a 501-style
// error citing the HRD that will implement it. Used by every flavor's
// internal/stubs/ to register §43 commands not yet implemented.
//
// Per Herald §107: the error message MUST contain the HRD pointer + the
// command name + the literal "not yet implemented" — these are the three
// substrings the e2e_bluff_hunt E31 invariant asserts on.
func StubCmd(name, hrd, description string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: description + " (not yet implemented — " + hrd + ")",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s: not yet implemented — see %s for status", name, hrd)
		},
	}
}
