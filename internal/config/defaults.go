package config

// DefaultConfig returns the default configuration used when no config file
// is specified (zero-config mode per DISC-03).
func DefaultConfig() *Config {
	return &Config{
		Secrets:       []SecretConfig{},
		Notifications: []NotifyConfig{},
	}
}
