package configfile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/giulio/secret-rotator/internal/domain"
)

// This file is the anti-corruption layer between the YAML file and the model.
//
// Everything above it speaks the configuration format: snake_case keys, string
// values, deprecated spellings. Everything below it speaks the domain. Because
// the translation happens here, adding a key to the file never reaches the
// domain, and renaming something in the domain never breaks an existing file.

// providerKeys maps the keys accepted under `provider:` onto their meaning.
// Listing them serves two purposes: translation, and catching typos, since a
// key that is not here is reported rather than silently ignored.
var providerKeys = map[string]string{
	"host":             "host",
	"port":             "port",
	"username":         "username",
	"password":         "password",
	"password_env":     "password_env",
	"database":         "database",
	"target_user":      "target_user",
	"target_user_host": "target_user_host",
	"length":           "length",
}

// DomainSecrets translates the loaded configuration into domain aggregates.
//
// Every secret is translated; the first invalid one aborts, because a daemon
// that silently skips a secret it cannot parse is worse than one that refuses
// to start.
func (c *Config) DomainSecrets() ([]*domain.Secret, error) {
	secrets := make([]*domain.Secret, 0, len(c.Definitions))
	seen := make(map[string]int, len(c.Definitions))

	for i, sc := range c.Definitions {
		if prev, dup := seen[sc.Name]; dup && sc.Name != "" {
			return nil, fmt.Errorf("secrets[%d] %q: duplicate name (already defined at secrets[%d])", i, sc.Name, prev)
		}
		seen[sc.Name] = i

		spec, err := c.specFor(i, sc)
		if err != nil {
			return nil, err
		}

		secret, warnings, err := domain.NewSecret(spec)
		if err != nil {
			return nil, fmt.Errorf("secrets[%d]: %w", i, err)
		}
		c.Warnings = append(c.Warnings, warnings...)
		secrets = append(secrets, secret)
	}

	return secrets, nil
}

// specFor turns one YAML entry into the unvalidated spec the domain accepts.
func (c *Config) specFor(index int, sc SecretConfig) (domain.SecretSpec, error) {
	target, length, err := c.targetFor(index, sc)
	if err != nil {
		return domain.SecretSpec{}, err
	}

	return domain.SecretSpec{
		Name:       sc.Name,
		Kind:       sc.Type,
		EnvKey:     sc.EnvKey,
		EnvFiles:   envFilesOf(sc),
		Containers: sc.Containers,
		Target:     target,
		Schedule:   sc.Schedule,
		Length:     length,
	}, nil
}

// targetFor translates the provider map into a typed target, and returns the
// effective password length, which may come from either the secret or the
// provider block.
func (c *Config) targetFor(index int, sc SecretConfig) (domain.Target, int, error) {
	length := sc.Length

	var unknown []string
	for key := range sc.Provider {
		if _, ok := providerKeys[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"secret %q: unrecognised provider key(s) %s; they are ignored",
			sc.Name, strings.Join(unknown, ", ")))
	}

	target := domain.Target{
		Host:             sc.Provider["host"],
		AdminUser:        sc.Provider["username"],
		AdminPasswordEnv: sc.Provider["password_env"],
		Database:         sc.Provider["database"],
		TargetUser:       sc.Provider["target_user"],
		TargetUserHost:   sc.Provider["target_user_host"],
	}

	if pw := sc.Provider["password"]; pw != "" {
		target.AdminPassword = domain.NewCredential(pw)
	}

	if raw := sc.Provider["port"]; raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return domain.Target{}, 0, fmt.Errorf(
				"secrets[%d] %q: provider.port must be a number, got %q", index, sc.Name, raw)
		}
		target.Port = port
	}

	// provider.length predates the top-level field and still wins, so that a
	// configuration written against the old shape keeps its behaviour.
	if raw := sc.Provider["length"]; raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return domain.Target{}, 0, fmt.Errorf(
				"secrets[%d] %q: provider.length must be a number, got %q", index, sc.Name, raw)
		}
		length = parsed
	}

	if key := placeholderKey(sc); key != "" {
		return domain.Target{}, 0, fmt.Errorf(
			"secrets[%d] %q: %s is still the TODO placeholder left by 'rotator init'; replace it with a real value",
			index, sc.Name, key)
	}

	return target, length, nil
}

// envFilesOf returns the files a secret lives in, accepting both the singular
// and the plural spelling.
func envFilesOf(sc SecretConfig) []string {
	if sc.EnvFile != "" {
		return []string{sc.EnvFile}
	}
	return sc.EnvFiles
}

// placeholderKey returns the name of the first provider field still holding
// the TODO marker that 'rotator init' writes, or "" when none does. Catching
// it here turns a confusing DNS failure at 3am into a clear error at load time.
func placeholderKey(sc SecretConfig) string {
	keys := make([]string, 0, len(sc.Provider))
	for key := range sc.Provider {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if strings.EqualFold(strings.TrimSpace(sc.Provider[key]), "TODO") {
			return "provider." + key
		}
	}
	return ""
}
