package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// PostgresProvider rotates passwords for PostgreSQL databases using ALTER ROLE.
type PostgresProvider struct{}

// Name returns "postgres".
func (p *PostgresProvider) Name() string { return "postgres" }

// Rotate generates a new password and applies it to the target PostgreSQL role
// via ALTER ROLE. It connects as the admin user defined in cfg.
func (p *PostgresProvider) Rotate(ctx context.Context, cfg ProviderConfig, currentSecret string) (*Result, error) {
	length := 0
	if cfg.Options != nil {
		if ls, ok := cfg.Options["length"]; ok {
			if n, err := strconv.Atoi(ls); err == nil {
				length = n
			}
		}
	}

	newSecret, err := GeneratePassword(length)
	if err != nil {
		return nil, fmt.Errorf("postgres: generating password: %w", err)
	}

	adminPass := p.adminPassword(cfg)
	connStr := pgConnStr(cfg.Username, adminPass, cfg.Host, cfg.Port, cfg.Database)

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres: connecting as admin: %w", err)
	}
	defer conn.Close(ctx)

	target := p.targetUser(cfg)
	sanitizedRole := pgx.Identifier{target}.Sanitize()
	query := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", sanitizedRole, newSecret)
	if _, err := conn.Exec(ctx, query); err != nil {
		return nil, fmt.Errorf("postgres: alter role %s: %w", target, err)
	}

	return &Result{
		OldSecret: currentSecret,
		NewSecret: newSecret,
	}, nil
}

// Verify connects as the target user with the given secret and pings the database.
func (p *PostgresProvider) Verify(ctx context.Context, cfg ProviderConfig, secret string) error {
	target := p.targetUser(cfg)
	connStr := pgConnStr(target, secret, cfg.Host, cfg.Port, cfg.Database)

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return fmt.Errorf("postgres: verify connect as %s: %w", target, err)
	}
	defer conn.Close(ctx)

	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: verify ping as %s: %w", target, err)
	}
	return nil
}

// Rollback connects as admin and restores the target role's password to oldSecret.
func (p *PostgresProvider) Rollback(ctx context.Context, cfg ProviderConfig, oldSecret string) error {
	adminPass := p.adminPassword(cfg)
	connStr := pgConnStr(cfg.Username, adminPass, cfg.Host, cfg.Port, cfg.Database)

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return fmt.Errorf("postgres: rollback connect: %w", err)
	}
	defer conn.Close(ctx)

	target := p.targetUser(cfg)
	sanitizedRole := pgx.Identifier{target}.Sanitize()
	query := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", sanitizedRole, oldSecret)
	if _, err := conn.Exec(ctx, query); err != nil {
		return fmt.Errorf("postgres: rollback alter role %s: %w", target, err)
	}
	return nil
}

// targetUser returns the user whose password will be rotated.
func (p *PostgresProvider) targetUser(cfg ProviderConfig) string {
	if cfg.Options != nil {
		if u, ok := cfg.Options["target_user"]; ok && u != "" {
			return u
		}
	}
	return cfg.Username
}

// adminPassword resolves the admin password from cfg.Options.
func (p *PostgresProvider) adminPassword(cfg ProviderConfig) string {
	if cfg.Options != nil {
		if pw, ok := cfg.Options["password"]; ok && pw != "" {
			return pw
		}
		if envKey, ok := cfg.Options["password_env"]; ok && envKey != "" {
			return os.Getenv(envKey)
		}
	}
	return ""
}

// pgConnStr builds a PostgreSQL connection string. Defaults port to 5432 if 0.
func pgConnStr(user, pass, host string, port int, database string) string {
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", user, pass, host, port, database)
}
