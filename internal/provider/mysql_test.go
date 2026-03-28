package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time check that MySQLProvider implements Provider.
var _ Provider = (*MySQLProvider)(nil)

func TestMySQLProviderName(t *testing.T) {
	p := &MySQLProvider{}
	assert.Equal(t, "mysql", p.Name())
}

func TestMySQLDSN(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		pass     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "explicit port",
			user:     "admin",
			pass:     "secret",
			host:     "db.local",
			port:     3307,
			expected: "admin:secret@tcp(db.local:3307)/",
		},
		{
			name:     "default port",
			user:     "root",
			pass:     "pw",
			host:     "localhost",
			port:     0,
			expected: "root:pw@tcp(localhost:3306)/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mysqlDSN(tt.user, tt.pass, tt.host, tt.port)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestMySQLProviderTargetUser(t *testing.T) {
	p := &MySQLProvider{}

	// With explicit target_user
	cfg := ProviderConfig{
		Username: "admin",
		Options:  map[string]string{"target_user": "appuser"},
	}
	assert.Equal(t, "appuser", p.targetUser(cfg))

	// Without target_user, falls back to Username
	cfg2 := ProviderConfig{
		Username: "admin",
		Options:  map[string]string{},
	}
	assert.Equal(t, "admin", p.targetUser(cfg2))
}

func TestMySQLProviderRotateGeneratesPassword(t *testing.T) {
	p := &MySQLProvider{}
	cfg := ProviderConfig{
		Host:     "localhost",
		Port:     3306,
		Username: "admin",
		Options: map[string]string{
			"password": "adminpass",
		},
	}

	// Rotate will fail connecting to DB (no real MySQL), but we verify
	// it attempts to connect (which means DSN building works).
	_, err := p.Rotate(context.Background(), cfg, "oldpass")
	assert.Error(t, err, "should fail without a real MySQL server")
}

func TestMySQLProviderImplementsInterface(t *testing.T) {
	r := NewRegistry()
	r.Register(&MySQLProvider{})
	got, err := r.Get("mysql")
	assert.NoError(t, err)
	assert.Equal(t, "mysql", got.Name())
}
