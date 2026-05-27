// Package stubs registers scherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
//
// scherald's sole §43 row — status-digest (HRD-047, §11.4.45) — is now LIVE
// (see cmd/scherald/digest_cmds.go, registered via registerDigestOps). No §43
// stub remains, so Register is a no-op kept for symmetry with the other flavors
// and as the landing point for any future scherald §43 stub.
package stubs

import (
	"github.com/spf13/cobra"
)

// Register attaches every §43 stub command targeted at scherald. There are none
// at present (HRD-047 status-digest landed); this is a no-op.
func Register(_ *cobra.Command) {}
