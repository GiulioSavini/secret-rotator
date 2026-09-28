package domain

import "fmt"

// Secret is the aggregate the whole application turns around: one credential,
// the files that carry it, the service that owns it and the containers that
// have to be restarted once it changes.
//
// Its fields are unexported and it can only be built through NewSecret, so an
// instance that exists is an instance whose invariants hold. Code downstream
// does not re-check them.
type Secret struct {
	name       SecretName
	kind       Kind
	envKey     EnvKey
	envFiles   []FilePath
	containers []ContainerRef
	target     Target
	schedule   Schedule
	length     PasswordLength
}

// SecretSpec is the unvalidated input to NewSecret. Adapters translate their
// own formats (YAML today) into this shape; the validation lives in one place
// regardless of where the definition came from.
type SecretSpec struct {
	Name       string
	Kind       string
	EnvKey     string
	EnvFiles   []string
	Containers []string
	Target     Target
	Schedule   string
	Length     int
}

// NewSecret validates a specification and builds the aggregate.
//
// Warnings describe setups that are workable but questionable; they are
// returned rather than logged so the caller decides how to surface them.
func NewSecret(spec SecretSpec) (secret *Secret, warnings []string, err error) {
	name, err := NewSecretName(spec.Name)
	if err != nil {
		return nil, nil, err
	}

	kind, err := ParseKind(spec.Kind)
	if err != nil {
		return nil, nil, fmt.Errorf("secret %q: %w", name, err)
	}

	envKey, err := NewEnvKey(spec.EnvKey)
	if err != nil {
		return nil, nil, fmt.Errorf("secret %q: %w", name, err)
	}

	envFiles, err := newEnvFileSet(name, spec.EnvFiles)
	if err != nil {
		return nil, nil, err
	}

	length, err := NewPasswordLength(spec.Length)
	if err != nil {
		return nil, nil, fmt.Errorf("secret %q: %w", name, err)
	}

	targetWarnings, err := spec.Target.validateFor(kind, name)
	if err != nil {
		return nil, nil, err
	}

	containers := make([]ContainerRef, 0, len(spec.Containers))
	for _, c := range spec.Containers {
		if c == "" {
			return nil, nil, fmt.Errorf("%w: secret %q: containers must not contain an empty entry",
				ErrInvalidSecret, name)
		}
		containers = append(containers, ContainerRef(c))
	}

	return &Secret{
		name:       name,
		kind:       kind,
		envKey:     envKey,
		envFiles:   envFiles,
		containers: containers,
		target:     spec.Target,
		schedule:   NewSchedule(spec.Schedule),
		length:     length,
	}, targetWarnings, nil
}

// newEnvFileSet validates the files a secret lives in: at least one, no
// duplicates, no empty entries.
func newEnvFileSet(name SecretName, raw []string) ([]FilePath, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: secret %q: env_file or env_files is required", ErrInvalidSecret, name)
	}
	seen := make(map[string]bool, len(raw))
	paths := make([]FilePath, 0, len(raw))
	for _, p := range raw {
		if p == "" {
			return nil, fmt.Errorf("%w: secret %q: env file path must not be empty", ErrInvalidSecret, name)
		}
		if seen[p] {
			return nil, fmt.Errorf("%w: secret %q: env file %s is listed twice", ErrInvalidSecret, name, p)
		}
		seen[p] = true
		paths = append(paths, FilePath(p))
	}
	return paths, nil
}

// Name returns the secret's identifier.
func (s *Secret) Name() SecretName { return s.name }

// Kind returns the type of backing service.
func (s *Secret) Kind() Kind { return s.kind }

// EnvKey returns the environment variable holding the value.
func (s *Secret) EnvKey() EnvKey { return s.envKey }

// PrimaryEnvFile is the file the current value is read from. When a secret
// spans several files they are expected to agree; this one is the reference.
func (s *Secret) PrimaryEnvFile() FilePath { return s.envFiles[0] }

// EnvFiles returns every file carrying the secret, primary first.
func (s *Secret) EnvFiles() []FilePath {
	out := make([]FilePath, len(s.envFiles))
	copy(out, s.envFiles)
	return out
}

// Containers returns the containers to restart after a rotation.
func (s *Secret) Containers() []ContainerRef {
	out := make([]ContainerRef, len(s.containers))
	copy(out, s.containers)
	return out
}

// WithContainers returns a copy whose restart list has been reordered or
// resolved. Used by the container adapter to apply dependency order without
// mutating the aggregate other callers hold.
func (s *Secret) WithContainers(refs []ContainerRef) *Secret {
	clone := *s
	clone.containers = make([]ContainerRef, len(refs))
	copy(clone.containers, refs)
	return &clone
}

// Target returns the backing service's connection details.
func (s *Secret) Target() Target { return s.target }

// WithTarget returns a copy carrying an updated target, used when the
// application layer resolves the admin password.
func (s *Secret) WithTarget(t Target) *Secret {
	clone := *s
	clone.target = t
	return &clone
}

// Schedule returns the cron expression, which may be unset.
func (s *Secret) Schedule() Schedule { return s.schedule }

// WithSchedule returns a copy carrying a different schedule, used when a
// container label overrides the configured one.
func (s *Secret) WithSchedule(sc Schedule) *Secret {
	clone := *s
	clone.schedule = sc
	return &clone
}

// Length returns the entropy, in bytes, of the value to generate.
func (s *Secret) Length() PasswordLength { return s.length }

// NeedsVerification reports whether the new credential must be proven to work
// against the backing service before the .env files are updated.
func (s *Secret) NeedsVerification() bool { return s.kind.HasBackingService() }
