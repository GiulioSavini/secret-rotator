// Package scheduler provides cron-based scheduling for automated secret rotation.
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/notify"
	"github.com/robfig/cron/v3"
)

// Scheduler manages cron-scheduled rotation jobs.
type Scheduler struct {
	cron       *cron.Cron
	rotateFunc func(ctx context.Context, secretCfg config.SecretConfig) error
	dispatcher *notify.Dispatcher
	locks      sync.Map // map[string]*sync.Mutex per secret name
}

// NewScheduler creates a Scheduler with the given rotation function and notification dispatcher.
// The cron parser supports standard 5-field expressions and descriptors like @daily, @weekly.
func NewScheduler(rotateFn func(ctx context.Context, cfg config.SecretConfig) error, dispatcher *notify.Dispatcher) *Scheduler {
	c := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor,
	)))
	return &Scheduler{
		cron:       c,
		rotateFunc: rotateFn,
		dispatcher: dispatcher,
	}
}

// getLock returns or creates a per-secret mutex.
func (s *Scheduler) getLock(name string) *sync.Mutex {
	val, _ := s.locks.LoadOrStore(name, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// jobFunc returns the closure that runs for a scheduled rotation job.
func (s *Scheduler) jobFunc(secretCfg config.SecretConfig) func() {
	return func() {
		mu := s.getLock(secretCfg.Name)
		if !mu.TryLock() {
			log.Printf("[scheduler] skipping %s: rotation already in progress", secretCfg.Name)
			return
		}
		defer mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := s.rotateFunc(ctx, secretCfg)

		event := notify.Event{
			SecretName: secretCfg.Name,
			Timestamp:  time.Now(),
		}
		if err != nil {
			event.Status = "failed"
			event.Details = err.Error()
			log.Printf("[scheduler] rotation failed for %s: %v", secretCfg.Name, err)
		} else {
			event.Status = "success"
			log.Printf("[scheduler] rotation succeeded for %s", secretCfg.Name)
		}

		if sendErr := s.dispatcher.Send(ctx, event); sendErr != nil {
			log.Printf("[scheduler] notification error for %s: %v", secretCfg.Name, sendErr)
		}
	}
}

// AddJob registers a cron job for the given secret config and cron expression.
// Returns an error if the cron expression is invalid.
func (s *Scheduler) AddJob(secretCfg config.SecretConfig, cronExpr string) error {
	_, err := s.cron.AddFunc(cronExpr, s.jobFunc(secretCfg))
	return err
}

// LoadFromConfig registers cron jobs for all secrets that have a non-empty Schedule field.
func (s *Scheduler) LoadFromConfig(secrets []config.SecretConfig) error {
	for _, sec := range secrets {
		if sec.Schedule == "" {
			continue
		}
		if err := s.AddJob(sec, sec.Schedule); err != nil {
			return err
		}
	}
	return nil
}

// LoadFromLabels registers cron jobs from Docker container label schedules.
// For each label, it finds the matching SecretConfig by name. If SecretName is
// empty (global label), the label's schedule applies to all provided secrets.
// Label schedules override config schedules for the same secret.
func (s *Scheduler) LoadFromLabels(labels []docker.ScheduleLabel, secrets []config.SecretConfig) error {
	for _, lbl := range labels {
		if lbl.SecretName == "" {
			// Global schedule: apply to all secrets
			for _, sec := range secrets {
				if err := s.AddJob(sec, lbl.CronExpr); err != nil {
					return err
				}
			}
		} else {
			// Per-secret schedule
			for _, sec := range secrets {
				if sec.Name == lbl.SecretName {
					if err := s.AddJob(sec, lbl.CronExpr); err != nil {
						return err
					}
					break
				}
			}
		}
	}
	return nil
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop halts the cron scheduler, waiting for any running jobs to complete.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
