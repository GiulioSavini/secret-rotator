package config

import (
	"fmt"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level configuration structure for rotator.
type Config struct {
	MasterKeyEnv  string         `koanf:"master_key_env"`
	Secrets       []SecretConfig `koanf:"secrets"`
	Notifications []NotifyConfig `koanf:"notifications"`
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

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
