package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time check that PostgresProvider implements Provider.
var _ Provider = (*PostgresProvider)(nil)

func TestPostgresProviderName(t *testing.T) {
	p := &PostgresProvider{}
	assert.Equal(t, "postgres", p.Name())
}

func TestPgConnStr(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		pass     string
		host     string
		port     int
		database string
		expected string
	}{
		{
			name:     "explicit port and database",
			user:     "admin",
			pass:     "secret",
			host:     "db.local",
			port:     5433,
			database: "mydb",
			expected: "postgres://admin:secret@db.local:5433/mydb",
		},
		{
			name:     "default port no database",
			user:     "postgres",
			pass:     "pw",
			host:     "localhost",
			port:     0,
			database: "",
			expected: "postgres://postgres:pw@localhost:5432/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pgConnStr(tt.user, tt.pass, tt.host, tt.port, tt.database)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPostgresProviderTargetUser(t *testing.T) {
	p := &PostgresProvider{}

	cfg := ProviderConfig{
		Username: "admin",
		Options:  map[string]string{"target_user": "appuser"},
	}
	assert.Equal(t, "appuser", p.targetUser(cfg))

	cfg2 := ProviderConfig{
		Username: "admin",
		Options:  map[string]string{},
	}
	assert.Equal(t, "admin", p.targetUser(cfg2))
}

func TestPostgresProviderRotateFailsWithoutServer(t *testing.T) {
	p := &PostgresProvider{}
	cfg := ProviderConfig{
		Host:     "localhost",
		Port:     5432,
		Username: "admin",
		Options: map[string]string{
			"password": "adminpass",
		},
	}

	_, err := p.Rotate(context.Background(), cfg, "oldpass")
	assert.Error(t, err, "should fail without a real PostgreSQL server")
}

func TestPostgresProviderImplementsInterface(t *testing.T) {
	r := NewRegistry()
	r.Register(&PostgresProvider{})
	got, err := r.Get("postgres")
	assert.NoError(t, err)
	assert.Equal(t, "postgres", got.Name())
}
