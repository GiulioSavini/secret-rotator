package provider

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLProvider rotates passwords for MySQL/MariaDB databases using ALTER USER.
type MySQLProvider struct{}

// Name returns "mysql".
func (m *MySQLProvider) Name() string { return "mysql" }

// Rotate generates a new password and applies it to the target MySQL user
// via ALTER USER. It connects as the admin user defined in cfg.
func (m *MySQLProvider) Rotate(ctx context.Context, cfg ProviderConfig, currentSecret string) (*Result, error) {
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
		return nil, fmt.Errorf("mysql: generating password: %w", err)
	}

	adminPass := m.adminPassword(cfg)
	dsn := mysqlDSN(cfg.Username, adminPass, cfg.Host, cfg.Port)
	target := m.targetUser(cfg)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mysql: connecting as admin: %w", err)
	}

	query := fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", target, newSecret)
	if _, err := db.ExecContext(ctx, query); err != nil {
		return nil, fmt.Errorf("mysql: alter user %s: %w", target, err)
	}

	return &Result{
		OldSecret: currentSecret,
		NewSecret: newSecret,
	}, nil
}

// Verify connects as the target user with the given secret and pings the database.
func (m *MySQLProvider) Verify(ctx context.Context, cfg ProviderConfig, secret string) error {
	target := m.targetUser(cfg)
	dsn := mysqlDSN(target, secret, cfg.Host, cfg.Port)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: verify open: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("mysql: verify ping as %s: %w", target, err)
	}
	return nil
}

// Rollback connects as admin and restores the target user's password to oldSecret.
func (m *MySQLProvider) Rollback(ctx context.Context, cfg ProviderConfig, oldSecret string) error {
	adminPass := m.adminPassword(cfg)
	dsn := mysqlDSN(cfg.Username, adminPass, cfg.Host, cfg.Port)
	target := m.targetUser(cfg)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: rollback open: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("mysql: rollback connect: %w", err)
	}

	query := fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", target, oldSecret)
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("mysql: rollback alter user %s: %w", target, err)
	}
	return nil
}

// targetUser returns the user whose password will be rotated.
// Prefers cfg.Options["target_user"], falls back to cfg.Username.
func (m *MySQLProvider) targetUser(cfg ProviderConfig) string {
	if cfg.Options != nil {
		if u, ok := cfg.Options["target_user"]; ok && u != "" {
			return u
		}
	}
	return cfg.Username
}

// adminPassword resolves the admin password from cfg.Options.
// Checks "password" first, then "password_env" (read from environment).
func (m *MySQLProvider) adminPassword(cfg ProviderConfig) string {
	if cfg.Options != nil {
		if p, ok := cfg.Options["password"]; ok && p != "" {
			return p
		}
		if envKey, ok := cfg.Options["password_env"]; ok && envKey != "" {
			return os.Getenv(envKey)
		}
	}
	return ""
}

// mysqlDSN builds a MySQL DSN string. Defaults port to 3306 if 0.
func mysqlDSN(user, pass, host string, port int) string {
	if port == 0 {
		port = 3306
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, pass, host, port)
}
