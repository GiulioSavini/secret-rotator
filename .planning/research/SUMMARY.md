# Project Research Summary

**Project:** Secret Rotator
**Domain:** Self-hosted secret rotation CLI for Docker Compose environments
**Researched:** 2026-03-27
**Confidence:** HIGH

## Executive Summary

Secret Rotator is a lightweight, self-hosted CLI tool that automates password and secret rotation for Docker Compose environments. The target audience — homelab users managing self-hosted applications — is underserved by existing tools (Vault, Infisical, Doppler), which are enterprise-grade and treat container lifecycle as someone else's problem. The key competitive insight from research is that no existing tool combines auto-discovery + rotation + container restart orchestration + rollback in a single lightweight package. This is the product's core differentiator, and the architecture must be designed from day one to deliver the full lifecycle: discover, rotate, verify, restart, rollback.

The recommended implementation approach is a Go CLI with a pipeline architecture. Foundation layers (config, .env file handling, Docker integration, encryption) must be built before the rotation engine, because every provider implementation depends on them. The Provider interface pattern is critical — define it in Phase 1 even if only one provider initially implements it, to avoid rewriting core logic as additional providers are added. The execution engine must be an explicit state machine that tracks rotation progress step-by-step; implicit rollback logic is a known failure mode in this domain.

The two primary risks are (1) partial rotation leaving the system in a split-brain state (DB has new password, .env has old), and (2) Docker socket over-privilege (socket mount grants effective root on the host). Both must be addressed architecturally in Phase 1, not patched later. The mitigation for split-brain is a step-tracking state machine with LIFO rollback. The mitigation for socket over-privilege is mandatory documentation and support for a socket proxy that whitelists only the required API endpoints.

## Key Findings

### Recommended Stack

The entire stack is pure Go with no CGO dependencies, enabling `CGO_ENABLED=0` static binaries and trivial cross-compilation via goreleaser. The library choices reflect a strong stdlib-first bias: slog for logging, net/http for webhooks, crypto/aes+crypto/cipher for encryption — external deps only where stdlib is genuinely insufficient.

**Core technologies:**
- **Go 1.23+**: Language runtime — project constraint, single binary distribution, native Docker SDK, strong crypto stdlib
- **spf13/cobra v2**: CLI framework — de-facto Go standard (kubectl, gh, hugo use it), maps directly to the four required subcommands
- **knadh/koanf v2**: Configuration — chosen over Viper specifically because Viper forces key lowercasing (breaks `MYSQL_PASSWORD`, `REDIS_PASSWORD` env var names); koanf supports YAML + env + CLI flag layering without this bug
- **docker/docker/client v27**: Docker SDK — official Moby client; do NOT use docker/go-sdk (too new, published Jul 2025) or fsouza/go-dockerclient (lags behind official)
- **go-sql-driver/mysql v1.9**: MySQL/MariaDB rotation — only serious Go MySQL driver
- **jackc/pgx v5**: PostgreSQL rotation — lib/pq is deprecated by its own maintainer; pgx is 10-20% faster with binary protocol
- **redis/go-redis v9.7.3+**: Redis rotation — official Redis Go client; version must be 9.5.5+, 9.6.3+, or 9.7.3+ to include CVE-2025-29923 fix
- **golang.org/x/crypto/argon2**: Key derivation — Argon2id (Password Hashing Competition winner) for deriving AES-256 key from master passphrase; stdlib AES-GCM for encryption
- **robfig/cron v3**: Scheduling — no serious alternatives in the Go ecosystem for cron expression parsing
- **testcontainers/testcontainers-go v0.34**: Integration testing — rotation logic must be tested against real databases, not mocks

### Expected Features

**Must have (table stakes — v1.0):**
- `scan`, `rotate`, `status`, `history` CLI commands — all comparable tools expose these four verbs
- MySQL/MariaDB, PostgreSQL, Redis, and Generic providers — covers the vast majority of homelab stacks
- Automatic rollback on failure — users will not trust a tool that can brick their stack; this is non-negotiable
- Encrypted secret history — AES-256-GCM with Argon2id key derivation; security-conscious target audience expects encryption at rest
- Container restart orchestration — rotation is pointless if containers keep running with the old secret
- YAML configuration file (`rotator.yml`) — declarative secret and provider definitions
- Dry-run mode — users need to preview before execution; every infrastructure tool worth using has this

