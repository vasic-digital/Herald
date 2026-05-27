// Package stubs registers bherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every §43 stub command targeted at bherald.
//
// HRD-035 evidence-capture + HRD-041 test-tier-verify are now LIVE command
// bodies (v1.0.0 Batch C, cluster C5 — see build_cmds.go / registerBuildOps);
// their stubs are removed. The gate-retest HRD-045 alias remains a stub:
// rherald owns the real impl, bherald keeps only the alias placeholder.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("gate-retest", "HRD-045", "Re-run composite gate post-fix per §11.4.38 (alias shared with rherald)"))
}
