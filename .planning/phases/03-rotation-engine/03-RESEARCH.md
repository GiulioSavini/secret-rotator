# Phase 3: Rotation Engine - Research

**Researched:** 2026-03-27
**Domain:** Secret rotation pipeline with provider implementations and automatic rollback
**Confidence:** HIGH

## Summary

Phase 3 is the core value delivery of the secret-rotator tool. It builds the rotation pipeline that connects all Phase 1 and Phase 2 foundations: the Provider interface and four implementations (Generic, MySQL, PostgreSQL, Redis), the Execution Engine state machine with LIFO rollback, and the wired `rotator rotate SECRET_NAME` CLI command. Every foundation component is already verified and tested -- this phase is pure integration and new business logic.

The primary technical challenge is the state machine design. Rotation is a multi-step distributed operation (generate new secret, apply to DB, verify, update .env, restart containers, health check) where failure at any step requires precise rollback of exactly the steps already completed. The research confirms that an explicit state machine with step-tracking is the correct approach -- implicit rollback logic is the #1 failure mode in this domain.

The secondary challenge is provider-specific behavior: MySQL ALTER USER takes effect immediately without restart, PostgreSQL ALTER ROLE likewise applies immediately for new connections, but Redis CONFIG SET requirepass has critical persistence and consumer-restart requirements. Redis 6+ treats requirepass as a compatibility layer over ACL -- the provider must handle both models (requirepass for simple setups, ACL SETUSER for Redis 6+ with ACL files).

**Primary recommendation:** Build the Generic provider first to prove the full pipeline end-to-end (generate + update .env + restart), then add database providers one at a time. Each provider is independently testable via the Provider interface.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| ROT-01 | User can manually rotate a specific secret via `rotator rotate SECRET_NAME` | CLI command wiring, Execution Engine state machine, Provider dispatch |
| ROT-04 | Tool automatically rolls back if rotation fails (restores old secret, restarts containers) | LIFO rollback state machine, history store backup-before-rotate, .env restore |
| PROV-01 | Generic provider regenerates password, updates .env, and restarts containers | crypto/rand password generation, envfile.Set + WriteAtomic, docker.RestartInOrder |
| PROV-02 | MySQL/MariaDB provider executes ALTER USER for password rotation | go-sql-driver/mysql, `ALTER USER 'user'@'host' IDENTIFIED BY 'newpass'` |
| PROV-03 | PostgreSQL provider executes ALTER ROLE for password rotation | jackc/pgx v5, `ALTER ROLE username WITH PASSWORD 'newpass'` |
| PROV-04 | Redis provider executes CONFIG SET requirepass + CONFIG REWRITE | redis/go-redis v9, ConfigSet + ConfigRewrite, ACL-aware detection |
| CLI-02 | `rotator rotate` command performs on-demand secret rotation | cobra command wiring with --dry-run support, confirmation output |
</phase_requirements>

## Standard Stack

### Core (new dependencies for this phase)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go-sql-driver/mysql | v1.9.x | MySQL/MariaDB ALTER USER | Only serious Go MySQL driver; supports MySQL 5.7+ and MariaDB 10.5+ |
| jackc/pgx | v5.8.x | PostgreSQL ALTER ROLE | Official recommendation; lib/pq is deprecated by its maintainer |
| redis/go-redis | v9.7.3+ | Redis CONFIG SET/REWRITE | Official Redis Go client; MUST be 9.5.5+/9.6.3+/9.7.3+ for CVE-2025-29923 fix |

### Already Available (from Phases 1-2)

