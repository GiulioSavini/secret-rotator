# Requirements: Secret Rotator

**Defined:** 2026-03-28
**Core Value:** Secrets in self-hosted Docker environments get rotated automatically without manual database console or container restart work.

## v1 Requirements

### Discovery

- [ ] **DISC-01**: Tool auto-scans `.env` files and identifies secrets by naming patterns (MYSQL_PASSWORD, REDIS_PASSWORD, JWT_SECRET, etc.)
- [ ] **DISC-02**: Tool audits password strength and flags weak, default, or short passwords
- [ ] **DISC-03**: Tool works zero-config without `rotator.yml`, scanning the current directory
- [ ] **DISC-04**: Tool supports multi-file `.env` setups (`.env`, `.env.local`, `docker-compose.override.yml`)

### Rotation

- [ ] **ROT-01**: User can manually rotate a specific secret via `rotator rotate SECRET_NAME`
- [ ] **ROT-02**: Tool supports scheduled rotation via cron expressions in config or Docker labels
- [ ] **ROT-03**: Tool writes `.env` files atomically (temp file + rename) to prevent corruption
- [ ] **ROT-04**: Tool automatically rolls back if rotation fails (restores old secret, restarts containers)

### Providers

- [ ] **PROV-01**: Generic provider regenerates password, updates `.env`, and restarts containers without DB interaction
- [ ] **PROV-02**: MySQL/MariaDB provider executes ALTER USER for password rotation
- [ ] **PROV-03**: PostgreSQL provider executes ALTER ROLE for password rotation
- [ ] **PROV-04**: Redis provider executes CONFIG SET requirepass + CONFIG REWRITE for password rotation

### Infrastructure

- [ ] **INFR-01**: Tool restarts affected containers in dependency order with readiness checks
- [ ] **INFR-02**: Old secrets are encrypted at rest using AES-256-GCM with Argon2id key derivation
- [ ] **INFR-03**: Tool sends webhook notifications (Discord, Slack, generic HTTP) on rotation success/failure
- [ ] **INFR-04**: Tool supports Docker label-based configuration (com.secret-rotator.schedule)

### CLI

- [ ] **CLI-01**: `rotator scan` command discovers and reports secrets with strength audit
- [ ] **CLI-02**: `rotator rotate` command performs on-demand secret rotation
- [ ] **CLI-03**: `rotator status` command shows current rotation state and schedules
- [ ] **CLI-04**: `rotator history` command shows rotation audit log

### Distribution

- [ ] **DIST-01**: Tool is distributed as a Docker container image
- [ ] **DIST-02**: Tool is distributed as a standalone Go binary (Linux, macOS, ARM)
- [ ] **DIST-03**: Tool uses YAML configuration file (`rotator.yml`) for secret definitions

## v2 Requirements

### Enhanced Rotation

- **ROT-V2-01**: Dual-credential (zero-downtime) rotation for supported providers
- **ROT-V2-02**: Cross-stack secret rotation (secrets shared across multiple Compose stacks)
- **ROT-V2-03**: Certificate/TLS rotation support

### UI

- **UI-V2-01**: Optional web dashboard for rotation status and history
- **UI-V2-02**: Visual secret dependency graph

### Integrations

- **INT-V2-01**: MongoDB provider
- **INT-V2-02**: LDAP/Active Directory provider
- **INT-V2-03**: Apprise notification integration for broader notification support

## Out of Scope

| Feature | Reason |
|---------|--------|
| HashiCorp Vault integration | This tool IS the lightweight alternative to Vault |
| Kubernetes/Swarm support | Docker Compose only for v1, keeps scope manageable |
| Multi-host rotation | Single Docker host for v1 |
| Cloud secret stores (AWS SSM, GCP) | Self-hosted only, cloud users have native tools |
| Web dashboard | CLI-first for v1, dashboard is v2 if needed |
| SOPS/age encryption wrapper | Different approach — we manage our own encrypted store |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| DISC-01 | Phase 2 | Pending |
| DISC-02 | Phase 2 | Pending |
| DISC-03 | Phase 1 | Pending |
| DISC-04 | Phase 1 | Pending |
| ROT-01 | Phase 3 | Pending |
| ROT-02 | Phase 4 | Pending |
| ROT-03 | Phase 1 | Pending |
| ROT-04 | Phase 3 | Pending |
| PROV-01 | Phase 3 | Pending |
| PROV-02 | Phase 3 | Pending |
| PROV-03 | Phase 3 | Pending |
| PROV-04 | Phase 3 | Pending |
| INFR-01 | Phase 1 | Pending |
| INFR-02 | Phase 2 | Pending |
| INFR-03 | Phase 4 | Pending |
| INFR-04 | Phase 4 | Pending |
| CLI-01 | Phase 2 | Pending |
| CLI-02 | Phase 3 | Pending |
| CLI-03 | Phase 4 | Pending |
| CLI-04 | Phase 2 | Pending |
| DIST-01 | Phase 5 | Pending |
| DIST-02 | Phase 5 | Pending |
| DIST-03 | Phase 1 | Pending |

**Coverage:**
- v1 requirements: 23 total
- Mapped to phases: 23
- Unmapped: 0

---
*Requirements defined: 2026-03-28*
*Last updated: 2026-03-27 after roadmap creation*
