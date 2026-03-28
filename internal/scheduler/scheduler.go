package scheduler

import (
	"context"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/notify"
	"github.com/robfig/cron/v3"
)

// Scheduler runs rotation jobs on cron schedules.
type Scheduler struct {
	cron       *cron.Cron
	rotateFunc func(ctx context.Context, secretCfg config.SecretConfig) error
	dispatcher *notify.Dispatcher
}

// NewScheduler creates a Scheduler with the given rotation function and dispatcher.
func NewScheduler(_ func(ctx context.Context, cfg config.SecretConfig) error, _ *notify.Dispatcher) *Scheduler {
	return &Scheduler{}
}

// AddJob registers a cron job for the given secret config.
func (s *Scheduler) AddJob(_ config.SecretConfig, _ string) error {
	return nil
}

// LoadFromConfig loads cron jobs from secret configurations.
func (s *Scheduler) LoadFromConfig(_ []config.SecretConfig) error {
	return nil
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() {}

// Stop halts the cron scheduler and waits for running jobs to complete.
func (s *Scheduler) Stop() {}

// jobFunc returns the closure that runs for a scheduled job.
func (s *Scheduler) jobFunc(_ config.SecretConfig) func() {
	return func() {}
}