| Library | Purpose | Phase 3 Usage |
|---------|---------|---------------|
| docker/docker/client v27 | Container restart orchestration | RestartInOrder after .env update |
| envfile (internal) | Atomic .env read/write | Set() + WriteAtomic() for secret updates |
| history (internal) | Encrypted audit log | Backup old secret before rotation, record outcome |
| crypto (internal) | AES-256-GCM encryption | Encrypt old secret values in history |
| config (internal) | Config loading | SecretConfig provider settings |
| docker (internal) | Manager interface + MockManager | Container restart, health checks |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| crypto/rand (stdlib) | Go 1.25 | Secure password generation | Generic provider and all providers needing new passwords |
| encoding/base64 (stdlib) | Go 1.25 | URL-safe password encoding | Default password format for Generic provider |
| database/sql (stdlib) | Go 1.25 | SQL execution for MySQL/PostgreSQL | ALTER USER / ALTER ROLE statements |
| context (stdlib) | Go 1.25 | Timeout and cancellation | All provider operations and container restarts |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| database/sql + pgx/stdlib | pgx native | database/sql provides a uniform interface for MySQL and PostgreSQL; pgx/stdlib adapter gives pgx performance with database/sql compatibility |
| Manual SQL strings | ORM | ALTER USER/ROLE are DDL statements; ORMs add complexity for zero benefit here |
| testcontainers-go | Docker mocks only | Integration tests with real databases catch provider bugs that mocks miss; use testcontainers for integration tests, mocks for unit tests |

**Installation:**
```bash
go get github.com/go-sql-driver/mysql@v1.9
go get github.com/jackc/pgx/v5@latest
go get github.com/redis/go-redis/v9@latest
```

## Architecture Patterns

### Recommended Project Structure (new files for Phase 3)

```
internal/
  provider/
    provider.go          # Provider interface, Result type, Registry
    registry.go          # Provider registry (type string -> Provider impl)
    generic.go           # Generic provider: generate + restart only
    mysql.go             # MySQL/MariaDB: ALTER USER
    postgres.go          # PostgreSQL: ALTER ROLE
    redis.go             # Redis: CONFIG SET + CONFIG REWRITE
    password.go          # Secure password generation (crypto/rand)
  engine/
    executor.go          # Execution Engine state machine
    state.go             # State enum and RotationState tracking
    rollback.go          # LIFO rollback logic
  cli/
    rotate.go            # Wire rotate command (replace stub)
```

### Pattern 1: Provider Interface

**What:** Strategy pattern for secret rotation per service type. Each provider implements three methods: Rotate (apply new credential), Verify (test new credential), Rollback (restore old credential).

**When to use:** Every rotation operation dispatches to the appropriate provider based on `SecretConfig.Type`.

```go
// internal/provider/provider.go
package provider

import "context"

// ProviderConfig holds connection details for a specific provider.
type ProviderConfig struct {
    Host     string
    Port     int
    Username string
    Database string
    Options  map[string]string
    // TLS      bool  // future: TLS enforcement
}

// Result holds the outcome of a rotation operation.
type Result struct {
    OldSecret string
    NewSecret string
}

// Provider defines the contract for service-specific secret rotation.
type Provider interface {
    // Name returns the provider identifier ("mysql", "postgres", "redis", "generic").
    Name() string

    // Rotate applies a new secret to the target service.
    // It receives the current secret and returns both old and new values.
    Rotate(ctx context.Context, cfg ProviderConfig, currentSecret string) (*Result, error)

    // Verify tests that the given secret works against the target service.
    Verify(ctx context.Context, cfg ProviderConfig, secret string) error

    // Rollback restores the old secret on the target service.
    Rollback(ctx context.Context, cfg ProviderConfig, oldSecret string) error
}
```

### Pattern 2: Execution Engine State Machine

**What:** Explicit state machine tracking each step of rotation with LIFO rollback.

**When to use:** Every `rotator rotate` invocation.

```
States: INIT -> BACKUP -> GENERATE -> APPLY_DB -> VERIFY_DB -> UPDATE_ENV -> RESTART -> HEALTH_CHECK -> RECORD -> DONE
                                         |            |            |            |            |
                                         v            v            v            v            v
                                    ROLLBACK      ROLLBACK     ROLLBACK     ROLLBACK     ROLLBACK
                                    (undo DB)     (undo DB)    (undo DB     (undo DB     (undo all
                                                               + ENV)       + ENV +       + ENV +
                                                                            restart)      restart)
```

**Key principle:** Each state transition records what was done. The rollback handler reads this record and reverses steps in LIFO order. The Execution Engine holds all state needed to reverse every step -- no component calls "up" the chain.

