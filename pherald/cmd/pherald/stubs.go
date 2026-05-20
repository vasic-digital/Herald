// Stub subcommands for pherald. Each one is wired up so `--help` works
// and the operator sees the spec section that will implement it; the
// actual body returns a "not implemented" error pointing at the HRD
// that tracks the work.
//
// This keeps the binary buildable + the CLI surface complete from r1
// while the implementation work lands incrementally.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	httpsrv "github.com/vasic-digital/herald/pherald/internal/http"
)

func newSendCmd() *cobra.Command {
	var (
		eventType  string
		source     string
		dataInline string
		dataFile   string
		tags       []string
		idemKey    string
		priority   int
	)
	cmd := &cobra.Command{
		Use:   "send",
		Short: "One-shot: ingest a CloudEvent and exit (spec §3.1)",
		Long: `Build a CloudEvents v1.0 envelope from flags + optional JSON data,
post it to the local serve daemon (or in-process router when --inline),
and exit non-zero on delivery failure within the deadline.

NOT YET IMPLEMENTED (HRD-008 / HRD-010 / HRD-011 dependencies).`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("pherald send: not implemented (HRD-008/HRD-010/HRD-011)")
		},
	}
	cmd.Flags().StringVar(&eventType, "type", "", "CloudEvents type (e.g. digital.vasic.herald.ci.failed)")
	cmd.Flags().StringVar(&source, "source", "", "CloudEvents source URI")
	cmd.Flags().StringVar(&dataInline, "data", "", "inline JSON data payload")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "read data payload from file")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "routing tag (repeatable)")
	cmd.Flags().StringVar(&idemKey, "idempotency-key", "", "explicit idempotency key (default: derived from event ID)")
	cmd.Flags().IntVar(&priority, "priority", 3, "ntfy-style priority 1..5")
	return cmd
}

func newServeCmd() *cobra.Command {
	var (
		configFile string
		httpPort   int
		adminPort  int
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Daemon: HTTP ingress + per-channel subscriber loops (spec §3.1)",
		Long: `Long-running daemon that:

  - exposes HTTP ingress on --http-port (default 24091) accepting
    CloudEvents (binary + structured mode) at POST /v1/events;
  - exposes admin endpoints on --admin-port (default 24090) for
    /livez, /readyz, /startupz, /metrics, /admin/version;
  - polls upstream channels per spec §32.2 (30 s minimum cadence);
  - dispatches inbound work through the §32 7-stage Worker pipeline;
  - traps SIGTERM/SIGINT for graceful drain per §3.1.

Foundation M3 status (2026-05-20): the Gin REST surface + middleware chain
are live. /v1/healthz + /metrics work end-to-end. /v1/events and
/v1/compliance return 501 with HRD-016 pointers until the Runner wiring
lands.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			srv := httpsrv.New(httpsrv.Config{
				Addr: fmt.Sprintf("0.0.0.0:%d", httpPort),
				Build: httpsrv.BuildInfo{
					Version:   version,
					GitCommit: commit,
					GoVersion: runtime.Version(),
				},
			})

			// Graceful shutdown per §3.1.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			errCh := make(chan error, 1)
			go func() { errCh <- srv.Start() }()
			fmt.Fprintf(os.Stderr, "pherald serve: listening on :%d\n", httpPort)
			select {
			case <-ctx.Done():
				fmt.Fprintln(os.Stderr, "pherald serve: shutdown signal received, draining...")
				return srv.Shutdown(context.Background())
			case err := <-errCh:
				return err
			}
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "c", "config.toml", "path to TOML config file")
	cmd.Flags().IntVar(&httpPort, "http-port", 24091, "HTTP ingress port")
	cmd.Flags().IntVar(&adminPort, "admin-port", 24090, "admin port (probes + metrics)")
	return cmd
}

func newDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify environment health (Postgres / Redis / channels / DNS) (spec §17.6)",
		Long: `Battery of environment checks:

  - Postgres connectivity + RLS policy presence
  - Redis connectivity + tenant ACL
  - Channel credentials valid (probes API per channel)
  - DKIM / SPF / DMARC DNS records for the configured sending domain
  - OTLP collector reachable
  - Disk space, port availability

Exits non-zero on any failure; with --json emits a structured report.

NOT YET IMPLEMENTED (HRD-010 / HRD-011 dependencies).`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("pherald doctor: not implemented (HRD-010/HRD-011)")
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON report")
	return cmd
}

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply / rollback / inspect database migrations (spec §9.6)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "apply pending migrations forward",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("pherald migrate up: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "down",
		Short: "rollback N migrations (default 1)",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("pherald migrate down: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "show current version + pending count",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("pherald migrate status: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "checksum every applied migration against the source files",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("pherald migrate validate: not implemented (HRD-010)") },
	})
	return cmd
}

func newSubscriberCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subscriber",
		Short: "Manage subscribers (list / add / verify / link / forget) (spec §7)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "list subscribers for the active tenant",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("subscriber list: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "add a new subscriber",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("subscriber add: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify <token>",
		Short: "complete the self-claim flow",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("subscriber verify: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "forget <id>",
		Short: "initiate GDPR right-to-erasure (§16.1)",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("subscriber forget: not implemented (HRD-010)") },
	})
	return cmd
}

func newDeadletterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deadletter",
		Short: "Inspect / replay / purge dead-lettered messages (§5.4)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "list dead-lettered messages",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("deadletter list: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "replay <id>",
		Short: "replay a single dead-lettered message",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("deadletter replay: not implemented (HRD-010)") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "purge",
		Short: "delete dead-lettered messages past retention (§16.1)",
		RunE:  func(_ *cobra.Command, _ []string) error { return errors.New("deadletter purge: not implemented (HRD-010)") },
	})
	return cmd
}
