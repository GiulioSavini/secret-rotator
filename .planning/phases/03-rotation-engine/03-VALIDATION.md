---
phase: 3
slug: rotation-engine
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-28
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | go.mod (from Phase 1) |
| **Quick run command** | `go test ./internal/rotation/... ./internal/provider/...` |
| **Full suite command** | `go test -race -cover ./...` |
| **Estimated runtime** | ~8 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick command
- **After every plan wave:** Run full suite
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 3-01-01 | 01 | 1 | ROT-01, ROT-04 | unit | `go test ./internal/rotation/...` | ❌ W0 | ⬜ pending |
| 3-01-02 | 01 | 1 | PROV-01 | unit | `go test ./internal/provider/...` | ❌ W0 | ⬜ pending |
| 3-02-01 | 02 | 2 | PROV-02, PROV-03, PROV-04 | unit | `go test ./internal/provider/...` | ❌ W0 | ⬜ pending |
| 3-02-02 | 02 | 2 | CLI-02 | unit | `go test ./internal/cli/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/rotation/engine_test.go` — state machine + rollback tests
- [ ] `internal/provider/generic_test.go` — generic provider tests
- [ ] `internal/provider/mysql_test.go` — MySQL provider tests (mocked)
- [ ] `internal/provider/postgres_test.go` — PostgreSQL provider tests (mocked)
- [ ] `internal/provider/redis_test.go` — Redis provider tests (mocked)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real MySQL ALTER USER | PROV-02 | Requires running MySQL | Use testcontainers in integration tests |
| Real PostgreSQL ALTER ROLE | PROV-03 | Requires running PostgreSQL | Use testcontainers in integration tests |
| Real Redis CONFIG SET | PROV-04 | Requires running Redis | Use testcontainers in integration tests |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
