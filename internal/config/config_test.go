package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rotator.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadYAML(t *testing.T) {
	yml := `
secrets:
  - name: db_password
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    containers:
      - mysql
      - app
    provider:
      host: localhost
      port: "3306"
      user: root
    schedule: "0 3 * * 0"
    length: 32
`
	path := writeTestConfig(t, yml)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Secrets, 1)

	s := cfg.Secrets[0]
	assert.Equal(t, "db_password", s.Name)
	assert.Equal(t, "mysql", s.Type)
	assert.Equal(t, "MYSQL_ROOT_PASSWORD", s.EnvKey)
	assert.Equal(t, ".env", s.EnvFile)
	assert.Equal(t, []string{"mysql", "app"}, s.Containers)
	assert.Equal(t, "localhost", s.Provider["host"])
	assert.Equal(t, "0 3 * * 0", s.Schedule)
	assert.Equal(t, 32, s.Length)
}

func TestLoadNoFile(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.Secrets)
	assert.Empty(t, cfg.Notifications)
}

func TestLoadInvalidType(t *testing.T) {
	yml := `
secrets:
  - name: test
    type: mongodb
    env_key: MONGO_PASS
    env_file: .env
`
	path := writeTestConfig(t, yml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
	assert.Contains(t, err.Error(), "mongodb")
}

func TestLoadMissingName(t *testing.T) {
	yml := `
secrets:
  - type: mysql
    env_key: MYSQL_PASS
    env_file: .env
`
	path := writeTestConfig(t, yml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestLoadMissingEnvKey(t *testing.T) {
	yml := `
secrets:
  - name: test
    type: mysql
    env_file: .env
`
	path := writeTestConfig(t, yml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env_key is required")
}

func TestLoadMissingEnvFile(t *testing.T) {
	yml := `
secrets:
  - name: test
    type: mysql
    env_key: MYSQL_PASS
`
	path := writeTestConfig(t, yml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env_file or env_files is required")
}

func TestLoadEnvVarOverride(t *testing.T) {
	yml := `
master_key_env: DEFAULT_KEY
secrets: []
`
	path := writeTestConfig(t, yml)

	t.Setenv("ROTATOR_MASTER_KEY_ENV", "OVERRIDDEN_KEY")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "OVERRIDDEN_KEY", cfg.MasterKeyEnv)
}

func TestMultiFileEnvConfig(t *testing.T) {
	yml := `
secrets:
  - name: db_password
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_files:
      - .env
      - .env.local
      - docker/.env
    containers:
      - mysql
`
	path := writeTestConfig(t, yml)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Secrets, 1)

	s := cfg.Secrets[0]
	assert.Equal(t, []string{".env", ".env.local", "docker/.env"}, s.EnvFiles)
	assert.Empty(t, s.EnvFile)
}
