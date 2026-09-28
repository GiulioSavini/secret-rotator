package domain

import "errors"

// Sentinel errors the application layer and adapters can match on, instead of
// comparing error strings.
var (
	// ErrInvalidSecret reports a secret definition that violates an invariant.
	ErrInvalidSecret = errors.New("invalid secret definition")

	// ErrUnknownKind reports a secret type with no registered rotator.
	ErrUnknownKind = errors.New("unknown secret type")

	// ErrSecretKeyMissing reports that the env key is absent from the file
	// that is supposed to hold it.
	ErrSecretKeyMissing = errors.New("secret key not present in env file")

	// ErrRotationFailed wraps any failure of the rotation pipeline.
	ErrRotationFailed = errors.New("rotation failed")

	// ErrRollbackFailed reports that the compensation for a failed rotation
	// did not fully succeed, so the system is in a mixed state.
	ErrRollbackFailed = errors.New("rollback failed")
)
