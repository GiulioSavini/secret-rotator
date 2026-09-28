package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version variables injected via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// NewVersionCmd returns the version subcommand.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "rotator version %s (commit: %s, built: %s)\n", version, commit, date)
		},
	}
}