**Should have (competitive differentiators — v1.x):**
- Scheduled rotation via cron expressions — standard across all competitors; add after manual rotation is proven
- Webhook notifications (Discord, Slack, HTTP) — homelab users live in Discord; low implementation cost, high perceived value
- Docker label-based configuration — per-container config co-located with service definition; add after YAML schema is stable
- Dependency-aware container restart ordering — prevents cascading failures during rotation
- Health check verification after restart — catches silent failures; triggers rollback if container becomes unhealthy
- Secret strength auditing in `scan` — flag weak/default passwords; low effort, high value

**Defer (v2+):**
- Dual-credential zero-downtime rotation — enterprise-oriented, high complexity; homelab users tolerate brief downtime
- Compose file `secrets:` injection — breaking workflow change requiring careful design
- TUI dashboard — only if CLI proves insufficient
- Multi-host support — requires agent architecture; address only if single-host adoption is strong

**Anti-features to avoid:** Web dashboard (contradicts lightweight value prop), Vault/SOPS integration (this IS the lightweight alternative), Kubernetes/Swarm support (fragments codebase, well-served by existing tools), plugin system (maintenance nightmare for small projects).

### Architecture Approach

The system follows a pipeline architecture with clearly separated concerns: discovery, planning, execution, verification, and rollback. The Execution Engine is the sole orchestrator — no component calls up the chain — which keeps the rollback path clean. The Provider interface is the core extensibility point, implementing a Strategy pattern where each database/service type implements Rotate, Verify, and Rollback methods. All container interaction is mediated through a narrow DockerManager interface to enable testability without requiring a live Docker daemon.

**Major components:**
1. **CLI Layer** (cobra commands) — parse commands, flags, output formatting; routes to command handlers
2. **Config Loader** (koanf) — reads/validates `rotator.yml`, merges env var overrides, provides config to all components
3. **Discovery Engine** — scans .env files for secret patterns, cross-references with config and Docker labels
4. **Rotation Planner** — builds ordered execution plan, resolves container dependency graph
5. **Execution Engine** — explicit state machine (PLAN → BACKUP → GENERATE → APPLY → VERIFY → UPDATE_ENV → RESTART → HEALTH_CHECK → DONE) with LIFO rollback at each step
6. **Provider Registry** — maps secret types to implementations (mysql, postgres, redis, generic)
7. **.env Writer** — atomic write via temp-file + `os.Rename` to prevent corruption on crash
8. **Docker Manager** — thin SDK wrapper exposing only the operations needed (list, inspect, restart, health); narrow interface enables mocking
9. **Secret Store** — AES-256-GCM encrypted file store for rotation history and rollback data
10. **Scheduler** — robfig/cron-based daemon mode for automated scheduled rotation
11. **Notifier** — webhook dispatcher for Discord, Slack, and generic HTTP endpoints

### Critical Pitfalls

1. **Non-atomic rotation (split-brain state)** — DB has new password, .env has old; system is broken. Prevention: explicit state machine tracking each completed step; LIFO rollback reverses exactly what was done. For v1, implement backup-before-rotate (store old secret before changing it), ensuring rollback always has the information it needs. Full dual-credential pattern (both credentials simultaneously valid) is a v2+ enhancement.

2. **Docker socket over-privilege** — mounting `/var/run/docker.sock` grants effective root on the host. Prevention: document and recommend Tecnativa/docker-socket-proxy that whitelists only container list, inspect, stop, and start. For the standalone binary, user-level Docker group membership is sufficient and less risky. Never use the Docker socket for operations that could be accomplished by parsing compose files directly.

3. **Non-atomic .env file writes** — `os.WriteFile` truncates before writing; a crash mid-write corrupts the file, breaking all services. Prevention: always write to a `.tmp` file in the same directory, then `os.Rename` (atomic on Linux within the same filesystem). Use line-level text editing (not dotenv library round-trip) to preserve comments, ordering, and quoting style.

4. **Redis connection time-bomb** — `CONFIG SET requirepass` changes the password immediately but existing authenticated connections remain open; they fail silently when connections cycle. Additionally, `CONFIG SET requirepass` does NOT persist across Redis restarts without `CONFIG REWRITE`. Prevention: discover ALL containers referencing the Redis password (not just the declared one), restart all of them, and call `CONFIG REWRITE` (or update `redis.conf`) after changing the runtime password.

