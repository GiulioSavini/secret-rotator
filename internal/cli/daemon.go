package cli

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/engine"
	"github.com/giulio/secret-rotator/internal/notify"
	"github.com/giulio/secret-rotator/internal/scheduler"
	"github.com/spf13/cobra"
)

// NewDaemonCmd creates the daemon subcommand that runs scheduled rotations.
func NewDaemonCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon executing scheduled rotations",
		Long: `Run in foreground as a daemon, executing cron-scheduled secret rotations.

Schedules are loaded from the rotator.yml configuration file (secrets[].schedule)
and from Docker container labels (com.secret-rotator.schedule or
com.secret-rotator.<name>.schedule).

The daemon sends webhook notifications on rotation success or failure, and
prevents concurrent rotation of the same secret.

Stops gracefully on SIGINT or SIGTERM.`,
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

	// Check that at least one secret has a schedule
	hasSchedule := false
	for _, s := range AppConfig.Secrets {
		if s.Schedule != "" {
			hasSchedule = true
			break
		}
	}

	registry := newRegistry()

	// Create Docker manager
	dockerMgr, err := docker.NewSDKClient()
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer dockerMgr.Close()

	histStore := openHistoryStore(cmd, passphrase)

	dryRun := dryRunFlag

	// Create notifiers from config
	notifiers := notify.NewNotifiersFromConfig(AppConfig.Notifications)
	dispatcher := notify.NewDispatcher(notifiers...)

	// Create rotation function that matches scheduler's expected signature
	rotateFn := func(ctx context.Context, secretCfg config.SecretConfig) error {
		prov, err := registry.Get(secretCfg.Type)
		if err != nil {
			return fmt.Errorf("resolving provider for type %q: %w", secretCfg.Type, err)
		}
		if err := applyDependencyOrder(cmd, AppConfig, &secretCfg); err != nil {
			return err
		}
		eng := engine.NewEngine(prov, dockerMgr, histStore, defaultStepTimeout, dryRun)
		return eng.Execute(ctx, secretCfg)
	}

	// Create scheduler
	sched := scheduler.NewScheduler(rotateFn, dispatcher)

	// Load schedules from config
	if err := sched.LoadFromConfig(AppConfig.Secrets); err != nil {
		return fmt.Errorf("loading config schedules: %w", err)
	}

	// Read Docker labels and load label schedules
	labels, err := docker.ReadScheduleLabels(cmd.Context(), dockerMgr)
	if err != nil {
		log.Printf("[daemon] warning: could not read Docker labels: %v", err)
		// Non-fatal: continue without label schedules
	} else if len(labels) > 0 {
		hasSchedule = true
		if err := sched.LoadFromLabels(labels, AppConfig.Secrets); err != nil {
			return fmt.Errorf("loading label schedules: %w", err)
		}
	}

	if !hasSchedule {
		return fmt.Errorf("no schedules found: add schedule fields to secrets in config or use Docker labels")
	}

	// Start the scheduler
	sched.Start()
	log.Printf("[daemon] scheduler started, waiting for cron triggers...")

	// Wait for shutdown signal
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Printf("[daemon] shutdown signal received, stopping scheduler...")

	sched.Stop()
	log.Printf("[daemon] scheduler stopped")

	return nil
}
