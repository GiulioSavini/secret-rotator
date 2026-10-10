package domain

import (
	"fmt"
	"strings"
	"time"
)

// CommandSpec describes how to rotate a credential by running a program.
//
// It is what turns four built-in providers into an open set: MinIO, RabbitMQ,
// Keycloak, Grafana, an SSH authorized_keys file, any HTTP API with a token
// endpoint. Nobody has to write Go for those.
//
// Running a program named in a configuration file is also the most dangerous
// thing this tool does, so the shape here is deliberately narrow:
//
//   - a command is an argv list, never a shell string, so there is no shell to
//     inject into and no quoting to get wrong;
//   - the credential is never placed in argv, because /proc/<pid>/cmdline is
//     world-readable and `ps` would show it to every user on the host;
//   - the child does not inherit rotator's environment, which holds the master
//     passphrase; it gets a minimal one plus what PassEnv names explicitly.
type CommandSpec struct {
	// Apply sets the new credential on whatever owns it. Required.
	Apply []string
	// Verify proves the new credential works. Optional; without it a rotation
	// is applied unverified.
	Verify []string
	// Restore puts a previous credential back after a failure. Optional:
	// when absent, Apply is run again with the previous value, which is
	// correct whenever applying is idempotent. That fallback is deliberately
	// silent rather than a warning: it is the right behaviour for most
	// services, and nagging about it on every command would train the reader
	// to ignore warnings that do matter.
	Restore []string
	// Timeout bounds each invocation. Zero means DefaultCommandTimeout.
	Timeout time.Duration
	// PassEnv names the environment variables to forward from rotator's own
	// environment. Everything not listed is withheld.
	PassEnv []string
	// WorkingDir is where the commands run. Empty means the directory holding
	// the configuration file, so a relative script path resolves predictably.
	WorkingDir string
}

// DefaultCommandTimeout bounds one invocation when the spec does not say.
const DefaultCommandTimeout = 30 * time.Second

// Environment variables handed to every invocation. A script reads what it
// needs from these; nothing sensitive is ever passed as an argument.
const (
	// EnvCommandOperation is "apply", "verify" or "restore".
	EnvCommandOperation = "ROTATOR_OPERATION"
	// EnvCommandSecretName is the secret's configured name.
	EnvCommandSecretName = "ROTATOR_SECRET_NAME"
	// EnvCommandEnvKey is the environment variable the secret lives under.
	EnvCommandEnvKey = "ROTATOR_ENV_KEY"
	// EnvCommandNewSecret is the value being rotated to, for apply and verify.
	EnvCommandNewSecret = "ROTATOR_NEW_SECRET"
	// EnvCommandPreviousSecret is the value to put back, for restore.
	EnvCommandPreviousSecret = "ROTATOR_PREVIOUS_SECRET"
	// EnvCommandCurrentSecret is the value the service accepts right now, for
	// restore: it is what a script must authenticate with to undo the change.
	EnvCommandCurrentSecret = "ROTATOR_CURRENT_SECRET"
	// EnvCommandHost, EnvCommandPort and the rest carry the target, for
	// scripts that talk to a service.
	EnvCommandHost       = "ROTATOR_HOST"
	EnvCommandPort       = "ROTATOR_PORT"
	EnvCommandUsername   = "ROTATOR_USERNAME"
	EnvCommandTargetUser = "ROTATOR_TARGET_USER"
	EnvCommandDatabase   = "ROTATOR_DATABASE"
)

// IsSet reports whether a command provider has been configured.
func (c CommandSpec) IsSet() bool { return len(c.Apply) > 0 }

// EffectiveTimeout returns the per-invocation timeout.
func (c CommandSpec) EffectiveTimeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultCommandTimeout
}

// RestoreCommand returns the argv for undoing a rotation, and whether it is
// the dedicated restore command or a re-run of Apply.
func (c CommandSpec) RestoreCommand() (argv []string, dedicated bool) {
	if len(c.Restore) > 0 {
		return c.Restore, true
	}
	return c.Apply, false
}

// validate checks the spec, returning warnings for workable but questionable
// configurations.
func (c CommandSpec) validate(name SecretName) (warnings []string, err error) {
	if !c.IsSet() {
		return nil, fmt.Errorf(
			"%w: secret %q: command.apply is required for type %s", ErrInvalidSecret, name, KindCommand)
	}

	for label, argv := range map[string][]string{
		"apply":   c.Apply,
		"verify":  c.Verify,
		"restore": c.Restore,
	} {
		if err := validateArgv(name, label, argv); err != nil {
			return nil, err
		}
	}

	if c.Timeout < 0 {
		return nil, fmt.Errorf("%w: secret %q: command.timeout must not be negative", ErrInvalidSecret, name)
	}

	if len(c.Verify) == 0 {
		warnings = append(warnings, fmt.Sprintf(
			"secret %q: no command.verify, so the new value is written without being proven to work", name))
	}

	return warnings, nil
}

// validateArgv rejects the shapes that would make execution unpredictable.
func validateArgv(name SecretName, label string, argv []string) error {
	if len(argv) == 0 {
		return nil
	}

	program := strings.TrimSpace(argv[0])
	if program == "" {
		return fmt.Errorf("%w: secret %q: command.%s has an empty program", ErrInvalidSecret, name, label)
	}

	// A single string holding spaces is almost always someone expecting a
	// shell. Saying so is better than running "/bin/sh -c" for them or
	// failing with "no such file or directory".
	if len(argv) == 1 && strings.ContainsAny(program, " \t") {
		return fmt.Errorf(
			"%w: secret %q: command.%s looks like a shell line (%q). Commands are argv lists, not shell "+
				"strings: write [\"/bin/sh\", \"-c\", \"...\"] if you really need a shell",
			ErrInvalidSecret, name, label, program)
	}

	for i, arg := range argv {
		if strings.Contains(arg, "\x00") {
			return fmt.Errorf("%w: secret %q: command.%s argument %d contains a null byte",
				ErrInvalidSecret, name, label, i)
		}
	}

	return nil
}
