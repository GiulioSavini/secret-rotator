package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewHistoryCmd creates the history subcommand.
func NewHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show rotation audit log",
		Long:  `History displays the audit log of past secret rotations with timestamps and outcomes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("history not yet implemented")
			return nil
		},
	}
}
