// Package stubs registers cherald's §43 stub commands. Each entry is a
// cli.StubCmd that returns a 501-style error with an HRD pointer until
// the corresponding implementation lands.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
)

// Register attaches every §43 stub command targeted at cherald.
func Register(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("creds-scan", "HRD-036", "Scan repo for leaked credentials per §16.2"))
	root.AddCommand(cli.StubCmd("docs-sync", "HRD-037", "Regenerate doc siblings (PDF/HTML) per §11.4.61"))
	root.AddCommand(cli.StubCmd("script-docs-check", "HRD-038", "Audit script docstrings per §11.4.62"))
	root.AddCommand(cli.StubCmd("fixed-align", "HRD-039", "Reconcile Issues.md ↔ Fixed.md per §11.4.55"))
	root.AddCommand(cli.StubCmd("submanifest-verify", "HRD-042", "Verify submodule manifests per §11.4.35"))
	root.AddCommand(cli.StubCmd("fixed-summary-sync", "HRD-048", "Auto-update Fixed summary lines per §11.4.55"))
	root.AddCommand(cli.StubCmd("readme-sync", "HRD-050", "Cross-link README ↔ guides per §11.4.61"))
	root.AddCommand(cli.StubCmd("composite-gate", "HRD-051", "Bundle every gate test under one entrypoint per §11.4.69"))
	root.AddCommand(cli.StubCmd("export", "HRD-052", "Doc export pipeline (Markdown → PDF/HTML/DOCX) per §11.4.61"))
	root.AddCommand(cli.StubCmd("spec-version-check", "HRD-054", "Audit spec revision bumps per §11.4.66"))
	root.AddCommand(cli.StubCmd("catalogue-check", "HRD-055", "Submodule-catalogue propagation per §11.4.74"))
}
