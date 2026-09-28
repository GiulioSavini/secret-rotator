package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInitIn executes `rotator init` against dir and returns stdout.
func runInitIn(t *testing.T, dir string, extraArgs ...string) (string, error) {
	t.Helper()

	cmd := NewInitCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(append([]string{dir}, extraArgs...))

	err := cmd.Execute()
	return out.String(), err
}

func writeEnv(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestInitGeneratesLoadableConfig(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_SECRET_KEY=changeme\nAPP_PORT=8080\n")

	_, err := runInitIn(t, dir)
	require.NoError(t, err)

	generated := filepath.Join(dir, "rotator.yml")
	require.FileExists(t, generated)

	cfg, err := config.Load(generated)
	require.NoError(t, err, "the generated file must be valid input for the loader")
	require.Len(t, cfg.Secrets, 1)
	assert.Equal(t, "APP_SECRET_KEY", cfg.Secrets[0].EnvKey)
	assert.Equal(t, "generic", cfg.Secrets[0].Type)
}

func TestInitNeverWritesSecretValues(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_SECRET_KEY=super-secret-value\n")

	_, err := runInitIn(t, dir)
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(dir, "rotator.yml"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "super-secret-value")
}

func TestInitInfersProviderTypes(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "POSTGRES_PASSWORD=a\nREDIS_PASSWORD=b\nMYSQL_ROOT_PASSWORD=c\nSTRIPE_API_KEY=d\n")

	_, err := runInitIn(t, dir)
	require.NoError(t, err)

	// The generated file carries TODO markers for the unknown hosts, so read
	// the raw text rather than loading it.
	body, err := os.ReadFile(filepath.Join(dir, "rotator.yml"))
	require.NoError(t, err)
	text := string(body)

	assert.Contains(t, text, "type: postgres")
	assert.Contains(t, text, "type: redis")
	assert.Contains(t, text, "type: mysql")
	assert.Contains(t, text, "type: generic")
	assert.Contains(t, text, `port: "5432"`)
	assert.Contains(t, text, `port: "6379"`)
	assert.Contains(t, text, `port: "3306"`)
}

func TestInitReportsRemainingWork(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "POSTGRES_PASSWORD=a\n")

	out, err := runInitIn(t, dir)
	require.NoError(t, err)
	assert.Contains(t, out, "Before the first rotation, fill in:")
	assert.Contains(t, out, "provider.host")
}

func TestInitRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_SECRET_KEY=a\n")
	existing := filepath.Join(dir, "rotator.yml")
	require.NoError(t, os.WriteFile(existing, []byte("# hand written\n"), 0o600))

	_, err := runInitIn(t, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")

	body, readErr := os.ReadFile(existing)
	require.NoError(t, readErr)
	assert.Equal(t, "# hand written\n", string(body), "the existing file must be left untouched")
}

func TestInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_SECRET_KEY=a\n")
	existing := filepath.Join(dir, "rotator.yml")
	require.NoError(t, os.WriteFile(existing, []byte("# hand written\n"), 0o600))

	_, err := runInitIn(t, dir, "--force")
	require.NoError(t, err)

	body, readErr := os.ReadFile(existing)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "APP_SECRET_KEY")
}

func TestInitSkipsTemplateEnvFiles(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_SECRET_KEY=a\n")
	writeEnv(t, dir, ".env.example", "TEMPLATE_TOKEN=placeholder\n")

	_, err := runInitIn(t, dir)
	require.NoError(t, err)

	body, readErr := os.ReadFile(filepath.Join(dir, "rotator.yml"))
	require.NoError(t, readErr)
	assert.NotContains(t, string(body), "TEMPLATE_TOKEN")
}

func TestInitWithoutEnvFiles(t *testing.T) {
	_, err := runInitIn(t, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .env files found")
}

func TestInitWithoutRecognisedSecrets(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "PORT=8080\nDEBUG=true\n")

	_, err := runInitIn(t, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no secrets discovered")
}

func TestInitRecordsComposeFile(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_SECRET_KEY=a\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"),
		[]byte("services:\n  app:\n    image: alpine\n"), 0o600))

	_, err := runInitIn(t, dir)
	require.NoError(t, err)

	cfg, err := config.Load(filepath.Join(dir, "rotator.yml"))
	require.NoError(t, err)
	assert.Equal(t, "docker-compose.yml", cfg.ComposeFile)

	resolved, err := cfg.ResolveComposeFile()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "docker-compose.yml"), resolved)
}
