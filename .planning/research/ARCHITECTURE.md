# Architecture Patterns

**Domain:** CLI-first secret rotation tool for self-hosted Docker environments
**Researched:** 2026-03-27
**Confidence:** HIGH (well-established Go patterns, mature Docker SDK, standard crypto primitives)

## Recommended Architecture

The system follows a **pipeline architecture** with clearly separated concerns: discovery, planning, execution, verification, and rollback. Each stage is a distinct component communicating through well-defined interfaces.

```
                        rotator.yml
                            |
                            v
              +----------------------------+
              |        Config Loader       |  (Viper + YAML)
              +----------------------------+
                            |
                            v
+--------+    +----------------------------+
| CLI    |--->|        Command Router      |  (Cobra)
| (user) |    +----------------------------+
+--------+         |    |    |    |
                   v    v    v    v
               scan  rotate status history
                 |      |
                 v      v
        +----------------------------+
        |     Discovery Engine       |  .env parser, Docker labels
        +----------------------------+
                    |
                    v
        +----------------------------+
        |     Rotation Planner       |  Dependency ordering, plan generation
        +----------------------------+
                    |
                    v
        +----------------------------+
        |     Execution Engine       |  Orchestrates the rotation pipeline
        +----------------------------+
               /        |        \
              v         v         v
    +----------+ +----------+ +----------+
    | Provider | | .env     | | Docker   |
    | Registry | | Writer   | | Manager  |
    +----------+ +----------+ +----------+
    | MySQL    |              | Stop     |
    | Postgres |              | Start    |
    | Redis    |              | Inspect  |
    | Generic  |              | Health   |
    +----------+              +----------+
                    |
                    v
        +----------------------------+
        |     Secret Store           |  Encrypted history (AES-GCM + Argon2)
        +----------------------------+
                    |
                    v
        +----------------------------+
        |     Notifier               |  Webhooks (Discord, Slack, HTTP)
        +----------------------------+
```

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| **CLI Layer** (cmd/) | Parse commands, flags, output formatting | Config Loader, all command handlers |
| **Config Loader** | Read/validate `rotator.yml`, merge env vars | CLI Layer, all internal components |
| **Discovery Engine** | Scan .env files, identify rotatable secrets via patterns and config | Docker Manager (container inspection), Config Loader |
| **Rotation Planner** | Build ordered execution plan, resolve container dependencies | Discovery Engine, Config Loader |
| **Execution Engine** | Run rotation pipeline: generate -> apply to DB -> update .env -> restart -> verify -> rollback on failure | Provider Registry, .env Writer, Docker Manager, Secret Store |
| **Provider Registry** | Map secret types to provider implementations | Individual providers (MySQL, Postgres, Redis, Generic) |
| **Provider (interface)** | Connect to target service, execute credential change, verify new credential works | Target database/service directly via network |
| **.env Writer** | Atomic read-modify-write of .env files | Filesystem |
| **Docker Manager** | Container lifecycle operations via Docker SDK | Docker socket (unix:///var/run/docker.sock) |
| **Secret Store** | Encrypt and persist rotation history, retrieve old secrets for rollback | Filesystem, Crypto subsystem |
| **Scheduler** | Run rotation jobs on cron schedules in daemon mode | Execution Engine, Config Loader |
| **Notifier** | Send rotation outcome notifications | External webhook endpoints |

### Data Flow

**Scan Command Flow:**
```
User runs `scan` -> Discovery Engine reads .env files from configured paths
  -> Matches known patterns (MYSQL_PASSWORD, REDIS_PASSWORD, *_SECRET, etc.)
  -> Cross-references with rotator.yml for explicit secret definitions
  -> Queries Docker Manager for container labels (optional discovery hints)
  -> Returns: list of discovered secrets with metadata (type, container, last rotated)
```

**Rotate Command Flow (the critical path):**
```
1. PLAN:    Load config -> Discover secrets -> Build dependency graph -> Generate execution plan
2. BACKUP:  For each secret: encrypt current value -> store in Secret Store
3. GENERATE: Provider generates new credential (random string or provider-specific)
4. APPLY:   Provider applies new credential to target service (ALTER USER, CONFIG SET, etc.)
5. VERIFY:  Provider verifies new credential works (test connection)
6. UPDATE:  .env Writer atomically updates .env file with new value
7. RESTART: Docker Manager restarts affected containers in dependency order
8. HEALTH:  Docker Manager waits for container health checks to pass
9. NOTIFY:  Notifier sends success/failure webhook

ROLLBACK (if any step 4-8 fails):
  -> Retrieve old secret from Secret Store
  -> Provider re-applies old credential to target service
  -> .env Writer restores old .env content (kept in memory)
  -> Docker Manager restarts containers
  -> Notifier sends failure + rollback notification
```

**Key Data Flow Principle:** Information flows downward through the pipeline. The Execution Engine is the sole orchestrator -- no component calls "up" the chain. This makes the rollback path clean: the Execution Engine holds all state needed to reverse every step.

## Project Structure

```
secret-rotator/
  cmd/
    rotator/
      main.go              # Entry point, minimal -- just wires things together
  internal/
    cli/                   # Cobra command definitions
      root.go              # Root command, global flags (--config, --verbose)
      scan.go              # scan command
      rotate.go            # rotate command (single secret or all due)
      status.go            # status command (show secret ages, schedule)
      history.go           # history command (show rotation log)
      daemon.go            # daemon mode (run scheduler)
    config/
      config.go            # rotator.yml schema definition and loading
      validation.go        # Config validation rules
    discovery/
      scanner.go           # .env file scanner, pattern matching
      patterns.go          # Known secret patterns (MYSQL_PASSWORD, etc.)
    planner/
      planner.go           # Builds rotation execution plans
      depgraph.go          # Container dependency ordering
    engine/
      executor.go          # Rotation pipeline orchestrator
      rollback.go          # Rollback state machine
    provider/
      provider.go          # Provider interface definition
      registry.go          # Provider registry (type -> implementation)
      mysql.go             # MySQL/MariaDB provider
      postgres.go          # PostgreSQL provider
      redis.go             # Redis provider
      generic.go           # Generic provider (regenerate + restart only)
    docker/
      client.go            # Docker SDK wrapper (inspect, stop, start, health)
      compose.go           # Compose-aware operations (dependency ordering from compose file)
    envfile/
      reader.go            # .env file parser
      writer.go            # Atomic .env file writer (write-temp + rename)
    store/
      store.go             # Secret history store interface
      encrypted.go         # AES-GCM encrypted file-based store
    crypto/
      crypto.go            # Key derivation (Argon2id), encryption (AES-256-GCM)
    scheduler/
      scheduler.go         # Cron-based job scheduling (daemon mode)
    notify/
      notifier.go          # Notification dispatcher
      discord.go           # Discord webhook
      slack.go             # Slack webhook
      http.go              # Generic HTTP webhook
  rotator.example.yml      # Example configuration file
```

**Why this structure:**
- `cmd/` contains only the entry point -- all logic lives in `internal/`
- `internal/` enforced by Go compiler, preventing external imports of private code
- Each package has a single, clear responsibility with no circular dependencies
- The `provider/` package uses an interface + registry pattern, making new providers trivial to add
- Flat package structure (no deep nesting) -- follows Go community best practice

## Patterns to Follow

### Pattern 1: Provider Interface

The provider interface is the core extensibility point. Every secret type (MySQL, Postgres, Redis, Generic) implements this interface. New providers are added by implementing the interface and registering in the registry.

**What:** Strategy pattern for secret rotation per service type.
**When:** Any time a new database or service needs rotation support.

```go
// internal/provider/provider.go
package provider

import "context"

type Config struct {
    Host     string
    Port     int
    Username string
    // Provider-specific config via map
    Options  map[string]string
}

type Result struct {
    OldSecret string
    NewSecret string
}

type Provider interface {
    // Name returns the provider identifier (e.g., "mysql", "postgres")
    Name() string

    // Rotate generates a new secret and applies it to the target service.
    // Returns the old and new secret values.
    Rotate(ctx context.Context, cfg Config, currentSecret string) (*Result, error)

    // Verify checks that the new secret works against the target service.
    Verify(ctx context.Context, cfg Config, secret string) error

    // Rollback restores the old secret on the target service.
    Rollback(ctx context.Context, cfg Config, oldSecret string) error
}
```

### Pattern 2: Atomic .env File Writes

**What:** Write-to-temp-file then atomic rename to prevent corrupted .env files on crash.
**When:** Every .env file update during rotation.

```go
// Write to temp file in same directory, then os.Rename (atomic on same filesystem)
func (w *Writer) Update(envPath string, key string, newValue string) error {
    // 1. Read current file
    // 2. Replace key=value line
    // 3. Write to envPath + ".tmp"
    // 4. os.Rename(envPath+".tmp", envPath)  // atomic on Linux
    return nil
}
```

**Why same directory:** `os.Rename` is atomic only within the same filesystem mount. Writing the temp file alongside the target guarantees this.

### Pattern 3: Execution Engine State Machine

**What:** The rotation pipeline as an explicit state machine with rollback at each step.
**When:** Every rotation operation.

```
States: PLAN -> BACKUP -> GENERATE -> APPLY -> VERIFY -> UPDATE_ENV -> RESTART -> HEALTH_CHECK -> DONE
                                        |         |           |           |            |
                                        v         v           v           v            v
                                   ROLLBACK_DB  ROLLBACK_DB  ROLLBACK_DB ROLLBACK_ENV ROLLBACK_ALL
                                                              + ENV       + DB         + ENV + DB
```

Each state transition records what was done, so the rollback handler knows exactly which steps to reverse. This avoids the classic "partial rotation" pitfall where the DB has the new password but the .env file still has the old one.

### Pattern 4: Docker Manager Abstraction

**What:** Thin wrapper over the Docker SDK client that exposes only the operations the rotator needs.
**When:** All container interactions.

```go
type DockerManager interface {
    // ListContainers returns containers matching the filter
    ListContainers(ctx context.Context, filter ContainerFilter) ([]Container, error)
    // InspectContainer returns full container details
    InspectContainer(ctx context.Context, id string) (*Container, error)
    // RestartContainer stops and starts a container
    RestartContainer(ctx context.Context, id string, timeout time.Duration) error
    // WaitHealthy blocks until container health check passes or timeout
    WaitHealthy(ctx context.Context, id string, timeout time.Duration) error
    // ReadEnvFile reads the .env file path from a container's compose config
    ReadEnvFile(ctx context.Context, id string) (string, error)
}
```

**Why wrap:** Testability. The Docker SDK client is large and hard to mock. A narrow interface is trivial to mock in tests.

### Pattern 5: Config Schema with Viper

**What:** YAML configuration loaded by Viper with sensible defaults and environment variable overrides.
**When:** Application startup.

```yaml
# rotator.yml
master_key_env: ROTATOR_MASTER_KEY   # env var holding the encryption passphrase

secrets:
  - name: mysql_root_password
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: /opt/stacks/nextcloud/.env
    containers: [nextcloud-db]
    provider:
      host: nextcloud-db
      port: 3306
      username: root
    schedule: "0 3 * * 0"  # weekly at 3am Sunday

  - name: redis_password
    type: redis
    env_key: REDIS_PASSWORD
    env_file: /opt/stacks/nextcloud/.env
    containers: [nextcloud-redis, nextcloud-app]
    provider:
      host: nextcloud-redis
      port: 6379
    schedule: "0 3 1 * *"  # monthly

  - name: jwt_secret
    type: generic
    env_key: JWT_SECRET
    env_file: /opt/stacks/authelia/.env
    containers: [authelia]
    schedule: "0 3 1 */3 *"  # quarterly
    length: 64

notifications:
  - type: discord
    url: https://discord.com/api/webhooks/...
  - type: slack
    url: https://hooks.slack.com/services/...
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: God Package
**What:** Putting all rotation logic in a single package.
**Why bad:** Untestable, hard to reason about, impossible to extend with new providers without risking regressions.
**Instead:** Separate packages per concern (provider, engine, docker, store). Each testable in isolation.

### Anti-Pattern 2: Direct Docker SDK Calls Throughout
**What:** Calling `client.ContainerStop()` directly wherever container operations are needed.
**Why bad:** Docker SDK is a wide interface with complex types. Spreading calls throughout makes testing require a full Docker daemon or heavy mocking.
**Instead:** Single `DockerManager` interface with narrow methods. Mock this interface in tests.

### Anti-Pattern 3: Implicit Rollback
**What:** Hoping things will be fine and handling rollback ad-hoc with scattered error checks.
**Why bad:** Partial rotation is the worst failure mode. The DB has password X, the .env has password Y, the container is down.
**Instead:** Explicit state machine where each step records its action, and the rollback handler reverses steps in LIFO order.

### Anti-Pattern 4: Storing Secrets in Plaintext History
**What:** Writing rotation history (old passwords) to a JSON or SQLite file unencrypted.
**Why bad:** Defeats the purpose of rotation. An attacker finding the history file gets every password ever used.
**Instead:** AES-256-GCM encryption with Argon2id key derivation from user passphrase. No plaintext secrets touch disk.

### Anti-Pattern 5: Blocking on Container Restart
**What:** Calling `RestartContainer` and immediately proceeding without waiting for health.
**Why bad:** The next rotation step may depend on the container being up (e.g., rotating a second secret that uses the same DB).
**Instead:** After restart, poll container health status with a timeout. Fail the rotation (triggering rollback) if health check does not pass.

## Suggested Build Order

The build order follows dependency chains -- each phase builds on components from prior phases.

### Phase 1: Foundation (no external dependencies)
**Build:** Project scaffold, CLI skeleton (Cobra), Config loader (Viper), .env file reader/writer

**Rationale:** These are pure infrastructure with no external service dependencies. They can be fully unit tested without Docker or databases. The CLI skeleton provides the user-facing surface immediately, and config loading is needed by everything else.

**Components:** `cmd/rotator/main.go`, `internal/cli/`, `internal/config/`, `internal/envfile/`

### Phase 2: Docker Integration
**Build:** Docker Manager (inspect, restart, health check, compose parsing)

**Rationale:** Docker is the second foundational dependency. Discovery and container restart both need it. Build and test the Docker abstraction layer before adding business logic on top.

**Components:** `internal/docker/`

### Phase 3: Discovery + Scan Command
**Build:** Discovery engine (pattern matching + config-based), wire up `scan` command

**Rationale:** `scan` is the first useful user-facing command. It requires Config + .env reader + Docker Manager (all from phases 1-2) but no rotation logic. This is the first "complete vertical slice" a user can try.

**Components:** `internal/discovery/`, wire `internal/cli/scan.go`

### Phase 4: Crypto + Secret Store
**Build:** Argon2id key derivation, AES-256-GCM encryption, encrypted file store, `history` command

**Rationale:** The execution engine needs the store for backup-before-rotate. Build and test crypto independently before integrating into the rotation pipeline. The `history` command provides a second vertical slice.

**Components:** `internal/crypto/`, `internal/store/`, wire `internal/cli/history.go`

### Phase 5: Provider System + Execution Engine
**Build:** Provider interface, registry, Generic provider first, then MySQL, Postgres, Redis. Execution engine (state machine), rollback logic. Wire up `rotate` command.

**Rationale:** This is the core value -- actual rotation. Generic provider is simplest (just generate new string + restart) and proves the pipeline works. Then add DB-specific providers one at a time. Each provider is independently testable.

**Components:** `internal/provider/`, `internal/planner/`, `internal/engine/`, wire `internal/cli/rotate.go`

### Phase 6: Status + Scheduling + Notifications
**Build:** `status` command (secret ages, next rotation), Scheduler (cron daemon mode), Notification webhooks

**Rationale:** These are operational features that layer on top of working rotation. Status shows what the system knows. Scheduling automates what the user can already do manually. Notifications are pure output -- no other component depends on them.

**Components:** `internal/scheduler/`, `internal/notify/`, wire `internal/cli/status.go`, `internal/cli/daemon.go`

### Phase 7: Distribution
**Build:** Dockerfile, goreleaser config, CI/CD pipeline

**Rationale:** Package what works. Dockerfile needs Docker socket mounting. Goreleaser handles cross-compilation for the standalone binary.

**Build order dependency graph:**
```
Phase 1 (Foundation) ──> Phase 2 (Docker) ──> Phase 3 (Discovery/Scan)
                    \                      \
                     ──> Phase 4 (Crypto)   ──> Phase 5 (Providers/Engine/Rotate)
                                                         |
                                                         v
                                                Phase 6 (Status/Schedule/Notify)
                                                         |
                                                         v
                                                Phase 7 (Distribution)
```

Note: Phases 2 and 4 can be built in parallel since they have no mutual dependency. Both feed into Phase 5.

## Scalability Considerations

| Concern | Homelab (1-10 stacks) | Power User (50+ stacks) |
|---------|----------------------|------------------------|
| Secret count | In-memory config, trivial | Still fine -- config is small, O(100s) of secrets max |
| Rotation time | Sequential is fine | Consider parallel rotation for independent secrets |
| .env file locking | Simple temp+rename | May need flock() if multiple rotator instances |
| Secret store size | Single JSON file | Still fine -- rotation history is small data |
| Docker API calls | No rate concern | Batch container inspections where possible |

The target audience (homelab users) means scalability is not a primary concern. The architecture supports parallelism in the execution engine if needed later, but sequential rotation is the correct starting point.

## Sources

- [Cobra GitHub Repository](https://github.com/spf13/cobra) -- CLI framework, de facto standard for Go CLIs
- [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) -- Official Docker client for Go
- [Docker SDK Examples](https://docs.docker.com/reference/api/engine/sdk/examples/) -- Container management patterns
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) -- Go project structure conventions
- [Go Official Module Layout Guide](https://go.dev/doc/modules/layout) -- Official guidance on module structure
- [OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html) -- Security patterns for secret rotation
- [Go x/crypto argon2 package](https://pkg.go.dev/golang.org/x/crypto/argon2) -- Argon2id key derivation
- [Go crypto/cipher package](https://pkg.go.dev/crypto/cipher) -- AES-GCM AEAD encryption
- [robfig/cron v3](https://pkg.go.dev/github.com/robfig/cron/v3) -- Cron scheduling library for Go
- [netresearch/go-cron](https://github.com/netresearch/go-cron) -- Maintained fork of robfig/cron with panic fixes
- [Go Project Structure Practices](https://www.glukhov.org/post/2025/12/go-project-structure/) -- Modern Go project layout guidance
- [Secret Rotation Strategies](https://devsecopsschool.com/blog/secret-rotation/) -- Rotation architecture and patterns
- [AWS Credential Rotation Without Container Restart](https://docs.aws.amazon.com/prescriptive-guidance/latest/patterns/rotate-database-credentials-without-restarting-containers.html) -- Dual-credential rotation pattern