```go
// internal/engine/state.go
package engine

type RotationStep int

const (
    StepInit RotationStep = iota
    StepBackup
    StepGenerate
    StepApplyDB
    StepVerifyDB
    StepUpdateEnv
    StepRestart
    StepHealthCheck
    StepRecord
    StepDone
)

// RotationState tracks the progress of a single rotation operation.
type RotationState struct {
    SecretName    string
    CurrentStep   RotationStep
    OldSecret     string       // backed up before rotation
    NewSecret     string       // generated during rotation
    OldEnvContent []byte       // .env file content before modification (for restore)
    EnvFilePath   string       // which .env file was modified
    Containers    []string     // which containers were restarted (for rollback)
    Error         error        // if rotation failed, the original error
}
```

### Pattern 3: Provider Registry

**What:** Map of type string to Provider implementation, enabling dispatch by config type.

```go
// internal/provider/registry.go
package provider

import "fmt"

type Registry struct {
    providers map[string]Provider
}

func NewRegistry() *Registry {
    return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
    r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, error) {
    p, ok := r.providers[name]
    if !ok {
        return nil, fmt.Errorf("unknown provider: %s", name)
    }
    return p, nil
}
```

### Pattern 4: Secure Password Generation

**What:** Generate cryptographically secure random passwords using crypto/rand.

```go
// internal/provider/password.go
package provider

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
)

const DefaultPasswordLength = 32 // bytes, produces 43-char base64 string

// GeneratePassword creates a cryptographically secure random password.
// The result is URL-safe base64 encoded without padding.
func GeneratePassword(length int) (string, error) {
    if length <= 0 {
        length = DefaultPasswordLength
    }
    b := make([]byte, length)
    if _, err := rand.Read(b); err != nil {
        return "", fmt.Errorf("generating random bytes: %w", err)
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}
```

### Anti-Patterns to Avoid

- **Rotating credentials on the same connection being rotated:** MySQL ALTER USER must use an admin/root connection, NOT the connection authenticated with the password being changed. After ALTER USER, that connection's credentials are invalidated.

- **Forgetting CONFIG REWRITE for Redis:** CONFIG SET requirepass changes the runtime password but does NOT persist across Redis restarts. Always call CONFIG REWRITE after CONFIG SET. If Redis uses an ACL file (Redis 6+), requirepass is a compatibility layer -- the provider must detect this.

- **Blocking without timeout on container restart:** Docker restart API returns when restart is initiated, not when the container is ready. Always use WaitHealthy with a configurable timeout after restart.

- **Logging secret values:** Never log old or new secret values. Log secret names, rotation timestamps, and success/failure only. Use `[REDACTED]` placeholders.

- **Using math/rand for password generation:** Always use crypto/rand. math/rand is deterministic and predictable.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Password generation | Custom character-set random generator | crypto/rand + base64.RawURLEncoding | Avoids bias in character distribution, cryptographically secure |
| MySQL password change | Raw TCP protocol | go-sql-driver/mysql + database/sql | ALTER USER is a SQL statement; driver handles auth, TLS, protocol |
| PostgreSQL password change | Raw TCP protocol | jackc/pgx + database/sql | ALTER ROLE is a SQL statement; driver handles SCRAM-SHA-256 |
| Redis password change | Raw RESP protocol | redis/go-redis ConfigSet/ConfigRewrite | go-redis handles connection auth, pipelining, error handling |
| State machine | ad-hoc if/else chains | Explicit step enum + switch | if/else chains cannot be reliably rolled back; step enum is auditable |
| Container dependency ordering | Manual ordering | docker.RestartInOrder (Phase 1) | Already built and tested with topological sort |

**Key insight:** The providers are thin wrappers around SQL/Redis commands. The real complexity is in the Execution Engine orchestration and rollback, not in the individual provider operations.

## Common Pitfalls

### Pitfall 1: Split-Brain State (DB has new password, .env has old)

**What goes wrong:** ALTER USER succeeds, then .env write fails or process crashes before .env update. The database expects the new password but all containers have the old one. Everything is broken.

**Why it happens:** Developers treat rotation as a simple sequential flow without considering partial failure.

**How to avoid:** The Execution Engine MUST:
1. Backup old secret to encrypted history BEFORE any mutation
2. Track current step in RotationState
3. On ANY failure after StepApplyDB, execute LIFO rollback: re-apply old password to DB, restore old .env if modified, restart containers if restarted
4. For v1, this is backup-before-rotate. Full dual-credential (both passwords valid simultaneously) is v2.

