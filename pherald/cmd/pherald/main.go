// pherald — Project Herald CLI per spec V3 §3 + §18.2.
//
// Single-binary, two-mode design (V3 R-02):
//
//   pherald send  …  — one-shot: ingest a CloudEvent and exit.
//   pherald serve     — daemon: HTTP ingress + per-channel subscriber loops.
//
// Plus the standard admin subcommands documented in §3.1.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
)

// version is overridden at build time:
//   go build -ldflags "-X main.version=$(git describe --tags)"
var version = "0.0.0-dev"

// commit is overridden at build time:
//   go build -ldflags "-X main.commit=$(git rev-parse --short HEAD)"
var commit = "unknown"

func main() {
	branding := commons.DefaultBranding("p", version)

	root := &cobra.Command{
		Use:   "pherald",
		Short: branding.AppName + " — event fan-out + LLM-dispatched reply pipeline",
		Long: branding.AppName + ` is the Project Herald flavor (spec V3 §18.2).

It ingests software-project lifecycle events (Git hooks, VCS webhooks,
AI CLI agents, code-review tools) and fans them out to messaging
channels (Telegram, Slack, Email, …). Subscriber replies trigger the
Investigation-before-Fixing flow (§18.2.1) with Claude Code as the
default LLM dispatcher (§33).

Run "pherald <subcommand> --help" for per-subcommand documentation.`,
		Version: version + " (" + commit + ")",
		SilenceUsage: true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd(branding))
	root.AddCommand(newSendCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newMigrateCmd())
	root.AddCommand(newSubscriberCmd())
	root.AddCommand(newDeadletterCmd())
	root.AddCommand(newWizardCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pherald:", err)
		os.Exit(1)
	}
}
