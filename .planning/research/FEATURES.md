# Feature Research

**Domain:** Self-hosted secret rotation for Docker Compose environments
**Researched:** 2026-03-27
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Secret auto-discovery from `.env` files | Users will not manually list every secret. Scanning for `*_PASSWORD`, `*_SECRET`, `*_KEY` patterns is the minimum viable workflow. | MEDIUM | Regex-based pattern matching on env var names. Must handle multi-file `.env` setups and docker-compose `env_file` directives. |
| CLI commands: `scan`, `rotate`, `status`, `history` | Every rotation tool (Vault, Infisical, Doppler) exposes these four verbs. Users need to see what will change, trigger changes, check current state, and audit past changes. | MEDIUM | `scan` = read-only discovery, `rotate` = execute rotation, `status` = show secret ages/health, `history` = audit trail. |
| MySQL/MariaDB password rotation | Most common homelab database. MariaDB ships in nearly every self-hosted stack (Nextcloud, Gitea, WordPress). | MEDIUM | `ALTER USER ... IDENTIFIED BY` on MySQL 5.7+/MariaDB 10.2+. Must handle both root and application user rotation. |
| PostgreSQL password rotation | Second most common homelab DB. Powers Immich, Authentik, Grafana, many others. | MEDIUM | `ALTER ROLE ... PASSWORD`. Nearly identical pattern to MySQL provider. |
| Redis password rotation | Redis is ubiquitous in self-hosted stacks (caching, session storage). Often left with no password at all. | LOW | `CONFIG SET requirepass` + `AUTH` verification. Simpler than SQL databases since there is no user concept in Redis <7. |
| Generic secret regeneration | JWT secrets, API keys, session tokens -- secrets that just need a new random value written to `.env` and containers restarted. No external system to update. | LOW | Generate cryptographically random string, update `.env`, restart affected containers. |
| Container restart orchestration | Rotation is pointless if containers keep running with the old secret in their environment. Users expect the tool handles the full lifecycle. | HIGH | Must stop/start containers in dependency order (DB before app). Requires parsing `depends_on` from compose files or explicit config. |
| Automatic rollback on failure | Vault, Infisical, and Doppler all guarantee rollback if rotation fails. Users will not trust a tool that can brick their stack. | HIGH | Must restore old secret in `.env`, re-apply old credentials in the database, and restart containers. This is the hardest table-stakes feature. |
| Encrypted secret history | Security-conscious users (the target audience) will not accept plaintext secret storage. Even homelab users expect encryption at rest. | MEDIUM | AES-256-GCM with key derived from user passphrase (Argon2id KDF). Store history in a local encrypted file, not a database. |
| YAML configuration file | Every comparable tool uses declarative config. Users expect to define rotation policies, provider connection details, and schedules in a config file. | LOW | `rotator.yml` with secret definitions, provider configs, schedules. Well-understood pattern. |
| Cron-based scheduled rotation | Rotation on a schedule (30/60/90 days) is standard across Vault, Infisical, Doppler. Manual-only rotation is a non-starter for the target audience. | MEDIUM | Parse cron expressions. Run as a long-lived process or register with system cron. Docker container mode makes long-lived daemon natural. |
| Dry-run mode | Users need to preview what will happen before rotation executes. Every infrastructure tool worth using has `--dry-run`. | LOW | Show what would rotate, which containers would restart, without making changes. |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not required, but valuable.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Zero-config auto-discovery via Docker labels | Competitors require extensive config files or a web dashboard. Discovering secrets by inspecting running containers and their `.env` files with no config at all is a major DX win. | MEDIUM | Inspect Docker containers for `env_file` mounts, parse compose files, cross-reference env var names with known secret patterns. |
| Docker label-based configuration | Allow per-container rotation config via Docker labels (e.g., `rotator.provider=mysql`, `rotator.schedule=@monthly`). Keeps config co-located with the service definition. | LOW | Read labels from Docker API. Merge with YAML config (YAML takes precedence). |
| Single binary / single container distribution | Vault requires a server + unsealing + HA backend. Infisical requires PostgreSQL + Redis + its own stack. Secret Rotator is one binary or one container, period. | LOW | Already planned. This is the core value proposition -- lightweight where competitors are heavy. |
| Webhook notifications (Discord, Slack, generic HTTP) | Homelab users live in Discord. Getting a notification that secrets were rotated (or that rotation failed) is high value for low effort. | LOW | POST JSON to configured webhook URLs. Discord/Slack have well-documented webhook formats. |
| Dependency-aware container restart ordering | Most tools restart containers individually. Restarting containers in dependency order (DB first, then app, then reverse proxy) prevents cascading failures during rotation. | HIGH | Parse `depends_on` from compose files, build dependency graph, topological sort for restart order. |
| Health check verification after rotation | After rotating and restarting, verify the container is actually healthy before considering rotation complete. Prevents silent failures. | MEDIUM | Poll Docker health check status. If container becomes unhealthy within a timeout, trigger rollback. |
| Dual-credential rotation (zero-downtime) | Vault and Doppler maintain two valid credentials during rotation. For homelabs this is less critical (brief downtime is acceptable), but it is a sophisticated differentiator. | HIGH | Requires creating two DB users per secret and alternating between them. Significant complexity for v2+. |
| Secret strength auditing | Scan existing secrets and flag weak ones (short passwords, default values like `changeme`, dictionary words). Helps users discover problems before rotation. | LOW | Check length, entropy, common default password list. Report in `scan` output. |
| Compose file secret injection | Instead of modifying `.env` files in-place, support writing secrets to Docker Compose `secrets:` with file-based secrets for better security posture. | MEDIUM | Write secrets to files, update compose to use `secrets:` directive. Breaking change for existing setups, so must be opt-in. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Web dashboard / UI | Visual management feels more accessible than CLI. | Adds massive surface area: auth, sessions, CSRF, a whole frontend framework. Contradicts the "lightweight" value prop. Every homelab user who wants this tool already lives in the terminal. | Rich CLI output with color, tables, and `--json` flag for scripting. Consider TUI (terminal UI) in v2 if demand exists. |
| Vault/SOPS integration | Users already storing secrets in Vault or SOPS want to keep using them. | This tool IS the lightweight alternative to Vault. Wrapping Vault defeats the purpose. SOPS integration adds decryption complexity and key management scope creep. | Document migration path FROM Vault/SOPS. Be the simpler replacement, not a wrapper. |
| Kubernetes / Swarm support | K8s users want rotation too. | K8s has External Secrets Operator, Sealed Secrets, native Secret rotation with CSI drivers. The ecosystem is well-served. Docker Compose is the underserved niche. | Stay focused on Docker Compose. K8s support fragments the codebase and dilutes the value prop. |
| Multi-host rotation | Users running Docker on multiple hosts want centralized rotation. | Requires service discovery, network coordination, distributed locking, SSH or agent-based access. Complexity explosion for a v1. | Single-host only in v1. Document how to run one instance per host. Consider agent architecture in v2+. |
| Certificate rotation (TLS) | Users want to rotate TLS certs alongside passwords. | Certificate rotation is a fundamentally different domain (ACME, CA trust chains, SNI). Certbot/Traefik already solve this well. | Out of scope. Recommend Traefik + Let's Encrypt or certbot for TLS. |
| Cloud provider secret store sync | Sync rotated secrets to AWS SSM, GCP Secret Manager, etc. | Self-hosted tool syncing to cloud stores creates a confusing identity. Target audience chose self-hosting to avoid cloud dependencies. | Out of scope. The tool manages local secrets, period. |
| Plugin system for custom providers | Extensibility for arbitrary secret backends. | Plugin systems are maintenance nightmares for small projects. Go plugin support is limited (shared libraries, same Go version required). | Ship built-in providers. Accept PRs for new providers. Use a well-defined Provider interface internally so adding providers is easy without a plugin system. |
| Real-time file watching for .env changes | Detect external .env modifications and re-sync. | Race conditions, infinite loops (tool writes .env, detects change, writes again). Unclear what the "correct" behavior should be when someone manually edits a managed secret. | Rotation is the source of truth. `status` command shows drift between expected and actual values. |

