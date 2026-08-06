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

Tasks 1-4 are implemented and independently reviewed. Task 5 focused verification and whole-branch review are clean, and the feature branch is pushed. The production gate stopped before build/mutation because the candidate migration set differs from production and requires downtime authorization. Production is unchanged.

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
- Task 5: in progress — focused verification passed (backend tests/vet; frontend 53 tests, lint, typecheck, build); whole-branch review finding fixed in `13a318be7` and scoped re-review clean; branch pushed; production gate blocked before mutation because candidate migrations hash `db075a23ed5363b0b9a1f1055a9377b381ab08e7acd8a07eac03962a5e318c65` differs from production `ac8b0b33d7ea31a1a4f0117716ba56efec4bd66be9c38267a88d4c512d01bf39`

## Production Gate

Zero-downtime deployment is authorized. Any `downtime_required=true`, migration stop, or service-stop requirement must halt before production mutation.

- Gate result (2026-08-06): stopped before image build or production mutation. Production source `9aab62c203ce9546d77ecf558558bb1a360a634e` contains migrations `192_group_profit_control.sql` and `193_group_profit_control_auth_cache_invalidation.sql`; the candidate branch lacks those files and adds `197_account_estimated_usable_quota.sql`. This is not zero-downtime eligible under the existing controller contract.