**Warning signs:** No RotationState tracking; rollback that only handles some steps.

### Pitfall 2: Redis CONFIG SET Without CONFIG REWRITE

**What goes wrong:** CONFIG SET requirepass changes the runtime password. Redis appears to work. But on next Redis restart, the old password from redis.conf takes effect. Services fail.

**Why it happens:** Developers test with a running Redis and never restart it during testing.

**How to avoid:** The Redis provider MUST call ConfigRewrite after ConfigSet. If ConfigRewrite fails (e.g., Redis started without a config file), the provider must return an error and trigger rollback. Additionally, if Redis 6+ is using an external ACL file (detected via CONFIG GET aclfile), use ACL SETUSER + ACL SAVE instead of CONFIG SET requirepass.

**Warning signs:** Redis provider that only calls ConfigSet; no integration test that restarts Redis after rotation.

### Pitfall 3: MySQL ALTER USER on the Wrong Connection

**What goes wrong:** The tool connects to MySQL using the credentials being rotated, runs ALTER USER, and the connection becomes invalid mid-operation.

**Why it happens:** Using the "same user" connection to change that user's own password.

**How to avoid:** The MySQL provider config MUST specify an admin user (typically root) that has ALTER USER privileges. The connection is authenticated as the admin user, and ALTER USER changes the target user's password. The admin user's own password is NOT being changed. Document this clearly in config schema: `provider.username` is the admin user, `env_key` is the secret being rotated.

**Warning signs:** Provider config that uses the same credentials for connection and rotation.

### Pitfall 4: Not Restarting All Consumers of a Shared Secret

**What goes wrong:** A Redis password is used by 3 containers. The tool rotates the password and restarts only 1 container (the one declared in config). The other 2 containers fail when their connection pools recycle.

**Why it happens:** Config declares `containers: [app1]` but app2 and app3 also use REDIS_PASSWORD.

**How to avoid:** The `containers` list in SecretConfig MUST include ALL containers that use the secret. The tool should warn (or error) if it detects other containers referencing the same env var key. For v1, rely on explicit config; for v2, auto-discover consumers by scanning all .env files for the same key.

**Warning signs:** Rotation test with only one consumer container.

### Pitfall 5: Rollback of Rollback Failure

**What goes wrong:** Rotation fails at StepHealthCheck. Rollback attempts to restore the old DB password, but the DB is unreachable (e.g., container crashed). Now we're stuck: new password in .env, DB has new password but is unreachable, rollback failed.

**Why it happens:** Rollback assumes the services being rolled back are accessible.

**How to avoid:** If rollback itself fails, log the failure clearly with the current state (which password is active in DB, which is in .env, which containers were restarted) and exit with a non-zero code. Do NOT retry rollback in a loop. Provide enough information for manual recovery. The history store has the old secret encrypted, so the user can always recover manually.

**Warning signs:** Rollback code that swallows errors; no logging of rollback failure details.

## Code Examples

Verified patterns from official sources:

### MySQL ALTER USER

```go
// Source: MySQL 8.0 Reference Manual, go-sql-driver/mysql docs
import (
    "database/sql"
    "fmt"
    _ "github.com/go-sql-driver/mysql"
)

func rotateMySQLPassword(host string, port int, adminUser, adminPass, targetUser, newPass string) error {
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", adminUser, adminPass, host, port)
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return fmt.Errorf("connecting to MySQL: %w", err)
    }
    defer db.Close()

    // ALTER USER takes effect immediately; no FLUSH PRIVILEGES needed
    _, err = db.Exec(fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", targetUser, newPass))
    if err != nil {
        return fmt.Errorf("ALTER USER: %w", err)
    }
    return nil
}

// Verify by connecting as the target user with the new password
func verifyMySQLPassword(host string, port int, user, pass string) error {
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, pass, host, port)
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return err
    }
    defer db.Close()
    return db.Ping()
}
```

**IMPORTANT:** Use parameterized queries or proper escaping for the password value to prevent SQL injection. The ALTER USER statement does not support `?` placeholders in MySQL for the IDENTIFIED BY clause, so the password must be escaped. Use `strings.ReplaceAll(newPass, "'", "\\'")` or the MySQL-specific escaping function.

### PostgreSQL ALTER ROLE

