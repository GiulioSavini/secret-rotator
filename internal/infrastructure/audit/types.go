// Package history provides an encrypted audit log for secret rotation events.
package audit

import "time"

// HistoryEntry represents a single rotation event in the audit log.
type HistoryEntry struct {
	SecretName string    `json:"secret_name"`
	RotatedAt  time.Time `json:"rotated_at"`
	// OldValue is the replaced credential in cleartext. It is only populated
	// when the trail is configured to keep it; see audit.NewTrail.
	OldValue string `json:"old_value,omitempty"`
	Status   string `json:"status"`
	// LastStep is how far the rotation got, which is what an operator needs
	// after a failure.
	LastStep string `json:"last_step,omitempty"`
	Details  string `json:"details,omitempty"`
}

// HistoryFile is the on-disk format for the encrypted history store.
type HistoryFile struct {
	Version int              `json:"version"`
	Salt    string           `json:"salt"` // base64-encoded salt for key derivation
	Entries []EncryptedEntry `json:"entries"`
}

// EncryptedEntry holds a single encrypted history entry with its creation timestamp.
type EncryptedEntry struct {
	Data      string `json:"data"`       // base64-encoded encrypted data
	CreatedAt string `json:"created_at"` // ISO 8601 timestamp
}
