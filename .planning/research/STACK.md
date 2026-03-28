# Technology Stack

**Project:** Secret Rotator
**Researched:** 2026-03-27

## Recommended Stack

### Language & Runtime

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.23+ | Language runtime | Single binary distribution, native Docker SDK, strong crypto stdlib. Project constraint per PROJECT.md. | HIGH |

### CLI Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| spf13/cobra | v2.x | CLI command structure | De-facto Go CLI standard. Used by kubectl, gh, hugo. Subcommand model (`scan`, `rotate`, `status`, `history`) maps directly. Active maintenance as of 2025. | HIGH |

Cobra provides auto-generated help, shell completions, and a natural command hierarchy. The project needs exactly four subcommands -- this is Cobra's sweet spot. Do NOT use urfave/cli; it has less adoption and weaker subcommand ergonomics.

### Configuration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| knadh/koanf | v2.2.0 | YAML config parsing, env overlay | Lighter than Viper (313% smaller binary), no forced key lowercasing (important for env var names like `MYSQL_PASSWORD`), modular deps. Supports YAML + env + CLI flag merging. | HIGH |
| go-yaml/yaml | v3 | YAML parser (koanf backend) | Used by koanf's YAML parser provider. Battle-tested, handles all YAML 1.2 features. | HIGH |

**Why NOT Viper:** Viper forcibly lowercases all config keys, which breaks case-sensitive env var names -- a dealbreaker for a tool that manages `MYSQL_PASSWORD`, `REDIS_PASSWORD`, etc. Viper also pulls in a massive dependency tree (HCL, etcd, consul clients) that this project will never use.

### Docker Integration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| docker/docker/client | v27.x | Docker socket API (container list, inspect, stop, start) | Official Moby client. Direct API access for container discovery and restart orchestration. Used by Docker Compose itself. | HIGH |

Use `client.NewClientWithOpts(client.FromEnv)` to auto-detect socket path. The project needs: `ContainerList`, `ContainerInspect`, `ContainerStop`, `ContainerStart`, and reading container labels/env. This is all low-level SDK territory -- the newer `docker/go-sdk` is higher-level but less mature and adds unnecessary abstraction.

**Why NOT docker/go-sdk:** Too new (published Jul 2025), less battle-tested, and this project needs fine-grained control over container lifecycle operations.

**Why NOT fsouza/go-dockerclient:** Community alternative that lags behind the official SDK. No reason to use it for new projects.

### Database Drivers

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| go-sql-driver/mysql | v1.9.0 | MySQL/MariaDB password rotation (`ALTER USER`) | The only serious Go MySQL driver. Supports MySQL 5.7+ and MariaDB 10.5+. Published Jun 2025. | HIGH |
| jackc/pgx | v5.8.x | PostgreSQL password rotation (`ALTER ROLE`) | Actively maintained, 10-20% faster than lib/pq, binary protocol. lib/pq is deprecated (maintainer notice). Use pgx/stdlib adapter for database/sql compatibility. | HIGH |
| redis/go-redis | v9.7.x+ | Redis password rotation (`CONFIG SET requirepass`) | Official Redis Go client since v9. Ensure v9.5.5+ or v9.6.3+ or v9.7.3+ to include CVE-2025-29923 fix (out-of-order response bug). | HIGH |

**Why NOT lib/pq:** The maintainer has posted a deprecation notice: "effectively in maintenance mode and is not actively developed. We recommend using pgx." No reason to start a new project on a deprecated driver.

**Why NOT database/sql for Redis:** Redis does not use SQL. go-redis provides native Redis command support including `ConfigSet`.

### Encryption & Key Derivation

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| crypto/aes + crypto/cipher (stdlib) | Go 1.23+ | AES-256-GCM encryption for secret history | Go stdlib provides production-grade AEAD. AES-GCM gives authenticated encryption with no padding concerns. Zero external dependencies for crypto. | HIGH |
| golang.org/x/crypto/argon2 | latest | Key derivation from user passphrase | Argon2id (winner of Password Hashing Competition) for deriving 256-bit AES key from master passphrase. Use `IDKey` with RFC 9106 recommended params: time=1, memory=64*1024. | HIGH |

**Encryption approach:** User provides master passphrase (or env var). Derive 256-bit key with Argon2id + random salt. Encrypt secret history entries with AES-256-GCM (unique nonce per entry). Store salt + nonce + ciphertext together.

**Why NOT age/SOPS/Vault:** Project explicitly excludes these (see PROJECT.md Out of Scope). The tool IS the lightweight alternative. Stdlib crypto is sufficient and avoids dependency on external encryption tooling.

**Why NOT nacl/box or XChaCha20:** AES-GCM is the standard AEAD, has hardware acceleration on most platforms (AES-NI), and Go's stdlib implementation is well-audited. No need for alternatives.

### Scheduling

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| robfig/cron | v3 | Cron expression parsing and scheduled rotation | De-facto Go cron library. Supports 5-field and 6-field expressions, `@every` syntax, timezone override (`CRON_TZ=`). Exactly what the project needs for `schedule: "0 3 * * 0"` in config. | HIGH |

The project needs cron expression parsing for scheduled rotation. robfig/cron v3 is the clear choice -- no serious alternatives exist in the Go ecosystem. It runs an in-process scheduler, which is fine for a long-running Docker container mode.

### Logging

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| log/slog (stdlib) | Go 1.23+ | Structured logging | Standard library since Go 1.21. Zero dependencies. JSON output for machine parsing, text output for humans. Sufficient for a CLI tool -- no need for zerolog/zap performance characteristics. | HIGH |

**Why NOT zerolog/zap:** This is a CLI tool, not a high-throughput web service. slog provides structured logging with zero dependencies. The performance difference is irrelevant for a tool that rotates secrets on a schedule.

