package domain

import "fmt"

// Target describes the backing service whose credential is being rotated.
//
// It replaces the untyped option bag the rotators used to parse individually:
// every rotator read "target_user", "password_env" and "length" out of a
// map[string]string, each with its own defaulting rules. Those rules now live
// here, once.
type Target struct {
	// Host is the address of the service as the rotator reaches it. Inside a
	// Compose network this is usually the service name.
	Host string
	// Port is the service port. Zero means "the default for the kind".
	Port int
	// AdminUser is the account used to change the credential.
	AdminUser string
	// AdminPassword is that account's current password. It may be empty until
	// the application layer resolves AdminPasswordEnv.
	AdminPassword Credential
	// AdminPasswordEnv names the environment variable, or .env entry, holding
	// the admin password.
	AdminPasswordEnv string
	// Database is the database to connect to, where the kind needs one.
	Database string
	// TargetUser is the account whose password changes. Empty means AdminUser,
	// which is the common case of an account rotating its own password.
	TargetUser string
	// TargetUserHost scopes the account for servers that identify accounts by
	// host, MySQL's 'user'@'host'. Empty means '%', matching any host, which
	// is how Compose stacks normally define their accounts.
	TargetUserHost string
}

// DefaultTargetUserHost is the host part used when none is configured.
const DefaultTargetUserHost = "%"

// Port resolution and target-user defaulting are the two rules every rotator
// used to reimplement; they are expressed once, here.

// EffectivePort returns the port to dial for the given kind.
func (t Target) EffectivePort(k Kind) int {
	if t.Port > 0 {
		return t.Port
	}
	return k.DefaultPort()
}

// EffectiveTargetUser returns the account whose password will change.
func (t Target) EffectiveTargetUser() string {
	if t.TargetUser != "" {
		return t.TargetUser
	}
	return t.AdminUser
}

// EffectiveTargetUserHost returns the host part of the account to alter, for
// servers that scope accounts by host.
func (t Target) EffectiveTargetUserHost() string {
	if t.TargetUserHost != "" {
		return t.TargetUserHost
	}
	return DefaultTargetUserHost
}

// WithAdminPassword returns a copy carrying a resolved admin password.
// Target is a value, so resolving a password never mutates shared state.
func (t Target) WithAdminPassword(c Credential) Target {
	t.AdminPassword = c
	return t
}

// Addr renders host:port for the given kind.
func (t Target) Addr(k Kind) string {
	return fmt.Sprintf("%s:%d", t.Host, t.EffectivePort(k))
}

// validateFor checks the target against what the kind needs to connect.
// Only genuinely blocking omissions are errors; questionable but workable
// setups are returned as warnings for the caller to surface.
func (t Target) validateFor(k Kind, name SecretName) (warnings []string, err error) {
	if !k.HasBackingService() {
		return nil, nil
	}
	if t.Host == "" {
		return nil, fmt.Errorf("%w: secret %q: provider.host is required for type %s",
			ErrInvalidSecret, name, k)
	}
	if k.RequiresAdminAccount() && t.AdminUser == "" {
		return nil, fmt.Errorf(
			"%w: secret %q: provider.username is required for type %s (the account used to change the password)",
			ErrInvalidSecret, name, k)
	}
	if k.RequiresAdminAccount() && t.AdminPassword.IsZero() && t.AdminPasswordEnv == "" {
		warnings = append(warnings, fmt.Sprintf(
			"secret %q: neither provider.password nor provider.password_env is set; connecting to %s as %q with an empty password",
			name, k, t.AdminUser))
	}
	if k.RequiresDatabase() && t.Database == "" {
		warnings = append(warnings, fmt.Sprintf(
			"secret %q: provider.database is not set; the server default will be used (set it to e.g. 'postgres' to be explicit)",
			name))
	}
	return warnings, nil
}
