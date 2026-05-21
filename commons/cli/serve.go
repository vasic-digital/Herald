package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
)

// ServeOpts configures ServeCmd. Branding drives the default port +
// healthz/metrics gauge name. Routes are flavor-specific routes
// appended after the base healthz/readyz/metrics. Middleware is the
// optional flavor-specific Gin middleware chain registered immediately
// after gin.Recovery() and before any handler — typical use is request-
// ID propagation, tenant extraction, OTel tracing. Nil/empty is fine.
type ServeOpts struct {
	Branding   commons.Branding
	Routes     []Route
	Middleware []gin.HandlerFunc
}

// ServeCmd is the `<flavor>herald serve` subcommand for serving flavors.
// Binds a Gin engine with healthz/readyz/metrics + every Route in
// opts.Routes (nil-Handler+non-empty-HRD routes get StubRouteHandler),
// listens on --http-port (default = branding.DefaultPort), graceful-
// shutdown on SIGTERM/SIGINT or context cancel. Returns nil on clean
// shutdown, error on bind failure.
//
// Per §107: a "serve started" PASS without observing a healthz round-
// trip is a bluff — the integration test starts the server and verifies
// it actually accepts traffic before declaring success.
func ServeCmd(opts ServeOpts) *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the " + opts.Branding.DisplayName + " HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if port == 0 {
				port = opts.Branding.DefaultPort
			}
			gin.SetMode(gin.ReleaseMode)
			r := gin.New()
			r.Use(gin.Recovery())
			for _, mw := range opts.Middleware {
				if mw != nil {
					r.Use(mw)
				}
			}
			r.GET("/v1/healthz", HealthzHandler(opts.Branding))
			r.GET("/v1/readyz", ReadyzHandler(opts.Branding))
			r.GET("/metrics", MetricsHandler(opts.Branding))
			for _, route := range opts.Routes {
				h := route.Handler
				if h == nil && route.HRD != "" {
					h = StubRouteHandler(route)
				}
				r.Handle(route.Method, route.Path, h)
			}
			srv := &http.Server{
				Addr:    fmt.Sprintf(":%d", port),
				Handler: r,
			}
			errCh := make(chan error, 1)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			select {
			case err := <-errCh:
				return fmt.Errorf("listen: %w", err)
			case <-sigCh:
				// graceful shutdown
			case <-ctx.Done():
				// graceful shutdown
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		},
	}
	cmd.Flags().IntVar(&port, "http-port", 0, "TCP port to bind (default = flavor's DefaultPort)")
	return cmd
}
