# Pitfalls Research

**Domain:** Secret rotation tool for self-hosted Docker environments
**Researched:** 2026-03-27
**Confidence:** HIGH

## Critical Pitfalls

### Pitfall 1: Non-Atomic Rotation Leaves System in Split-Brain State

**What goes wrong:**
The rotation process changes the password in the database (e.g., MySQL ALTER USER) but crashes or fails before updating the `.env` file or restarting the container. Now the running application has the old password, the database expects the new one, and the service is down. Worse: if multiple containers share a credential, some may be updated while others are not.

**Why it happens:**
Secret rotation is inherently a multi-step distributed transaction: (1) generate new secret, (2) update the authoritative source (database), (3) write the new secret to `.env`, (4) restart dependent containers. Any step can fail independently, and developers treat this as a simple sequential flow without considering partial failure.

**How to avoid:**
Implement the **dual-credential pattern**: keep both old and new credentials valid simultaneously during the rotation window. The sequence becomes: (1) create new credential in DB (both old and new work), (2) update `.env` file, (3) restart containers, (4) verify new credential works, (5) only then revoke old credential. If any step fails, the old credential still works and the system is never broken. Store rotation state in a state file so interrupted rotations can be resumed or rolled back.

**Warning signs:**
- Rotation code that calls `ALTER USER` before writing the `.env` file with no rollback path
- No state machine or transaction log tracking which step the rotation is on
- Tests that only cover the happy path (all steps succeed)

**Phase to address:**
Phase 1 (Core rotation engine). This is the foundational design pattern -- getting this wrong means a rewrite of the rotation logic later. The provider interface must enforce the dual-credential lifecycle from day one.

---

### Pitfall 2: Docker Socket Mounting Grants Full Host Root Access

**What goes wrong:**
The tool requires Docker socket access to discover and restart containers. Mounting `/var/run/docker.sock` into a container gives that container effectively root access to the entire host. A vulnerability in the rotation tool (or a compromised dependency) means full host compromise: an attacker can mount the host filesystem, start privileged containers, or exfiltrate any data on the host.

**Why it happens:**
The Docker socket is a Unix socket that accepts the full Docker API. There is no built-in fine-grained permission model. Read-only mounting does NOT meaningfully help -- `docker inspect` can leak secrets from other containers, and the API surface is still large. Developers assume "I only need restart" but the socket grants everything.

**How to avoid:**
- Use a Docker socket proxy (e.g., Tecnativa/docker-socket-proxy or HAProxy) that whitelists only the specific API endpoints needed: container list, container inspect, container restart, container stop, container start. Block image pull, container create, exec, volume mount, and all write operations beyond restart.
- Document this as a hard security recommendation in setup instructions.
- For the standalone binary distribution, this is less of an issue since it runs as a host process with user-level Docker permissions.
- Consider dropping the Docker socket requirement for the "scan" command by parsing `docker-compose.yml` files directly instead of querying the Docker API.

**Warning signs:**
- Mounting `docker.sock` with no proxy and no documentation of the risk
- Using the Docker SDK for operations that could be done by parsing config files
- No mention of socket proxy in deployment documentation

**Phase to address:**
Phase 1 (Docker integration). Decide the socket proxy strategy before writing Docker interaction code. The API surface you use determines which proxy rules you need.

---

### Pitfall 3: .env File Write Corruption Destroys Running Configuration

**What goes wrong:**
Writing to `.env` files is not atomic on most filesystems. If the process crashes mid-write, or if the file is being read by another process simultaneously (e.g., `docker compose up`), the file can end up truncated or corrupted. A corrupted `.env` file means all services referencing it cannot start. Additionally, `.env` files have no formal specification -- different tools parse quotes, multiline values, comments, and variable interpolation differently.

**Why it happens:**
Developers use simple file write operations (open, truncate, write, close). If the process is killed between truncate and write-complete, the file is empty or partial. Go's `os.WriteFile` does exactly this -- it truncates first. Additionally, `.env` parsing is deceptively complex: godotenv (the standard Go library) is self-described as "pretty stupidly naive" and may not match how Docker Compose or other tools parse the same file.

