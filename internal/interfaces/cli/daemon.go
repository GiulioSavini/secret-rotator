package cli

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/container"
	"github.com/giulio/secret-rotator/internal/infrastructure/notifier"
	"github.com/giulio/secret-rotator/internal/infrastructure/scheduling"
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

	if scheduled == 0 {
		return fmt.Errorf(
			"no schedules found: add a 'schedule:' to a secret in %s, or a com.secret-rotator.schedule label to a container",
			AppConfig.Path)
	}

	sched.Start()
	log.Printf("[daemon] started with %d scheduled rotation(s)", scheduled)

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Printf("[daemon] shutdown signal received, waiting for running rotations...")

	sched.Stop()
	log.Printf("[daemon] stopped")

	return nil
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
