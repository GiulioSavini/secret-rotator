package scheduling

import (
	"context"
	"log"
	"time"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

// DefaultPolicyInterval is how often max_age policies are re-evaluated.
//
// Hourly is deliberate: a policy is expressed in days or weeks, so checking
// more often buys nothing, and each check decrypts the rotation log.
const DefaultPolicyInterval = time.Hour

// DueFunc returns the secrets a policy requires rotating now.
type DueFunc func(ctx context.Context) ([]*domain.Secret, error)

// PolicyRunner rotates secrets that have exceeded their max_age.
//
// It complements the cron scheduler rather than replacing it: cron answers
// "every first of the month", a policy answers "never older than 90 days".
// The second survives the daemon being down over the date the first would
// have fired, which is the point of expressing a requirement as an age.
//
// Rotations go through the scheduler's executor, so a policy check and a cron
// trigger share the per-secret lock and report the same way.
type PolicyRunner struct {
	due      DueFunc
	runner   *Scheduler
	interval time.Duration
}

// NewPolicyRunner builds the runner. A zero interval selects the default.
func NewPolicyRunner(due DueFunc, runner *Scheduler, interval time.Duration) *PolicyRunner {
	if interval <= 0 {
		interval = DefaultPolicyInterval
	}
	return &PolicyRunner{due: due, runner: runner, interval: interval}
}

// Run evaluates policies until the context is cancelled.
//
// The first evaluation happens immediately, so a daemon started after a long
// outage catches up instead of waiting out the interval.
func (p *PolicyRunner) Run(ctx context.Context) {
	p.evaluate(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.evaluate(ctx)
		}
	}
}

// evaluate rotates whatever is currently due.
func (p *PolicyRunner) evaluate(ctx context.Context) {
	secrets, err := p.due(ctx)
	if err != nil {
		log.Printf("[policy] could not evaluate max_age policies: %v", err)
		return
	}

	for _, secret := range secrets {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[policy] %s is past its max_age of %s", secret.Name(), secret.MaxAge())
		p.runner.Execute(ctx, secret, "policy")
	}
}
