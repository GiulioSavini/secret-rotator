package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewStatusCmd creates the status subcommand.
func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show secret rotation status",
		Long:  `Status displays the current state of all managed secrets including age and next rotation time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("status not yet implemented")
			return nil
		},
	}
}
