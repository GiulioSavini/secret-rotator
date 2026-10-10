package application

import (
	"context"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

// CheckCompliance evaluates every secret against its own max-age policy.
//
// It is the read side of policy-driven rotation: `rotator status` renders it,
// and the daemon uses it to decide what to rotate without being told a cron
// expression.
type CheckCompliance struct {
	history domain.RotationHistory
	clock   domain.Clock
}

// NewCheckCompliance builds the use case. A nil history means the rotation
// log is unavailable, which yields ComplianceUnknown rather than a clean
// result: a policy that cannot be evaluated has not been met.
func NewCheckCompliance(history domain.RotationHistory, clock domain.Clock) *CheckCompliance {
	if clock == nil {
		clock = systemClock{}
	}
	return &CheckCompliance{history: history, clock: clock}
}

// ComplianceReport is the standing of every secret, plus whether the history
// could be read at all.
type ComplianceReport struct {
	Standings []domain.Standing
	// HistoryReadable is false when there is no rotation log to consult,
	// typically because no master passphrase is configured.
	HistoryReadable bool
	// Reason explains an unreadable history, for the caller to surface.
	Reason string
}

// Execute evaluates the given secrets.
func (uc *CheckCompliance) Execute(ctx context.Context, secrets []*domain.Secret) *ComplianceReport {
	now := uc.clock.Now()

	report := &ComplianceReport{HistoryReadable: uc.history != nil}
	if uc.history == nil {
		report.Reason = "no rotation history is available (set ROTATOR_MASTER_KEY to record and read it)"
	}

	for _, secret := range secrets {
		var (
			last    = now
			present bool
			ok      = report.HistoryReadable
		)

		if uc.history != nil {
			t, found, err := uc.history.LastSuccess(ctx, secret.Name())
			if err != nil {
				// One unreadable entry must not be reported as "never
				// rotated", which would invite a needless rotation.
				ok = false
				if report.Reason == "" {
					report.Reason = "the rotation history could not be read: " + err.Error()
				}
			} else {
				last, present = t, found
			}
		}

		report.Standings = append(report.Standings, secret.Evaluate(last, present, ok, now))
	}

	return report
}

// NeedsAttention returns the standings that should make a check fail.
func (r *ComplianceReport) NeedsAttention() []domain.Standing {
	var out []domain.Standing
	for _, s := range r.Standings {
		if s.Compliance.NeedsAttention() {
			out = append(out, s)
		}
	}
	return out
}

// DueForRotation returns the secrets a policy requires rotating now.
func (r *ComplianceReport) DueForRotation() []*domain.Secret {
	var out []*domain.Secret
	for _, s := range r.Standings {
		if s.DueForRotation() {
			out = append(out, s.Secret)
		}
	}
	return out
}
