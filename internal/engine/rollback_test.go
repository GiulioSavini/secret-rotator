package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRollbackAfterUpdateEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	oldContent := []byte("DB_PASSWORD=old-secret\nOTHER=keep\n")
	require.NoError(t, os.WriteFile(envPath, oldContent, 0o644))

	prov := &mockProvider{name: "mysql"}
	state := &RotationState{
		SecretName:    "db-password",
		CurrentStep:   StepUpdateEnv,
		OldSecret:     "old-secret",
		NewSecret:     "new-secret",
		OldEnvContent: oldContent,
		EnvFilePath:   envPath,
	}

	err := rollback(context.Background(), state, prov, &mockDockerManager{}, 30*time.Second)
	require.NoError(t, err)

	// .env should be restored
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, string(oldContent), string(data))

	// Provider rollback should have been called
	assert.Equal(t, 1, prov.rollbackCalls)
}

func TestRollbackAfterRestart(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	oldContent := []byte("DB_PASSWORD=old-secret\n")
	// Write the "new" content (simulating post-update state)
	require.NoError(t, os.WriteFile(envPath, []byte("DB_PASSWORD=new-secret\n"), 0o644))

	prov := &mockProvider{name: "mysql"}
	dockerMgr := &mockDockerManager{}
	state := &RotationState{
		SecretName:    "db-password",
		CurrentStep:   StepRestart,
		OldSecret:     "old-secret",
		NewSecret:     "new-secret",
		OldEnvContent: oldContent,
		EnvFilePath:   envPath,
		Containers:    []string{"app", "worker"},
	}

	err := rollback(context.Background(), state, prov, dockerMgr, 30*time.Second)
	require.NoError(t, err)

	// .env should be restored
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, string(oldContent), string(data))

	// Containers should be restarted (to pick up restored .env)
	assert.Equal(t, []string{"app", "worker"}, dockerMgr.restartCalls)

	// Provider rollback should have been called
	assert.Equal(t, 1, prov.rollbackCalls)
}

func TestRollbackAfterVerifyDB(t *testing.T) {
	prov := &mockProvider{name: "mysql"}
	state := &RotationState{
		SecretName:  "db-password",
		CurrentStep: StepVerifyDB,
		OldSecret:   "old-secret",
		NewSecret:   "new-secret",
	}

	err := rollback(context.Background(), state, prov, &mockDockerManager{}, 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, prov.rollbackCalls, "should rollback DB password")
}

func TestRollbackGenericProviderSkipsDB(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	oldContent := []byte("API_KEY=old-key\n")
	require.NoError(t, os.WriteFile(envPath, []byte("API_KEY=new-key\n"), 0o644))

	prov := &mockProvider{name: "generic"}
	state := &RotationState{
		SecretName:    "api-key",
		CurrentStep:   StepUpdateEnv,
		OldSecret:     "old-key",
		NewSecret:     "new-key",
		OldEnvContent: oldContent,
		EnvFilePath:   envPath,
	}

	err := rollback(context.Background(), state, prov, &mockDockerManager{}, 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, prov.rollbackCalls, "generic provider should not rollback DB")

	// .env should still be restored
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, string(oldContent), string(data))
}

func TestRollbackFailureReturnsDetailedError(t *testing.T) {
	prov := &mockProvider{
		name: "mysql",
		rollbackFunc: func(_ context.Context, _ provider.ProviderConfig, _ string) error {
			return fmt.Errorf("db connection lost")
		},
	}
	state := &RotationState{
		SecretName:  "db-password",
		CurrentStep: StepVerifyDB,
		OldSecret:   "old-secret",
		NewSecret:   "new-secret",
	}

	err := rollback(context.Background(), state, prov, &mockDockerManager{}, 30*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback failed")
	assert.Contains(t, err.Error(), "db connection lost")
}

func TestRollbackLIFOOrder(t *testing.T) {
	// When rolling back after restart, order should be:
	// 1. Restore .env
	// 2. Restart containers (with old .env)
	// 3. Rollback DB
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	oldContent := []byte("DB_PASSWORD=old-secret\n")
	require.NoError(t, os.WriteFile(envPath, []byte("DB_PASSWORD=new-secret\n"), 0o644))

	var order []string
	prov := &mockProvider{
		name: "mysql",
		rollbackFunc: func(_ context.Context, _ provider.ProviderConfig, _ string) error {
			order = append(order, "db_rollback")
			return nil
		},
	}
	dockerMgr := &mockDockerManager{
		restartFunc: func(_ context.Context, id string, _ time.Duration) error {
			order = append(order, "restart_"+id)
			return nil
		},
	}
	state := &RotationState{
		SecretName:    "db-password",
		CurrentStep:   StepRestart,
		OldSecret:     "old-secret",
		NewSecret:     "new-secret",
		OldEnvContent: oldContent,
		EnvFilePath:   envPath,
		Containers:    []string{"app"},
	}

	err := rollback(context.Background(), state, prov, dockerMgr, 30*time.Second)
	require.NoError(t, err)

	// LIFO: restore env first, then restart, then DB rollback
	require.Len(t, order, 2) // restart_app + db_rollback (env restore is os.WriteFile, not tracked)
	assert.Equal(t, "restart_app", order[0])
	assert.Equal(t, "db_rollback", order[1])

	// Verify .env was restored before restart
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, string(oldContent), string(data))
}
