---
phase: 2
slug: discovery-and-crypto
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-28
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | go.mod (from Phase 1) |
| **Quick run command** | `go test ./internal/discovery/... ./internal/crypto/... ./internal/history/...` |
| **Full suite command** | `go test -race -cover ./...` |
| **Estimated runtime** | ~5 seconds |

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
| 2-01-01 | 01 | 1 | DISC-01 | unit | `go test ./internal/discovery/...` | ❌ W0 | ⬜ pending |
| 2-01-02 | 01 | 1 | DISC-02 | unit | `go test ./internal/discovery/...` | ❌ W0 | ⬜ pending |
| 2-01-03 | 01 | 1 | CLI-01 | unit | `go test ./internal/cli/...` | ❌ W0 | ⬜ pending |
| 2-02-01 | 02 | 1 | INFR-02 | unit | `go test ./internal/crypto/...` | ❌ W0 | ⬜ pending |
| 2-02-02 | 02 | 1 | CLI-04 | unit | `go test ./internal/history/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/discovery/discovery_test.go` — discovery engine tests
- [ ] `internal/crypto/crypto_test.go` — encryption/decryption tests
- [ ] `internal/history/history_test.go` — history store tests

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
