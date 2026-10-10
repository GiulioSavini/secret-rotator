package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/container"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/notifier"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/scheduling"
)

// NewDaemonCmd creates the daemon subcommand that runs scheduled rotations.
func NewDaemonCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon executing scheduled rotations",
		Long: `Run in the foreground, executing cron-scheduled secret rotations.

Schedules come from secrets[].schedule in rotator.yml and from container labels
(com.secret-rotator.schedule, or com.secret-rotator.<name>.schedule).

Rotations of the same secret never overlap, and webhook notifications are sent
on every outcome. Stops gracefully on SIGINT or SIGTERM.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDaemon(cmd, passphrase)
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for history encryption")

	return cmd
}

func runDaemon(cmd *cobra.Command, passphrase string) error {
	if err := AppConfig.RequireSecrets(); err != nil {
		return err
	}

	rt, err := newRuntime(cmd, runtimeOptions{
		needsDocker: true,
		passphrase:  resolvePassphrase(passphrase),
	})
	if err != nil {
		return err
	}
	defer rt.close()

	rotate := rt.rotateUseCase()

	// Dependency ordering is resolved per run rather than once at startup, so
	// a Compose file edited while the daemon is up takes effect.
	rotateFn := func(ctx context.Context, secret *domain.Secret) error {
		ordered, err := applyDependencyOrder(cmd, AppConfig, secret)
		if err != nil {
			return err
		}
		return rotate.Execute(ctx, ordered)
	}

	dispatcher := notifier.NewDispatcher(
		notifier.NewNotifiersFromConfig(AppConfig.Notifications)...)

	sched := scheduling.NewScheduler(rotateFn, dispatcher)
	secrets := AppConfig.Secrets()

	scheduled, err := sched.LoadFromConfig(secrets)
	if err != nil {
		return fmt.Errorf("loading schedules from %s: %w", AppConfig.Path, err)
	}

	labels, err := readScheduleLabels(cmd)
	if err != nil {
		log.Printf("[daemon] warning: could not read container labels: %v", err)
	} else if len(labels) > 0 {
		fromLabels, err := sched.LoadFromLabels(labels, secrets)
		if err != nil {
			return fmt.Errorf("loading schedules from container labels: %w", err)
		}
		scheduled += fromLabels
	}

	// A max_age policy is a second, independent reason to rotate, so a secret
	// with only max_age and no cron expression is a valid configuration.
	withPolicy := 0
	for _, secret := range secrets {
		if secret.MaxAge().IsSet() {
			withPolicy++
		}
	}

	if scheduled == 0 && withPolicy == 0 {
		return fmt.Errorf(
			"nothing to do: add a 'schedule:' or a 'max_age:' to a secret in %s, "+
				"or a com.secret-rotator.schedule label to a container",
			AppConfig.Path)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if withPolicy > 0 {
		if rt.history == nil {
			return fmt.Errorf(
				"%d secret(s) define max_age, but ages come from the encrypted rotation history: "+
					"set ROTATOR_MASTER_KEY (or pass --passphrase) so the policy can be evaluated",
				withPolicy)
		}

		compliance := rt.complianceUseCase()
		due := func(ctx context.Context) ([]*domain.Secret, error) {
			report := compliance.Execute(ctx, secrets)
			if !report.HistoryReadable {
				return nil, errors.New(report.Reason)
			}
			return report.DueForRotation(), nil
		}

		policy := scheduling.NewPolicyRunner(due, sched, policyInterval())
		go policy.Run(ctx)
	}

	sched.Start()
	log.Printf("[daemon] started with %d scheduled rotation(s) and %d max_age policy/policies",
		scheduled, withPolicy)

	<-ctx.Done()
	log.Printf("[daemon] shutdown signal received, waiting for running rotations...")

	sched.Stop()
	log.Printf("[daemon] stopped")

	return nil
}

// policyInterval returns how often max_age policies are re-evaluated.
// ROTATOR_POLICY_INTERVAL overrides the default, mainly so the behaviour can
// be exercised without waiting an hour.
func policyInterval() time.Duration {
	raw := os.Getenv("ROTATOR_POLICY_INTERVAL")
	if raw == "" {
		return scheduling.DefaultPolicyInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("[daemon] ignoring invalid ROTATOR_POLICY_INTERVAL %q", raw)
		return scheduling.DefaultPolicyInterval
	}
	return d
}

// readScheduleLabels opens its own Docker connection, because label discovery
// is optional: a failure here degrades the daemon to config-only schedules
// rather than stopping it.
func readScheduleLabels(cmd *cobra.Command) ([]container.ScheduleLabel, error) {
	manager, err := container.NewSDKClient()
	if err != nil {
		return nil, err
	}
	defer manager.Close()

	return container.ReadScheduleLabels(cmd.Context(), manager)
}
