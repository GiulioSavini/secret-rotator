package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

func TestCommandBlockBecomesATypedSpec(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["/opt/rotate-minio.sh", "--user", "app"]
      verify: ["/opt/verify-minio.sh"]
      restore: ["/opt/rotate-minio.sh", "--restore"]
      timeout: 90s
      pass_env: [AWS_REGION, AWS_ENDPOINT]
`)

	spec := secret.Target().Command
	assert.Equal(t, []string{"/opt/rotate-minio.sh", "--user", "app"}, spec.Apply)
	assert.Equal(t, []string{"/opt/verify-minio.sh"}, spec.Verify)
	assert.Equal(t, []string{"/opt/rotate-minio.sh", "--restore"}, spec.Restore)
	assert.Equal(t, 90*time.Second, spec.EffectiveTimeout())
	assert.Equal(t, []string{"AWS_REGION", "AWS_ENDPOINT"}, spec.PassEnv)
}

func TestRelativeProgramPathsAreAnchoredToTheConfig(t *testing.T) {
	// The same rule the .env paths follow: a script path must mean the same
	// thing to a manual run from elsewhere and to the daemon.
	yml := `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["./hooks/rotate.sh", "--flag"]
`
	path := writeTestConfig(t, yml)
	cfg, err := Load(path)
	require.NoError(t, err)

	base := filepath.Dir(path)
	spec := cfg.Secrets()[0].Target().Command

	assert.Equal(t, filepath.Join(base, "hooks/rotate.sh"), spec.Apply[0])
	assert.Equal(t, "--flag", spec.Apply[1], "only the program is anchored, not the arguments")
	assert.Equal(t, base, spec.WorkingDir, "commands run next to the config by default")
}

func TestABareProgramNameIsLeftForPathLookup(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["mc", "admin", "user", "add"]
`)
	assert.Equal(t, "mc", secret.Target().Command.Apply[0])
}

func TestAnExplicitWorkingDirectoryIsAnchoredToo(t *testing.T) {
	yml := `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["/opt/rotate.sh"]
      working_dir: hooks
`
	path := writeTestConfig(t, yml)
	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(filepath.Dir(path), "hooks"),
		cfg.Secrets()[0].Target().Command.WorkingDir)
}

func TestAnUnparsableTimeoutIsRejected(t *testing.T) {
	_, err := Load(writeTestConfig(t, `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["/opt/rotate.sh"]
      timeout: soon
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command.timeout")
}

func TestAMissingProgramIsWarnedAboutAtLoadTime(t *testing.T) {
	// Finding out at 3am that the script is not there is worse than being
	// told now.
	_, cfg := loadOne(t, `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["/opt/definitely-not-here.sh"]
      verify: ["/opt/also-not-here.sh"]
      restore: ["/opt/nor-this.sh"]
`)

	var mentioned int
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "is not present right now") {
			mentioned++
		}
	}
	assert.Equal(t, 3, mentioned, "each configured program is checked")
}

func TestAPresentProgramIsNotWarnedAbout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "rotate.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700))

	cfgPath := filepath.Join(dir, "rotator.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply: ["./rotate.sh"]
      verify: ["./rotate.sh"]
      restore: ["./rotate.sh"]
`), 0o600))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	for _, w := range cfg.Warnings {
		assert.NotContains(t, w, "is not present right now")
	}
}

func TestCommandKindIsAcceptedWithNoProviderBlock(t *testing.T) {
	secret, _ := loadOne(t, `
secrets:
  - name: minio_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    max_age: 90d
    command:
      apply: ["/opt/rotate.sh"]
`)
	assert.Equal(t, domain.KindCommand, secret.Kind())
	assert.Equal(t, "90d", secret.MaxAge().String())
}