## Feature Dependencies

```
[Auto-discovery from .env]
    └──requires──> [Docker API access (container inspection)]
    └──enhances──> [YAML configuration] (discovery can pre-populate config)

[Container restart orchestration]
    └──requires──> [Docker API access (read-write)]
    └──enhances──> [Dependency-aware restart ordering]

[Automatic rollback]
    └──requires──> [Encrypted secret history] (need old secret to restore)
    └──requires──> [Container restart orchestration] (need to restart after restore)
    └──requires──> [Provider implementation] (need to revert DB credentials)

[Scheduled rotation]
    └──requires──> [Rotate command implementation]
    └──requires──> [YAML configuration] (schedule defined in config)

[Health check verification]
    └──requires──> [Container restart orchestration]
    └──enhances──> [Automatic rollback] (health failure triggers rollback)

[Webhook notifications]
    └──enhances──> [Rotate command] (notify on success/failure)
    └──enhances──> [Scheduled rotation] (notify on automated rotations)

[Secret strength auditing]
    └──enhances──> [Scan command]
    └──independent of──> [Rotation logic]

[Dry-run mode]
    └──requires──> [Scan + Rotate command logic]
    └──independent of──> [Provider implementations] (simulates without connecting)
```

### Dependency Notes

- **Automatic rollback requires encrypted secret history:** You cannot roll back without knowing the previous credential. History must be written BEFORE rotation begins.
- **Container restart requires Docker API read-write access:** Scan can work read-only, but rotation needs container control.
- **Health check verification enhances rollback:** Without health checks, rollback is only triggered by explicit provider errors. Health checks catch silent failures (e.g., app starts but cannot authenticate).
- **Webhook notifications are independent:** Can be added at any phase without changing core logic. Just hooks into success/failure events.
- **Secret strength auditing is independent:** Pure analysis, no mutation. Can ship in v1 scan command with minimal effort.

