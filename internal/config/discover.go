package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvConfigPath is the environment variable that overrides config discovery.
const EnvConfigPath = "ROTATOR_CONFIG"

// EnvDataDir is the environment variable that overrides the data directory.
const EnvDataDir = "ROTATOR_DATA_DIR"

// searchPaths lists the locations probed for a configuration file, in order.
// Relative entries are resolved against the working directory so that running
// rotator from a project root just works; the absolute entries cover the
// container layout shipped in the Dockerfile and a system-wide install.
var searchPaths = []string{
	"rotator.yml",
	"rotator.yaml",
	".rotator.yml",
	"/config/rotator.yml",
	"/config/rotator.yaml",
	"/etc/rotator/rotator.yml",
	"/etc/rotator/rotator.yaml",
}

// ErrNoConfig is returned by Discover when no configuration file is found.
// It carries the probed locations so callers can render an actionable message.
type ErrNoConfig struct {
	Searched []string
}

func (e *ErrNoConfig) Error() string {
	return fmt.Sprintf("no configuration file found (searched: %s)", strings.Join(e.Searched, ", "))
}

// Discover returns the path of the configuration file to use.
//
// An explicit path always wins and must exist. Otherwise ROTATOR_CONFIG is
// honoured, and finally the well-known locations in searchPaths are probed.
// When nothing is found it returns an *ErrNoConfig listing what was tried;
// callers that can operate without a config (scan, init) may ignore it.
func Discover(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("config file %s: %w", explicit, err)
		}
		return explicit, nil
	}

	if fromEnv := os.Getenv(EnvConfigPath); fromEnv != "" {
		if _, err := os.Stat(fromEnv); err != nil {
			return "", fmt.Errorf("config file %s (from %s): %w", fromEnv, EnvConfigPath, err)
		}
		return fromEnv, nil
	}

	searched := make([]string, 0, len(searchPaths))
	for _, candidate := range searchPaths {
		abs := candidate
		if !filepath.IsAbs(candidate) {
			if cwd, err := os.Getwd(); err == nil {
				abs = filepath.Join(cwd, candidate)
			}
		}
		searched = append(searched, abs)
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			return abs, nil
		}
	}

	return "", &ErrNoConfig{Searched: searched}
}

// LoadDiscovered resolves the configuration path and loads it.
//
// The returned path is empty when no configuration exists; in that case the
// returned Config is the zero-config default and err is nil, so commands that
// work without a config (scan, init) keep functioning. Commands that require
// secrets should call Config.RequireSecrets.
func LoadDiscovered(explicit string) (*Config, string, error) {
	path, err := Discover(explicit)
	if err != nil {
		var missing *ErrNoConfig
		if explicit == "" && asErrNoConfig(err, &missing) {
			cfg := DefaultConfig()
			cfg.searched = missing.Searched
			return cfg, "", nil
		}
		return nil, "", err
	}

	cfg, err := Load(path)
	if err != nil {
		return nil, "", err
	}
	cfg.Path = path
	return cfg, path, nil
}

// asErrNoConfig reports whether err is an *ErrNoConfig, storing it in target.
func asErrNoConfig(err error, target **ErrNoConfig) bool {
	if e, ok := err.(*ErrNoConfig); ok {
		*target = e
		return true
	}
	return false
}

// DataDir returns the directory used for mutable state (the rotation history).
//
// Precedence: explicit flag, then ROTATOR_DATA_DIR, then a ".rotator"
// directory next to the configuration file, then ".rotator" in the working
// directory. Keeping state next to the config means a single mounted volume
// covers both in the container image.
func (c *Config) DataDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if fromEnv := os.Getenv(EnvDataDir); fromEnv != "" {
		return fromEnv
	}
	if c != nil && c.Path != "" {
		return filepath.Join(filepath.Dir(c.Path), ".rotator")
	}
	return ".rotator"
}

// HistoryPath returns the full path of the encrypted rotation history file.
func (c *Config) HistoryPath(explicitDataDir string) string {
	return filepath.Join(c.DataDir(explicitDataDir), "history.json")
}

// RequireSecrets returns an actionable error when no secrets are configured.
// The message names the locations that were probed so the user knows exactly
// where to put the file.
func (c *Config) RequireSecrets() error {
	if c != nil && len(c.Secrets) > 0 {
		return nil
	}
	if c != nil && c.Path != "" {
		return fmt.Errorf(
			"configuration required: %s defines no secrets; add at least one entry under 'secrets:' (run 'rotator init' to generate one)",
			c.Path)
	}
	searched := searchPaths
	if c != nil && len(c.searched) > 0 {
		searched = c.searched
	}
	return fmt.Errorf(
		"configuration required: no rotator.yml found; searched:\n  %s\n\nRun 'rotator init' to generate one, or pass --config /path/to/rotator.yml",
		strings.Join(searched, "\n  "),
	)
}
