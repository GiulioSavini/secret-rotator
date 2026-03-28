package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/history"
	"github.com/giulio/secret-rotator/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Provider ---

type mockProvider struct {
	name         string
	rotateFunc   func(ctx context.Context, cfg provider.ProviderConfig, cur string) (*provider.Result, error)
	verifyFunc   func(ctx context.Context, cfg provider.ProviderConfig, secret string) error
	rollbackFunc func(ctx context.Context, cfg provider.ProviderConfig, old string) error

	rotateCalls   int
	verifyCalls   int
	rollbackCalls int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Rotate(ctx context.Context, cfg provider.ProviderConfig, cur string) (*provider.Result, error) {
	m.rotateCalls++
	if m.rotateFunc != nil {
		return m.rotateFunc(ctx, cfg, cur)
	}
	return &provider.Result{OldSecret: cur, NewSecret: "new-secret-value"}, nil
}

func (m *mockProvider) Verify(ctx context.Context, cfg provider.ProviderConfig, secret string) error {
	m.verifyCalls++
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, cfg, secret)
	}
	return nil
}

func (m *mockProvider) Rollback(ctx context.Context, cfg provider.ProviderConfig, old string) error {
	m.rollbackCalls++
	if m.rollbackFunc != nil {
		return m.rollbackFunc(ctx, cfg, old)
	}
	return nil
}

// --- Mock Docker Manager ---

type mockDockerManager struct {
	restartFunc func(ctx context.Context, id string, timeout time.Duration) error
	healthFunc  func(ctx context.Context, id string, timeout time.Duration) error

	restartCalls []string
	healthCalls  []string
}

func (m *mockDockerManager) ListContainers(_ context.Context, _ docker.ContainerFilter) ([]docker.Container, error) {
	return nil, nil
}
func (m *mockDockerManager) InspectContainer(_ context.Context, _ string) (*docker.Container, error) {
	return nil, nil
}
func (m *mockDockerManager) StopContainer(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (m *mockDockerManager) StartContainer(_ context.Context, _ string) error { return nil }

func (m *mockDockerManager) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.restartCalls = append(m.restartCalls, id)
	if m.restartFunc != nil {
		return m.restartFunc(ctx, id, timeout)
	}
	return nil
}

func (m *mockDockerManager) WaitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	m.healthCalls = append(m.healthCalls, id)
	if m.healthFunc != nil {
		return m.healthFunc(ctx, id, timeout)
	}
	return nil
}

// --- Test Helpers ---

func writeEnvFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func newTestSecretConfig(envFile string) config.SecretConfig {
	return config.SecretConfig{
		Name:       "db-password",
		Type:       "mysql",
		EnvKey:     "DB_PASSWORD",
		EnvFile:    envFile,
		Containers: []string{"app", "worker"},
		Provider:   map[string]string{"host": "localhost", "port": "3306"},
	}
}

// --- Tests ---

func TestExecuteHappyPath(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\nOTHER=keep\n")
	histPath := filepath.Join(dir, "history.enc")

	prov := &mockProvider{name: "mysql"}
	dockerMgr := &mockDockerManager{}
	histStore := history.NewStore(histPath, []byte("test-passphrase"))

	eng := NewEngine(prov, dockerMgr, histStore, 30*time.Second, false)
	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))

	require.NoError(t, err)
	assert.Equal(t, 1, prov.rotateCalls)
	assert.Equal(t, 1, prov.verifyCalls)
	assert.Equal(t, 0, prov.rollbackCalls)
	assert.Equal(t, []string{"app", "worker"}, dockerMgr.restartCalls)
	assert.Equal(t, []string{"app", "worker"}, dockerMgr.healthCalls)

	// Verify .env was updated
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "DB_PASSWORD=new-secret-value")
	assert.Contains(t, string(data), "OTHER=keep")

	// Verify history recorded success
	entries, err := histStore.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "success", entries[0].Status)
	assert.Equal(t, "db-password", entries[0].SecretName)
}

func TestExecuteGenericProviderSkipsDBSteps(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "API_KEY=old-key\n")

	prov := &mockProvider{name: "generic"}
	dockerMgr := &mockDockerManager{}

	eng := NewEngine(prov, dockerMgr, nil, 30*time.Second, false)
	cfg := config.SecretConfig{
		Name:       "api-key",
		Type:       "generic",
		EnvKey:     "API_KEY",
		EnvFile:    envPath,
		Containers: []string{"app"},
	}

	err := eng.Execute(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, prov.rotateCalls)
	assert.Equal(t, 0, prov.verifyCalls, "generic provider should skip Verify")
	assert.Equal(t, 0, prov.rollbackCalls)
}

