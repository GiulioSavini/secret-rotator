package config

import "fmt"

var validTypes = map[string]bool{
	"mysql":    true,
	"postgres": true,
	"redis":    true,
	"generic":  true,
}

// validate checks that the configuration is well-formed.
func validate(cfg *Config) error {
	for i, s := range cfg.Secrets {
		if s.Name == "" {
			return fmt.Errorf("secrets[%d]: name is required", i)
		}
		if s.Type != "" && !validTypes[s.Type] {
			return fmt.Errorf("secrets[%d] %q: invalid type %q (valid: mysql, postgres, redis, generic)", i, s.Name, s.Type)
		}
		if s.EnvKey == "" {
			return fmt.Errorf("secrets[%d] %q: env_key is required", i, s.Name)
		}
		if s.EnvFile == "" && len(s.EnvFiles) == 0 {
			return fmt.Errorf("secrets[%d] %q: env_file or env_files is required", i, s.Name)
		}
	}
	return nil
}
