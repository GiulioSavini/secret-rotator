package engine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteRestartsResolvedComposeContainers(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")

	mockDocker := &mockDockerManager{containers: []docker.Container{
		{Name: "myproj-db-1", Labels: map[string]string{docker.ComposeServiceLabel: "db"}},
		{Name: "myproj-app-1", Labels: map[string]string{docker.ComposeServiceLabel: "app"}},
	}}

	cfg := config.SecretConfig{
		Name:       "db-password",
		Type:       "generic",
		EnvKey:     "DB_PASSWORD",
		EnvFile:    envPath,
		Containers: []string{"db", "app"},
	}

	eng := NewEngine(&mockProvider{name: "generic"}, mockDocker, nil, time.Second, false)
	require.NoError(t, eng.Execute(context.Background(), cfg))

	assert.Equal(t, []string{"myproj-db-1", "myproj-app-1"}, mockDocker.restartCalls,
		"Compose service names must be mapped onto real container names")
}

func TestExecuteFailsEarlyOnUnknownContainer(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")

	mockDocker := &mockDockerManager{containers: []docker.Container{{Name: "myproj-db-1"}}}
	prov := &mockProvider{name: "generic"}

	cfg := config.SecretConfig{
		Name:       "db-password",
		Type:       "generic",
		EnvKey:     "DB_PASSWORD",
		EnvFile:    envPath,
		Containers: []string{"does-not-exist"},
	}

	eng := NewEngine(prov, mockDocker, nil, time.Second, false)
	err := eng.Execute(context.Background(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
	assert.Zero(t, prov.rotateCalls, "nothing should be mutated when a container cannot be resolved")

	body, readErr := os.ReadFile(envPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "DB_PASSWORD=old-secret", ".env must be untouched")
}

func TestAdminPasswordComesFromEnvFile(t *testing.T) {
	// A daemon keeps the environment it started with, so after the first
	// rotation only the .env holds the current admin password.
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "MYSQL_ROOT_PASSWORD=current-admin-pw\n")

	var seen string
	prov := &mockProvider{
		name: "mysql",
		rotateFunc: func(_ context.Context, cfg provider.ProviderConfig, cur string) (*provider.Result, error) {
			seen = cfg.Options["password"]
			return &provider.Result{OldSecret: cur, NewSecret: "brand-new-pw"}, nil
		},
	}

	cfg := config.SecretConfig{
		Name:    "mysql_root_password",
		Type:    "mysql",
		EnvKey:  "MYSQL_ROOT_PASSWORD",
		EnvFile: envPath,
		Provider: map[string]string{
			"host":         "db",
			"username":     "root",
			"password_env": "MYSQL_ROOT_PASSWORD",
		},
	}

	eng := NewEngine(prov, &mockDockerManager{}, nil, time.Second, false)
	require.NoError(t, eng.Execute(context.Background(), cfg))

	assert.Equal(t, "current-admin-pw", seen)
}

func TestExplicitAdminPasswordIsNotOverridden(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "MYSQL_ROOT_PASSWORD=from-env-file\n")

	var seen string
	prov := &mockProvider{
		name: "mysql",
		rotateFunc: func(_ context.Context, cfg provider.ProviderConfig, cur string) (*provider.Result, error) {
			seen = cfg.Options["password"]
			return &provider.Result{OldSecret: cur, NewSecret: "brand-new-pw"}, nil
		},
	}

	cfg := config.SecretConfig{
		Name:    "mysql_root_password",
		Type:    "mysql",
		EnvKey:  "MYSQL_ROOT_PASSWORD",
		EnvFile: envPath,
		Provider: map[string]string{
			"host":         "db",
			"username":     "root",
			"password":     "explicit-pw",
			"password_env": "MYSQL_ROOT_PASSWORD",
		},
	}

	eng := NewEngine(prov, &mockDockerManager{}, nil, time.Second, false)
	require.NoError(t, eng.Execute(context.Background(), cfg))

	assert.Equal(t, "explicit-pw", seen)
}

func TestProviderOptionsAreNotSharedWithConfig(t *testing.T) {
	// The resolved admin password must not leak back into the configuration,
	// which the daemon reuses on every scheduled run.
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "MYSQL_ROOT_PASSWORD=from-env-file\n")

	cfg := config.SecretConfig{
		Name:    "mysql_root_password",
		Type:    "mysql",
		EnvKey:  "MYSQL_ROOT_PASSWORD",
		EnvFile: envPath,
		Provider: map[string]string{
			"host":         "db",
			"username":     "root",
			"password_env": "MYSQL_ROOT_PASSWORD",
		},
	}

	eng := NewEngine(&mockProvider{name: "mysql"}, &mockDockerManager{}, nil, time.Second, false)
	require.NoError(t, eng.Execute(context.Background(), cfg))

	assert.NotContains(t, cfg.Provider, "password")
}
