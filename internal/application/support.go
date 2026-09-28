package application

import (
	"context"
	"time"

	"github.com/giulio/secret-rotator/internal/domain"
)

// systemClock is the default Clock, reading the wall clock in UTC so that
// audit timestamps are comparable across hosts.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock returns the default clock implementation.
func SystemClock() domain.Clock { return systemClock{} }

// noopAudit discards records. Used when no passphrase is available, so that a
// rotation still runs without an audit trail.
type noopAudit struct{}

func (noopAudit) Record(context.Context, domain.Record) error { return nil }

// NoopAudit returns an audit trail that records nothing.
func NoopAudit() domain.AuditTrail { return noopAudit{} }

// noopReporter discards progress messages.
type noopReporter struct{}

func (noopReporter) Step(domain.Step, string) {}
func (noopReporter) Warn(string)              {}

// NoopReporter returns a reporter that says nothing.
func NoopReporter() domain.Reporter { return noopReporter{} }

// noopFleet stands in when no container runtime is available, for example in a
// dry run. Resolve echoes the references back and Restart does nothing.
type noopFleet struct{}

func (noopFleet) Resolve(_ context.Context, refs []domain.ContainerRef) ([]domain.ContainerRef, error) {
	return refs, nil
}
func (noopFleet) Restart(context.Context, []domain.ContainerRef) error { return nil }

// NoopFleet returns a container fleet that touches nothing.
func NoopFleet() domain.ContainerFleet { return noopFleet{} }
