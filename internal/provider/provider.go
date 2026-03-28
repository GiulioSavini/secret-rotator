package provider

import "context"

// Provider defines the contract for secret rotation providers.
type Provider interface {
	// Name returns the provider type identifier (e.g. "generic", "mysql").
	Name() string

	// Rotate generates a new secret, applying it to the target service if applicable.
	Rotate(ctx context.Context, cfg ProviderConfig, currentSecret string) (*Result, error)

	// Verify checks that a secret is valid for the target service.
	Verify(ctx context.Context, cfg ProviderConfig, secret string) error

	// Rollback reverts the target service to the old secret.
	Rollback(ctx context.Context, cfg ProviderConfig, oldSecret string) error
}

// ProviderConfig holds connection details for provider operations.
type ProviderConfig struct {
	Host     string
	Port     int
	Username string
	Database string
	Options  map[string]string
}

// Result contains the outcome of a rotation operation.
type Result struct {
	OldSecret string
	NewSecret string
}
