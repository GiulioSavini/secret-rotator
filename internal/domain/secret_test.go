package domain

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validSpec() SecretSpec {
	return SecretSpec{
		Name:     "db_password",
		Kind:     "postgres",
		EnvKey:   "DB_PASSWORD",
		EnvFiles: []string{".env"},
		Target:   Target{Host: "db", AdminUser: "postgres", Database: "app"},
	}
}

func TestNewSecretRejectsInvalidDefinitions(t *testing.T) {
	cases := map[string]func(*SecretSpec){
		"empty name":        func(s *SecretSpec) { s.Name = "" },
		"name with spaces":  func(s *SecretSpec) { s.Name = "db password" },
		"unknown kind":      func(s *SecretSpec) { s.Kind = "mongodb" },
		"empty kind":        func(s *SecretSpec) { s.Kind = "" },
		"empty env key":     func(s *SecretSpec) { s.EnvKey = "" },
		"env key with dash": func(s *SecretSpec) { s.EnvKey = "DB-PASSWORD" },
		"env key starting with a digit": func(s *SecretSpec) {
			s.EnvKey = "1DB"
		},
		"no env files":        func(s *SecretSpec) { s.EnvFiles = nil },
		"empty env file":      func(s *SecretSpec) { s.EnvFiles = []string{""} },
		"duplicate env files": func(s *SecretSpec) { s.EnvFiles = []string{".env", ".env"} },
		"empty container":     func(s *SecretSpec) { s.Containers = []string{"app", ""} },
		"length too small":    func(s *SecretSpec) { s.Length = 4 },
		"length too large":    func(s *SecretSpec) { s.Length = 100000 },
		"missing host":        func(s *SecretSpec) { s.Target.Host = "" },
		"missing admin user":  func(s *SecretSpec) { s.Target.AdminUser = "" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := validSpec()
			mutate(&spec)

			_, _, err := NewSecret(spec)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidSecret)
		})
	}
}

func TestNewSecretAcceptsAValidDefinition(t *testing.T) {
	secret, warnings, err := NewSecret(validSpec())
	require.NoError(t, err)

	assert.Equal(t, SecretName("db_password"), secret.Name())
	assert.Equal(t, KindPostgres, secret.Kind())
	assert.Equal(t, DefaultPasswordLength, secret.Length())
	assert.Equal(t, FilePath(".env"), secret.PrimaryEnvFile())
	assert.True(t, secret.NeedsVerification())

	// No admin password configured: workable, but worth saying out loud.
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "empty password")
}

func TestGenericSecretNeedsNoConnectionDetails(t *testing.T) {
	spec := validSpec()
	spec.Kind = "generic"
	spec.Target = Target{}

	secret, warnings, err := NewSecret(spec)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.False(t, secret.NeedsVerification())
}

func TestAccessorsReturnCopies(t *testing.T) {
	spec := validSpec()
	spec.Containers = []string{"db", "app"}
	spec.EnvFiles = []string{".env", ".env.local"}

	secret, _, err := NewSecret(spec)
	require.NoError(t, err)

	// Mutating what an accessor returned must not reach inside the aggregate.
	secret.Containers()[0] = "tampered"
	secret.EnvFiles()[0] = "tampered"

	assert.Equal(t, []ContainerRef{"db", "app"}, secret.Containers())
	assert.Equal(t, FilePath(".env"), secret.PrimaryEnvFile())
}

func TestWithContainersLeavesTheOriginalAlone(t *testing.T) {
	spec := validSpec()
	spec.Containers = []string{"db", "app"}

	secret, _, err := NewSecret(spec)
	require.NoError(t, err)

	reordered := secret.WithContainers([]ContainerRef{"app", "db"})

	assert.Equal(t, []ContainerRef{"db", "app"}, secret.Containers())
	assert.Equal(t, []ContainerRef{"app", "db"}, reordered.Containers())
}

func TestTargetDefaults(t *testing.T) {
	target := Target{Host: "db", AdminUser: "postgres"}

	assert.Equal(t, 5432, target.EffectivePort(KindPostgres))
	assert.Equal(t, 3306, target.EffectivePort(KindMySQL))
	assert.Equal(t, 6379, target.EffectivePort(KindRedis))
	assert.Equal(t, "postgres", target.EffectiveTargetUser(),
		"an account with no explicit target rotates its own password")
	assert.Equal(t, "%", target.EffectiveTargetUserHost())

	explicit := Target{Host: "db", Port: 6543, AdminUser: "postgres", TargetUser: "app", TargetUserHost: "localhost"}
	assert.Equal(t, 6543, explicit.EffectivePort(KindPostgres))
	assert.Equal(t, "app", explicit.EffectiveTargetUser())
	assert.Equal(t, "localhost", explicit.EffectiveTargetUserHost())
	assert.Equal(t, "db:6543", explicit.Addr(KindPostgres))
}

func TestCredentialIsRedactedEverywhere(t *testing.T) {
	c := NewCredential("super-secret-value")

	rendered := []string{
		c.String(),
		fmt.Sprintf("%v", c),
		fmt.Sprintf("%s", c),
		fmt.Sprintf("%q", c),
		fmt.Sprintf("%#v", c),
		fmt.Sprintf("%+v", struct{ Password Credential }{c}),
		fmt.Errorf("connecting with %v failed", c).Error(),
	}
	for _, r := range rendered {
		assert.NotContains(t, r, "super-secret-value")
		assert.Contains(t, r, "REDACTED")
	}

	encoded, err := json.Marshal(struct {
		Password Credential `json:"password"`
	}{c})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "super-secret-value")

	// Expose is the one way through, and it is greppable.
	assert.Equal(t, "super-secret-value", c.Expose())
}

func TestPasswordLengthDefaults(t *testing.T) {
	l, err := NewPasswordLength(0)
	require.NoError(t, err)
	assert.Equal(t, DefaultPasswordLength, l, "an omitted length must not mean zero entropy")

	_, err = NewPasswordLength(-1)
	require.Error(t, err)
}
