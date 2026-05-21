package main

import (
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/pherald/internal/wizard"
)

// newWizardCmd returns the `pherald wizard` parent command with its
// subcommands wired in. Per Universal §11.4.10 + Herald §107: the wizard
// validates every credential against its source-of-truth API (e.g.
// Telegram getMe, claude --version) before saving — a token that
// "looks right" but fails validation is rejected at the prompt, not
// silently persisted.
func newWizardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wizard",
		Short: "Interactive credential-setup wizards",
		Long: "Interactive wizards that walk the operator through obtaining " +
			"credentials for every supported Herald integration and persist " +
			"them to the operator's shell startup file (~/.zshrc or " +
			"~/.bashrc) + a git-ignored ~/.herald/credentials.md summary.",
	}
	cmd.AddCommand(newWizardCredentialsCmd())
	return cmd
}

func newWizardCredentialsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "credentials [service]",
		Short: "Configure credentials for a Herald integration (interactive)",
		Long: "Walks the operator through obtaining credentials for the named " +
			"service (telegram | claude-code | all). Without an argument, " +
			"prompts the operator to pick from a menu.\n\n" +
			"Supported services (LIVE):\n" +
			"  telegram     — HRD-011 Telegram bot token + chat ID (validated via getMe)\n" +
			"  claude-code  — HRD-012 Claude Code binary + project name (validated via --version)\n" +
			"  all          — both, in order\n\n" +
			"Per Universal Constitution §11.4.10:\n" +
			"  - Raw credentials are NEVER printed back to the terminal.\n" +
			"  - The ~/.herald/credentials.md summary stores MASKED values only.\n" +
			"  - The shell-startup file write is verified by re-reading the file.\n\n" +
			"Detailed guides:\n" +
			"  docs/guides/messengers/TELEGRAM.md\n" +
			"  docs/guides/dispatchers/CLAUDE_CODE.md\n" +
			"  docs/guides/OPERATOR_CREDENTIALS.md",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := ""
			if len(args) == 1 {
				service = args[0]
			}
			return wizard.Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), service)
		},
	}
}
