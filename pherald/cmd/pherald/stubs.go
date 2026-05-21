// §43 GitOps stub commands for pherald.
//
// Wave 2 r1 refactor (2026-05-21): replaced the previous one-off
// newSendCmd / newDoctorCmd / newSubscriberCmd / newDeadletterCmd
// stubs (none of which had real bodies anyway) with the §43 mandate
// commands that pherald actually owns per spec V3 §43. Each is
// registered as a cli.StubCmd that returns a 501-style error with an
// HRD pointer so the operator knows where the implementation is tracked.
//
// Wave 2 Task 6 refactor (2026-05-21): newServeCmd now delegates to the
// shared cli.ServeCmd scaffold — the previous 150-line gin.Engine
// owner (pherald/internal/http/server.go) is gone; pherald owns only
// its flavor-specific Routes + RequestIDMiddleware which are injected
// into cli.ServeCmd via ServeOpts.

package main

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
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

// newServeCmd returns pherald's `serve` subcommand. Wave 2 Task 6:
// delegates to cli.ServeCmd with pherald's two flavor-specific routes
// (/v1/events + /v1/compliance, both 501-stubs under HRD-016) and the
// pherald RequestIDMiddleware injected through ServeOpts.Middleware.
// All graceful-shutdown / port-binding / healthz wiring lives in
// cli.ServeCmd now — no duplicate engine ownership in pherald.
func newServeCmd(br commons.Branding) *cobra.Command {
	return cli.ServeCmd(cli.ServeOpts{
		Branding:   br,
		Routes:     httpsrv.Routes(),
		Middleware: []gin.HandlerFunc{httpsrv.RequestIDMiddleware()},
	})
}