**How to avoid:**
- Use atomic file writes: write to a temporary file in the same directory, then `os.Rename()` (which is atomic on Linux when source and dest are on the same filesystem). Never truncate-then-write the original file.
- Before modifying a `.env` file, create a timestamped backup copy.
- For parsing, do NOT use a dotenv library to round-trip the file. Instead, use line-by-line text manipulation that preserves comments, whitespace, ordering, and quoting style. Only modify the specific key=value line that needs to change. This avoids the parser normalizing or destroying formatting that other tools depend on.
- Write integration tests that verify the tool's `.env` output is parseable by Docker Compose.

**Warning signs:**
- Using `os.WriteFile` directly on the `.env` file
- Using a dotenv library to parse and then re-serialize the entire file
- No backup of the original `.env` before modification
- No tests with edge-case `.env` content (multiline values, comments, special characters)

**Phase to address:**
Phase 1 (`.env` file handling). The file manipulation strategy is foundational -- every provider depends on it. Get the atomic-write and line-level-edit patterns right before building providers.

---

### Pitfall 4: Redis Password Rotation Breaks All Existing Connections

**What goes wrong:**
Redis `CONFIG SET requirepass` changes the password immediately, but already-authenticated connections remain authenticated with the old password and continue working. However, when those connections drop (timeout, network blip, connection pool recycling), they cannot reconnect with the old password. This creates a time-bomb: the rotation appears successful, but services fail minutes or hours later when connections cycle. Worse, Redis does not support dual passwords natively (unlike MySQL/PostgreSQL which can have the ALTER USER approach).

**Why it happens:**
Developers test rotation by running `CONFIG SET requirepass`, updating the `.env`, restarting the dependent container, and seeing it connect successfully. They declare success. But any other container or sidecar that also connects to Redis (and was not restarted) will fail silently when its connection recycles.

**How to avoid:**
- The Redis provider must discover ALL containers that reference the Redis password, not just the one declared in config. Scan all `.env` files for the same password value or environment variable name.
- Implement a hard requirement: restart ALL containers that reference the rotated Redis password, not just the "primary" one.
- Add a post-rotation health check that verifies connectivity from all affected containers, not just one.
- Document that Redis rotation is higher-risk than database rotation due to the lack of dual-password support.

**Warning signs:**
- Redis rotation only restarts a single container
- No discovery of all consumers of a given secret
- No post-rotation health checks
- Test environment has only one Redis consumer

**Phase to address:**
Phase 2 (Provider implementations). The Redis provider needs special handling compared to MySQL/PostgreSQL. This should be designed into the provider interface as a "consumer discovery" capability.

---

### Pitfall 5: Container Restart Without Dependency Ordering Causes Cascading Failures

**What goes wrong:**
After rotating a database password, the tool restarts the database container and the application container. But if the application container starts before the database is ready to accept connections (even with the new password), the application crashes on startup. If the application has no retry logic (common in homelab apps), it stays crashed. Docker Compose `depends_on` only controls start order, not readiness.

**Why it happens:**
Docker Compose starts containers in dependency order but does NOT wait for a container to be "ready" (accepting connections). It only waits for the container to be "running" (process started). A database container may take 5-30 seconds to initialize after the process starts. Developers test with warm databases that restart quickly, but cold starts or large databases take longer.

**How to avoid:**
- After restarting a database container, poll for readiness (TCP connect to the port, or run a health check query) before restarting dependent containers.
- Parse Docker Compose files or container labels to determine dependency order, then restart in reverse-dependency order: databases first, then applications.
- Implement configurable readiness timeouts per provider (MySQL may need 30s, Redis is usually instant).
- For database containers specifically: if only the application password changed (not the root password), consider whether the database container needs restarting at all. MySQL/PostgreSQL `ALTER USER` takes effect immediately without restart.

**Warning signs:**
- Restarting all affected containers simultaneously or in arbitrary order
- No readiness check between restarting a dependency and its dependents
- Testing only with already-running (warm) containers

**Phase to address:**
Phase 1 (Container restart orchestration). This is core infrastructure that every provider relies on. Build the dependency-aware restart with readiness checks before implementing individual providers.

---

### Pitfall 6: Master Key Loss Makes Encrypted History Irrecoverable

