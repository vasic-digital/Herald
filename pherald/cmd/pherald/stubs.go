// §43 GitOps stub commands for pherald.
//
// Wave 2 r1 refactor (2026-05-21): replaced the previous one-off
// newSendCmd / newDoctorCmd / newSubscriberCmd / newDeadletterCmd
// stubs (none of which had real bodies anyway) with the §43 mandate
// commands that pherald actually owns per spec V3 §43. Each is
// registered as a cli.StubCmd that returns a 501-style error with an
// HRD pointer so the operator knows where the implementation is tracked.
//
// newServeCmd is preserved here pending the Task 6 refactor. Its real
// implementation moved here in commit 92ecdc6 (Foundation M3 Gin REST
// surface). Will be moved to its own serve.go in Task 6.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons/cli"
	httpsrv "github.com/vasic-digital/herald/pherald/internal/http"
)

// registerStubs adds every §43 GitOps command targeted at pherald as a
// 501-stub. HRD pointers track implementation status.
func registerStubs(root *cobra.Command) {
	root.AddCommand(cli.StubCmd("commit-push", "HRD-029", "Single-entrypoint locked commit + multi-mirror push (§2)"))
	root.AddCommand(cli.StubCmd("submodule-propagate", "HRD-030", "Owned-submodule walk in propagation order (§3)"))
	root.AddCommand(cli.StubCmd("install-upstreams", "HRD-043", "install_upstreams wrapper (§11.4.36)"))
	root.AddCommand(cli.StubCmd("fetch-guard", "HRD-044", "Pre-edit fetch + rebase enforcement (§11.4.37)"))
	root.AddCommand(cli.StubCmd("reopen", "HRD-049", "Issues→Fixed reversal + Reopens history (§11.4.55)"))
	root.AddCommand(cli.StubCmd("pre-push", "HRD-053", "Fetch + investigate + integrate hook (§11.4.71)"))
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
