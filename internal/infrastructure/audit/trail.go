package audit

import (
	"context"

	"github.com/giulio/secret-rotator/internal/domain"
)

// Trail is the encrypted implementation of domain.AuditTrail.
type Trail struct {
	store *Store
	// keepPreviousValue controls whether the replaced credential is written
	// into the entry. The store is encrypted, but an entry holding every
	// password the system has ever used is a credential archive, so this is a
	// deliberate switch rather than an accident of the data model.
	keepPreviousValue bool
}

// NewTrail builds an audit trail over an encrypted history file.
func NewTrail(store *Store, keepPreviousValue bool) *Trail {
	return &Trail{store: store, keepPreviousValue: keepPreviousValue}
}

// Record appends a rotation outcome to the encrypted log.
func (t *Trail) Record(_ context.Context, record domain.Record) error {
	entry := HistoryEntry{
		SecretName: record.SecretName.String(),
		RotatedAt:  record.FinishedAt,
		Status:     string(record.Outcome),
		LastStep:   record.LastStep.String(),
		Details:    record.Details,
	}
	if t.keepPreviousValue {
		// Expose() is called here and nowhere else in this package: the one
		// place the cleartext is deliberately persisted.
		entry.OldValue = record.Previous.Expose()
	}
	return t.store.Append(entry)
}

var _ domain.AuditTrail = (*Trail)(nil)
