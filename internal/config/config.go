package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level configuration structure for rotator.
type Config struct {
	MasterKeyEnv  string         `koanf:"master_key_env"`
	ComposeFile   string         `koanf:"compose_file"`
	Secrets       []SecretConfig `koanf:"secrets"`
	Notifications []NotifyConfig `koanf:"notifications"`

	// Path is the file this configuration was loaded from. Empty in
	// zero-config mode. It anchors relative paths and the data directory.
	Path string `koanf:"-"`

	// Warnings collects non-fatal configuration problems (deprecated keys,
	// ignored fields) so the CLI can surface them without failing.
	Warnings []string `koanf:"-"`

	// searched records the locations probed during discovery, used to build
	// an actionable error when a command requires a configuration file.
	searched []string
}

// SecretConfig defines a single secret to be managed.
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

// NotifyConfig defines a notification target.
type NotifyConfig struct {
	Type string `koanf:"type"`
	URL  string `koanf:"url"`
}

// Load reads configuration from a YAML file (if path is non-empty) and overlays
// ROTATOR_ prefixed environment variables. When path is empty, it returns a
// default configuration (zero-config mode per DISC-03).
//
// Most callers should use LoadDiscovered, which additionally probes the
// well-known configuration locations.
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

	if err := validate(&cfg); err != nil {
		return nil, err
	}

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

// normalize rewrites deprecated keys onto their canonical names and pushes
// top-level secret settings down into the provider options map.
//
// Without this, secrets[].length is silently ignored: providers read the
// length from Options, which only ever carried the provider map.
func normalize(cfg *Config) {
	base := cfg.Dir()

	for i := range cfg.Secrets {
		s := &cfg.Secrets[i]

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

		// Propagate the secret-level length into provider options, which is
		// where every provider actually reads it from. An explicit
		// provider.length wins so the more specific setting is respected.
		if s.Length > 0 {
			if _, ok := s.Provider["length"]; !ok {
				s.Provider["length"] = fmt.Sprintf("%d", s.Length)
			}
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
