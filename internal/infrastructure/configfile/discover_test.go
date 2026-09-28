package configfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalYAML = `
secrets:
  - name: api_key
    type: generic
    env_key: API_KEY
    env_file: .env
`

func TestDiscoverExplicitPathWins(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "custom.yml")
	require.NoError(t, os.WriteFile(explicit, []byte(minimalYAML), 0o600))

	// A rotator.yml in the working directory must not take precedence.
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rotator.yml"), []byte(minimalYAML), 0o600))

	got, err := Discover(explicit)
	require.NoError(t, err)
	assert.Equal(t, explicit, got)
}

func TestDiscoverExplicitPathMustExist(t *testing.T) {
	_, err := Discover(filepath.Join(t.TempDir(), "absent.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absent.yml")
}

func TestDiscoverEnvVarBeatsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rotator.yml"), []byte(minimalYAML), 0o600))

	fromEnv := filepath.Join(dir, "elsewhere.yml")
	require.NoError(t, os.WriteFile(fromEnv, []byte(minimalYAML), 0o600))
	t.Setenv(EnvConfigPath, fromEnv)

	got, err := Discover("")
	require.NoError(t, err)
	assert.Equal(t, fromEnv, got)
}

func TestDiscoverFindsWorkingDirectoryConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "rotator.yml")
	require.NoError(t, os.WriteFile(path, []byte(minimalYAML), 0o600))

	got, err := Discover("")
	require.NoError(t, err)
	assert.Equal(t, path, got)
}

func TestDiscoverReportsSearchedPaths(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := Discover("")
	require.Error(t, err)

	var missing *ErrNoConfig
	require.True(t, asErrNoConfig(err, &missing))
	assert.Contains(t, missing.Error(), "rotator.yml")
	assert.NotEmpty(t, missing.Searched)
}

func TestLoadDiscoveredFallsBackToZeroConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, path, err := LoadDiscovered("")
	require.NoError(t, err, "a missing config must not break commands that do not need one")
	assert.Empty(t, path)
	assert.Empty(t, cfg.Secrets())

	// A command that does need secrets gets an actionable message instead.
	err = cfg.RequireSecrets()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration required")
	assert.Contains(t, err.Error(), "rotator init")
}

func TestDataDirPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "rotator.yml")
	cfg := &Config{Path: cfgPath}

	t.Run("explicit flag wins", func(t *testing.T) {
		t.Setenv(EnvDataDir, "/from/env")
		assert.Equal(t, "/from/flag", cfg.DataDir("/from/flag"))
	})

	t.Run("env var beats config location", func(t *testing.T) {
		t.Setenv(EnvDataDir, "/from/env")
		assert.Equal(t, "/from/env", cfg.DataDir(""))
	})

	t.Run("defaults next to the config", func(t *testing.T) {
		assert.Equal(t, filepath.Join(dir, ".rotator"), cfg.DataDir(""))
		assert.Equal(t, filepath.Join(dir, ".rotator", "history.json"), cfg.HistoryPath(""))
	})

	t.Run("zero-config falls back to the working directory", func(t *testing.T) {
		var zero *Config
		assert.Equal(t, ".rotator", zero.DataDir(""))
	})
}

func TestResolveComposeFile(t *testing.T) {
	t.Run("auto-discovers next to the config", func(t *testing.T) {
		dir := t.TempDir()
		compose := filepath.Join(dir, "docker-compose.yml")
		require.NoError(t, os.WriteFile(compose, []byte("services: {}\n"), 0o600))

		cfg := &Config{Path: filepath.Join(dir, "rotator.yml")}
		got, err := cfg.ResolveComposeFile()
		require.NoError(t, err)
		assert.Equal(t, compose, got)
	})

	t.Run("returns empty when absent", func(t *testing.T) {
		cfg := &Config{Path: filepath.Join(t.TempDir(), "rotator.yml")}
		got, err := cfg.ResolveComposeFile()
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("an explicit but missing file is an error", func(t *testing.T) {
		cfg := &Config{Path: filepath.Join(t.TempDir(), "rotator.yml"), ComposeFile: "nope.yml"}
		_, err := cfg.ResolveComposeFile()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope.yml")
	})
}
