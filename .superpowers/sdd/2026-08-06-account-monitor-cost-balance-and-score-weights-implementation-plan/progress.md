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

`task_5_in_progress`

Tasks 1-4 are implemented and independently reviewed. Task 5 focused verification and whole-branch review are clean; visual verification, push, production gate, deployment, and online verification remain. Production is unchanged.

## Baseline Verification

- Backend: `go test ./internal/service ./internal/handler/admin ./internal/repository` — PASS.
- Frontend: 4 focused files, 37 tests — PASS.

## Tasks

- Task 1: completed — estimated quota field and unified cost scoring; final reviewed commit `d2cc2d0dd`
- Task 2: completed — balance snapshot and explicit refresh policy; fix round 1 reviewed and approved at `32b9c2602`
- Task 3: complete — restored group score-weight entry; fix round 1 independently re-reviewed and approved at `5f45f1b0d`
- Task 4: complete — initial implementation `3ebb24a2e`, fix `95a5277c6`; scoped re-review clean
- Task 4: minor (deferred): non-API-Key desktop card retains an empty sixth metric track
- Task 4: minor (deferred): multiplier dialog does not surface freshness/status metadata
- Task 5: in progress — focused verification passed (backend tests/vet; frontend 53 tests, lint, typecheck, build); whole-branch review finding fixed in `13a318be7` and scoped re-review clean; visual verification, push, and production gate remain

## Production Gate

Zero-downtime deployment is authorized. Any `downtime_required=true`, migration stop, or service-stop requirement must halt before production mutation.