**What goes wrong:**
The tool encrypts old secrets with a master key derived from a user-provided passphrase. If the user loses the passphrase, ALL rollback capability is lost. If the key derivation is weak (e.g., plain SHA-256 of the passphrase instead of a proper KDF), the encrypted history can be brute-forced. If nonces are reused in AES-GCM, the encryption is broken entirely.

**Why it happens:**
Homelab users are not security professionals. They set a passphrase once during setup and forget it. They may store it in the same `.env` file that the tool manages (circular dependency). Developers implementing encryption often use simple constructs (SHA-256 for key derivation, sequential nonces) that are technically functional but cryptographically weak.

**How to avoid:**
- Use Argon2id for key derivation from passphrase (not bcrypt, not PBKDF2, not raw SHA). Go has `golang.org/x/crypto/argon2`.
- Use AES-256-GCM with random nonces from `crypto/rand`. Never use sequential or deterministic nonces.
- Store the salt alongside the encrypted data (it is not secret).
- Provide a `verify-key` CLI command that checks the master key can decrypt a test record, so users can verify they have the right passphrase without attempting a rollback.
- Support key rotation: allow changing the master passphrase (re-encrypts all history with new key).
- Document clearly: "If you lose this passphrase, rollback history is gone. Your services still work, you just cannot roll back to previous secrets."

**Warning signs:**
- Using `sha256.Sum256([]byte(passphrase))` as the encryption key
- Using a counter or timestamp as nonce
- No test that verifies encryption/decryption round-trips correctly
- No documentation about what happens if the passphrase is lost

**Phase to address:**
Phase 2 (Encrypted history storage). Do not rush this into Phase 1. Get rotation working with plaintext history first (or no history), then add encryption correctly in a dedicated phase.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Shell exec (`docker restart`) instead of Docker SDK | Faster to implement, no SDK dependency | Fragile, no error handling, no timeout control, platform-dependent | Never -- the Go Docker SDK is well-maintained and not complex to use |
| Storing rotation state in memory only | Simpler code, no persistence layer | Interrupted rotations cannot be resumed, leaving system in unknown state | Never -- even a simple JSON file is sufficient |
| Single `.env` file path per secret | Simpler config, fewer edge cases | Real setups share secrets across multiple compose files; rotating one misses others | MVP only -- add multi-file support in Phase 2 |
| Hardcoding provider connection logic | Faster to ship first provider | Each new provider requires touching core rotation logic | MVP only -- define provider interface from day one, even if only MySQL implements it |
| Skipping post-rotation health checks | Rotation is faster, less code | Silent failures where rotation "succeeded" but services are broken | Never -- a basic TCP connect check is trivial and essential |

## Integration Gotchas

Common mistakes when connecting to external services.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| MySQL/MariaDB | Using `ALTER USER` on the same connection that authenticates with the old password, then expecting the connection to still work | Open a separate admin connection (root or rotation-specific user) to perform the ALTER USER. Never rotate the admin user's own password with itself. |
| PostgreSQL | Assuming `ALTER ROLE ... PASSWORD` takes effect only after restart | PostgreSQL applies password changes immediately for new connections. Existing connections with the old password continue to work. Plan for this -- it is actually helpful for the dual-credential window. |
| Redis | Assuming `CONFIG SET requirepass` persists across restarts | It does NOT persist unless you also call `CONFIG REWRITE` or update `redis.conf`. After a Redis restart, the password reverts to the config file value. The tool must update both the runtime config AND the persistent config. |
| Docker API | Using `container.Restart()` and assuming it is synchronous | The restart API returns when the restart is initiated, not when the container is ready. Poll `container.Inspect()` for running state, then check application-level readiness. |
| Docker Compose `.env` | Assuming `.env` is always in the same directory as `docker-compose.yml` | Compose supports `env_file` directives pointing to arbitrary paths. The tool must handle both `.env` (conventional) and explicitly declared env files. |
| Cron scheduling | Using Go's `time.Ticker` for scheduled rotation | Use a proper cron library (e.g., `robfig/cron`) that handles cron expressions correctly, including timezone handling and missed-schedule catch-up. |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Scanning all containers on every operation | Slow CLI response, high Docker API load | Cache container/service discovery with TTL; invalidate on compose file changes | 50+ containers on host |
| Reading and parsing every `.env` file for every rotation | Unnecessary I/O and parsing overhead | Build an in-memory index on `scan`, reuse for `rotate` | 20+ compose stacks with large `.env` files |
| Synchronous rotation of multiple secrets | User waits for all rotations to complete sequentially | Parallelize independent rotations (secrets with no shared containers), serialize dependent ones | 10+ secrets to rotate in one run |
| Unbounded encrypted history growth | Disk usage grows forever, decryption scans slow down | Implement configurable retention (e.g., keep last N rotations per secret), prune old entries | After months of automated rotation |

