---
phase: 02-discovery-and-crypto
plan: 02
subsystem: crypto
tags: [argon2id, aes-256-gcm, encryption, history, audit-log]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: CLI scaffold with cobra commands, config types with MasterKeyEnv
provides:
  - "Argon2id key derivation + AES-256-GCM encrypt/decrypt primitives"
  - "Encrypted history store with single key derivation per command"
  - "rotator history CLI command with table output"
affects: [03-rotation-engine, 04-providers, 05-notifications]

# Tech tracking
tech-stack:
  added: [golang.org/x/crypto v0.49.0]
  patterns: [argon2id-key-derivation, aes-256-gcm-authenticated-encryption, encrypted-append-only-log, atomic-file-write]

key-files:
  created:
    - internal/crypto/crypto.go
    - internal/crypto/crypto_test.go
    - internal/history/types.go
    - internal/history/store.go
    - internal/history/store_test.go
    - internal/cli/history_test.go
  modified:
    - internal/cli/history.go
    - go.mod
    - go.sum

key-decisions:
  - "Fixed salt per history file (stored in header) enables single key derivation per Store instance"
  - "EncryptWithKey/DecryptWithKey separation allows key caching without per-entry Argon2id overhead"
  - "Corrupted entries skipped silently during List to provide partial results over total failure"

patterns-established:
  - "Crypto pattern: salt||nonce||ciphertext binary format for encrypted blobs"
  - "History store pattern: append-only JSON file with base64-encoded encrypted entries"
  - "Passphrase resolution: flag > ROTATOR_MASTER_KEY env > config-specified env var"

requirements-completed: [INFR-02, CLI-04]

# Metrics
duration: 7min
completed: 2026-03-28
---

# Phase 2 Plan 2: Crypto & History Summary

**Argon2id + AES-256-GCM crypto primitives with encrypted append-only history store and `rotator history` table output**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-28T15:55:21Z
- **Completed:** 2026-03-28T16:02:39Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- AES-256-GCM encryption with Argon2id key derivation, random salt+nonce per encryption for unique ciphertext
- Encrypted history store with key caching (one Argon2id derivation per command, not per entry)
- `rotator history` command with tabwriter output, passphrase resolution from flag/env/config, limit flag

## Task Commits

Each task was committed atomically:

1. **Task 1: Crypto primitives and encrypted history store** - `0047368` (test) + `cbed41d` (feat) -- TDD RED then GREEN
2. **Task 2: Wire history command into CLI** - `8334cba` (feat)

_Note: Task 1 followed TDD with separate RED/GREEN commits_

## Files Created/Modified
- `internal/crypto/crypto.go` - Argon2id key derivation, AES-256-GCM encrypt/decrypt, with-key variants
- `internal/crypto/crypto_test.go` - Round-trip, uniqueness, wrong passphrase, too-short, key derivation tests
- `internal/history/types.go` - HistoryEntry, HistoryFile, EncryptedEntry types
- `internal/history/store.go` - Encrypted append-only store with atomic writes and key caching
- `internal/history/store_test.go` - Append/list, empty file, corrupted entry, key cache tests
- `internal/cli/history.go` - History command with passphrase resolution and tabwriter output
- `internal/cli/history_test.go` - No-file, with-entries, no-passphrase, limit tests
- `go.mod` / `go.sum` - Promoted golang.org/x/crypto v0.49.0 to direct dependency

## Decisions Made
- Fixed salt per history file stored in JSON header enables single Argon2id derivation per Store instance, avoiding expensive per-entry key derivation
- EncryptWithKey/DecryptWithKey pair separates key derivation from encryption for caching
- Corrupted entries are silently skipped during List (partial results over total failure)
- Passphrase resolution order: --passphrase flag > ROTATOR_MASTER_KEY env var > AppConfig.MasterKeyEnv env var

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Go 1.25.0 toolchain auto-download from GOMODCACHE had runtime compilation errors with map redeclaration; resolved by using Go 1.24.3 local toolchain (GOTOOLCHAIN=auto resolves correctly for builds)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Crypto primitives ready for rotation engine to encrypt old secret values before storage
- History store ready to receive rotation events from the rotation engine
- `rotator history` command functional for viewing audit log

---
*Phase: 02-discovery-and-crypto*
*Completed: 2026-03-28*

## Self-Check: PASSED

All 7 created files verified on disk. All 3 commits (0047368, cbed41d, 8334cba) verified in git log.
