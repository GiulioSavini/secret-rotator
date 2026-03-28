package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/giulio/secret-rotator/internal/history"
	"github.com/spf13/cobra"
)

// NewHistoryCmd creates the history subcommand.
func NewHistoryCmd() *cobra.Command {
	var passphrase string
	var dir string
	var limit int

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show rotation audit log",
		Long:  `History displays the audit log of past secret rotations with timestamps and outcomes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve passphrase: flag > env var > config-specified env var
			pp := resolvePassphrase(passphrase)
			if pp == "" {
				return fmt.Errorf("master passphrase required: set --passphrase flag, ROTATOR_MASTER_KEY env var, or configure master_key_env in rotator.yml")
			}

			historyPath := filepath.Join(dir, ".rotator", "history.json")
			store := history.NewStore(historyPath, []byte(pp))

			entries, err := store.List()
			if err != nil {
				return fmt.Errorf("reading history: %w", err)
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No rotation history found.")
				return nil
			}

			// Apply limit (show last N entries)
			if limit > 0 && limit < len(entries) {
				entries = entries[len(entries)-limit:]
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TIME\tSECRET\tSTATUS\tDETAILS")
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					e.RotatedAt.Format("2006-01-02 15:04:05"),
					e.SecretName,
					e.Status,
					e.Details,
				)
			}
			w.Flush()

			fmt.Fprintf(cmd.OutOrStdout(), "\nShowing %d entries\n", len(entries))
			return nil
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for decryption")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing .rotator/history.json")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit number of entries shown (0 = all)")

	return cmd
}

// resolvePassphrase resolves the master passphrase from multiple sources.
// Priority: explicit value > ROTATOR_MASTER_KEY env > config-specified env var.
func resolvePassphrase(explicit string) string {
	if explicit != "" {
		return explicit
	}

	if v := os.Getenv("ROTATOR_MASTER_KEY"); v != "" {
		return v
	}

	if AppConfig != nil && AppConfig.MasterKeyEnv != "" {
		if v := os.Getenv(AppConfig.MasterKeyEnv); v != "" {
			return v
		}
	}

	return ""
}