## Security Mistakes

Domain-specific security issues beyond general web security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Logging new or old secret values | Secrets appear in log files, terminal scrollback, Docker logs | Never log secret values. Log secret names, rotation timestamps, and success/failure only. Use `[REDACTED]` placeholders. |
| Storing master key in the same `.env` file being managed | Circular dependency: tool needs master key to decrypt history, but master key is in a file the tool modifies | Master key must come from a separate source: environment variable, separate file, or interactive prompt. Never from managed `.env` files. |
| Generating weak replacement secrets | Passwords like `abc123` or 8-character random strings | Use `crypto/rand` with configurable length (minimum 32 characters) and character set. Default to URL-safe base64. Never use `math/rand`. |
| Running the rotation container as root | Compromise of the tool means root shell on host (especially with Docker socket) | Run as non-root user inside the container. The Docker socket group membership is sufficient for Docker API access. |
| Leaving old secrets valid indefinitely after rotation | Extends the attack window; if old secret was compromised, attacker still has access | Implement the revocation step in the dual-credential pattern. After confirming the new secret works, explicitly disable the old one. Log a warning if revocation fails. |
| Transmitting secrets over unencrypted connections | Database password change sent in plaintext over the network | Verify provider connections use TLS/SSL where available. Warn (or refuse) if connecting to a database without TLS in non-localhost scenarios. |

## UX Pitfalls

