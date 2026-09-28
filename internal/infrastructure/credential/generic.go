package credential

import (
	"context"

	"github.com/giulio/secret-rotator/internal/domain"
)

// GenericRotator handles secrets with no backing service: API keys, JWT
// secrets, session keys. The value only lives in .env files, so applying,
// verifying and restoring are all no-ops -- the env store and the container
// fleet do the actual work.
type GenericRotator struct{}

// Kind returns domain.KindGeneric.
func (GenericRotator) Kind() domain.Kind { return domain.KindGeneric }

// Apply does nothing: there is no service to tell.
func (GenericRotator) Apply(context.Context, domain.Target, domain.Credential) error { return nil }

// Verify does nothing: there is nothing to authenticate against.
func (GenericRotator) Verify(context.Context, domain.Target, domain.Credential) error { return nil }

// Restore does nothing: compensation is limited to the files.
func (GenericRotator) Restore(context.Context, domain.Target, domain.Credential, domain.Credential) error {
	return nil
}

var _ domain.CredentialRotator = GenericRotator{}
