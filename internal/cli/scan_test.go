package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanCmd_NoEnvFiles(t *testing.T) {
	dir := t.TempDir()

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"scan", "--dir", dir})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No secrets discovered")
}

func TestScanCmd_WithSecrets(t *testing.T) {
	dir := t.TempDir()

	// Create a .env file with known secrets.
	envContent := `# Database config
MYSQL_ROOT_PASSWORD=changeme
REDIS_PASSWORD=xK9mP2vL5nQ8jR4wT7yU0zA3bCdEfGh
APP_NAME=myapp
DB_PASSWORD_FILE=/run/secrets/db_pass
`
	err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644)
	require.NoError(t, err)

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"scan", "--dir", dir})

	err = cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Table header.
	assert.Contains(t, output, "SECRET")
	assert.Contains(t, output, "TYPE")
	assert.Contains(t, output, "STRENGTH")
	assert.Contains(t, output, "SOURCE")
	assert.Contains(t, output, "ISSUES")

	// Secrets found.
	assert.Contains(t, output, "MYSQL_ROOT_PASSWORD")
	assert.Contains(t, output, "REDIS_PASSWORD")
	assert.Contains(t, output, "DB_PASSWORD_FILE")

	// APP_NAME should NOT appear (not a secret).
	assert.NotContains(t, output, "APP_NAME")

	// Strength ratings.
	assert.Contains(t, output, "weak")
	assert.Contains(t, output, "common default")
	assert.Contains(t, output, "n/a") // file-referenced

	// File-referenced type annotation.
	assert.Contains(t, output, "password (file)")

	// Summary line.
	assert.Contains(t, output, "Found")
	assert.Contains(t, output, "secrets")
}
