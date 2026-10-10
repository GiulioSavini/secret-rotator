package audit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

// History answers when each secret last rotated successfully, reading the
// encrypted log.
//
// Decrypting the log costs one Argon2id derivation plus one AES-GCM open per
// entry, so the answers are cached: a daemon evaluating policies every hour
// should not re-derive a key every time. Record invalidates the cache, which
// is why the writer is held here too.
type History struct {
	store *Store
	trail *Trail

	mu     sync.Mutex
	loaded bool
	last   map[string]time.Time
}

// NewHistory wraps a store as a readable rotation history.
func NewHistory(store *Store, trail *Trail) *History {
	return &History{store: store, trail: trail}
}

// LastSuccess returns the most recent successful rotation of a secret.
func (h *History) LastSuccess(_ context.Context, name domain.SecretName) (time.Time, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.loaded {
		if err := h.load(); err != nil {
			return time.Time{}, false, err
		}
	}

	t, ok := h.last[name.String()]
	return t, ok, nil
}

// Record writes an outcome and drops the cache, so the next policy check sees
// the rotation that just happened.
func (h *History) Record(ctx context.Context, record domain.Record) error {
	err := h.trail.Record(ctx, record)

	h.mu.Lock()
	h.loaded = false
	h.last = nil
	h.mu.Unlock()

	return err
}

// load decrypts the log and indexes the latest success per secret.
func (h *History) load() error {
	entries, err := h.store.List()
	if err != nil {
		return fmt.Errorf("reading the rotation history: %w", err)
	}

	h.last = make(map[string]time.Time, len(entries))
	for _, e := range entries {
		if e.Status != string(domain.OutcomeSuccess) {
			continue
		}
		if prev, ok := h.last[e.SecretName]; !ok || e.RotatedAt.After(prev) {
			h.last[e.SecretName] = e.RotatedAt
		}
	}
	h.loaded = true
	return nil
}

var (
	_ domain.RotationHistory = (*History)(nil)
	_ domain.AuditTrail      = (*History)(nil)
)
