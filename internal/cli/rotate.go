package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRotateCmd creates the rotate subcommand.
func NewRotateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate SECRET_NAME",
		Short: "Rotate a secret",
		Long:  `Rotate generates a new value for the named secret, updates .env files, and restarts affected containers.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("rotate not yet implemented (secret: %s)\n", args[0])
			return nil
		},
	}
}
