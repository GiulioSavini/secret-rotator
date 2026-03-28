---
phase: 01-foundation
plan: 03
subsystem: infra
tags: [docker, compose, container-lifecycle, topological-sort, health-check]

# Dependency graph
requires:
  - phase: 01-foundation-01
    provides: "Go module with project scaffold and dependencies"
provides:
  - "Manager interface for all Docker container operations"
  - "SDKClient wrapping Docker SDK with full Manager implementation"
  - "Compose file parser extracting depends_on for restart ordering"
  - "Health check polling with timeout and no-healthcheck support"
  - "RestartInOrder function for dependency-ordered container restarts"
  - "Topological sort with cycle detection"
affects: [02-discovery, 03-rotation, 04-orchestration]

# Tech tracking
tech-stack:
  added: [docker/docker v27, compose-spec/compose-go/v2]
  patterns: [Manager interface for Docker ops, mock-based testing without daemon, Kahn's algorithm topo sort]

key-files:
  created:
    - internal/docker/manager.go
    - internal/docker/client.go
    - internal/docker/compose.go
    - internal/docker/health.go
    - internal/docker/manager_test.go
    - internal/docker/compose_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "Hand-written mock instead of testify/mock codegen for simplicity and zero-dependency mock"
  - "Health check treats running-without-healthcheck as healthy (pragmatic default for containers without HEALTHCHECK)"
  - "Deterministic topo sort output via sorted queue insertion for reproducible restart ordering"

patterns-established:
  - "Manager interface: all Docker operations go through the Manager interface, only client.go imports Docker SDK"
  - "Mock-based testing: all Docker tests use MockManager, no real daemon required"
  - "Free functions over methods: RestartInOrder takes Manager as param for easy testing"

requirements-completed: [INFR-01]

# Metrics
duration: 7min
completed: 2026-03-28
---

# Phase 1 Plan 3: Docker Manager Summary

**Docker Manager interface with SDK client, compose-based dependency ordering via topological sort, and health check polling**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-28T15:25:18Z
- **Completed:** 2026-03-28T15:32:18Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Manager interface with 6 operations: ListContainers, InspectContainer, StopContainer, StartContainer, RestartContainer, WaitHealthy
- SDKClient implementing Manager by wrapping Docker SDK with proper error wrapping
- Compose file parser using compose-spec/compose-go with Kahn's algorithm topological sort and cycle detection
- 16 tests covering all operations, dependency orderings (simple, chain, diamond, no-deps, cycle), and filtering

## Task Commits

Each task was committed atomically:

1. **Task 1: Manager interface and mock-based tests** - `bfc179a` (feat)
2. **Task 2: Compose file parsing for dependency ordering** - `11066c2` (feat)

## Files Created/Modified
- `internal/docker/manager.go` - Manager interface, Container/ContainerFilter types, RestartInOrder function
- `internal/docker/client.go` - SDKClient wrapping Docker SDK, implements all Manager methods
- `internal/docker/health.go` - waitHealthy polling logic with timeout, no-healthcheck handling
- `internal/docker/compose.go` - LoadDependencyOrder, FilterDependencyOrder, topoSort (Kahn's algorithm)
- `internal/docker/manager_test.go` - MockManager and 10 tests for all Manager operations
- `internal/docker/compose_test.go` - 6 tests for compose parsing and dependency ordering
- `go.mod` - Added docker/docker v27, compose-spec/compose-go/v2 and transitive deps
- `go.sum` - Updated checksums

## Decisions Made
- Used hand-written MockManager struct with function fields instead of testify/mock codegen -- simpler, no extra dependencies, explicit control
- Containers without a HEALTHCHECK directive that are in "running" state are treated as healthy -- pragmatic default since many containers lack health checks
- Topological sort uses sorted queue insertion for deterministic output ordering across runs
- Used compose-go loader.WithSkipValidation for parsing since we only need depends_on relationships

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed Docker SDK ContainerStartOptions API change**
- **Found during:** Task 1 (SDK client implementation)
- **Issue:** Docker SDK v27 changed `types.ContainerStartOptions` to `container.StartOptions`
- **Fix:** Updated import to use `containerapi.StartOptions{}` from the container package
- **Files modified:** internal/docker/client.go
- **Verification:** go vet passes, all tests pass
- **Committed in:** bfc179a (Task 1 commit)

**2. [Rule 3 - Blocking] Installed Docker SDK transitive dependencies**
- **Found during:** Task 1 (SDK client compilation)
- **Issue:** Docker SDK v27 (incompatible module) requires many transitive deps not auto-resolved by go get
- **Fix:** Installed all 9 missing transitive dependencies and ran go mod tidy
- **Files modified:** go.mod, go.sum
- **Verification:** go vet and go test pass
- **Committed in:** bfc179a (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** Both auto-fixes were necessary to compile against Docker SDK v27. No scope creep.

## Issues Encountered
- Race detection (`go test -race`) unavailable because CGO_ENABLED=0 in this environment. Not a blocker -- race tests can run in CI.
- Go version was auto-upgraded from 1.23 to 1.25 by go mod tidy due to opentelemetry dependency requiring go >= 1.25.0.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Docker Manager interface ready for use in discovery and rotation phases
- Compose parser ready for extracting service dependencies from user compose files
- All docker package tests pass without Docker daemon (mock-based)
- RestartInOrder provides the core restart-in-dependency-order flow needed by INFR-01

## Self-Check: PASSED

- All 6 source files verified on disk
- Commit bfc179a verified in git log
- Commit 11066c2 verified in git log

---
*Phase: 01-foundation*
*Completed: 2026-03-28*
