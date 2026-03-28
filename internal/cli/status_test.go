package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusCmd_NoConfig(t *testing.T) {
	origConfig := AppConfig
	AppConfig = nil
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No secrets configured")
}

func TestStatusCmd_EmptySecrets(t *testing.T) {
	origConfig := AppConfig
	AppConfig = &config.Config{Secrets: []config.SecretConfig{}}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No secrets configured")
}

func TestStatusCmd_ShowsTableHeaders(t *testing.T) {
	origConfig := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "DB_PASSWORD", Type: "postgres"},
		},
	}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "TYPE")
	assert.Contains(t, output, "AGE")
	assert.Contains(t, output, "SCHEDULE")
	assert.Contains(t, output, "NEXT ROTATION")
	assert.Contains(t, output, "DB_PASSWORD")
	assert.Contains(t, output, "postgres")
}

func TestStatusCmd_NeverAgeWithoutHistory(t *testing.T) {
	origConfig := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "API_KEY", Type: "generic"},
		},
	}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	// No passphrase, so no history will be loaded
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "never")
}

func TestStatusCmd_HumanReadableAge(t *testing.T) {
	dir := t.TempDir()
	passphrase := "test-passphrase"

	histDir := filepath.Join(dir, ".rotator")
	require.NoError(t, os.MkdirAll(histDir, 0o700))

	store := history.NewStore(filepath.Join(histDir, "history.json"), []byte(passphrase))
	rotatedAt := time.Now().Add(-72*time.Hour - 2*time.Hour) // 3 days 2 hours ago
	require.NoError(t, store.Append(history.HistoryEntry{
		SecretName: "DB_PASSWORD",
		RotatedAt:  rotatedAt,
		Status:     "success",
		Details:    "rotated",
	}))

	origConfig := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "DB_PASSWORD", Type: "postgres"},
		},
	}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--passphrase", passphrase, "--dir", dir})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	// Should show "3d 2h" (approximately)
	assert.Contains(t, output, "3d 2h", "expected human-readable duration for 74-hour-old secret")
}

func TestStatusCmd_NoScheduleShowsNone(t *testing.T) {
	origConfig := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "API_KEY", Type: "generic", Schedule: ""},
		},
	}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "none")
	assert.Contains(t, output, "-")
}

func TestStatusCmd_ScheduleShowsNextRotation(t *testing.T) {
	origConfig := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "DB_PASSWORD", Type: "postgres", Schedule: "0 3 * * *"},
		},
	}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	// Should show the cron expression
	assert.Contains(t, output, "0 3 * * *")
	// Should NOT show "-" for next rotation when schedule is set
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "DB_PASSWORD") {
			assert.NotContains(t, line, "\t-\t", "next rotation should be computed, not '-'")
			break
		}
	}
}

func TestStatusCmd_SummaryLine(t *testing.T) {
	origConfig := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "SECRET_A", Type: "generic"},
			{Name: "SECRET_B", Type: "postgres"},
			{Name: "SECRET_C", Type: "mysql"},
		},
	}
	defer func() { AppConfig = origConfig }()

	cmd := NewStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "3 secrets configured")
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"minutes only", 45 * time.Minute, "45m"},
		{"hours and minutes", 3*time.Hour + 15*time.Minute, "3h 15m"},
		{"days and hours", 3*24*time.Hour + 2*time.Hour, "3d 2h"},
		{"exactly one day", 24 * time.Hour, "1d 0h"},
		{"zero", 0, "0m"},
		{"just under an hour", 59 * time.Minute, "59m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}
