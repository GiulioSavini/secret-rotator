---
phase: 02-discovery-and-crypto
plan: 01
subsystem: discovery
tags: [env-scanning, password-strength, entropy, pattern-matching, tabwriter]

requires:
  - phase: 01-foundation
    provides: "EnvFile reader/types, CLI root command with cobra, config loader"
provides:
  - "Secret discovery scanner classifying env vars by naming patterns"
  - "Password strength auditor with entropy/length/default-detection scoring"
  - "Working `rotator scan` CLI command with table output"
affects: [02-discovery-and-crypto, 03-rotation-engine]

tech-stack:
  added: [text/tabwriter, math/log2-entropy]
  patterns: [suffix-pattern-matching, entropy-based-scoring, tdd-red-green]

key-files:
  created:
    - internal/discovery/patterns.go
    - internal/discovery/scanner.go
    - internal/discovery/strength.go
    - internal/discovery/scanner_test.go
    - internal/discovery/strength_test.go
    - internal/cli/scan_test.go
  modified:
    - internal/cli/scan.go
    - internal/envfile/types.go

key-decisions:
  - "Suffix pattern order matters: longer suffixes (_API_KEY) checked before shorter (_KEY) to avoid misclassification"
  - "Strength score is min(length_score, entropy_score) -- both must be high for strong rating"
  - "File-referenced secrets (_FILE suffix) excluded from strength auditing since value is a path"

patterns-established:
  - "Discovery pattern: classify by suffix/exact match, then audit value strength"
  - "EnvFile.Reindex() for test construction of EnvFile values without file I/O"

requirements-completed: [DISC-01, DISC-02, CLI-01]

duration: 7min
completed: 2026-03-28
---

# Phase 02 Plan 01: Discovery Engine Summary

**Secret scanner with suffix/exact pattern classification, entropy-based strength auditor, and `rotator scan` CLI command with tabwriter table output**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-28T15:55:30Z
- **Completed:** 2026-03-28T16:02:30Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Secret classification engine identifying passwords, tokens, API keys, secrets, and connection strings by naming patterns
- Password strength auditor scoring weak/fair/good/strong based on entropy, length, character classes, and 30 common defaults
- File-referenced secret detection (_FILE suffix) without strength scoring the file path
- Working `rotator scan` command outputting formatted table with SECRET/TYPE/STRENGTH/SOURCE/ISSUES columns

## Task Commits

Each task was committed atomically:

1. **Task 1: Discovery engine -- patterns, scanner, and strength auditor** - `51de3a8` (test: RED), `36a0bae` (feat: GREEN)
2. **Task 2: Wire scan command into CLI** - `2c9a06f` (feat)

_Note: Task 1 followed TDD with separate RED and GREEN commits_

## Files Created/Modified
- `internal/discovery/patterns.go` - SecretPattern definitions, suffix/exact patterns, 30 common defaults
- `internal/discovery/scanner.go` - Scanner with classifyKey, ScanFile, ScanFiles
- `internal/discovery/strength.go` - AuditStrength with entropy calculation and multi-factor scoring
- `internal/discovery/scanner_test.go` - Tests for classification, _FILE handling, full scan
- `internal/discovery/strength_test.go` - Tests for all strength levels, defaults, entropy
- `internal/cli/scan.go` - Working scan command with --dir flag, table output, summary line
- `internal/cli/scan_test.go` - Tests for empty dir and dir with secrets
- `internal/envfile/types.go` - Added Reindex() method for test construction

## Decisions Made
- Suffix pattern order: longer suffixes (_API_KEY) checked before shorter (_KEY) to prevent misclassification
- Strength score = min(length_score, entropy_score) ensuring both dimensions must be strong
- File-referenced secrets excluded from strength audit (value is a path, not a password)
- Added EnvFile.Reindex() to enable test construction without file I/O

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added EnvFile.Reindex() method**
- **Found during:** Task 1 (test construction)
- **Issue:** EnvFile.index is unexported, no way to construct test EnvFiles with working Get()/Keys()
- **Fix:** Added Reindex() method to envfile.types.go that rebuilds the index from Lines
- **Files modified:** internal/envfile/types.go
- **Verification:** All scanner tests pass using constructed EnvFiles
- **Committed in:** 36a0bae (Task 1 commit)

**2. [Rule 1 - Bug] Fixed test string length for strong password**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Test password string was 31 chars (not 32+), causing StrengthGood instead of StrengthStrong
- **Fix:** Added one character to test string to make it 32 chars
- **Files modified:** internal/discovery/scanner_test.go, internal/discovery/strength_test.go
- **Verification:** Strong strength tests pass
- **Committed in:** 36a0bae (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both auto-fixes necessary for test correctness. No scope creep.

## Issues Encountered
- Pre-existing build error in internal/cli/history_test.go (undefined: fmt) -- out of scope, not caused by this plan's changes

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Discovery engine ready for use by rotation engine (Phase 3)
- Scanner and strength types available for crypto plan (02-02) integration
- `rotator scan` provides first user-facing feature

---
*Phase: 02-discovery-and-crypto*
*Completed: 2026-03-28*