func TestExecuteKeyNotFound(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "OTHER_KEY=value\n")

	prov := &mockProvider{name: "mysql"}
	eng := NewEngine(prov, &mockDockerManager{}, nil, 30*time.Second, false)

	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in")
	assert.Equal(t, 0, prov.rotateCalls, "should not have started rotation")
}

func TestExecuteRotateFails(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")
	histPath := filepath.Join(dir, "history.enc")

	prov := &mockProvider{
		name: "mysql",
		rotateFunc: func(_ context.Context, _ provider.ProviderConfig, _ string) (*provider.Result, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	histStore := history.NewStore(histPath, []byte("test-passphrase"))
	eng := NewEngine(prov, &mockDockerManager{}, histStore, 30*time.Second, false)

	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	assert.Equal(t, 0, prov.rollbackCalls, "no rollback needed when Rotate fails")

	// History should record failure
	entries, err := histStore.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "failed", entries[0].Status)
}

func TestExecuteVerifyFails(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")
	histPath := filepath.Join(dir, "history.enc")

	prov := &mockProvider{
		name: "mysql",
		verifyFunc: func(_ context.Context, _ provider.ProviderConfig, _ string) error {
			return fmt.Errorf("verify failed: auth denied")
		},
	}
	histStore := history.NewStore(histPath, []byte("test-passphrase"))
	eng := NewEngine(prov, &mockDockerManager{}, histStore, 30*time.Second, false)

	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))
	require.Error(t, err)
	assert.Equal(t, 1, prov.rollbackCalls, "should rollback DB after verify fails")

	// .env should NOT have been updated
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "DB_PASSWORD=old-secret")

	// History records failure
	entries, err := histStore.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "failed", entries[0].Status)
}

func TestExecuteRestartFails(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")
	histPath := filepath.Join(dir, "history.enc")

	prov := &mockProvider{name: "mysql"}
	dockerMgr := &mockDockerManager{
		restartFunc: func(_ context.Context, id string, _ time.Duration) error {
			if id == "worker" {
				return fmt.Errorf("restart failed: worker")
			}
			return nil
		},
	}
	histStore := history.NewStore(histPath, []byte("test-passphrase"))
	eng := NewEngine(prov, dockerMgr, histStore, 30*time.Second, false)

	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))
	require.Error(t, err)
	assert.Equal(t, 1, prov.rollbackCalls, "should rollback DB")

	// .env should be restored to old value
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "DB_PASSWORD=old-secret")
}

func TestExecuteDryRun(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")

	prov := &mockProvider{name: "mysql"}
	dockerMgr := &mockDockerManager{}
	eng := NewEngine(prov, dockerMgr, nil, 30*time.Second, true)

	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))
	require.NoError(t, err)
	assert.Equal(t, 0, prov.rotateCalls, "dry run should not call Rotate")
	assert.Equal(t, 0, prov.verifyCalls, "dry run should not call Verify")
	assert.Len(t, dockerMgr.restartCalls, 0, "dry run should not restart")

	// .env should remain unchanged
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "DB_PASSWORD=old-secret")
}

func TestExecuteMultipleEnvFiles(t *testing.T) {
	dir := t.TempDir()
	envPath1 := writeEnvFile(t, dir, ".env.app", "DB_PASSWORD=old-secret\n")
	envPath2 := writeEnvFile(t, dir, ".env.worker", "DB_PASSWORD=old-secret\n")

	prov := &mockProvider{name: "mysql"}
	dockerMgr := &mockDockerManager{}
	eng := NewEngine(prov, dockerMgr, nil, 30*time.Second, false)

	cfg := config.SecretConfig{
		Name:       "db-password",
		Type:       "mysql",
		EnvKey:     "DB_PASSWORD",
		EnvFiles:   []string{envPath1, envPath2},
		Containers: []string{"app"},
	}

	err := eng.Execute(context.Background(), cfg)
	require.NoError(t, err)

	// Both files should be updated
	for _, p := range []string{envPath1, envPath2} {
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Contains(t, string(data), "DB_PASSWORD=new-secret-value", "file %s should be updated", p)
	}
}

func TestExecuteNilHistoryStore(t *testing.T) {
	dir := t.TempDir()
	envPath := writeEnvFile(t, dir, ".env", "DB_PASSWORD=old-secret\n")

	prov := &mockProvider{name: "mysql"}
	eng := NewEngine(prov, &mockDockerManager{}, nil, 30*time.Second, false)

	err := eng.Execute(context.Background(), newTestSecretConfig(envPath))
	require.NoError(t, err, "should work without history store")
}
