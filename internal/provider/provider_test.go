package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider is a minimal Provider implementation for registry tests.
type stubProvider struct {
	name string
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Rotate(_ context.Context, _ ProviderConfig, _ string) (*Result, error) {
	return &Result{}, nil
}
func (s *stubProvider) Verify(_ context.Context, _ ProviderConfig, _ string) error  { return nil }
func (s *stubProvider) Rollback(_ context.Context, _ ProviderConfig, _ string) error { return nil }

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	stub := &stubProvider{name: "generic"}
	r.Register(stub)

	got, err := r.Get("generic")
	require.NoError(t, err)
	assert.Equal(t, "generic", got.Name())
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider: unknown")
}

func TestRegistryRegisterOverwrite(t *testing.T) {
	r := NewRegistry()
	stub1 := &stubProvider{name: "generic"}
	stub2 := &stubProvider{name: "generic"}

	r.Register(stub1)
	r.Register(stub2) // should overwrite, not panic

	got, err := r.Get("generic")
	require.NoError(t, err)
	assert.Same(t, stub2, got)
}
