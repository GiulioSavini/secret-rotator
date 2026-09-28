package credential

import (
	"fmt"

	"github.com/giulio/secret-rotator/internal/domain"
)

// Registry resolves a secret kind to the rotator that handles it.
type Registry struct {
	rotators map[domain.Kind]domain.CredentialRotator
}

// NewRegistry returns a registry holding every built-in rotator.
func NewRegistry() *Registry {
	r := &Registry{rotators: make(map[domain.Kind]domain.CredentialRotator)}
	r.Register(&GenericRotator{})
	r.Register(&MySQLRotator{})
	r.Register(&PostgresRotator{})
	r.Register(&RedisRotator{})
	return r
}

// Register adds or replaces the rotator for a kind.
func (r *Registry) Register(rotator domain.CredentialRotator) {
	r.rotators[rotator.Kind()] = rotator
}

// For returns the rotator for a kind.
func (r *Registry) For(kind domain.Kind) (domain.CredentialRotator, error) {
	rotator, ok := r.rotators[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %s (valid: %s)", domain.ErrUnknownKind, kind, domain.KindList())
	}
	return rotator, nil
}

var _ domain.RotatorRegistry = (*Registry)(nil)
