package config

import (
	"fmt"
	"strconv"
	"strings"
)

var validTypes = map[string]bool{
	"mysql":    true,
	"postgres": true,
	"redis":    true,
	"generic":  true,
}

// validTypeList is the human-readable list used in error messages.
const validTypeList = "mysql, postgres, redis, generic"

// validate checks that the configuration is well-formed. Errors name the
// offending secret and state exactly which key to add, so a misconfiguration
// is fixable without reading the source.
func validate(cfg *Config) error {
	seen := make(map[string]int, len(cfg.Secrets))

	for i, s := range cfg.Secrets {
		if s.Name == "" {
			return fmt.Errorf("secrets[%d]: name is required", i)
		}
		if prev, dup := seen[s.Name]; dup {
			return fmt.Errorf("secrets[%d] %q: duplicate name (already defined at secrets[%d])", i, s.Name, prev)
		}
		seen[s.Name] = i

		if s.Type == "" {
			return fmt.Errorf("secrets[%d] %q: type is required (valid: %s)", i, s.Name, validTypeList)
		}
		if !validTypes[s.Type] {
			return fmt.Errorf("secrets[%d] %q: invalid type %q (valid: %s)", i, s.Name, s.Type, validTypeList)
		}
		if s.EnvKey == "" {
			return fmt.Errorf("secrets[%d] %q: env_key is required", i, s.Name)
		}
		if s.EnvFile == "" && len(s.EnvFiles) == 0 {
			return fmt.Errorf("secrets[%d] %q: env_file or env_files is required", i, s.Name)
		}
		if s.Length < 0 {
			return fmt.Errorf("secrets[%d] %q: length must be positive, got %d", i, s.Name, s.Length)
		}
		if key := placeholderKey(s); key != "" {
			return fmt.Errorf(
				"secrets[%d] %q: %s is still the TODO placeholder left by 'rotator init'; replace it with a real value",
				i, s.Name, key)
		}
		if err := validateProvider(cfg, i, s); err != nil {
			return err
		}
	}
	return nil
}

// placeholderKey returns the name of the first field still holding the TODO
// marker that 'rotator init' writes, or "" when none does. Catching it here
// turns a confusing DNS failure at 3am into a clear error at load time.
func placeholderKey(s SecretConfig) string {
	for key, value := range s.Provider {
		if strings.EqualFold(strings.TrimSpace(value), "TODO") {
			return "provider." + key
		}
	}
	return ""
}

// validateProvider enforces the connection settings each provider type needs
// to reach its backing service.
//
// Only settings whose absence makes rotation impossible are hard errors.
// Settings that merely indicate a questionable setup (an admin account with no
// password, an unspecified Postgres database) are reported as warnings so that
// legitimate development stacks keep working.
func validateProvider(cfg *Config, i int, s SecretConfig) error {
	if v, ok := s.Provider["port"]; ok && v != "" {
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("secrets[%d] %q: provider.port must be a number, got %q", i, s.Name, v)
		}
	}

	switch s.Type {
	case "generic":
		return nil

	case "redis":
		if s.Provider["host"] == "" {
			return fmt.Errorf("secrets[%d] %q: provider.host is required for type redis", i, s.Name)
		}

	case "mysql", "postgres":
		if s.Provider["host"] == "" {
			return fmt.Errorf("secrets[%d] %q: provider.host is required for type %s", i, s.Name, s.Type)
		}
		if s.Provider["username"] == "" {
			return fmt.Errorf(
				"secrets[%d] %q: provider.username is required for type %s (the admin account used to run ALTER USER/ROLE)",
				i, s.Name, s.Type)
		}
		if s.Provider["password"] == "" && s.Provider["password_env"] == "" {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
				"secret %q: neither provider.password nor provider.password_env is set; connecting to %s as %q with an empty password",
				s.Name, s.Type, s.Provider["username"]))
		}
		if s.Type == "postgres" && s.Provider["database"] == "" {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
				"secret %q: provider.database is not set; the server default will be used (set it to e.g. 'postgres' to be explicit)",
				s.Name))
		}
	}

	return nil
}