5. **Container restart ordering without readiness** — restarting a database container and immediately starting dependent apps causes cascading failures when the database is not yet ready. Prevention: parse `depends_on` from compose files to determine dependency order; poll for database readiness (TCP connect or health check query) before restarting dependents; implement configurable readiness timeouts per provider type.

## Implications for Roadmap

Based on combined research, a 7-phase structure maps directly to architectural dependencies:

### Phase 1: Foundation — Project scaffold, CLI skeleton, config loading, .env file handling

**Rationale:** These components have no external service dependencies and can be fully unit tested in isolation. The CLI skeleton provides user-facing surface immediately. Config loading is required by every subsequent component. Getting .env file handling right (atomic writes, line-level editing, backup) is a phase-1 safety requirement — every provider depends on this.

**Delivers:** Working `rotator --help`, `rotator scan --help`, etc.; `rotator.yml` loading with validation; atomic .env read/write with backup; zero external service dependencies.

**Addresses:** YAML configuration (table stakes), dry-run mode scaffold, project structure that prevents the "god package" anti-pattern.

**Avoids:** .env file corruption pitfall (atomic write pattern established from day one), Viper key-lowercasing trap (koanf chosen), CGO dependency creep.

**Research flag:** Standard patterns, well-documented. Skip research-phase.

---

### Phase 2: Docker Integration

**Rationale:** Docker is the second foundational dependency. Both the Discovery Engine and the restart orchestration layer need Docker Manager. Building the narrow DockerManager interface before adding business logic on top enables clean mocking in all subsequent tests.

**Delivers:** DockerManager interface + implementation: container list, inspect, stop, start, health check polling; compose file dependency parsing.

**Addresses:** Container restart orchestration (table stakes), dependency-aware restart ordering.

**Avoids:** Direct Docker SDK call spread (DockerManager interface enforces narrow surface), Docker socket over-privilege (document socket proxy approach here), container restart ordering pitfall (readiness polling built in from the start).

**Research flag:** Docker SDK is well-documented. Socket proxy configuration may benefit from targeted research on Tecnativa/docker-socket-proxy setup. Mostly standard patterns.

---

### Phase 3: Discovery + Scan Command

**Rationale:** `scan` is the first complete vertical slice a user can try — it produces visible, useful output. It requires Phase 1 (config, .env reader) and Phase 2 (Docker Manager for container label inspection) but no rotation logic. Delivering this early validates the discovery foundation before building the execution engine on top of it.

**Delivers:** `scan` command that discovers secrets from .env files via pattern matching (`*_PASSWORD`, `*_SECRET`, `*_KEY`), cross-references with config, optionally reads Docker labels; reports secret names, types, containers, last-rotated timestamps.

**Addresses:** Auto-discovery from .env files (table stakes), secret strength auditing (low effort addition to scan output), zero-config first run.

**Avoids:** Requiring full config before first scan (scan should work with zero config as a differentiator).

**Research flag:** Standard patterns. Skip research-phase.

---

### Phase 4: Crypto + Secret Store + History Command

**Rationale:** The Execution Engine (Phase 5) needs the Secret Store to backup old secrets before rotation begins. Build and validate the crypto primitives independently before integrating them into the rotation pipeline — crypto bugs are easier to catch in isolation. The `history` command provides a second vertical slice with immediate user value.

**Delivers:** Argon2id key derivation, AES-256-GCM encryption/decryption, encrypted file-based secret store, `history` command showing rotation audit trail, `verify-key` subcommand for passphrase validation.

**Addresses:** Encrypted secret history (table stakes).

**Avoids:** Master key loss / weak crypto pitfall (Argon2id + random nonces enforced, never SHA-256 of passphrase, never sequential nonces), storing secrets in plaintext history anti-pattern, secrets appearing in logs (establish redaction policy here).

**Research flag:** Crypto patterns are well-established. Argon2id RFC 9106 parameters are documented. Skip research-phase. Validate against known test vectors in tests.

---

### Phase 5: Provider System + Execution Engine + Rotate Command

**Rationale:** This is the core value of the product. All prior phases feed into this one. Build the Provider interface and registry first, then the Generic provider (simplest: generate random string + restart) to prove the pipeline works end-to-end. Add MySQL, PostgreSQL, and Redis providers one at a time. The Execution Engine state machine is the most complex component and must be built with explicit rollback handling.

**Delivers:** Provider interface + registry; Generic, MySQL, PostgreSQL, Redis providers; Execution Engine state machine with LIFO rollback; Rotation Planner with dependency graph; `rotate` command with `--dry-run` flag and `--yes` confirmation requirement for batch operations.

