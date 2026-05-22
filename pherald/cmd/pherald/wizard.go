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
	cmd := &cobra.Command{
		Use:   "credentials [service]",
		Short: "Configure credentials for a Herald integration",
		Long: "Walks the operator through obtaining credentials for the named " +
			"service (telegram | claude-code | all). Without an argument, " +
			"prompts the operator to pick from a menu (interactive only).\n\n" +
			"Supported services (LIVE):\n" +
			"  telegram     — HRD-011 Telegram bot token + chat ID (validated via getMe + getChat)\n" +
			"  claude-code  — HRD-012 Claude Code binary + project name (validated via --version)\n" +
			"  all          — both, in order\n\n" +
			"Skipping prompts via CLI flags / env (resolution order: flag → env → prompt):\n" +
			"  Telegram:    --bot-token            OR HERALD_TGRAM_BOT_TOKEN\n" +
			"               --chat-id              OR HERALD_TGRAM_CHAT_ID\n" +
			"  Claude Code: --claude-bin           OR HERALD_CLAUDE_BIN\n" +
			"               --claude-project       OR HERALD_CLAUDE_PROJECT_NAME\n" +
			"               --claude-session-uuid  OR HERALD_CLAUDE_SESSION_UUID\n" +
			"  Behaviour:   --non-interactive (fail loud if a required value is missing)\n" +
			"               --shell-target=zshrc|bashrc|profile (override the picker)\n\n" +
			"Env-supplied values are NEVER re-persisted; they're already in your shell file.\n\n" +
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
			opts := wizard.Opts{}
			opts.BotToken, _ = cmd.Flags().GetString("bot-token")
			opts.ChatID, _ = cmd.Flags().GetString("chat-id")
			opts.ClaudeBin, _ = cmd.Flags().GetString("claude-bin")
			opts.ClaudeProject, _ = cmd.Flags().GetString("claude-project")
			opts.ClaudeSession, _ = cmd.Flags().GetString("claude-session-uuid")
			opts.NonInteractive, _ = cmd.Flags().GetBool("non-interactive")
			opts.ShellTarget, _ = cmd.Flags().GetString("shell-target")
			return wizard.Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), service, opts)
		},
	}
	cmd.Flags().String("bot-token", "", "Telegram bot token (also accepts HERALD_TGRAM_BOT_TOKEN env; flag wins if both set)")
	cmd.Flags().String("chat-id", "", "Telegram chat ID (also accepts HERALD_TGRAM_CHAT_ID env; skips getUpdates polling)")
	cmd.Flags().String("claude-bin", "", "Path to `claude` CLI (also accepts HERALD_CLAUDE_BIN env)")
	cmd.Flags().String("claude-project", "", "Claude Code project anchor name (also accepts HERALD_CLAUDE_PROJECT_NAME env)")
	cmd.Flags().String("claude-session-uuid", "", "Pre-bootstrapped Claude Code session UUID (also accepts HERALD_CLAUDE_SESSION_UUID env)")
	cmd.Flags().Bool("non-interactive", false, "Fail loud if any required value is missing — no prompts")
	cmd.Flags().String("shell-target", "", "Override the shell-startup file picker (zshrc|bashrc|profile)")
	return cmd
}
