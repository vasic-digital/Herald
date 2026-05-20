package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
)

func newVersionCmd(b commons.Branding) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version, build info, and runtime",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := map[string]string{
				"binary":     b.BinaryName,
				"app_name":   b.AppName,
				"version":    version,
				"commit":     commit,
				"go_version": runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
			}
			for _, k := range []string{"binary", "app_name", "version", "commit", "go_version", "os", "arch"} {
				fmt.Fprintf(cmd.OutOrStdout(), "%-10s %s\n", k+":", info[k])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}
