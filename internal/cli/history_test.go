package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryCmd_NoFile(t *testing.T) {
	dir := t.TempDir()

	cmd := NewHistoryCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--passphrase", "test-pass", "--dir", dir})

	// Bypass PersistentPreRunE which tries to load config
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No rotation history found.")
}

func TestHistoryCmd_WithEntries(t *testing.T) {
	dir := t.TempDir()
	passphrase := "test-passphrase"

	// Create history directory and store
	histDir := filepath.Join(dir, ".rotator")
	require.NoError(t, os.MkdirAll(histDir, 0o700))

	store := history.NewStore(filepath.Join(histDir, "history.json"), []byte(passphrase))

	require.NoError(t, store.Append(history.HistoryEntry{
		SecretName: "MYSQL_PASSWORD",
		RotatedAt:  time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC),
		Status:     "success",
		Details:    "Rotated via mysql provider",
	}))

	require.NoError(t, store.Append(history.HistoryEntry{
		SecretName: "REDIS_PASSWORD",
		RotatedAt:  time.Date(2026, 3, 28, 14, 30, 5, 0, time.UTC),
		Status:     "failed",
		Details:    "Connection refused",
	}))

	cmd := NewHistoryCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--passphrase", passphrase, "--dir", dir})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "MYSQL_PASSWORD")
	assert.Contains(t, output, "REDIS_PASSWORD")
	assert.Contains(t, output, "success")
	assert.Contains(t, output, "failed")
	assert.Contains(t, output, "Connection refused")
	assert.Contains(t, output, "Showing 2 entries")
}

func TestHistoryCmd_NoPassphrase(t *testing.T) {
	// Ensure env vars are unset
	t.Setenv("ROTATOR_MASTER_KEY", "")

	// Clear AppConfig to avoid config-based passphrase
	origConfig := AppConfig
	AppConfig = nil
	defer func() { AppConfig = origConfig }()

	cmd := NewHistoryCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	errBuf := new(bytes.Buffer)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"--dir", t.TempDir()})

	err := cmd.Execute()
	require.Error(t, err)

	// The error message should mention passphrase requirement but NOT contain actual passphrase values
	errMsg := err.Error()
	assert.True(t, strings.Contains(errMsg, "master passphrase required"),
		"error should mention passphrase requirement, got: %s", errMsg)
}

func TestHistoryCmd_Limit(t *testing.T) {
	dir := t.TempDir()
	passphrase := "test-passphrase"

	histDir := filepath.Join(dir, ".rotator")
	require.NoError(t, os.MkdirAll(histDir, 0o700))

	store := history.NewStore(filepath.Join(histDir, "history.json"), []byte(passphrase))

	for i := 0; i < 5; i++ {
		require.NoError(t, store.Append(history.HistoryEntry{
			SecretName: fmt.Sprintf("SECRET_%d", i),
			RotatedAt:  time.Date(2026, 3, 28, 10, i, 0, 0, time.UTC),
			Status:     "success",
		}))
	}

	cmd := NewHistoryCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--passphrase", passphrase, "--dir", dir, "--limit", "2"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Showing 2 entries")
	// Should show last 2 entries (SECRET_3 and SECRET_4)
	assert.Contains(t, output, "SECRET_3")
	assert.Contains(t, output, "SECRET_4")
	assert.NotContains(t, output, "SECRET_0")
}
