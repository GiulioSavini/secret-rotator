// Package configfile loads rotator.yml and translates it into domain
// aggregates.
//
// It is a driven adapter and the only place in the module that knows the file
// format exists: the structs here mirror the YAML, translate.go maps them onto
// the domain, and nothing above this package sees a koanf tag.
package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/giulio/secret-rotator/internal/domain"
)

// Config is the parsed rotator.yml, plus what was learned while loading it.
type Config struct {
	MasterKeyEnv  string         `koanf:"master_key_env"`
	ComposeFile   string         `koanf:"compose_file"`
	Definitions   []SecretConfig `koanf:"secrets"`
	Notifications []NotifyConfig `koanf:"notifications"`

	// Path is the file this configuration was loaded from. Empty in
	// zero-config mode. It anchors relative paths and the data directory.
	Path string `koanf:"-"`

	// Warnings collects non-fatal problems (deprecated keys, questionable
	// setups) so the CLI can surface them without failing.
	Warnings []string `koanf:"-"`

	// secrets holds the translated aggregates, built once at load time.
	secrets []*domain.Secret

	// searched records the locations probed during discovery, used to build
	// an actionable error when a command requires a configuration file.
	searched []string
}

// SecretConfig mirrors one entry under `secrets:`.
type SecretConfig struct {
	Name       string            `koanf:"name"`
	Type       string            `koanf:"type"`
	EnvKey     string            `koanf:"env_key"`
	EnvFile    string            `koanf:"env_file"`
	EnvFiles   []string          `koanf:"env_files"`
	Containers []string          `koanf:"containers"`
	Provider   map[string]string `koanf:"provider"`
	Schedule   string            `koanf:"schedule"`
	Length     int               `koanf:"length"`
}

// NotifyConfig mirrors one entry under `notifications:`.
type NotifyConfig struct {
	Type string `koanf:"type"`
	URL  string `koanf:"url"`
}

// New builds a configuration around already-validated aggregates, for callers
// that assemble one in memory rather than reading a file.
func New(secrets ...*domain.Secret) *Config {
	return &Config{secrets: secrets}
}

// Secrets returns the domain aggregates this configuration describes.
func (c *Config) Secrets() []*domain.Secret {
	if c == nil {
		return nil
	}
	return c.secrets
}

// Find returns the secret with the given name.
func (c *Config) Find(name string) (*domain.Secret, bool) {
	for _, s := range c.Secrets() {
		if s.Name().String() == name {
			return s, true
		}
	}
	return nil, false
}

// SecretNames lists the configured names, for error messages.
func (c *Config) SecretNames() []string {
	names := make([]string, 0, len(c.Secrets()))
	for _, s := range c.Secrets() {
		names = append(names, s.Name().String())
	}
	return names
}

// Load reads a YAML file, overlays ROTATOR_ prefixed environment variables,
// and translates the result into domain aggregates.
//
// An empty path returns the zero-config default, which is what lets `scan` and
// `init` run in a directory that has no rotator.yml yet. Most callers should
// use LoadDiscovered, which additionally probes the well-known locations.
func Load(path string) (*Config, error) {
	if path == "" {
		return DefaultConfig(), nil
	}

	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	// Overlay with ROTATOR_ prefixed env vars.
	// Only strip the prefix and lowercase; do not replace underscores with dots
	// because top-level koanf keys like "master_key_env" use underscores.
	_ = k.Load(env.Provider("ROTATOR_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "ROTATOR_"))
	}), nil)

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	cfg.Path = path
	normalize(&cfg)

	secrets, err := cfg.DomainSecrets()
	if err != nil {
		return nil, err
	}
	cfg.secrets = secrets

	return &cfg, nil
}

// ComposeFileNames lists the Compose file names probed when compose_file is
// not set explicitly, in the order Docker Compose itself uses.
var ComposeFileNames = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

// Dir returns the directory holding the configuration file, or "." in
// zero-config mode. Relative paths in the configuration are resolved against
// it so that behaviour does not depend on the caller's working directory.
func (c *Config) Dir() string {
	if c == nil || c.Path == "" {
		return "."
	}
	return filepath.Dir(c.Path)
}

// ResolveComposeFile returns the Compose file to use for dependency ordering,
// or "" when there is none. An explicit compose_file that does not exist is
// reported as an error, since silently ignoring it would change restart order
// without telling anyone.
func (c *Config) ResolveComposeFile() (string, error) {
	if c == nil {
		return "", nil
	}
	if c.ComposeFile != "" {
		path := c.ComposeFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.Dir(), path)
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("compose_file %s: %w", c.ComposeFile, err)
		}
		return path, nil
	}
	for _, name := range ComposeFileNames {
		candidate := filepath.Join(c.Dir(), name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", nil
}

// deprecatedProviderKeys maps superseded provider keys to their replacement.
// The old key keeps working but produces a warning, so existing configs do
// not break on upgrade.
var deprecatedProviderKeys = map[string]string{
	"user": "username",
	"pass": "password",
}

// normalize rewrites the loaded file into its canonical shape before it is
// translated: deprecated keys onto their current names, relative paths onto
// the configuration's own directory.
func normalize(cfg *Config) {
	base := cfg.Dir()

	for i := range cfg.Definitions {
		s := &cfg.Definitions[i]

		if s.Provider == nil {
			s.Provider = map[string]string{}
		}

		// Anchor env file paths to the configuration file rather than the
		// working directory, so a scheduled run and a manual run from another
		// directory touch the same files.
		s.EnvFile = resolveRelative(base, s.EnvFile)
		for j, p := range s.EnvFiles {
			s.EnvFiles[j] = resolveRelative(base, p)
		}

		for old, canonical := range deprecatedProviderKeys {
			val, hasOld := s.Provider[old]
			if !hasOld || val == "" {
				continue
			}
			if existing, hasNew := s.Provider[canonical]; hasNew && existing != "" {
				cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
					"secret %q: provider.%s and provider.%s are both set; using provider.%s",
					s.Name, old, canonical, canonical))
			} else {
				s.Provider[canonical] = val
				cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
					"secret %q: provider.%s is deprecated, rename it to provider.%s",
					s.Name, old, canonical))
			}
			delete(s.Provider, old)
		}
	}
}

// resolveRelative joins a relative path onto base, leaving empty and absolute
// paths untouched.
func resolveRelative(base, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
