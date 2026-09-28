package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/configfile"
)

// useDataDir points the history store at dir for the duration of a test.
// Subcommands read the data directory from the root command's persistent
// flag, which is not present when a subcommand is constructed in isolation.
func useDataDir(t *testing.T, dir string) {
	t.Helper()
	prev := dataDirFlag
	dataDirFlag = dir
	t.Cleanup(func() { dataDirFlag = prev })
}

// useHistoryIn points the history store at the ".rotator" subdirectory of
// dir, matching the layout the CLI creates at runtime.
func useHistoryIn(t *testing.T, dir string) {
	t.Helper()
	useDataDir(t, filepath.Join(dir, ".rotator"))
}

// fakeSecret builds a valid aggregate for tests that only care about the
// command's behaviour, not the secret's contents.
func fakeSecret(t *testing.T, name, kind string, opts ...func(*domain.SecretSpec)) *domain.Secret {
	t.Helper()

	spec := domain.SecretSpec{
		Name:     name,
		Kind:     kind,
		EnvKey:   "SOME_PASSWORD",
		EnvFiles: []string{".env"},
	}
	if kind != "generic" {
		spec.Target = domain.Target{Host: "localhost", AdminUser: "admin", AdminPassword: domain.NewCredential("pw")}
	}
	for _, opt := range opts {
		opt(&spec)
	}

	secret, _, err := domain.NewSecret(spec)
	require.NoError(t, err)
	return secret
}

// withConfig installs a configuration for the duration of a test.
func withConfig(t *testing.T, secrets ...*domain.Secret) {
	t.Helper()
	prev := AppConfig
	AppConfig = configfile.New(secrets...)
	t.Cleanup(func() { AppConfig = prev })
}
