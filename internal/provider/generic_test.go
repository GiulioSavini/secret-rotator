package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericProviderName(t *testing.T) {
	p := &GenericProvider{}
	assert.Equal(t, "generic", p.Name())
}

func TestGenericProviderRotate(t *testing.T) {
	p := &GenericProvider{}
	ctx := context.Background()
	cfg := ProviderConfig{}

	result, err := p.Rotate(ctx, cfg, "oldpass")
	require.NoError(t, err)
	assert.Equal(t, "oldpass", result.OldSecret)
	assert.NotEmpty(t, result.NewSecret)
	assert.NotEqual(t, "oldpass", result.NewSecret)
}

func TestGenericProviderRotateRespectsLength(t *testing.T) {
	p := &GenericProvider{}
	ctx := context.Background()
	cfg := ProviderConfig{
		Options: map[string]string{"length": "16"},
	}

	result, err := p.Rotate(ctx, cfg, "oldpass")
	require.NoError(t, err)
	// 16 bytes -> 22 chars in base64 RawURL encoding (ceil(16*4/3) = 22)
	assert.Len(t, result.NewSecret, 22)
}

func TestGenericProviderRotateDefaultLength(t *testing.T) {
	p := &GenericProvider{}
	ctx := context.Background()
	cfg := ProviderConfig{}

	result, err := p.Rotate(ctx, cfg, "old")
	require.NoError(t, err)
	// DefaultPasswordLength=32 -> 43 chars
	assert.Len(t, result.NewSecret, 43)
}

func TestGenericProviderVerify(t *testing.T) {
	p := &GenericProvider{}
	err := p.Verify(context.Background(), ProviderConfig{}, "any")
	assert.NoError(t, err)
}

func TestGenericProviderRollback(t *testing.T) {
	p := &GenericProvider{}
	err := p.Rollback(context.Background(), ProviderConfig{}, "any")
	assert.NoError(t, err)
}

func TestGenericProviderImplementsInterface(t *testing.T) {
	// Compile-time check that GenericProvider implements Provider.
	var _ Provider = (*GenericProvider)(nil)
}

func TestGenericProviderRegistered(t *testing.T) {
	r := NewRegistry()
	r.Register(&GenericProvider{})

	got, err := r.Get("generic")
	require.NoError(t, err)
	assert.Equal(t, "generic", got.Name())
}
