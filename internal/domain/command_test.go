package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func commandSpec(mutate func(*SecretSpec)) SecretSpec {
	spec := SecretSpec{
		Name:     "minio_key",
		Kind:     "command",
		EnvKey:   "MINIO_SECRET_KEY",
		EnvFiles: []string{".env"},
		Target: Target{
			Command: CommandSpec{Apply: []string{"/opt/rotate.sh"}},
		},
	}
	if mutate != nil {
		mutate(&spec)
	}
	return spec
}

func TestCommandKindNeedsAnApplyCommand(t *testing.T) {
	_, _, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{}
	}))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSecret)
	assert.Contains(t, err.Error(), "command.apply is required")
}

func TestCommandKindNeedsNoHostOrAccount(t *testing.T) {
	// The script decides what it talks to, so the connection fields a
	// database needs do not apply.
	secret, _, err := NewSecret(commandSpec(nil))
	require.NoError(t, err)
	assert.Equal(t, KindCommand, secret.Kind())
	assert.True(t, secret.Kind().UsesCommands())
	assert.True(t, secret.NeedsVerification())
}

func TestAShellLineIsRejectedWithAnExplanation(t *testing.T) {
	// A single string with spaces means someone expected a shell. Running
	// /bin/sh -c for them would be worse than saying so.
	_, _, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{Apply: []string{"/opt/rotate.sh --user app"}}
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "argv lists, not shell")
	assert.Contains(t, err.Error(), `["/bin/sh", "-c"`)
}

func TestAnArgvListWithSpacedArgumentsIsFine(t *testing.T) {
	_, _, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{Apply: []string{"/opt/rotate.sh", "--note", "two words"}}
	}))
	require.NoError(t, err)
}

func TestAnEmptyProgramIsRejected(t *testing.T) {
	_, _, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{Apply: []string{"  "}}
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty program")
}

func TestANullByteIsRejected(t *testing.T) {
	_, _, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{Apply: []string{"/opt/rotate.sh", "bad\x00arg"}}
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null byte")
}

func TestANegativeTimeoutIsRejected(t *testing.T) {
	_, _, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command.Timeout = -time.Second
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be negative")
}

func TestMissingVerifyIsWarnedAbout(t *testing.T) {
	// Writing a credential nobody proved is worth saying out loud.
	_, warnings, err := NewSecret(commandSpec(nil))
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "no command.verify")
}

func TestMissingRestoreIsNotWarnedAbout(t *testing.T) {
	// Re-applying the previous value is the right fallback for most
	// services, so it is silent: a warning on every single command would
	// teach the reader to skip the ones that matter.
	_, warnings, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{
			Apply:  []string{"/opt/rotate.sh"},
			Verify: []string{"/opt/verify.sh"},
		}
	}))
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestAFullSpecWarnsAboutNothing(t *testing.T) {
	_, warnings, err := NewSecret(commandSpec(func(s *SecretSpec) {
		s.Target.Command = CommandSpec{
			Apply:   []string{"/opt/rotate.sh"},
			Verify:  []string{"/opt/verify.sh"},
			Restore: []string{"/opt/rotate.sh", "--restore"},
		}
	}))
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestRestoreFallsBackToApply(t *testing.T) {
	spec := CommandSpec{Apply: []string{"/opt/rotate.sh"}}
	argv, dedicated := spec.RestoreCommand()
	assert.Equal(t, []string{"/opt/rotate.sh"}, argv)
	assert.False(t, dedicated)

	spec.Restore = []string{"/opt/undo.sh"}
	argv, dedicated = spec.RestoreCommand()
	assert.Equal(t, []string{"/opt/undo.sh"}, argv)
	assert.True(t, dedicated)
}

func TestCommandTimeoutDefaults(t *testing.T) {
	assert.Equal(t, DefaultCommandTimeout, CommandSpec{}.EffectiveTimeout())
	assert.Equal(t, 90*time.Second, CommandSpec{Timeout: 90 * time.Second}.EffectiveTimeout())
}

func TestACommandBlockOnAnotherKindIsReportedNotSilentlyIgnored(t *testing.T) {
	_, warnings, err := NewSecret(SecretSpec{
		Name:     "api_key",
		Kind:     "generic",
		EnvKey:   "API_KEY",
		EnvFiles: []string{".env"},
		Target:   Target{Command: CommandSpec{Apply: []string{"/opt/rotate.sh"}}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "type: command")
}

func TestCommandIsAValidKind(t *testing.T) {
	kind, err := ParseKind("command")
	require.NoError(t, err)
	assert.Equal(t, KindCommand, kind)
	assert.Contains(t, KindList(), "command")
}