## MVP Definition

### Launch With (v1.0)

Minimum viable product -- what is needed to validate the concept.

- [x] `scan` command -- discover secrets in `.env` files across Docker Compose projects
- [x] `rotate` command -- execute rotation for a single secret or all due secrets
- [x] `status` command -- show secret ages, next rotation, health
- [x] `history` command -- show rotation audit trail
- [x] MySQL/MariaDB provider -- most common homelab DB
- [x] PostgreSQL provider -- second most common
- [x] Redis provider -- simple, common, good for proving the pattern
- [x] Generic provider -- random regeneration for JWT secrets, API keys
- [x] Automatic rollback on failure -- non-negotiable for trust
- [x] Encrypted secret history -- AES-256-GCM with passphrase-derived key
- [x] Container restart orchestration -- stop/start affected containers
- [x] YAML configuration file -- declarative secret and provider definitions
- [x] Dry-run mode -- preview before executing
- [x] Docker container distribution -- mount socket + config
- [x] Standalone Go binary distribution

### Add After Validation (v1.x)

Features to add once core is working and users are giving feedback.

- [ ] Scheduled rotation via cron expressions -- add once manual rotation is battle-tested
- [ ] Webhook notifications (Discord, Slack, HTTP) -- add once rotation events are well-defined
- [ ] Docker label-based configuration -- add once YAML config schema is stable
- [ ] Secret strength auditing in `scan` output -- low effort, high value
- [ ] Dependency-aware container restart ordering -- add once basic restart works reliably
- [ ] Health check verification after rotation -- add once rollback is proven

### Future Consideration (v2+)

Features to defer until product-market fit is established.