Common user experience mistakes in this domain.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| No dry-run mode | Users are afraid to run the tool because rotation is destructive and irreversible | Implement `rotate --dry-run` that shows exactly what would happen (which secrets, which containers) without changing anything |
| Opaque error messages during rotation failure | User sees "rotation failed" but does not know if the system is in a broken state | Report exactly which step failed, what the current state is (old password still active? new password active? containers restarted?), and what manual recovery steps are needed |
| Requiring full config before first scan | User has to write a complete `rotator.yml` before seeing any value from the tool | Let `scan` work with zero config -- discover and report what it finds. Let config be additive (only needed for provider-specific connection details). |
| No confirmation prompt for multi-secret rotation | User accidentally rotates 15 secrets when they meant to rotate one | Require explicit `--yes` flag for batch operations, or show a summary and ask for confirmation in interactive mode |
| Silent success with no summary | User runs `rotate` and sees nothing, unsure if it worked | Print a clear summary: "Rotated 3 secrets. Restarted 5 containers. All health checks passed." with timing information |
| Cron-scheduled rotation with no notification on failure | Automated rotation fails silently for weeks; user discovers when manually checking | Webhook notifications must fire on failure, not just success. Make failure notifications the default; success notifications opt-in. |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **Rotation provider:** Often missing the revocation of old credentials -- verify old password is explicitly disabled after new one is confirmed working
- [ ] **Container restart:** Often missing readiness verification -- verify the container is not just running but actually accepting connections
- [ ] **`.env` writing:** Often missing atomic write -- verify a crash mid-write does not corrupt the file (test with `kill -9` during write)
- [ ] **Rollback:** Often missing the "rollback the rollback" case -- verify what happens if rollback itself fails (e.g., database unreachable during rollback)
- [ ] **Scan command:** Often missing secrets that are in `env_file` directives, Docker Compose interpolation, or non-standard `.env` file locations
- [ ] **History encryption:** Often missing key verification -- verify the user can check their passphrase is correct without attempting a real rollback
- [ ] **Scheduled rotation:** Often missing overlap protection -- verify two scheduled rotations of the same secret cannot run concurrently
- [ ] **Webhook notifications:** Often missing failure context -- verify the webhook payload includes which step failed and what the current system state is
- [ ] **Multi-container secrets:** Often missing exhaustive consumer discovery -- verify ALL containers using a secret are identified, not just the "obvious" one

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Split-brain (DB has new password, `.env` has old) | MEDIUM | If state file exists, read which credential is active and update `.env` manually. If no state file, try both old and new passwords against DB to determine current state, then update `.env` to match. |
| Corrupted `.env` file | LOW if backup exists, HIGH if not | Restore from the timestamped backup the tool should have created. If no backup, reconstruct from Docker container environment (`docker inspect`) which still has the last-loaded values. |
| Master key lost | MEDIUM | History is irrecoverable, but the system still works. Current secrets are in `.env` files (plaintext). Create a new master key and start fresh history. Document this recovery path. |
| Redis password out of sync with config file | LOW | Connect to Redis with current runtime password, call `CONFIG REWRITE` to persist it, or update `redis.conf` manually to match the runtime password. |
| Cascading container failures after rotation | MEDIUM | Run `docker compose up -d` in the affected stack to restart all services with current `.env` values. If the `.env` was updated correctly, this recovers the stack. If not, restore `.env` backup first. |
| Concurrent rotation corruption | HIGH | Stop the rotation scheduler. Check state files for each secret. Manually verify each secret's current state (which password is active in DB vs. `.env`). Reconcile one at a time. |

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Non-atomic rotation / split-brain | Phase 1: Core rotation engine | Integration test that kills the process at each rotation step and verifies recovery |
| Docker socket over-privilege | Phase 1: Docker integration | Document socket proxy setup; test with proxy that blocks disallowed endpoints |
| `.env` file corruption | Phase 1: File handling | Test atomic write with concurrent reads; test crash-during-write recovery |
| Redis connection time-bomb | Phase 2: Provider implementations | Test with multiple Redis consumers; verify all are restarted and reconnected |
| Container restart ordering | Phase 1: Restart orchestration | Test with slow-starting database; verify dependent containers wait for readiness |
| Master key loss / weak crypto | Phase 2: Encrypted history | Crypto review; test with known test vectors; verify Argon2id parameters |
| No dry-run mode | Phase 1: CLI design | Verify `--dry-run` produces accurate output matching actual rotation behavior |
| Silent scheduled failures | Phase 3: Scheduling and notifications | Test webhook fires on rotation failure; verify payload contains failure details |
| Concurrent rotation races | Phase 3: Scheduling | Test two simultaneous rotations of same secret; verify file-lock prevents corruption |
| Secrets in logs | Phase 1: Logging framework | Grep test output and log files for any secret values; fail CI if found |

## Sources

- [Secret Rotation: How It Works, Challenges & Best Practices - Groundcover](https://www.groundcover.com/learn/security/secret-rotation-how-it-works-challenges-best-practices)
- [Rotating PostgreSQL Passwords with No Downtime - Jannik Arndt](https://www.jannikarndt.de/blog/2018/08/rotating_postgresql_passwords_with_no_downtime/)
- [MySQL Password Rotation with AWS Secrets Manager and Lambda](https://hackmysql.com/mysql-password-rotation-lambda/)
- [Docker Socket Security: A Critical Vulnerability Guide](https://medium.com/@instatunnel/docker-socket-security-a-critical-vulnerability-guide-76f4137a68c5)
- [Why is Exposing the Docker Socket a Really Bad Idea? - Quarkslab](https://blog.quarkslab.com/why-is-exposing-the-docker-socket-a-really-bad-idea.html)
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [Control Startup Order - Docker Compose Docs](https://docs.docker.com/compose/how-tos/startup-order/)
- [5 Common Mistakes with Encryption at Rest - Evervault](https://evervault.com/blog/common-mistakes-encryption-at-rest)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
- [CONFIG SET requirepass leaves auth enabled with empty password - Redis GitHub Issue #7898](https://github.com/redis/redis/issues/7898)
- [Stop Worrying, Start Rotating Your Secrets - Oasis Security](https://www.oasis.security/blog/stop-worrying-start-rotating)
- [How to Effectively Rotate Secrets - Security Boulevard](https://securityboulevard.com/2025/06/how-to-effectively-rotate-secrets-to-improve-security-and-efficiency/)
- [godotenv - Go dotenv library](https://github.com/joho/godotenv)

---
*Pitfalls research for: Secret rotation tool for self-hosted Docker environments*
*Researched: 2026-03-27*