```go
// Source: PostgreSQL docs, jackc/pgx wiki
import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5"
)

func rotatePostgresPassword(ctx context.Context, host string, port int, adminUser, adminPass, targetUser, newPass string) error {
    connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres", adminUser, adminPass, host, port)
    conn, err := pgx.Connect(ctx, connStr)
    if err != nil {
        return fmt.Errorf("connecting to PostgreSQL: %w", err)
    }
    defer conn.Close(ctx)

    // ALTER ROLE takes effect immediately for new connections
    // Existing connections with old password continue to work (helpful for the rotation window)
    // Note: password_encryption setting determines hash algorithm (SCRAM-SHA-256 in modern PG)
    _, err = conn.Exec(ctx, fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'",
        pgx.Identifier{targetUser}.Sanitize(), newPass))
    if err != nil {
        return fmt.Errorf("ALTER ROLE: %w", err)
    }
    return nil
}
```

**NOTE:** Use `pgx.Identifier{targetUser}.Sanitize()` for the role name to prevent SQL injection. The password value in the IDENTIFIED BY / WITH PASSWORD clause must be escaped manually since it is a string literal, not an identifier.

### Redis CONFIG SET + CONFIG REWRITE

```go
// Source: Redis docs, redis/go-redis v9 API
import (
    "context"
    "fmt"
    "github.com/redis/go-redis/v9"
)

func rotateRedisPassword(ctx context.Context, host string, port int, currentPass, newPass string) error {
    client := redis.NewClient(&redis.Options{
        Addr:     fmt.Sprintf("%s:%d", host, port),
        Password: currentPass,
    })
    defer client.Close()

    // Step 1: Set new password at runtime (takes effect immediately)
    if err := client.ConfigSet(ctx, "requirepass", newPass).Err(); err != nil {
        return fmt.Errorf("CONFIG SET requirepass: %w", err)
    }

    // Step 2: Re-authenticate with new password (our connection now needs it)
    // Note: The current connection is still authenticated, but we need to
    // re-auth for subsequent commands if Redis requires it
    // Actually, the connection that SET the password remains authenticated.
    // But we must CONFIG REWRITE using this same connection before it drops.

    // Step 3: Persist to redis.conf (CRITICAL -- without this, restart reverts password)
    if err := client.ConfigRewrite(ctx).Err(); err != nil {
        // CONFIG REWRITE failed -- rollback the runtime password change
        _ = client.ConfigSet(ctx, "requirepass", currentPass)
        return fmt.Errorf("CONFIG REWRITE failed (password reverted): %w", err)
    }

    return nil
}
```

**CRITICAL REDIS NOTE:** The connection that executed CONFIG SET requirepass remains authenticated. New connections must use the new password. CONFIG REWRITE must succeed or the runtime change must be rolled back. If Redis uses an external ACL file (Redis 6+), use `ACL SETUSER default >newpass` + `ACL SAVE` instead.

### Password Generation

