package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/GiulioSavini/secret-rotator/internal/application"
	"github.com/GiulioSavini/secret-rotator/internal/domain"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/scheduling"
)

// ErrOverdue is returned by `status --overdue` when at least one secret is
// past its policy. It gives monitoring something to act on: the command exits
// non-zero without anyone having to parse its output.
var ErrOverdue = fmt.Errorf("one or more secrets are past their max_age")

// NewStatusCmd creates the status subcommand.
func NewStatusCmd() *cobra.Command {
	var passphrase string
	var overdueOnly bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show secret rotation status",
		Long: `Status shows every managed secret with the age of its current value, the
policy it has to meet, and whether it meets it.

With --overdue it prints only the secrets that need attention and exits
non-zero when there are any, which makes it usable directly as a monitoring
check or a CI gate.

Ages come from the encrypted rotation history, so a master passphrase is
required to evaluate a max_age policy.`,
		// The only error this command returns is ErrOverdue, which main
		// turns into its own exit code. Letting cobra print it as
		// "Error: ..." would make a successful check look like a crash.
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, passphrase, overdueOnly)
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for history decryption")
	cmd.Flags().BoolVar(&overdueOnly, "overdue", false,
		"list only secrets past their max_age, and exit non-zero if any")

	return cmd
}

func runStatus(cmd *cobra.Command, passphrase string, overdueOnly bool) error {
	out := cmd.OutOrStdout()

	secrets := AppConfig.Secrets()
	if len(secrets) == 0 {
		// status is read-only, so guide the user instead of failing.
		fmt.Fprintln(out, "No secrets configured.")
		if err := AppConfig.RequireSecrets(); err != nil {
			fmt.Fprintf(out, "\n%v\n", err)
		}
		return nil
	}

	report := application.NewCheckCompliance(openHistory(passphrase), application.SystemClock()).
		Execute(cmd.Context(), secrets)

	if !report.HistoryReadable && report.Reason != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", report.Reason)
	}

	rows := report.Standings
	if overdueOnly {
		rows = report.NeedsAttention()
		if len(rows) == 0 {
			fmt.Fprintln(out, "All secrets are within their max_age.")
			return nil
		}
	}

	printStandings(out, rows)

	if overdueOnly {
		fmt.Fprintf(out, "\n%d secret(s) need attention\n", len(rows))
		return ErrOverdue
	}

	fmt.Fprintf(out, "\n%d secrets configured", len(secrets))
	if attention := report.NeedsAttention(); len(attention) > 0 {
		fmt.Fprintf(out, ", %d past their max_age (see 'rotator status --overdue')", len(attention))
	}
	fmt.Fprintln(out)
	return nil
}

// printStandings renders the status table.
func printStandings(out io.Writer, rows []domain.Standing) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tAGE\tMAX AGE\tSTATE\tSCHEDULE\tNEXT ROTATION")

	parser := scheduling.Parser()
	now := time.Now()

	for _, st := range rows {
		secret := st.Secret

		age := "never"
		if st.HasRotated {
			age = formatDuration(st.Age)
		}

		state := string(st.Compliance)
		if st.Compliance == domain.ComplianceDue {
			state = fmt.Sprintf("overdue by %s", formatDuration(st.OverdueBy))
		}

		schedule := "none"
		next := "-"
		if secret.Schedule().IsSet() {
			schedule = secret.Schedule().String()
			// The same parser the daemon uses, so status never shows a next
			// run the scheduler would reject.
			if parsed, err := parser.Parse(schedule); err == nil {
				next = parsed.Next(now).Format("2006-01-02 15:04")
			} else {
				next = "invalid expression"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			secret.Name(), secret.Kind(), age, secret.MaxAge(), state, schedule, next)
	}
	w.Flush()
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
