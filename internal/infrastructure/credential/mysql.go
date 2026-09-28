package credential

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/giulio/secret-rotator/internal/domain"
)

// MySQLRotator changes a MySQL or MariaDB account password with ALTER USER.
type MySQLRotator struct{}

// Kind returns domain.KindMySQL.
func (MySQLRotator) Kind() domain.Kind { return domain.KindMySQL }

// Apply connects as the administrative account and sets the target account's
// password.
func (r MySQLRotator) Apply(ctx context.Context, target domain.Target, next domain.Credential) error {
	db, err := r.connect(ctx, target, target.AdminUser, target.AdminPassword)
	if err != nil {
		return fmt.Errorf("mysql: connecting as %s: %w", target.AdminUser, err)
	}
	defer db.Close()

	return r.alterPassword(ctx, db, target, next)
}

// Verify authenticates as the target account with the given credential.
func (r MySQLRotator) Verify(ctx context.Context, target domain.Target, c domain.Credential) error {
	db, err := r.connect(ctx, target, target.EffectiveTargetUser(), c)
	if err != nil {
		return fmt.Errorf("mysql: authenticating as %s: %w", target.EffectiveTargetUser(), err)
	}
	defer db.Close()
	return nil
}

// Restore puts the previous password back.
//
// It first tries the administrative credential as configured. If the admin
// account is the one that was just rotated, that credential is now stale, so
// it retries with the new value before giving up.
func (r MySQLRotator) Restore(ctx context.Context, target domain.Target, previous, current domain.Credential) error {
	db, err := r.connect(ctx, target, target.AdminUser, target.AdminPassword)
	if err != nil && r.adminIsTarget(target) {
		db, err = r.connect(ctx, target, target.AdminUser, current)
	}
	if err != nil {
		return fmt.Errorf("mysql: connecting as %s to restore: %w", target.AdminUser, err)
	}
	defer db.Close()

	return r.alterPassword(ctx, db, target, previous)
}

// alterPassword runs the ALTER USER statement.
//
// MySQL has no placeholder support in ALTER USER, so the account and the
// password are interpolated. Both are single-quoted string literals, so they
// are escaped for the quote and the backslash: with the default
// NO_BACKSLASH_ESCAPES off, a backslash starts an escape sequence and would
// otherwise let a crafted value change the statement.
func (r MySQLRotator) alterPassword(ctx context.Context, db *sql.DB, target domain.Target, c domain.Credential) error {
	account := fmt.Sprintf("'%s'@'%s'",
		escapeMySQLLiteral(target.EffectiveTargetUser()),
		escapeMySQLLiteral(target.EffectiveTargetUserHost()))

	stmt := fmt.Sprintf("ALTER USER %s IDENTIFIED BY '%s'", account, escapeMySQLLiteral(c.Expose()))
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql: ALTER USER %s: %w", account, err)
	}
	return nil
}

// adminIsTarget reports whether the administrative account is also the one
// being rotated.
func (r MySQLRotator) adminIsTarget(target domain.Target) bool {
	return target.AdminUser == target.EffectiveTargetUser()
}

// connect opens and pings a connection with the given credentials.
//
// The DSN is built with mysql.Config rather than by string concatenation, so a
// password containing '@', '/' or ':' cannot corrupt it.
func (r MySQLRotator) connect(ctx context.Context, target domain.Target, user string, pass domain.Credential) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = target.Addr(domain.KindMySQL)
	cfg.User = user
	cfg.Passwd = pass.Expose()
	cfg.DBName = target.Database
	cfg.AllowNativePasswords = true

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// escapeMySQLLiteral escapes a value for use inside a single-quoted MySQL
// string literal.
func escapeMySQLLiteral(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
	)
	return replacer.Replace(s)
}

var _ domain.CredentialRotator = MySQLRotator{}
