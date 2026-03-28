# Requirements: Secret Rotator

**Defined:** 2026-03-28
**Core Value:** Secrets in self-hosted Docker environments get rotated automatically without manual database console or container restart work.

## v1 Requirements

### Discovery

- [x] **DISC-01**: Tool auto-scans `.env` files and identifies secrets by naming patterns (MYSQL_PASSWORD, REDIS_PASSWORD, JWT_SECRET, etc.)
- [x] **DISC-02**: Tool audits password strength and flags weak, default, or short passwords
- [x] **DISC-03**: Tool works zero-config without `rotator.yml`, scanning the current directory
- [x] **DISC-04**: Tool supports multi-file `.env` setups (`.env`, `.env.local`, `docker-compose.override.yml`)

### Rotation

- [x] **ROT-01**: User can manually rotate a specific secret via `rotator rotate SECRET_NAME`
- [x] **ROT-02**: Tool supports scheduled rotation via cron expressions in config or Docker labels
- [x] **ROT-03**: Tool writes `.env` files atomically (temp file + rename) to prevent corruption
- [x] **ROT-04**: Tool automatically rolls back if rotation fails (restores old secret, restarts containers)

### Providers

- [x] **PROV-01**: Generic provider regenerates password, updates `.env`, and restarts containers without DB interaction
- [x] **PROV-02**: MySQL/MariaDB provider executes ALTER USER for password rotation
- [x] **PROV-03**: PostgreSQL provider executes ALTER ROLE for password rotation
- [x] **PROV-04**: Redis provider executes CONFIG SET requirepass + CONFIG REWRITE for password rotation

### Infrastructure

- [x] **INFR-01**: Tool restarts affected containers in dependency order with readiness checks
- [x] **INFR-02**: Old secrets are encrypted at rest using AES-256-GCM with Argon2id key derivation
- [x] **INFR-03**: Tool sends webhook notifications (Discord, Slack, generic HTTP) on rotation success/failure
- [x] **INFR-04**: Tool supports Docker label-based configuration (com.secret-rotator.schedule)

### CLI

- [x] **CLI-01**: `rotator scan` command discovers and reports secrets with strength audit
- [x] **CLI-02**: `rotator rotate` command performs on-demand secret rotation
- [x] **CLI-03**: `rotator status` command shows current rotation state and schedules
- [x] **CLI-04**: `rotator history` command shows rotation audit log

### Distribution

- [ ] **DIST-01**: Tool is distributed as a Docker container image
- [x] **DIST-02**: Tool is distributed as a standalone Go binary (Linux, macOS, ARM)
- [x] **DIST-03**: Tool uses YAML configuration file (`rotator.yml`) for secret definitions

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
| DISC-01 | Phase 2 | Complete |
| DISC-02 | Phase 2 | Complete |
| DISC-03 | Phase 1 | Complete |
| DISC-04 | Phase 1 | Complete |
| ROT-01 | Phase 3 | Complete |
| ROT-02 | Phase 4 | Complete |
| ROT-03 | Phase 1 | Complete |
| ROT-04 | Phase 3 | Complete |
| PROV-01 | Phase 3 | Complete |
| PROV-02 | Phase 3 | Complete |
| PROV-03 | Phase 3 | Complete |
| PROV-04 | Phase 3 | Complete |
| INFR-01 | Phase 1 | Complete |
| INFR-02 | Phase 2 | Complete |
| INFR-03 | Phase 4 | Complete |
| INFR-04 | Phase 4 | Complete |
| CLI-01 | Phase 2 | Complete |
| CLI-02 | Phase 3 | Complete |
| CLI-03 | Phase 4 | Complete |
| CLI-04 | Phase 2 | Complete |
| DIST-01 | Phase 5 | Pending |
| DIST-02 | Phase 5 | Complete |
| DIST-03 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 23 total
- Mapped to phases: 23
- Unmapped: 0

---
*Requirements defined: 2026-03-28*
*Last updated: 2026-03-27 after roadmap creation*
