package provider

import (
	"context"
	"strconv"
)

// GenericProvider handles rotation for secrets that have no external service
// (e.g., API keys, JWT secrets). It generates a new password without any
// database interaction; rollback and verify are no-ops because the engine
// handles .env file restore and container restarts.
type GenericProvider struct{}

// Name returns "generic".
func (g *GenericProvider) Name() string { return "generic" }

// Rotate generates a new random password, returning the old and new secrets.
// If cfg.Options["length"] is set, it is parsed as the byte length for password
// generation; otherwise DefaultPasswordLength is used.
func (g *GenericProvider) Rotate(_ context.Context, cfg ProviderConfig, currentSecret string) (*Result, error) {
	length := 0
	if cfg.Options != nil {
		if ls, ok := cfg.Options["length"]; ok {
			if n, err := strconv.Atoi(ls); err == nil {
				length = n
			}
		}
	}

	newSecret, err := GeneratePassword(length)
	if err != nil {
		return nil, err
	}

	return &Result{
		OldSecret: currentSecret,
		NewSecret: newSecret,
	}, nil
}

// Verify is a no-op for generic secrets (no external service to verify against).
func (g *GenericProvider) Verify(_ context.Context, _ ProviderConfig, _ string) error {
	return nil
}

// Rollback is a no-op for generic secrets (the engine handles .env file
// restore and container restarts).
func (g *GenericProvider) Rollback(_ context.Context, _ ProviderConfig, _ string) error {
	return nil
}
