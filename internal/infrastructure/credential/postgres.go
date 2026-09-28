package credential

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/giulio/secret-rotator/internal/domain"
)

// PostgresRotator changes a PostgreSQL role's password with ALTER ROLE.
type PostgresRotator struct{}

// Kind returns domain.KindPostgres.
func (PostgresRotator) Kind() domain.Kind { return domain.KindPostgres }

// Apply connects as the administrative role and sets the target role's password.
func (r PostgresRotator) Apply(ctx context.Context, target domain.Target, next domain.Credential) error {
	conn, err := r.connect(ctx, target, target.AdminUser, target.AdminPassword)
	if err != nil {
		return fmt.Errorf("postgres: connecting as %s: %w", target.AdminUser, err)
	}
	defer conn.Close(ctx)

	return r.alterPassword(ctx, conn, target, next)
}

// Verify authenticates as the target role with the given credential.
func (r PostgresRotator) Verify(ctx context.Context, target domain.Target, c domain.Credential) error {
	conn, err := r.connect(ctx, target, target.EffectiveTargetUser(), c)
	if err != nil {
		return fmt.Errorf("postgres: authenticating as %s: %w", target.EffectiveTargetUser(), err)
	}
	defer conn.Close(ctx)
	return nil
}

// Restore puts the previous password back, falling back to the new credential
// when the administrative role is the one that was just rotated.
func (r PostgresRotator) Restore(ctx context.Context, target domain.Target, previous, current domain.Credential) error {
	conn, err := r.connect(ctx, target, target.AdminUser, target.AdminPassword)
	if err != nil && target.AdminUser == target.EffectiveTargetUser() {
		conn, err = r.connect(ctx, target, target.AdminUser, current)
	}
	if err != nil {
		return fmt.Errorf("postgres: connecting as %s to restore: %w", target.AdminUser, err)
	}
	defer conn.Close(ctx)

	return r.alterPassword(ctx, conn, target, previous)
}

// alterPassword runs ALTER ROLE.
//
// PostgreSQL does not accept a placeholder for the password in ALTER ROLE, so
// the role name goes through pgx's identifier quoting and the password through
// single-quote doubling, which is the only escape a standard-conforming string
// literal has.
func (r PostgresRotator) alterPassword(ctx context.Context, conn *pgx.Conn, target domain.Target, c domain.Credential) error {
	role := pgx.Identifier{target.EffectiveTargetUser()}.Sanitize()
	literal := strings.ReplaceAll(c.Expose(), "'", "''")

	stmt := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", role, literal)
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("postgres: ALTER ROLE %s: %w", role, err)
	}
	return nil
}

// connect opens a connection with the given credentials.
func (r PostgresRotator) connect(ctx context.Context, target domain.Target, user string, pass domain.Credential) (*pgx.Conn, error) {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass.Expose()),
		Host:   target.Addr(domain.KindPostgres),
		Path:   "/" + strings.TrimPrefix(target.Database, "/"),
	}
	return pgx.Connect(ctx, u.String())
}

var _ domain.CredentialRotator = PostgresRotator{}
