# Roadmap: Secret Rotator

## Overview

Secret Rotator goes from zero to a distributable CLI tool in 5 phases. Foundation work (Go project scaffold, config loading, .env file handling, Docker integration) comes first because every subsequent phase depends on it. Discovery and crypto are built next as independently testable vertical slices that prove the scan and history commands before touching rotation logic. The rotation engine and all four providers form the core phase where the product's central value is delivered. Scheduling and operational commands layer on top of working rotation. Distribution packages the result.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation** - Project scaffold, CLI skeleton, config loading, .env file handling, Docker integration (completed 2026-03-28)
- [x] **Phase 2: Discovery and Crypto** - Secret scanning, strength auditing, encrypted secret store, scan and history commands (completed 2026-03-28)
- [x] **Phase 3: Rotation Engine** - Provider system, execution state machine, rollback, all four providers, rotate command (completed 2026-03-28)
- [x] **Phase 4: Scheduling and Operations** - Cron-based scheduling, status command, webhook notifications, Docker label config (completed 2026-03-28)
- [x] **Phase 5: Distribution** - Docker container image, standalone Go binary, goreleaser cross-compilation (completed 2026-03-28)

## Phase Details

### Phase 1: Foundation
**Goal**: A working CLI skeleton that can load configuration, read/write .env files atomically, and interact with the Docker daemon for container lifecycle operations
**Depends on**: Nothing (first phase)
**Requirements**: DISC-03, DISC-04, DIST-03, INFR-01, ROT-03
**Success Criteria** (what must be TRUE):
  1. User can run `rotator --help` and see available commands (scan, rotate, status, history)
  2. Tool loads and validates a `rotator.yml` configuration file, reporting clear errors for invalid config
  3. Tool reads `.env` files (including `.env.local` and multi-file setups) preserving comments and formatting
  4. Tool writes `.env` files atomically via temp-file-plus-rename without corrupting existing content
  5. Tool can list, inspect, stop, start, and health-check Docker containers, restarting them in dependency order
**Plans**: 3 plans

Plans:
- [ ] 01-01-PLAN.md — Project scaffold, config loading (koanf), CLI skeleton (Cobra)
- [ ] 01-02-PLAN.md — .env file reader/writer with atomic writes
- [ ] 01-03-PLAN.md — Docker manager interface, SDK client, compose dependency ordering

### Phase 2: Discovery and Crypto
**Goal**: Users can scan their environment to discover secrets with strength auditing, and the tool can encrypt and store rotation history
**Depends on**: Phase 1
**Requirements**: DISC-01, DISC-02, CLI-01, CLI-04, INFR-02
**Success Criteria** (what must be TRUE):
  1. `rotator scan` discovers secrets in .env files by naming patterns and reports their type, associated containers, and strength rating
  2. `rotator scan` flags weak, default, or short passwords with actionable warnings
  3. `rotator scan` works zero-config without a `rotator.yml` file present
  4. `rotator history` displays the encrypted rotation audit log with timestamps and outcomes
  5. Old secrets are encrypted at rest using AES-256-GCM with Argon2id key derivation from a master passphrase
**Plans**: 2 plans

Plans:
- [ ] 02-01-PLAN.md — Discovery engine (scanner, patterns, strength auditor) and scan command
- [ ] 02-02-PLAN.md — Crypto primitives (Argon2id + AES-256-GCM), encrypted history store, and history command

### Phase 3: Rotation Engine
**Goal**: Users can rotate secrets on demand with automatic rollback on failure, using any of the four supported providers
**Depends on**: Phase 2
**Requirements**: ROT-01, ROT-04, PROV-01, PROV-02, PROV-03, PROV-04, CLI-02
**Success Criteria** (what must be TRUE):
  1. `rotator rotate SECRET_NAME` rotates a specific secret end-to-end (generate, apply to DB, update .env, restart containers)
  2. Generic provider regenerates a password, updates .env, and restarts affected containers without any database interaction
  3. MySQL/MariaDB provider executes ALTER USER to rotate the database password and updates all referencing .env files and containers
  4. PostgreSQL provider executes ALTER ROLE to rotate the database password and updates all referencing .env files and containers
  5. Redis provider executes CONFIG SET requirepass plus CONFIG REWRITE and restarts all consumers of that password
  6. If any step in rotation fails, the tool automatically rolls back (restores old secret in DB and .env, restarts containers) leaving the system in its pre-rotation state
**Plans**: 3 plans

Plans:
- [ ] 03-01-PLAN.md — Provider interface, registry, password generation, generic provider
- [ ] 03-02-PLAN.md — Execution engine state machine with LIFO rollback
- [ ] 03-03-PLAN.md — MySQL, PostgreSQL, Redis providers and rotate CLI wiring

### Phase 4: Scheduling and Operations
**Goal**: Users can automate rotation on a schedule, check system status, and receive notifications on rotation events
**Depends on**: Phase 3
**Requirements**: ROT-02, CLI-03, INFR-03, INFR-04
**Success Criteria** (what must be TRUE):
  1. `rotator status` shows each managed secret's current state, age, and next scheduled rotation time
  2. Tool runs in daemon mode executing scheduled rotations defined by cron expressions in config or Docker labels
  3. Tool sends webhook notifications (Discord, Slack, generic HTTP) on rotation success and failure
  4. Tool reads rotation schedule from Docker labels (com.secret-rotator.schedule) as an alternative to YAML config
**Plans**: 3 plans

Plans:
- [ ] 04-01-PLAN.md — Webhook notifier (Discord, Slack, generic HTTP) and Docker label schedule reader
- [ ] 04-02-PLAN.md — Status command showing secret state, age, and next rotation
- [ ] 04-03-PLAN.md — Cron-based scheduler daemon with notification integration

### Phase 5: Distribution
**Goal**: Users can install and run the tool as either a Docker container or a standalone binary on Linux, macOS, and ARM
**Depends on**: Phase 4
**Requirements**: DIST-01, DIST-02
**Success Criteria** (what must be TRUE):
  1. Tool is available as a Docker container image that mounts the Docker socket and config directory, running as a non-root user
  2. Tool is available as a standalone Go binary for linux/amd64, linux/arm64, darwin/amd64, and darwin/arm64 via goreleaser
  3. A new user can go from download to first `rotator scan` in under 5 minutes following the provided example config
**Plans**: 2 plans

Plans:
- [ ] 05-01-PLAN.md — Docker container image, version injection, non-root user
- [ ] 05-02-PLAN.md — Goreleaser cross-compilation config and Makefile

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 3/3 | Complete    | 2026-03-28 |
| 2. Discovery and Crypto | 2/2 | Complete    | 2026-03-28 |
| 3. Rotation Engine | 2/3 | Complete    | 2026-03-28 |
| 4. Scheduling and Operations | 0/3 | Complete    | 2026-03-28 |
| 5. Distribution | 0/2 | Complete    | 2026-03-28 |
