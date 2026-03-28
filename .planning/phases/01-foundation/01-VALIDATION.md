---
phase: 1
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-28
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — Wave 0 creates go.mod and test files |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test -race -cover ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test -race -cover ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 1 | DIST-03 | unit | `go test ./internal/config/...` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 1 | DISC-03, DISC-04 | unit | `go test ./internal/envfile/...` | ❌ W0 | ⬜ pending |
| 1-01-03 | 01 | 1 | ROT-03 | unit | `go test ./internal/envfile/...` | ❌ W0 | ⬜ pending |
| 1-02-01 | 02 | 1 | INFR-01 | unit | `go test ./internal/docker/...` | ❌ W0 | ⬜ pending |
| 1-02-02 | 02 | 1 | CLI-01 | unit | `go test ./cmd/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` — Go module initialization
- [ ] `internal/config/config_test.go` — config loading tests
- [ ] `internal/envfile/envfile_test.go` — .env read/write/atomic tests
- [ ] `internal/docker/docker_test.go` — Docker wrapper tests (mocked)
- [ ] `cmd/root_test.go` — CLI command registration tests

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Docker container restart in order | INFR-01 | Requires running Docker daemon | Start test containers, verify restart ordering |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
