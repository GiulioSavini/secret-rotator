package configfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giulio/secret-rotator/internal/domain"
)

// loadOne loads a configuration expected to hold exactly one secret and
// returns both the aggregate and the configuration it came from.
func loadOne(t *testing.T, yml string) (*domain.Secret, *Config) {
	t.Helper()
	cfg, err := Load(writeTestConfig(t, yml))
	require.NoError(t, err)
	require.Len(t, cfg.Secrets(), 1)
	return cfg.Secrets()[0], cfg
}

func TestSecretLengthReachesTheModel(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: api_key
    type: generic
    env_key: API_KEY
    env_file: .env
    length: 48
`)
	assert.Equal(t, domain.PasswordLength(48), secret.Length())
}

func TestOmittedLengthFallsBackToTheDefault(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: api_key
    type: generic
    env_key: API_KEY
    env_file: .env
`)
	assert.Equal(t, domain.DefaultPasswordLength, secret.Length())
}

func TestProviderLengthStillWins(t *testing.T) {
	// provider.length predates the top-level field; a configuration written
	// against the old shape must keep its behaviour.
	secret, _ := loadOne(t, `
secrets:
  - name: api_key
    type: generic
    env_key: API_KEY
    env_file: .env
    length: 48
    provider:
      length: "16"
`)
	assert.Equal(t, domain.PasswordLength(16), secret.Length())
}

func TestDeprecatedProviderUserIsAccepted(t *testing.T) {
	secret, cfg := loadOne(t, `
secrets:
  - name: db
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    provider:
      host: db
      user: root
      password: admin
`)
	assert.Equal(t, "root", secret.Target().AdminUser, "provider.user must map onto the admin user")
	require.NotEmpty(t, cfg.Warnings)
	assert.Contains(t, cfg.Warnings[0], "deprecated")
}

func TestCanonicalKeyWinsOverDeprecatedOne(t *testing.T) {
	secret, _ := loadOne(t, `
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
`)
	assert.Equal(t, "canonical", secret.Target().AdminUser)
}

func TestProviderMapBecomesATypedTarget(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: db
    type: postgres
    env_key: POSTGRES_PASSWORD
    env_file: .env
    provider:
      host: db
      port: "6543"
      username: postgres
      database: app
      target_user: app_user
      password_env: PG_ADMIN_PASSWORD
`)
	target := secret.Target()
	assert.Equal(t, "db", target.Host)
	assert.Equal(t, 6543, target.Port)
	assert.Equal(t, "postgres", target.AdminUser)
	assert.Equal(t, "app", target.Database)
	assert.Equal(t, "app_user", target.EffectiveTargetUser())
	assert.Equal(t, "PG_ADMIN_PASSWORD", target.AdminPasswordEnv)
}

func TestPortDefaultsToTheKindDefault(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: cache
    type: redis
    env_key: REDIS_PASSWORD
    env_file: .env
    provider:
      host: cache
`)
	assert.Equal(t, 6379, secret.Target().EffectivePort(secret.Kind()))
}

func TestUnknownProviderKeyIsReported(t *testing.T) {
	// A typo in a provider key used to be silently ignored, which is how a
	// secret ends up rotating against the wrong account.
	_, cfg := loadOne(t, `
secrets:
  - name: db
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    provider:
      host: db
      username: root
      password: admin
      target_usr: app_user
`)
	require.NotEmpty(t, cfg.Warnings)
	assert.Contains(t, cfg.Warnings[0], "target_usr")
}

func TestTODOPlaceholderIsRejected(t *testing.T) {
	_, err := Load(writeTestConfig(t, `
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
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider.host")
	assert.Contains(t, err.Error(), "TODO")
}

func TestDuplicateSecretNamesRejected(t *testing.T) {
	_, err := Load(writeTestConfig(t, `
secrets:
  - name: dup
    type: generic
    env_key: A
    env_file: .env
  - name: dup
    type: generic
    env_key: B
    env_file: .env
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate name")
}

func TestMissingAdminUsernameIsAnError(t *testing.T) {
	_, err := Load(writeTestConfig(t, `
secrets:
  - name: db
    type: postgres
    env_key: POSTGRES_PASSWORD
    env_file: .env
    provider:
      host: db
      database: app
      password: admin
`))
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidSecret)
	assert.Contains(t, err.Error(), "provider.username is required")
}

func TestMissingAdminPasswordIsOnlyAWarning(t *testing.T) {
	// An admin account with no password is poor practice but valid, so it
	// must not block a rotation.
	_, cfg := loadOne(t, `
secrets:
  - name: db
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    provider:
      host: db
      username: root
`)
	require.NotEmpty(t, cfg.Warnings)
	assert.Contains(t, cfg.Warnings[0], "empty password")
}

func TestNonNumericPortIsRejected(t *testing.T) {
	_, err := Load(writeTestConfig(t, `
secrets:
  - name: db
    type: redis
    env_key: REDIS_PASSWORD
    env_file: .env
    provider:
      host: cache
      port: "six-three-seven-nine"
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider.port must be a number")
}

func TestInvalidEnvKeyIsRejected(t *testing.T) {
	_, err := Load(writeTestConfig(t, `
secrets:
  - name: db
    type: generic
    env_key: "not a valid key"
    env_file: .env
`))
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidSecret)
}
