package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/giulio/secret-rotator/internal/history"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the status subcommand.
func NewStatusCmd() *cobra.Command {
	var passphrase string
	var dir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show secret rotation status",
		Long:  `Status displays the current state of all managed secrets including age and next rotation time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if AppConfig == nil || len(AppConfig.Secrets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No secrets configured.")
				return nil
			}

			// Build map of secretName -> lastRotatedAt from history
			lastRotated := make(map[string]time.Time)

			pp := resolvePassphrase(passphrase)
			if pp != "" {
				historyPath := filepath.Join(dir, ".rotator", "history.json")
				if _, err := os.Stat(historyPath); err == nil {
					store := history.NewStore(historyPath, []byte(pp))
					entries, err := store.List()
					if err == nil {
						for _, e := range entries {
							if e.Status != "success" {
								continue
							}
							if prev, ok := lastRotated[e.SecretName]; !ok || e.RotatedAt.After(prev) {
								lastRotated[e.SecretName] = e.RotatedAt
							}
						}
					}
				}
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tAGE\tSCHEDULE\tNEXT ROTATION")

			now := time.Now()
			for _, sec := range AppConfig.Secrets {
				// Compute age
				age := "never"
				if rotatedAt, ok := lastRotated[sec.Name]; ok {
					age = formatDuration(now.Sub(rotatedAt))
				}

				// Compute schedule and next rotation
				schedule := "none"
				nextRotation := "-"
				if sec.Schedule != "" {
					schedule = sec.Schedule
					parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
					sched, err := parser.Parse(sec.Schedule)
					if err == nil {
						next := sched.Next(now)
						nextRotation = next.Format("2006-01-02 15:04")
					}
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					sec.Name,
					sec.Type,
					age,
					schedule,
					nextRotation,
				)
			}
			w.Flush()

			fmt.Fprintf(cmd.OutOrStdout(), "\n%d secrets configured\n", len(AppConfig.Secrets))
			return nil
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for history decryption")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing .rotator/history.json")

	return cmd
}

// formatDuration formats a time.Duration as a human-readable string.
// <1h: "{m}m", <1d: "{h}h {m}m", >=1d: "{d}d {h}h"
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	totalMinutes := int(d.Minutes())
	totalHours := totalMinutes / 60
	minutes := totalMinutes % 60
	days := totalHours / 24
	hours := totalHours % 24

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if totalHours > 0 {
		return fmt.Sprintf("%dh %dm", totalHours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
