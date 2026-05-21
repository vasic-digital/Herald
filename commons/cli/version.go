package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
)

// Build-info variables. Set via -ldflags at build time; otherwise
// default to "dev" / unknown markers so a raw `go run` invocation is
// still operational. §107: an empty version field is a bluff —
// VersionCmd's JSON shape MUST populate every required key.
var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildTime    = "unknown"
)

// VersionCmd is the `<flavor>herald version` subcommand. Prints human-
// readable build info by default; `--json` returns the canonical JSON
// shape used by e2e_bluff_hunt E2/E19-E24 (required fields: binary,
// flavor, version, go_version, os, arch — none may be empty).
func VersionCmd(br commons.Branding) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print " + br.DisplayName + " version + build info",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := map[string]string{
				"binary":     br.Flavor,
				"flavor":     br.Flavor,
				"version":    BuildVersion,
				"commit":     BuildCommit,
				"build_time": BuildTime,
				"go_version": runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s %s\n", br.DisplayName, BuildVersion)
			fmt.Fprintf(out, "  flavor:     %s\n", br.Flavor)
			fmt.Fprintf(out, "  commit:     %s\n", BuildCommit)
			fmt.Fprintf(out, "  built:      %s\n", BuildTime)
			fmt.Fprintf(out, "  go_version: %s\n", runtime.Version())
			fmt.Fprintf(out, "  os/arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of human-readable text")
	return cmd
}
