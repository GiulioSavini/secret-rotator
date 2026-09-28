package domain

import (
	"fmt"
	"strings"
)

// SecretName identifies a managed secret. It is the handle used on the command
// line, in schedules and in the audit trail, so it has to be stable and free of
// whitespace.
type SecretName string

// NewSecretName validates and builds a secret name.
func NewSecretName(raw string) (SecretName, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidSecret)
	}
	if strings.ContainsAny(trimmed, " \t\n") {
		return "", fmt.Errorf("%w: name %q must not contain whitespace", ErrInvalidSecret, raw)
	}
	return SecretName(trimmed), nil
}

func (n SecretName) String() string { return string(n) }

// EnvKey is the name of the environment variable holding a secret's value.
type EnvKey string

// NewEnvKey validates and builds an env key. The accepted shape is the one
// POSIX shells and Docker Compose agree on.
func NewEnvKey(raw string) (EnvKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: env_key is required", ErrInvalidSecret)
	}
	for i, r := range trimmed {
		valid := r == '_' ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(i > 0 && r >= '0' && r <= '9')
		if !valid {
			return "", fmt.Errorf(
				"%w: env_key %q is not a valid environment variable name", ErrInvalidSecret, raw)
		}
	}
	return EnvKey(trimmed), nil
}

func (k EnvKey) String() string { return string(k) }

// FilePath is the location of an .env file holding a secret.
type FilePath string

func (p FilePath) String() string { return string(p) }

// ContainerRef names a container to restart. It is either a real container
// name or a Compose service name; resolving one to the other belongs to the
// container adapter, not here.
type ContainerRef string

func (c ContainerRef) String() string { return string(c) }

// PasswordLength is the number of random bytes behind a generated secret,
// before encoding. It is not the length of the resulting string.
type PasswordLength int

const (
	// DefaultPasswordLength is used when a secret does not specify one.
	DefaultPasswordLength PasswordLength = 32
	// MinPasswordLength is the floor below which a generated secret would be
	// indefensible. It is deliberately low rather than best-practice, so that
	// upgrading does not reject a configuration that already worked; 32 bytes
	// remains the default for anything that does not ask otherwise.
	MinPasswordLength PasswordLength = 8
	// MaxPasswordLength guards against configurations that would produce
	// values some services refuse to store.
	MaxPasswordLength PasswordLength = 256
)

// NewPasswordLength validates a length in bytes. Zero selects the default,
// which keeps an omitted field meaning "whatever is sensible".
func NewPasswordLength(n int) (PasswordLength, error) {
	if n == 0 {
		return DefaultPasswordLength, nil
	}
	l := PasswordLength(n)
	if l < MinPasswordLength || l > MaxPasswordLength {
		return 0, fmt.Errorf("%w: length %d must be between %d and %d bytes",
			ErrInvalidSecret, n, MinPasswordLength, MaxPasswordLength)
	}
	return l, nil
}

func (l PasswordLength) Int() int { return int(l) }

// Schedule is a cron expression. The domain only cares whether a secret is
// scheduled at all; interpreting the expression is the scheduler's job, so the
// text is carried verbatim.
type Schedule string

// NewSchedule builds a schedule. An empty value is valid and means "on demand
// only".
func NewSchedule(raw string) Schedule { return Schedule(strings.TrimSpace(raw)) }

// IsSet reports whether the secret rotates automatically.
func (s Schedule) IsSet() bool { return s != "" }

func (s Schedule) String() string { return string(s) }