**Addresses:** All four providers (table stakes), automatic rollback (table stakes), rotate command (table stakes), dry-run mode (table stakes).

**Avoids:** Non-atomic rotation / split-brain state (state machine with LIFO rollback), Redis connection time-bomb (all consumers discovered and restarted, CONFIG REWRITE called), MySQL ALTER USER on admin connection (not on the connection being rotated), no blocking on container restart (readiness polling from Phase 2), secrets in logs (redaction policy from Phase 4).

**Research flag:** MySQL ALTER USER and PostgreSQL ALTER ROLE patterns are standard. Redis CONFIG SET + CONFIG REWRITE interaction needs careful implementation — review Redis documentation on password persistence. Integration tests with testcontainers are critical here; this phase benefits most from the test harness.

---

### Phase 6: Status Command + Scheduling + Notifications

**Rationale:** These are operational features that layer on top of working rotation. `status` shows what the system knows (secret ages, next scheduled rotation). Scheduling automates what the user can already do manually — add only after manual rotation is battle-tested. Notifications are pure output with no coupling to rotation logic.

**Delivers:** `status` command showing secret ages, rotation schedule, health; `daemon` mode with robfig/cron scheduler; webhook notifications to Discord, Slack, and generic HTTP (failure notifications default-on, success notifications opt-in); scheduled rotation overlap protection via file lock.

**Addresses:** Cron-based scheduled rotation (table stakes), webhook notifications (differentiator), Docker label-based configuration (differentiator), secret strength auditing additions.

**Avoids:** Silent scheduled failures (failure webhooks are default, not opt-in), concurrent rotation races (file lock prevents two scheduled rotations of the same secret from running simultaneously), opaque error messages (webhook payload includes which step failed and current system state).

**Research flag:** Discord and Slack webhook formats are well-documented. Cron scheduling with robfig/cron v3 is standard. Skip research-phase.

---

### Phase 7: Distribution + Documentation

**Rationale:** Package what works. This phase produces the deliverables users actually install.

**Delivers:** Dockerfile (non-root user, Docker socket group membership), goreleaser config (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64), CI/CD pipeline, example `rotator.yml`, socket proxy setup documentation.

**Addresses:** Single binary and Docker container distribution (differentiator), setup complexity (very low is the explicit goal).

**Avoids:** Running container as root (non-root user in Dockerfile), socket over-privilege (document and provide example socket proxy compose service).

**Research flag:** goreleaser patterns are well-documented. Skip research-phase.

---

### Phase Ordering Rationale

- **Phases 1-4 establish foundations** before any rotation logic: the .env writer, Docker Manager, discovery engine, and crypto store are all independent of each other and of the rotation pipeline. Phases 2 and 4 can be built in parallel since they have no mutual dependency.
- **Phase 5 is the integration point** where all prior components combine. Building it last means each dependency is tested and stable before integration.
- **Phase 6 layers operational features** on a working rotation core — adding scheduling to an unproven rotation engine would hide bugs behind timing complexity.
- **Phase 7 packages the result** — goreleaser and Docker distribution are last because they depend on a complete, working tool.

### Research Flags

Phases with standard patterns (no additional research needed):
- **Phase 1:** Foundation patterns (Cobra, koanf, file I/O) are thoroughly documented
- **Phase 3:** .env pattern matching and Docker label reading are standard
- **Phase 4:** Argon2id + AES-GCM patterns are documented in OWASP and Go stdlib docs
- **Phase 6:** Webhook and cron patterns are standard; robfig/cron v3 is well-documented
- **Phase 7:** goreleaser and Docker distribution are well-documented

Phases that may benefit from targeted research during planning:
- **Phase 2:** Socket proxy configuration (Tecnativa/docker-socket-proxy API whitelist rules for the specific endpoints needed)
- **Phase 5:** Redis CONFIG REWRITE persistence behavior and edge cases; testcontainers integration test setup for MySQL, PostgreSQL, and Redis simultaneously; rollback-of-rollback failure handling

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All libraries are well-established Go ecosystem choices with clear rationale. CVE version requirements for go-redis are documented. Viper vs koanf tradeoff is clearly documented with a specific, verifiable bug as the reason. |
| Features | HIGH | Competitor analysis (Vault, Infisical, Doppler) is thorough. Table stakes vs differentiators vs anti-features are well-reasoned. MVP definition is conservative and achievable. |
| Architecture | HIGH | Pipeline architecture with state machine rollback is a well-understood pattern. Provider interface design is idiomatic Go. Component boundaries and build order are clearly derived from dependency analysis. |
| Pitfalls | HIGH | Pitfalls are specific (not generic "test your code"), grounded in known failure modes with sources, and each has a concrete prevention strategy. Redis CONFIG REWRITE gotcha and Docker socket root access are particularly well-sourced. |

