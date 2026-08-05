# SDD ledger — plan: docs/superpowers/plans/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan.md

## Baseline

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-monitor-cost-balance-implementation`
- Branch: `codex/account-monitor-cost-balance-implementation`
- Remote baseline: `origin/main@69caeaf816e3e01f9e0c6059c3c5262a4a12c2f6`
- Production source: `05985e62ec88b04d1e647a815eecdb1cf1155776`
- Shared `upstream/sub2api` tree: `fc455d6aecfdb07ab90587000d7c5e77902f5bb6`
- Design commit in this branch: `264427f838c2e6010434de0735f2dfd526f5e063`
- Plan commit in this branch: `ea30ebde3543a12170e9d43600216c76ecc2fee6`

## Status

`task_1_in_progress`

User approved subagent-driven execution on a fresh worktree based on main. Dependencies are installed and the baseline is clean. Task 1 is ready for dispatch; business code is unchanged and production is unchanged.

## Baseline Verification

- Backend: `go test ./internal/service ./internal/handler/admin ./internal/repository` — PASS.
- Frontend: 4 focused files, 37 tests — PASS.

## Tasks

- Task 1: in_progress — estimated quota field and unified cost scoring
- Task 2: pending — balance snapshot and explicit refresh policy
- Task 3: pending — restore group score-weight entry
- Task 4: pending — lightweight cost dialog and balance card
- Task 5: pending — focused verification, whole-branch review, push, and production gate

## Production Gate

Zero-downtime deployment is authorized. Any `downtime_required=true`, migration stop, or service-stop requirement must halt before production mutation.
