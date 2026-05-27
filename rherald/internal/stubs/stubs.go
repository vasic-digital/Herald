// Package stubs registers rherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
//
// v1.0.0 Batch C cluster C4 landed the three rherald-owned §43 release commands
// as REAL command bodies (HRD-031 tag-mirror / HRD-032 changelog-generate /
// HRD-045 gate-retest — see cmd/rherald/release_cmds.go + registerReleaseOps).
// Those three stubs were therefore removed; rherald owns no remaining §43 stub,
// so Register is now a no-op kept for symmetry with the other flavors' wiring
// (main.go still calls it so a future rherald-owned stub has a home).
package stubs

import (
	"github.com/spf13/cobra"
)

// Register attaches every §43 stub command targeted at rherald. rherald has no
// remaining unimplemented §43 command (cluster C4 landed all three), so this is
// currently a no-op.
func Register(_ *cobra.Command) {}