### Testing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| testing (stdlib) | Go 1.23+ | Test runner | Standard Go test runner. Always the base. | HIGH |
| stretchr/testify | v1.9.x | Assertions and mocks | Reduces test boilerplate significantly. `assert.NoError`, `assert.Equal`, `require.NotNil` are much more readable than manual if-checks. Mock generation for provider interfaces. | HIGH |
| testcontainers/testcontainers-go | v0.34.x | Integration tests with real DB containers | Spins up real MySQL, PostgreSQL, Redis containers for provider integration tests. Critical -- secret rotation logic must be tested against real databases, not mocks. | MEDIUM |

### Notifications (Webhooks)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| net/http (stdlib) | Go 1.23+ | HTTP webhook delivery (Discord, Slack, generic) | Stdlib HTTP client is fully sufficient for POST-ing JSON payloads to webhook URLs. No need for a library. | HIGH |

### Distribution

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| goreleaser | v2.x | Cross-compilation, release packaging | Standard Go release tool. Produces binaries for linux/amd64, linux/arm64 (homelab RPi), darwin/amd64, darwin/arm64. Generates checksums, changelogs, Docker images. | HIGH |

No CGO dependencies in this stack, so cross-compilation is straightforward with `CGO_ENABLED=0`.

### File Watching (Optional, Phase 2+)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| fsnotify/fsnotify | v1.8.x | Watch .env file changes | Only needed if implementing auto-detection of env file changes. Defer to later phase. | MEDIUM |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| CLI | Cobra v2 | urfave/cli v2 | Less ecosystem adoption, weaker subcommand model |
| Config | koanf v2 | Viper | Forces key lowercasing (breaks env var names), bloated deps |
| PostgreSQL | pgx v5 | lib/pq | Deprecated by maintainer, slower, no binary protocol |
| Redis | go-redis v9 | redigo | go-redis is official Redis client, redigo is community |
| Logging | slog (stdlib) | zerolog | Unnecessary perf overhead for a CLI tool, extra dependency |
| Encryption | stdlib crypto | age, SOPS | Out of scope per project goals -- this IS the lightweight alternative |
| Docker | docker/docker/client | docker/go-sdk | go-sdk is too new (Jul 2025), less battle-tested |
| Config format | YAML only | TOML, JSON | YAML is the Docker Compose ecosystem standard, users expect it |

## Full Dependency List

```bash
# Core
go get github.com/spf13/cobra@latest
go get github.com/knadh/koanf/v2@latest
go get github.com/knadh/koanf/parsers/yaml@latest
go get github.com/knadh/koanf/providers/file@latest
go get github.com/knadh/koanf/providers/env@latest

# Docker
go get github.com/docker/docker/client@v27

# Database drivers
go get github.com/go-sql-driver/mysql@v1.9
go get github.com/jackc/pgx/v5@latest
go get github.com/redis/go-redis/v9@latest

# Crypto (key derivation -- AES is stdlib)
go get golang.org/x/crypto@latest

# Scheduling
go get github.com/robfig/cron/v3@latest

# Dev dependencies
go get github.com/stretchr/testify@latest
go get github.com/testcontainers/testcontainers-go@latest

# Distribution (installed as tool, not Go dependency)
# go install github.com/goreleaser/goreleaser/v2@latest
```

## Architecture Implications

- **Zero CGO:** All dependencies are pure Go. This enables `CGO_ENABLED=0` cross-compilation and true static binaries. Do not introduce CGO dependencies.
- **Stdlib bias:** Prefer stdlib for crypto, HTTP, logging. External deps only where stdlib is genuinely insufficient (CLI, config, DB drivers, Docker SDK).
- **Interface-driven providers:** Database drivers should sit behind a `Provider` interface so adding new providers (e.g., MongoDB) later does not require changing core rotation logic.
- **Config layering:** koanf supports merging YAML file + env vars + CLI flags in priority order. Use this for: config file as base, env vars as override, CLI flags as highest priority.

## Version Pinning Strategy

Pin major versions in go.mod (e.g., `pgx/v5`, `go-redis/v9`, `cobra`). Let minor/patch versions float with `go get -u`. Run `go mod tidy` in CI to catch dependency issues.

## Sources

- [spf13/cobra - GitHub](https://github.com/spf13/cobra)
- [knadh/koanf - GitHub](https://github.com/knadh/koanf)
- [koanf vs Viper comparison](https://github.com/knadh/koanf/wiki/Comparison-with-spf13-viper)
- [Docker SDK docs](https://docs.docker.com/reference/api/engine/sdk/)
- [go-sql-driver/mysql releases](https://github.com/go-sql-driver/mysql/releases)
- [jackc/pgx - GitHub](https://github.com/jackc/pgx)
- [lib/pq deprecation notice](https://github.com/lib/pq)
- [redis/go-redis - GitHub](https://github.com/redis/go-redis)
- [CVE-2025-29923 go-redis fix](https://windowsforum.com/threads/cve-2025-29923-fix-for-out-of-order-responses-in-go-redis-v9.392766/)
- [Go crypto/cipher docs](https://pkg.go.dev/crypto/cipher)
- [golang.org/x/crypto/argon2 docs](https://pkg.go.dev/golang.org/x/crypto/argon2)
- [robfig/cron v3 - pkg.go.dev](https://pkg.go.dev/github.com/robfig/cron/v3)
- [Go slog logging guide](https://www.dash0.com/guides/logging-in-go-with-slog)
- [GoReleaser](https://goreleaser.com/)
- [Go ecosystem trends 2025 - JetBrains](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/)
- [Three Dots Labs - recommended Go libraries](https://threedots.tech/post/list-of-recommended-libraries/)
