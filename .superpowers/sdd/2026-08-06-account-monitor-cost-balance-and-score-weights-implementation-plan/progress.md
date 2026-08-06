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

`task_5_native_multiplier_convergence`

Tasks 1-4 are implemented and independently reviewed. The earlier whole-branch verification and push are historical evidence only: after production synchronization, official v0.1.171 review found the candidate still had a parallel multiplier policy and measurement value. Task 5 now corrects and migrates that state; Task 6 will repeat the whole-branch verification, push and production gate. Production is unchanged.

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
- Task 5: implementation complete at `ded650e06` — replaced custom multiplier policy and measurement value with official `accounts.rate_multiplier + upstream_billing_rate_sync_enabled`; migration 198 converts and deletes legacy JSON data without runtime compatibility. Fresh backend tests/vet and frontend focused tests/lint/typecheck/build passed; `git diff --check` and legacy-symbol search passed.
- Task 6: in progress — compatibility fix `4282c487d` accepts both `apikey` and `api_key`, keeps API Key cost bound to the native multiplier, and adds reload/card regression coverage. Whole-branch review found and the coordinator fixed probe freshness metadata in `account_multiplier.go` plus dead `measured` frontend semantics; focused backend/frontend checks pass. Code is pushed; production mutation has not occurred.

## Production Gate

Zero-downtime deployment is authorized. Any `downtime_required=true`, migration stop, or service-stop requirement must halt before production mutation.

- Gate result (2026-08-06): stopped before image build or production mutation. Production source `9aab62c203ce9546d77ecf558558bb1a360a634e` contains migrations `192_group_profit_control.sql` and `193_group_profit_control_auth_cache_invalidation.sql`; the candidate branch lacks those files and adds `197_account_estimated_usable_quota.sql`. This is not zero-downtime eligible under the existing controller contract.
- Root cause (2026-08-06): the candidate was based on `origin/main@69caeaf816e3e01f9e0c6059c3c5262a4a12c2f6`, while production runs the separate qualified release chain ending at `9aab62c203ce9546d77ecf558558bb1a360a634e`. The production chain has eight commits not present in the candidate. Current action: merge the complete qualified production chain into the candidate, resolve only genuine overlaps, then rerun migration and whole-branch reviews.
- Production sync result (2026-08-06): complete and independently reviewed. Merge commit `f64f93e4c689125d667af16ff96e0bff79e8d729` contains the full qualified production chain. Integration API compatibility fix `3016c6951462832aa1547b30a7fda652acca17c0` was independently re-reviewed clean.
- Native multiplier correction (2026-08-06): independent audit found the candidate still writes custom policy and persists a second measurement value, so Monitor score can diverge from real billing. User approved a one-time migration to official fields with no application compatibility branch; Task 5 now owns removal and migration before production gate recheck.
- Production read-only inventory (2026-08-06): 74 non-deleted accounts; 38 legacy policy rows (all `upstream_managed`, zero `manual_override`), 16 measurement rows, and 29 OpenAI API-key rows with managed policy plus probe enabled. No production mutation was performed.
- Merged migration set: production's full set plus `197_account_estimated_usable_quota.sql`; canonical hash `9f341792b3dcf631b84b6c8701150f471cbe803c842ce4a99f471374a05b2627`. Production gate must now be rechecked against this exact candidate.
- Native convergence migration set: production's full set plus migrations 197 and 198; canonical candidate hash `0204f39423f3218ffa0c8d4e3d665f7113c4990610e0dd22e9f5910c4d578c6d`, while the active production hash remains `ac8b0b33d7ea31a1a4f0117716ba56efec4bd66be9c38267a88d4c512d01bf39`. Under `ops/deploy-sub2api-blue-green-host.sh`, this mismatch reaches the fail-closed `migration_set_changed` downtime gate unless a separately reviewed maintenance hash pair is explicitly authorized. No production mutation has been performed.
- Whole-branch review (2026-08-06): no Critical findings. Important freshness finding was fixed by reading `UpstreamBillingProbeSnapshot` status/timestamps while retaining native `accounts.rate_multiplier` as the only numeric source; focused Go tests/vet and 53 frontend tests/lint/typecheck pass after the fix. Minor dead `measured` type/i18n entries were removed.
