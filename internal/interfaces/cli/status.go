package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/audit"
	"github.com/giulio/secret-rotator/internal/infrastructure/scheduling"
)

// NewStatusCmd creates the status subcommand.
func NewStatusCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show secret rotation status",
		Long:  `Status displays the current state of all managed secrets including age and next rotation time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(AppConfig.Secrets()) == 0 {
				// status is read-only, so guide the user instead of failing.
				fmt.Fprintln(cmd.OutOrStdout(), "No secrets configured.")
				if err := AppConfig.RequireSecrets(); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "\n%v\n", err)
				}
				return nil
			}

			// Build map of secretName -> lastRotatedAt from history
			lastRotated := make(map[string]time.Time)

			pp := resolvePassphrase(passphrase)
			if pp != "" {
				path := historyPath()
				if _, err := os.Stat(path); err == nil {
					store := audit.NewStore(path, []byte(pp))
					entries, err := store.List()
					if err == nil {
						for _, e := range entries {
							if e.Status != string(domain.OutcomeSuccess) {
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
			parser := scheduling.Parser()
			for _, sec := range AppConfig.Secrets() {
				age := "never"
				if rotatedAt, ok := lastRotated[sec.Name().String()]; ok {
					age = formatDuration(now.Sub(rotatedAt))
				}

				schedule := "none"
				nextRotation := "-"
				if sec.Schedule().IsSet() {
					schedule = sec.Schedule().String()
					// The same parser the daemon uses, so status never shows a
					// next run the scheduler would reject.
					if parsed, err := parser.Parse(schedule); err == nil {
						nextRotation = parsed.Next(now).Format("2006-01-02 15:04")
					} else {
						nextRotation = "invalid expression"
					}
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					sec.Name(), sec.Kind(), age, schedule, nextRotation)
			}
			w.Flush()

			fmt.Fprintf(cmd.OutOrStdout(), "\n%d secrets configured\n", len(AppConfig.Secrets()))
			return nil
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for history decryption")

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
