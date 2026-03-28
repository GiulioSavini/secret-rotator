---
phase: 03-rotation-engine
plan: 03
subsystem: provider
tags: [mysql, postgres, redis, go-sql-driver, pgx, go-redis, cli, cobra]

requires:
  - phase: 03-rotation-engine/01
    provides: Provider interface, Registry, GenericProvider, password generation
  - phase: 03-rotation-engine/02
    provides: Engine executor with LIFO rollback pipeline

provides:
  - MySQL provider (ALTER USER via go-sql-driver/mysql)
  - PostgreSQL provider (ALTER ROLE via jackc/pgx with Identifier sanitization)
  - Redis provider (CONFIG SET requirepass + CONFIG REWRITE with rollback on REWRITE failure)
  - Fully wired rotate CLI command dispatching to engine pipeline

affects: [04-scheduling, 05-notifications]

tech-stack:
  added: [go-sql-driver/mysql v1.9, jackc/pgx/v5, redis/go-redis/v9]
  patterns: [admin/target user separation, DSN builder helpers, CONFIG SET + REWRITE atomic pattern]

key-files:
  created:
    - internal/provider/mysql.go
    - internal/provider/postgres.go
    - internal/provider/redis.go
    - internal/provider/mysql_test.go
    - internal/provider/postgres_test.go
    - internal/provider/redis_test.go
    - internal/cli/rotate_test.go
  modified:
    - internal/cli/rotate.go
    - go.mod
    - go.sum

key-decisions:
  - "Admin/target user separation: Options[target_user] overrides Username for password rotation target"
  - "Admin password resolution: Options[password] > Options[password_env] via os.Getenv"
  - "Redis rollback on CONFIG REWRITE failure: immediate CONFIG SET to old password before returning error"
  - "History store optional in rotate command: nil disables recording, rotation still works"

patterns-established:
  - "DB provider pattern: generate password, connect as admin, ALTER statement, verify as target user"
  - "Redis atomic pair: CONFIG SET + CONFIG REWRITE with rollback on REWRITE failure"
  - "CLI wiring: registry.Register all providers, registry.Get by config type, engine.Execute"

requirements-completed: [PROV-02, PROV-03, PROV-04, CLI-02]

duration: 8min
completed: 2026-03-28
---

# Phase 3 Plan 3: Database Providers and Rotate CLI Summary

**MySQL, PostgreSQL, and Redis password rotation providers plus fully wired `rotator rotate SECRET_NAME` CLI command**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-28T16:43:39Z
- **Completed:** 2026-03-28T16:51:36Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- MySQL provider rotates passwords via ALTER USER with go-sql-driver/mysql
- PostgreSQL provider rotates passwords via ALTER ROLE with pgx v5 and Identifier sanitization
- Redis provider rotates via CONFIG SET requirepass + CONFIG REWRITE with automatic rollback on REWRITE failure
- Rotate CLI command fully wired: finds secret in config, resolves provider via registry, creates engine, executes pipeline
- All unit tests pass, go vet clean, binary builds

## Task Commits

Each task was committed atomically:

1. **Task 1: MySQL, PostgreSQL, and Redis provider implementations**
   - `2954609` (test: add failing tests for MySQL, PostgreSQL, and Redis providers)
   - `7ecc6bc` (feat: implement MySQL, PostgreSQL, and Redis providers)

2. **Task 2: Wire rotate CLI command to execution engine**
   - `9d6f71b` (test: add failing tests for wired rotate CLI command)
   - `d130b78` (feat: wire rotate CLI command to execution engine)

_Note: TDD tasks have two commits each (test then feat)_

## Files Created/Modified
- `internal/provider/mysql.go` - MySQL provider: ALTER USER via go-sql-driver/mysql
- `internal/provider/postgres.go` - PostgreSQL provider: ALTER ROLE via jackc/pgx v5
- `internal/provider/redis.go` - Redis provider: CONFIG SET requirepass + CONFIG REWRITE
- `internal/provider/mysql_test.go` - MySQL unit tests (Name, DSN, target user, interface)
- `internal/provider/postgres_test.go` - PostgreSQL unit tests (Name, connstr, target user, interface)
- `internal/provider/redis_test.go` - Redis unit tests (Name, addr, interface)
- `internal/cli/rotate.go` - Wired rotate command replacing stub
- `internal/cli/rotate_test.go` - CLI tests (args, config required, secret lookup, flags)
- `go.mod` - Added mysql, pgx, go-redis dependencies
- `go.sum` - Updated checksums

## Decisions Made
- Admin/target user separation: Options["target_user"] overrides Username for the password rotation target, so admin can rotate another user's password
- Admin password resolved from Options["password"] first, then Options["password_env"] from environment
- Redis rollback on CONFIG REWRITE failure: immediately CONFIG SET to old password to prevent locked-out state
- History store is optional in rotate command: nil store disables recording but rotation still works
- Docker client created per rotate invocation; deferred Close ensures cleanup

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All four provider types (generic, mysql, postgres, redis) fully implemented and tested
- `rotator rotate SECRET_NAME` works end-to-end for any configured secret type
- Phase 3 (Rotation Engine) is complete: provider interface, engine pipeline, all providers, wired CLI
- Ready for Phase 4 (Scheduling) and Phase 5 (Notifications)

---
*Phase: 03-rotation-engine*
*Completed: 2026-03-28*
