---
phase: 04-scheduling-and-operations
plan: 03
subsystem: scheduling
tags: [cron, robfig-cron, daemon, scheduler, cobra]

requires:
  - phase: 03-rotation-engine
    provides: Engine with Execute for single-secret rotation pipeline
  - phase: 04-scheduling-and-operations
    provides: Notifier/Dispatcher for webhook events, Docker label schedule reader

provides:
  - Cron-based scheduler that runs rotation jobs on configurable schedules
  - Daemon CLI command for long-running automated rotation
  - Concurrent rotation prevention via per-secret TryLock
  - Notification integration on rotation success/failure

affects: [05-polish-and-release]

tech-stack:
  added: [robfig/cron/v3]
  patterns: [injectable-rotate-func, per-secret-mutex-trylock, signal-context-shutdown]

key-files:
  created:
    - internal/scheduler/scheduler.go
    - internal/scheduler/scheduler_test.go
    - internal/cli/daemon.go
    - internal/cli/daemon_test.go
  modified:
    - internal/cli/root.go

key-decisions:
  - "Injectable rotateFunc on Scheduler for testability without real engine"
  - "TryLock-based concurrency guard: skip silently rather than queue or error"
  - "Docker label read failure is non-fatal in daemon: logs warning and continues"

patterns-established:
  - "Injectable function pattern: Scheduler accepts func signature, not concrete Engine"
  - "Signal-context shutdown: signal.NotifyContext for clean daemon lifecycle"

requirements-completed: [ROT-02]

duration: 4min
completed: 2026-03-28
---

# Phase 4 Plan 3: Scheduler and Daemon Summary

**Cron-based scheduler with per-secret concurrency lock, notification dispatch, and daemon CLI command for automated rotation**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-28T17:12:53Z
- **Completed:** 2026-03-28T17:17:01Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Scheduler registers cron jobs from config schedules and Docker labels with 5-field + descriptor support
- Per-secret TryLock prevents concurrent rotation; skipped rotations log rather than error
- Webhook notifications fire on every rotation outcome (success or failure)
- Daemon command runs in foreground with graceful SIGINT/SIGTERM shutdown

## Task Commits

Each task was committed atomically:

1. **Task 1: Scheduler package (TDD RED)** - `bf6d232` (test)
2. **Task 1: Scheduler package (TDD GREEN)** - `6aa70c4` (feat)
3. **Task 2: Daemon CLI command** - `f5d9397` (feat)

## Files Created/Modified
- `internal/scheduler/scheduler.go` - Cron scheduler with AddJob, LoadFromConfig, LoadFromLabels, Start, Stop
- `internal/scheduler/scheduler_test.go` - 8 tests covering cron registration, concurrency, notifications, stop
- `internal/cli/daemon.go` - NewDaemonCmd wiring scheduler to provider registry, Docker, history, notifiers
- `internal/cli/daemon_test.go` - 4 tests for command registration, flags, error handling
- `internal/cli/root.go` - Added daemon subcommand registration

## Decisions Made
- Injectable rotateFunc on Scheduler for testability: accepts `func(ctx, SecretConfig) error` rather than concrete Engine, enabling channel-based mocks in tests
- TryLock-based concurrency guard: if a rotation is already in progress for a secret, silently skip rather than queue or return error
- Docker label read failure treated as non-fatal in daemon: logs warning and continues with config-only schedules

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All Phase 4 plans complete: notifications, Docker labels, status command, and scheduler/daemon
- Ready for Phase 5 polish and release

## Self-Check: PASSED

All 5 files verified present. All 3 commit hashes verified in git log.

---
*Phase: 04-scheduling-and-operations*
*Completed: 2026-03-28*
