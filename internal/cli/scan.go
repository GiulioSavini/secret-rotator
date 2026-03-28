package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewScanCmd creates the scan subcommand.
func NewScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Discover and audit secrets in .env files",
		Long:  `Scan discovers .env files, identifies secrets, and reports their rotation status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("scan not yet implemented")
			return nil
		},
	}
}
