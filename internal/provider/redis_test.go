package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time check that RedisProvider implements Provider.
var _ Provider = (*RedisProvider)(nil)

func TestRedisProviderName(t *testing.T) {
	p := &RedisProvider{}
	assert.Equal(t, "redis", p.Name())
}

func TestRedisAddr(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "explicit port",
			host:     "redis.local",
			port:     6380,
			expected: "redis.local:6380",
		},
		{
			name:     "default port",
			host:     "localhost",
			port:     0,
			expected: "localhost:6379",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redisAddr(tt.host, tt.port)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRedisProviderRotateFailsWithoutServer(t *testing.T) {
	p := &RedisProvider{}
	cfg := ProviderConfig{
		Host: "localhost",
		Port: 6379,
	}

	_, err := p.Rotate(context.Background(), cfg, "oldpass")
	assert.Error(t, err, "should fail without a real Redis server")
}

func TestRedisProviderImplementsInterface(t *testing.T) {
	r := NewRegistry()
	r.Register(&RedisProvider{})
	got, err := r.Get("redis")
	assert.NoError(t, err)
	assert.Equal(t, "redis", got.Name())
}
