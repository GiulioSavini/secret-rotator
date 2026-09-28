package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretLengthReachesTheProvider(t *testing.T) {
	// Providers read the password length from the provider options map, so a
	// top-level length: is only honoured if it is propagated there.
	yml := `
secrets:
  - name: api_key
    type: generic
    env_key: API_KEY
    env_file: .env
    length: 48
`
	cfg, err := Load(writeTestConfig(t, yml))
	require.NoError(t, err)
	assert.Equal(t, "48", cfg.Secrets[0].Provider["length"])
}

func TestExplicitProviderLengthWins(t *testing.T) {
	yml := `
secrets:
  - name: api_key
    type: generic
    env_key: API_KEY
    env_file: .env
    length: 48
    provider:
      length: "16"
`
	cfg, err := Load(writeTestConfig(t, yml))
	require.NoError(t, err)
	assert.Equal(t, "16", cfg.Secrets[0].Provider["length"])
}

func TestDeprecatedProviderUserIsAccepted(t *testing.T) {
	yml := `
secrets:
  - name: db
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    provider:
      host: db
      user: root
      password: admin
`
	cfg, err := Load(writeTestConfig(t, yml))
	require.NoError(t, err)

	assert.Equal(t, "root", cfg.Secrets[0].Provider["username"], "provider.user must map onto username")
	assert.NotContains(t, cfg.Secrets[0].Provider, "user")
	require.NotEmpty(t, cfg.Warnings)
	assert.Contains(t, cfg.Warnings[0], "deprecated")
}

func TestCanonicalKeyWinsOverDeprecatedOne(t *testing.T) {
	yml := `
secrets:
  - name: db
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    provider:
      host: db
      user: legacy
      username: canonical
      password: admin
`
	cfg, err := Load(writeTestConfig(t, yml))
	require.NoError(t, err)
	assert.Equal(t, "canonical", cfg.Secrets[0].Provider["username"])
}

func TestTODOPlaceholderIsRejected(t *testing.T) {
	yml := `
secrets:
  - name: db
    type: postgres
    env_key: POSTGRES_PASSWORD
    env_file: .env
    provider:
      host: TODO
      username: postgres
      database: postgres
      password: admin
`
	_, err := Load(writeTestConfig(t, yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider.host")
	assert.Contains(t, err.Error(), "TODO")
}

func TestDuplicateSecretNamesRejected(t *testing.T) {
	yml := `
secrets:
  - name: dup
    type: generic
    env_key: A
    env_file: .env
  - name: dup
    type: generic
    env_key: B
    env_file: .env
`
	_, err := Load(writeTestConfig(t, yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate name")
}

func TestMissingAdminUsernameIsAnError(t *testing.T) {
	yml := `
secrets:
  - name: db
    type: postgres
    env_key: POSTGRES_PASSWORD
    env_file: .env
    provider:
      host: db
      database: app
      password: admin
`
	_, err := Load(writeTestConfig(t, yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider.username is required")
}

func TestMissingAdminPasswordIsOnlyAWarning(t *testing.T) {
	// An admin account with no password is poor practice but valid, so it
	// must not block a rotation.
	yml := `
secrets:
  - name: db
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    provider:
      host: db
      username: root
`
	cfg, err := Load(writeTestConfig(t, yml))
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Warnings)
	assert.Contains(t, cfg.Warnings[0], "empty password")
}

func TestNonNumericPortIsRejected(t *testing.T) {
	yml := `
secrets:
  - name: db
    type: redis
    env_key: REDIS_PASSWORD
    env_file: .env
    provider:
      host: cache
      port: "six-three-seven-nine"
`
	_, err := Load(writeTestConfig(t, yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider.port must be a number")
}
