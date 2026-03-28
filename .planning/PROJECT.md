# Secret Rotator

## What This Is

A lightweight, CLI-first secret rotation tool for self-hosted Docker environments. It auto-discovers secrets in `.env` files, rotates them on schedule or on demand, updates the affected containers, and rolls back if anything fails. Distributed as a single Go binary and Docker container.

## Core Value

Secrets in self-hosted homelab environments get rotated automatically without the user needing to touch a database console or restart containers manually.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Auto-discover secrets in `.env` files (MYSQL_PASSWORD, REDIS_PASSWORD, JWT_SECRET, etc.)
- [ ] CLI commands: `scan`, `rotate`, `status`, `history`
- [ ] Provider: MySQL/MariaDB (ALTER USER for password rotation)
- [ ] Provider: PostgreSQL (ALTER ROLE for password rotation)
- [ ] Provider: Redis (CONFIG SET requirepass)
- [ ] Provider: Generic (regenerate + update .env + restart containers, no DB interaction)
- [ ] Automatic rollback if rotation fails (restore old secret, restart containers)
- [ ] Old secrets encrypted with master key before storage
- [ ] Scheduled rotation via cron expressions in config or Docker labels
- [ ] Container restart orchestration (stop/start affected containers in dependency order)
- [ ] Notification via webhooks (Discord, Slack, generic HTTP)
- [ ] YAML configuration file (`rotator.yml`) for secret definitions and provider config
- [ ] Distribute as Docker container (mounts Docker socket + config directory)
- [ ] Distribute as standalone Go binary

### Out of Scope

- Web dashboard / UI — CLI is the interface for v1
- Vault/SOPS integration — this is the lightweight alternative, not a wrapper
- Kubernetes / Swarm support — Docker Compose only for v1
- Multi-host rotation — single Docker host for v1
- Certificate rotation (TLS certs) — focused on application secrets only
- Cloud provider secret stores (AWS SSM, GCP Secret Manager) — self-hosted only

## Context

- Target audience: self-hosted / homelab users running Docker Compose stacks
- Common pain point: passwords set once during setup, never rotated, often left as defaults
- Existing tools (Vault, SOPS, sealed-secrets) are designed for enterprise/K8s, overkill for homelabs
- The tool needs Docker socket access to discover and restart containers
- Secret providers need direct DB connectivity to perform credential rotation
- Go chosen for: single binary distribution, native Docker SDK, familiarity from Arcane contributions

## Constraints

- **Tech stack**: Go backend, no frontend framework for v1
- **Distribution**: Docker container + standalone binary (goreleaser for cross-compilation)
- **Security**: Master key required for encrypted secret history — derived from user-provided passphrase or env var
- **Docker API**: Requires Docker socket access (read-only for scan, read-write for restart)
- **Database connectivity**: Providers need network access to the target databases (same Docker network)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| CLI-first, no dashboard | Lightest possible footprint, users want a tool not another webapp | — Pending |
| Auto-discovery from .env | Reduces config burden, most homelabs use .env files | — Pending |
| Go single binary | Consistent with Arcane ecosystem, easy distribution | — Pending |
| Encrypted rollback history | Security-conscious users expect secrets not stored in plaintext | — Pending |
| YAML config over labels-only | More expressive than Docker labels for provider connection strings | — Pending |

---
*Last updated: 2026-03-28 after initialization*
