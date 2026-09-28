package credential

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/giulio/secret-rotator/internal/domain"
)

// RedisRotator changes the Redis password with CONFIG SET requirepass,
// persisting it with CONFIG REWRITE.
//
// CONFIG REWRITE only works when the server was started from a configuration
// file it can write back to. A server started with --requirepass on the
// command line has nothing to rewrite, and Apply reports that rather than
// leaving the new password in memory only, where a restart would silently
// revert it.
type RedisRotator struct{}

// Kind returns domain.KindRedis.
func (RedisRotator) Kind() domain.Kind { return domain.KindRedis }

// Apply sets the new password and persists it.
func (r RedisRotator) Apply(ctx context.Context, target domain.Target, next domain.Credential) error {
	client := r.client(target, target.AdminPassword)
	defer client.Close()

	if err := client.ConfigSet(ctx, "requirepass", next.Expose()).Err(); err != nil {
		return fmt.Errorf("redis: CONFIG SET requirepass: %w", err)
	}

	if err := client.ConfigRewrite(ctx).Err(); err != nil {
		// The in-memory password already changed. Put it back with the new
		// credential, which is what the server now accepts, so the failure
		// leaves nothing behind.
		revert := r.client(target, next)
		defer revert.Close()
		if revertErr := revert.ConfigSet(ctx, "requirepass", target.AdminPassword.Expose()).Err(); revertErr != nil {
			return fmt.Errorf(
				"redis: CONFIG REWRITE failed (%w) and reverting the in-memory password also failed (%v); "+
					"the server is running with the new password but has not persisted it", err, revertErr)
		}
		return fmt.Errorf("redis: CONFIG REWRITE failed, reverted: %w "+
			"(the server must be started from a config file it can rewrite)", err)
	}
	return nil
}

// Verify authenticates with the given credential.
func (r RedisRotator) Verify(ctx context.Context, target domain.Target, c domain.Credential) error {
	client := r.client(target, c)
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: authenticating against %s: %w", target.Addr(domain.KindRedis), err)
	}
	return nil
}

// Restore puts the previous password back, connecting with whichever of the
// two credentials the server currently accepts.
func (r RedisRotator) Restore(ctx context.Context, target domain.Target, previous, current domain.Credential) error {
	client := r.client(target, previous)
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		client = r.client(target, current)
		if err := client.Ping(ctx).Err(); err != nil {
			client.Close()
			return fmt.Errorf("redis: neither the previous nor the current password is accepted: %w", err)
		}
	}
	defer client.Close()

	if err := client.ConfigSet(ctx, "requirepass", previous.Expose()).Err(); err != nil {
		return fmt.Errorf("redis: CONFIG SET requirepass while restoring: %w", err)
	}
	if err := client.ConfigRewrite(ctx).Err(); err != nil {
		return fmt.Errorf("redis: CONFIG REWRITE while restoring: %w", err)
	}
	return nil
}

// client builds a Redis client for the target with the given password.
func (r RedisRotator) client(target domain.Target, pass domain.Credential) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     target.Addr(domain.KindRedis),
		Username: target.AdminUser,
		Password: pass.Expose(),
	})
}

var _ domain.CredentialRotator = RedisRotator{}