- [ ] Dual-credential zero-downtime rotation -- complex, enterprise-oriented
- [ ] Compose file `secrets:` injection -- breaking workflow change, needs careful design
- [ ] TUI (terminal UI) dashboard -- only if CLI proves insufficient
- [ ] Multi-host support via agent architecture -- only if single-host adoption is strong
- [ ] Additional providers (MongoDB, LDAP, SMTP credentials) -- driven by user requests

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Auto-discovery from `.env` | HIGH | MEDIUM | P1 |
| CLI: scan/rotate/status/history | HIGH | MEDIUM | P1 |
| MySQL/MariaDB provider | HIGH | MEDIUM | P1 |
| PostgreSQL provider | HIGH | MEDIUM | P1 |
| Redis provider | MEDIUM | LOW | P1 |
| Generic secret provider | HIGH | LOW | P1 |
| Container restart orchestration | HIGH | HIGH | P1 |
| Automatic rollback | HIGH | HIGH | P1 |
| Encrypted secret history | HIGH | MEDIUM | P1 |
| YAML configuration | HIGH | LOW | P1 |
| Dry-run mode | MEDIUM | LOW | P1 |
| Scheduled rotation (cron) | HIGH | MEDIUM | P2 |
| Webhook notifications | MEDIUM | LOW | P2 |
| Docker label config | MEDIUM | LOW | P2 |
| Secret strength auditing | MEDIUM | LOW | P2 |
| Dependency-aware restart | MEDIUM | HIGH | P2 |
| Health check verification | MEDIUM | MEDIUM | P2 |
| Dual-credential rotation | LOW | HIGH | P3 |
| Compose secrets injection | LOW | MEDIUM | P3 |
| TUI dashboard | LOW | MEDIUM | P3 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add in v1.x releases
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | Vault | Infisical | Doppler | Secret Rotator (Ours) |
|---------|-------|-----------|---------|----------------------|
| Database rotation | Yes (dynamic secrets, many DBs) | Yes (MySQL, PostgreSQL, MSSQL, Oracle) | Yes (AWS, DB integrations) | Yes (MySQL, PostgreSQL, Redis, Generic) |
| Dual-credential rotation | Yes (two DB users) | Yes ("rolling lifecycle") | Yes (active/inactive pair) | No (v1), Yes (v2+) |
| Auto-discovery | No (manual policy config) | No (manual project setup) | No (manual project setup) | Yes (scan .env files automatically) |
| Rollback | Partial (revoke lease) | Partial (revert to previous version) | Yes (atomic operations) | Yes (full rollback: DB + .env + containers) |
| Container restart | No (separate concern) | No (separate concern) | No (separate concern) | Yes (built-in, dependency-aware) |
| Setup complexity | High (server + unseal + HA backend) | Medium (PostgreSQL + Redis + app) | Low (SaaS, but cloud-only) | Very Low (single binary or container) |
| Self-hosted | Yes (complex) | Yes (complex) | No (cloud SaaS) | Yes (trivially simple) |
| Scheduling | Yes (TTL-based) | Yes (interval-based) | Yes (interval-based) | Yes (cron expressions) |
| Notifications | Yes (audit log + plugins) | Yes (webhooks, Slack) | Yes (webhooks, Slack) | Yes (webhooks: Discord, Slack, HTTP) |
| Encryption at rest | Yes (barrier encryption) | Yes (KMS) | Yes (cloud KMS) | Yes (AES-256-GCM, passphrase-derived) |
| Target audience | Enterprise | Teams / Enterprise | Teams / Enterprise | Homelab / Self-hosted individuals |

**Key competitive insight:** No existing tool combines auto-discovery + rotation + container restart + rollback in a single lightweight package for Docker Compose environments. Vault, Infisical, and Doppler handle rotation but treat container lifecycle as someone else's problem. Secret Rotator owns the full lifecycle from discovery to verified restart.

## Sources

- [Infisical Secret Rotation Overview](https://infisical.com/docs/documentation/platform/secret-rotation/overview) -- rotation architecture, dual-phase model
- [Infisical PostgreSQL Rotation](https://infisical.com/docs/documentation/platform/secret-rotation/postgres-credentials) -- provider implementation details
- [Doppler Zero Downtime Rotation Guide](https://www.doppler.com/blog/10-step-secrets-rotation-guide) -- rotation best practices, rollback strategies
- [Doppler Rotation Engine Architecture](https://www.doppler.com/blog/doppler-secrets-rotation-core-logic) -- atomic rotation, dual-credential pattern
- [HashiCorp Vault Auto-Rotation](https://developer.hashicorp.com/hcp/docs/vault-secrets/auto-rotation) -- schedule-based rotation, provider integrations
- [HashiCorp Vault Database Secrets Engine](https://developer.hashicorp.com/vault/docs/secrets/databases) -- DB credential rotation patterns
- [Docker Compose Secrets](https://docs.docker.com/compose/how-tos/use-secrets/) -- native Docker secret management
- [Self-Hosted Secrets Management for Homelabs](https://www.antlatt.com/blog/self-hosted-secrets-management/) -- homelab pain points, .env security issues
- [Secret Rotation Strategies](https://oneuptime.com/blog/post/2026-01-30-security-secret-rotation-strategies/view) -- rotation patterns, rollback approaches

---
*Feature research for: self-hosted Docker secret rotation*
*Researched: 2026-03-27*
