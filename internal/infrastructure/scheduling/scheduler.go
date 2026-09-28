// Package scheduling triggers rotations on a cron schedule.
//
// It is a driving adapter: it decides when a use case runs, never what the use
// case does. It works with domain aggregates, so a schedule read from a
// container label and one read from the configuration file are the same thing
// by the time they reach it.
package scheduling

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/container"
	"github.com/giulio/secret-rotator/internal/infrastructure/notifier"
)

// RotateFunc runs one rotation. It is the use case, injected rather than
// imported, so this package does not depend on the application layer.
type RotateFunc func(ctx context.Context, secret *domain.Secret) error

// JobTimeout bounds a single scheduled rotation, so a hung database cannot
// block the schedule forever.
const JobTimeout = 10 * time.Minute

// Scheduler runs rotations on cron schedules.
type Scheduler struct {
	cron       *cron.Cron
	rotate     RotateFunc
	dispatcher *notifier.Dispatcher
	locks      sync.Map // secret name -> *sync.Mutex
}

// Parser accepts standard 5-field expressions plus descriptors like @daily.
func Parser() cron.Parser {
	return cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

// NewScheduler builds a scheduler around a rotation function.
func NewScheduler(rotate RotateFunc, dispatcher *notifier.Dispatcher) *Scheduler {
	return &Scheduler{
		cron:       cron.New(cron.WithParser(Parser())),
		rotate:     rotate,
		dispatcher: dispatcher,
	}
}

// getLock returns the mutex guarding one secret.
func (s *Scheduler) getLock(name domain.SecretName) *sync.Mutex {
	val, _ := s.locks.LoadOrStore(name.String(), &sync.Mutex{})
	return val.(*sync.Mutex)
}

// jobFunc builds the closure a cron entry runs.
func (s *Scheduler) jobFunc(secret *domain.Secret) func() {
	return func() {
		name := secret.Name()

		// A rotation that overruns its schedule must not overlap with the
		// next tick: two concurrent ALTER USER statements on the same account
		// would race, and the loser would leave .env and the server disagreeing.
		mu := s.getLock(name)
		if !mu.TryLock() {
			log.Printf("[scheduler] skipping %s: a rotation is already in progress", name)
			return
		}
		defer mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), JobTimeout)
		defer cancel()

		err := s.rotate(ctx, secret)

		event := notifier.Event{SecretName: name.String(), Timestamp: time.Now()}
		if err != nil {
			event.Status = string(domain.OutcomeFailed)
			event.Details = err.Error()
			log.Printf("[scheduler] rotation failed for %s: %v", name, err)
		} else {
			event.Status = string(domain.OutcomeSuccess)
			log.Printf("[scheduler] rotation succeeded for %s", name)
		}

		if sendErr := s.dispatcher.Send(ctx, event); sendErr != nil {
			log.Printf("[scheduler] notification error for %s: %v", name, sendErr)
		}
	}
}

// AddJob registers one cron entry.
func (s *Scheduler) AddJob(secret *domain.Secret, expr string) error {
	_, err := s.cron.AddFunc(expr, s.jobFunc(secret))
	return err
}

// LoadFromConfig registers an entry for every secret carrying a schedule.
// It returns how many were registered.
func (s *Scheduler) LoadFromConfig(secrets []*domain.Secret) (int, error) {
	count := 0
	for _, secret := range secrets {
		if !secret.Schedule().IsSet() {
			continue
		}
		if err := s.AddJob(secret, secret.Schedule().String()); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// LoadFromLabels registers entries from container labels. A label without a
// secret name applies to every secret; one naming a secret applies to that
// secret only. Label schedules are added on top of configured ones.
func (s *Scheduler) LoadFromLabels(labels []container.ScheduleLabel, secrets []*domain.Secret) (int, error) {
	count := 0
	for _, label := range labels {
		for _, secret := range secrets {
			if label.SecretName != "" && label.SecretName != secret.Name().String() {
				continue
			}
			if err := s.AddJob(secret, label.CronExpr); err != nil {
				return count, err
			}
			count++
			if label.SecretName != "" {
				break
			}
		}
	}
	return count, nil
}

// Start begins running scheduled jobs.
func (s *Scheduler) Start() { s.cron.Start() }

// Stop halts the scheduler, waiting for running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
