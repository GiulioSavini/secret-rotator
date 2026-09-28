package domain

import (
	"context"
	"time"
)

// The interfaces below are driven ports: capabilities the domain needs, named
// in the domain's own vocabulary and implemented by adapters under
// internal/infrastructure. They are declared here, next to the code that
// consumes them, so that adding an adapter never requires touching the core.

// PasswordGenerator produces new credentials.
//
// Generation is deliberately separate from application: every rotator used to
// generate its own password, duplicating the length handling four times and
// making it impossible to rotate to a value chosen elsewhere.
type PasswordGenerator interface {
	Generate(length PasswordLength) (Credential, error)
}

// CredentialRotator changes a credential on one type of backing service.
type CredentialRotator interface {
	// Kind is the secret type this rotator handles.
	Kind() Kind
	// Apply makes the service accept next as the target account's credential.
	Apply(ctx context.Context, target Target, next Credential) error
	// Verify proves that the credential works, by authenticating with it.
	Verify(ctx context.Context, target Target, credential Credential) error
	// Restore puts a previous credential back after a failed rotation. It is
	// given the credential currently in force so it can authenticate with
	// whichever of the two the service accepts.
	Restore(ctx context.Context, target Target, previous, current Credential) error
}

// RotatorRegistry resolves a rotator for a secret's kind.
type RotatorRegistry interface {
	For(kind Kind) (CredentialRotator, error)
}

// EnvDocument is a parsed .env file.
type EnvDocument interface {
	// Path is where the document came from.
	Path() FilePath
	// Lookup returns the raw value of a key.
	Lookup(key EnvKey) (string, bool)
	// Keys returns every key in file order.
	Keys() []EnvKey
	// Snapshot returns the file's exact bytes, for compensation.
	Snapshot() []byte
}

// EnvStore reads and writes the .env files that carry secrets.
type EnvStore interface {
	// Open parses a file.
	Open(path FilePath) (EnvDocument, error)
	// Write sets a key to a new value, preserving the file's formatting and
	// permissions, and returns the bytes the file held beforehand.
	Write(path FilePath, key EnvKey, value Credential) (before []byte, err error)
	// Restore rewrites a file with previously captured bytes, preserving its
	// permissions.
	Restore(path FilePath, snapshot []byte) error
}

// ContainerFleet is the container runtime, as the domain sees it.
type ContainerFleet interface {
	// Resolve maps configured references (container names or Compose service
	// names) onto the containers that actually exist.
	Resolve(ctx context.Context, refs []ContainerRef) ([]ContainerRef, error)
	// Restart restarts the given containers in order, waiting for each to be
	// healthy before moving to the next.
	Restart(ctx context.Context, refs []ContainerRef) error
}

// AuditTrail records how rotations ended.
type AuditTrail interface {
	Record(ctx context.Context, record Record) error
}

// Clock supplies the current time, so that rotation timestamps are
// deterministic under test.
type Clock interface {
	Now() time.Time
}

// Reporter receives human-facing progress messages from a use case. It exists
// so the application layer can explain itself without importing a logger or
// writing to stdout directly.
type Reporter interface {
	Step(step Step, message string)
	Warn(message string)
}
