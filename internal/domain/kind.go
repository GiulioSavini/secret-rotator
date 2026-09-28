package domain

import (
	"fmt"
	"sort"
	"strings"
)

// Kind is the type of backing service a secret belongs to. It determines which
// rotator handles the secret and what connection details it needs.
type Kind string

const (
	// KindMySQL covers MySQL and MariaDB, rotated with ALTER USER.
	KindMySQL Kind = "mysql"
	// KindPostgres is rotated with ALTER ROLE.
	KindPostgres Kind = "postgres"
	// KindRedis is rotated with CONFIG SET requirepass.
	KindRedis Kind = "redis"
	// KindGeneric has no backing service: the value only lives in .env files.
	KindGeneric Kind = "generic"
)

// kinds is the registry of known kinds and their traits.
var kinds = map[Kind]kindTraits{
	KindMySQL:    {defaultPort: 3306, needsAdmin: true},
	KindPostgres: {defaultPort: 5432, needsAdmin: true, needsDatabase: true},
	KindRedis:    {defaultPort: 6379},
	KindGeneric:  {},
}

type kindTraits struct {
	defaultPort   int
	needsAdmin    bool
	needsDatabase bool
}

// ParseKind validates a kind read from configuration.
func ParseKind(raw string) (Kind, error) {
	trimmed := Kind(strings.ToLower(strings.TrimSpace(string(raw))))
	if trimmed == "" {
		return "", fmt.Errorf("%w: type is required (valid: %s)", ErrInvalidSecret, KindList())
	}
	if _, ok := kinds[trimmed]; !ok {
		return "", fmt.Errorf("%w: invalid type %q (valid: %s)", ErrInvalidSecret, raw, KindList())
	}
	return trimmed, nil
}

// KindList renders the valid kinds for an error message.
func KindList() string {
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, string(k))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// DefaultPort is the port used when the configuration omits one. Zero for
// kinds that do not connect anywhere.
func (k Kind) DefaultPort() int { return kinds[k].defaultPort }

// HasBackingService reports whether rotating this kind involves a service
// outside the .env files. Generic secrets do not, so they are neither verified
// nor rolled back at the service level.
func (k Kind) HasBackingService() bool { return k != KindGeneric }

// RequiresAdminAccount reports whether an administrative account is needed to
// change the credential.
func (k Kind) RequiresAdminAccount() bool { return kinds[k].needsAdmin }

// RequiresDatabase reports whether a database name is needed to connect.
func (k Kind) RequiresDatabase() bool { return kinds[k].needsDatabase }

func (k Kind) String() string { return string(k) }
