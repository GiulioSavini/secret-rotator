// Package history provides an encrypted audit log for secret rotation events.
package history

import "time"

// HistoryEntry represents a single rotation event in the audit log.
type HistoryEntry struct {
	SecretName string    `json:"secret_name"`
	RotatedAt  time.Time `json:"rotated_at"`
	OldValue   string    `json:"old_value"`
	NewHash    string    `json:"new_hash"`
	Status     string    `json:"status"`
	Details    string    `json:"details"`
}

// HistoryFile is the on-disk format for the encrypted history store.
type HistoryFile struct {
	Version int              `json:"version"`
	Salt    string           `json:"salt"`    // base64-encoded salt for key derivation
	Entries []EncryptedEntry `json:"entries"`
}

// EncryptedEntry holds a single encrypted history entry with its creation timestamp.
type EncryptedEntry struct {
	Data      string `json:"data"`       // base64-encoded encrypted data
	CreatedAt string `json:"created_at"` // ISO 8601 timestamp
}