```go
// Source: Go crypto/rand docs
import (
    "crypto/rand"
    "encoding/base64"
)

func generatePassword(byteLength int) (string, error) {
    b := make([]byte, byteLength)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    // RawURLEncoding: URL-safe, no padding characters
    // 32 bytes -> 43 characters, all safe for .env files and DB passwords
    return base64.RawURLEncoding.EncodeToString(b), nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `SET PASSWORD FOR user = PASSWORD('pass')` | `ALTER USER user IDENTIFIED BY 'pass'` | MySQL 5.7+ | SET PASSWORD deprecated in MySQL 8.0 |
| MD5 password hashing in PostgreSQL | SCRAM-SHA-256 (default since PG 14) | PostgreSQL 10+ | ALTER ROLE auto-uses server's password_encryption setting |
| Redis requirepass only | Redis ACL system (Redis 6+) | Redis 6.0 (2020) | requirepass is now a compatibility shim for the default user |
| CONFIG SET requirepass | ACL SETUSER default >pass + ACL SAVE | Redis 6.0+ with ACL file | Must detect which model is in use |
| lib/pq for PostgreSQL | jackc/pgx v5 | lib/pq deprecated 2024 | pgx is faster, actively maintained |

**Deprecated/outdated:**
- `SET PASSWORD FOR` syntax: deprecated in MySQL 8.0, removed in 8.4
- `lib/pq`: maintainer-deprecated in favor of pgx
- Redis `requirepass` as standalone: still works but is a shim over ACL in Redis 6+

## Integration with Existing Code

### Using Phase 1 Components

**Docker Manager (internal/docker/manager.go):**
- `RestartInOrder(ctx, mgr, serviceNames, timeout)` -- restart containers in dependency order after .env update
- `MockManager` -- available for unit testing the Execution Engine without Docker

**Envfile Writer (internal/envfile/writer.go):**
- `envfile.Read(path)` -- read current .env file
- `ef.Set(key, newValue)` -- update a key preserving formatting
- `ef.WriteAtomic()` -- atomic write via temp+rename
- `ef.Get(key)` -- read current value (for backup)

**Note:** `ef.Set()` on a nonexistent key is a no-op (Phase 1 decision). The Execution Engine must verify the key exists before attempting rotation.

### Using Phase 2 Components

**History Store (internal/history/store.go):**
- `store.Append(entry)` -- record rotation event (backup old secret before mutation)
- HistoryEntry has `OldValue`, `Status`, `Details` fields

**Config (internal/config/config.go):**
- `SecretConfig.Type` -- dispatches to provider ("mysql", "postgres", "redis", "generic")
- `SecretConfig.Provider` -- `map[string]string` with host, port, username
- `SecretConfig.EnvKey` -- the env var name being rotated
- `SecretConfig.EnvFile` / `SecretConfig.EnvFiles` -- which .env file(s) to update
- `SecretConfig.Containers` -- which containers to restart
- `SecretConfig.Length` -- password length (for Generic provider)

### CLI Wiring (internal/cli/rotate.go)

The current stub accepts `cobra.ExactArgs(1)` (the secret name). The implementation must:
1. Load config to find the matching SecretConfig
2. Build ProviderConfig from SecretConfig.Provider map
3. Read the current secret from the .env file
4. Execute the rotation pipeline via the Execution Engine
5. Print a summary: secret name, status, containers restarted
6. Support `--dry-run` (show what would happen without mutating)

## Open Questions

1. **SQL Injection in ALTER USER/ROLE password clause**
   - What we know: ALTER USER/ROLE do not support parameterized `?` placeholders for the password value in MySQL or PostgreSQL
   - What's unclear: The exact escaping needed for passwords containing single quotes, backslashes, or special characters
   - Recommendation: Generate passwords using base64.RawURLEncoding which produces only `[A-Za-z0-9_-]` characters, avoiding all special characters entirely. This eliminates the SQL injection risk by constraining the password character set.

2. **Redis ACL file detection**
   - What we know: Redis 6+ with an ACL file needs `ACL SETUSER + ACL SAVE` instead of `CONFIG SET requirepass + CONFIG REWRITE`
   - What's unclear: Exact detection method (CONFIG GET aclfile returns empty string when not using ACL file)
   - Recommendation: For v1, use CONFIG SET requirepass + CONFIG REWRITE (works for both models). Add ACL SETUSER support in v1.1 if user requests it. Document the limitation.

3. **Provider admin credential source**
   - What we know: MySQL/PostgreSQL providers need an admin connection to run ALTER USER/ROLE
   - What's unclear: Whether the admin password should come from config, env var, or a separate secret
   - Recommendation: Use `SecretConfig.Provider["username"]` and `SecretConfig.Provider["password"]` for admin credentials. If `Provider["password"]` is empty, look for a `Provider["password_env"]` key that names an env var containing the admin password.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | None needed (go test convention) |
| Quick run command | `go test ./internal/provider/... ./internal/engine/... -v -count=1` |
| Full suite command | `go test ./... -v -count=1` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ROT-01 | rotate command executes end-to-end | unit (mock provider + mock docker) | `go test ./internal/engine/... -run TestExecuteRotation -v` | Wave 0 |
| ROT-04 | rollback on failure restores old state | unit (mock provider fails at each step) | `go test ./internal/engine/... -run TestRollback -v` | Wave 0 |
| PROV-01 | generic provider generates + returns new password | unit | `go test ./internal/provider/... -run TestGeneric -v` | Wave 0 |
| PROV-02 | MySQL provider runs ALTER USER | unit (mock db) + integration (testcontainers) | `go test ./internal/provider/... -run TestMySQL -v` | Wave 0 |
| PROV-03 | PostgreSQL provider runs ALTER ROLE | unit (mock db) + integration (testcontainers) | `go test ./internal/provider/... -run TestPostgres -v` | Wave 0 |
| PROV-04 | Redis provider runs CONFIG SET + CONFIG REWRITE | unit (mock redis) + integration (testcontainers) | `go test ./internal/provider/... -run TestRedis -v` | Wave 0 |
| CLI-02 | rotate command wiring with flags | unit (mock engine) | `go test ./internal/cli/... -run TestRotate -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/provider/... ./internal/engine/... -v -count=1`
- **Per wave merge:** `go test ./... -v -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/provider/provider_test.go` -- mock provider for registry tests
- [ ] `internal/provider/generic_test.go` -- generic provider unit tests
- [ ] `internal/provider/mysql_test.go` -- MySQL provider unit tests (mock sql.DB)
- [ ] `internal/provider/postgres_test.go` -- PostgreSQL provider unit tests (mock pgx)
- [ ] `internal/provider/redis_test.go` -- Redis provider unit tests (mock go-redis)
- [ ] `internal/provider/password_test.go` -- password generation tests
- [ ] `internal/engine/executor_test.go` -- execution engine state machine tests
- [ ] `internal/engine/rollback_test.go` -- LIFO rollback tests at each failure step
- [ ] `internal/cli/rotate_test.go` -- rotate command integration test

Note: Integration tests with testcontainers (real MySQL/PostgreSQL/Redis) are valuable but should be gated behind a build tag (`//go:build integration`) to avoid requiring Docker for unit test runs.