**Overall confidence:** HIGH

### Gaps to Address

- **Dual-credential rotation design:** Research flags this as a v2+ feature, but the Provider interface should be designed with it in mind. During Phase 5 planning, consider whether the `Rotate` method signature supports a future "create alternate credential" step without breaking changes.
- **Multi-file .env resolution:** When a secret appears in multiple .env files (e.g., shared across compose stacks), the tool needs a consistent resolution strategy. Research notes this as a v1 simplification; the config schema should accommodate multi-file declarations from day one even if only single-file is implemented initially.
- **Redis ACL users (Redis 6+):** Research covers `CONFIG SET requirepass` (Redis <7 single-password model). Redis 6+ with ACL users is a more complex rotation target. The Redis provider in Phase 5 should detect which model is in use and handle both, or explicitly document the Redis version constraint.
- **testcontainers CI performance:** Integration tests with real database containers add significant CI time. During Phase 5 planning, decide on test tagging strategy (unit vs integration build tags) and CI caching approach.

## Sources

### Primary (HIGH confidence)

- [spf13/cobra GitHub](https://github.com/spf13/cobra) — CLI framework patterns and subcommand structure
- [knadh/koanf GitHub](https://github.com/knadh/koanf) — config loading, Viper comparison
- [Docker Go SDK docs](https://docs.docker.com/reference/api/engine/sdk/) — container lifecycle API
- [jackc/pgx GitHub](https://github.com/jackc/pgx) — PostgreSQL driver; lib/pq deprecation notice
- [golang.org/x/crypto/argon2 docs](https://pkg.go.dev/golang.org/x/crypto/argon2) — Argon2id key derivation
- [Go crypto/cipher docs](https://pkg.go.dev/crypto/cipher) — AES-GCM AEAD
- [robfig/cron v3 pkg.go.dev](https://pkg.go.dev/github.com/robfig/cron/v3) — cron scheduling
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html) — socket privilege model
- [OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html) — rotation patterns
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html) — encryption at rest
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) — Go project structure
- [Redis CONFIG SET requirepass issue #7898](https://github.com/redis/redis/issues/7898) — CONFIG REWRITE persistence requirement
- [CVE-2025-29923 go-redis fix](https://windowsforum.com/threads/cve-2025-29923-fix-for-out-of-order-responses-in-go-redis-v9.392766/) — version requirement for go-redis

### Secondary (MEDIUM confidence)

- [Doppler Zero Downtime Rotation Guide](https://www.doppler.com/blog/10-step-secrets-rotation-guide) — dual-credential pattern, rollback strategies
- [Infisical Secret Rotation Overview](https://infisical.com/docs/documentation/platform/secret-rotation/overview) — rotation architecture
- [HashiCorp Vault Database Secrets Engine](https://developer.hashicorp.com/vault/docs/secrets/databases) — DB credential rotation patterns
- [Why is Exposing the Docker Socket a Really Bad Idea? - Quarkslab](https://blog.quarkslab.com/why-is-exposing-the-docker-socket-a-really-bad-idea.html) — socket privilege analysis
- [Rotating PostgreSQL Passwords with No Downtime - Jannik Arndt](https://www.jannikarndt.de/blog/2018/08/rotating_postgresql_passwords_with_no_downtime/) — PostgreSQL rotation patterns
- [Self-Hosted Secrets Management for Homelabs](https://www.antlatt.com/blog/self-hosted-secrets-management/) — homelab target audience pain points

### Tertiary (LOW confidence)

- [Secret Rotation Strategies - oneuptime](https://oneuptime.com/blog/post/2026-01-30-security-secret-rotation-strategies/view) — general rotation patterns (recent, less established)
- [Go Project Structure Practices - glukhov.org](https://www.glukhov.org/post/2025/12/go-project-structure/) — modern layout guidance (single source, 2025)

---
*Research completed: 2026-03-27*
*Ready for roadmap: yes*
