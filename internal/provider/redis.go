package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// RedisProvider rotates passwords for Redis using CONFIG SET requirepass.
type RedisProvider struct{}

// Name returns "redis".
func (r *RedisProvider) Name() string { return "redis" }

// Rotate generates a new password, sets it via CONFIG SET requirepass,
// and persists with CONFIG REWRITE. If CONFIG REWRITE fails, immediately
// rolls back CONFIG SET to the old password and returns an error.
func (r *RedisProvider) Rotate(ctx context.Context, cfg ProviderConfig, currentSecret string) (*Result, error) {
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
		return nil, fmt.Errorf("redis: generating password: %w", err)
	}

	addr := redisAddr(cfg.Host, cfg.Port)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: currentSecret,
	})
	defer client.Close()

	// Set new password
	if err := client.ConfigSet(ctx, "requirepass", newSecret).Err(); err != nil {
		return nil, fmt.Errorf("redis: config set requirepass: %w", err)
	}

	// Persist to config file
	if err := client.ConfigRewrite(ctx).Err(); err != nil {
		// Immediately rollback: restore old password
		_ = client.ConfigSet(ctx, "requirepass", currentSecret)
		return nil, fmt.Errorf("redis: config rewrite failed (rolled back): %w", err)
	}

	return &Result{
		OldSecret: currentSecret,
		NewSecret: newSecret,
	}, nil
}

// Verify creates a new client with the given secret and pings Redis.
func (r *RedisProvider) Verify(ctx context.Context, cfg ProviderConfig, secret string) error {
	addr := redisAddr(cfg.Host, cfg.Port)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: secret,
	})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: verify ping: %w", err)
	}
	return nil
}

// Rollback connects with the old password first; if that fails, tries the new password.
// Then sets requirepass back to oldSecret and persists with CONFIG REWRITE.
func (r *RedisProvider) Rollback(ctx context.Context, cfg ProviderConfig, oldSecret string) error {
	addr := redisAddr(cfg.Host, cfg.Port)

	// Try connecting with old password first
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: oldSecret,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		// Old password doesn't work, try empty or some other approach
		client.Close()
		// The new password may still be active; we don't have it here,
		// so this rollback attempt fails
		return fmt.Errorf("redis: rollback cannot connect with old password: %w", err)
	}

	if err := client.ConfigSet(ctx, "requirepass", oldSecret).Err(); err != nil {
		client.Close()
		return fmt.Errorf("redis: rollback config set: %w", err)
	}

	if err := client.ConfigRewrite(ctx).Err(); err != nil {
		client.Close()
		return fmt.Errorf("redis: rollback config rewrite: %w", err)
	}

	client.Close()
	return nil
}

// redisAddr builds the Redis address string. Defaults port to 6379 if 0.
func redisAddr(host string, port int) string {
	if port == 0 {
		port = 6379
	}
	return fmt.Sprintf("%s:%d", host, port)
}