## Sources

### Primary (HIGH confidence)

- [MySQL 8.0 ALTER USER Reference](https://dev.mysql.com/doc/refman/8.0/en/alter-user.html) -- ALTER USER syntax, immediate effect, no FLUSH PRIVILEGES needed
- [jackc/pgx GitHub](https://github.com/jackc/pgx) -- PostgreSQL driver, v5 API, pgx.Identifier sanitization
- [redis/go-redis GitHub](https://github.com/redis/go-redis) -- ConfigSet, ConfigRewrite methods
- [Redis ACL Documentation](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/) -- requirepass as ACL compatibility layer, ACL SETUSER, CONFIG REWRITE vs ACL SAVE distinction
- [Redis CONFIG REWRITE](https://redis.io/docs/latest/commands/config-rewrite/) -- persistence of runtime config changes
- [Go crypto/rand docs](https://pkg.go.dev/crypto/rand) -- cryptographically secure random generation
- Phase 1 Verification Report -- confirmed Docker Manager interface, envfile atomic writes, MockManager
- Phase 2 Verification Report -- confirmed history store, crypto primitives, scanner

### Secondary (MEDIUM confidence)

- [PostgreSQL Password Authentication docs](https://www.postgresql.org/docs/current/auth-password.html) -- SCRAM-SHA-256 default, password_encryption setting
- [testcontainers-go modules](https://github.com/testcontainers/testcontainers-go) -- MySQL, PostgreSQL, Redis test containers
- [go-sql-driver/mysql Discussion #1613](https://github.com/go-sql-driver/mysql/discussions/1613) -- credential rotation with connection pools

### Tertiary (LOW confidence)

- [Redis CONFIG SET requirepass GitHub Issue #7898](https://github.com/redis/redis/issues/7898) -- edge case with empty password after CONFIG SET
- [Redis CONFIG REWRITE permission denied Issue #13462](https://github.com/redis/redis/issues/13462) -- Redis 7.2.5 CONFIG REWRITE permission issues

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries are well-established, already selected in project research
- Architecture: HIGH -- provider interface + state machine pattern is well-understood, consistent with project research
- Pitfalls: HIGH -- split-brain and Redis persistence are well-documented failure modes with clear mitigations
- Provider SQL: HIGH -- ALTER USER/ROLE syntax is stable MySQL 5.7+/PG 10+; verified against official docs
- Redis ACL: MEDIUM -- v1 can use requirepass path; ACL file detection needs runtime validation

**Research date:** 2026-03-27
**Valid until:** 2026-04-27 (stable domain, 30-day validity)
