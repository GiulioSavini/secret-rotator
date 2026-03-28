package cli

import (
	"testing"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonCmd_Exists(t *testing.T) {
	root := NewRootCmd()

	// Verify daemon command is registered
	var found bool
	for _, cmd := range root.Commands() {
		if cmd.Use == "daemon" {
			found = true
			break
		}
	}
	assert.True(t, found, "daemon command should be registered on root")
}

func TestDaemonCmd_HasPassphraseFlag(t *testing.T) {
	cmd := NewDaemonCmd()
	flag := cmd.Flags().Lookup("passphrase")
	require.NotNil(t, flag, "--passphrase flag should exist")
	assert.Equal(t, "string", flag.Value.Type())
}

func TestDaemonCmd_RequiresConfig(t *testing.T) {
	// With no AppConfig set, daemon should fail
	old := AppConfig
	AppConfig = nil
	defer func() { AppConfig = old }()

	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err, "daemon should fail without config")
	assert.Contains(t, err.Error(), "configuration required")
}

func TestDaemonCmd_RequiresSchedules(t *testing.T) {
	old := AppConfig
	AppConfig = &config.Config{
		Secrets: []config.SecretConfig{
			{Name: "no-schedule", Type: "generic", Schedule: ""},
		},
	}
	defer func() { AppConfig = old }()

	cmd := NewDaemonCmd()
	err := cmd.RunE(cmd, nil)
	assert.Error(t, err, "daemon should fail without any schedules")
	assert.Contains(t, err.Error(), "no schedules found")
}
