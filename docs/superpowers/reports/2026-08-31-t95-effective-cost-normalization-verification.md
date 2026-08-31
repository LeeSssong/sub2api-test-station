# T95 Effective Cost Normalization Verification

## Candidate

- Refreshed baseline: `main@879787096a7bc4b3ff4ab4820d4a5f3c3a63a29a`
- Refresh merge: `747eaa9d9`
- Implementation commits: `01d1bd30e`, `f5c5a5fee`
- Branch: `codex/t95-effective-cost-normalization`
- Worktree: `.worktrees/t95-effective-cost-normalization`

## Delivered Contract

- API Key defaults to `direct_multiplier`: `U=R`.
- API Key can opt into `ratio_based_upstream`: `U=(actual_cost/obtained_quota)*R`.
- Native OAuth-family accounts (`oauth` and `setup-token`) are locked to `self_owned`: `U=procurement_cost/estimated_usable_quota`, with no exchange conversion and no upstream R.
- The three managed configuration values persist in `accounts.extra`; unrelated keys survive updates and duplicate-account creation.
- Invalid administrator inputs are native HTTP 400 application errors.
- The existing profit gate reads live U. Unknown U remains the native invalid-cost veto; T96 owns the approved full-pool availability fallback.
- The existing Account Monitor cost dialog is the administrator input surface and previews U before saving.

## Verification

- Go 1.27 focused effective-cost and profit-control tests: PASS.
- Go 1.27 `unit` duplicate-account configuration test: PASS.
- Admin handler procurement/effective-cost focused tests: PASS.
- `go build ./cmd/server`: PASS.
- Account Monitor cost dialog Vitest: 1 file, 9 tests PASS.
- `vue-tsc --noEmit`: PASS.
- `vue-tsc -b`: PASS.
- Vite production build: PASS, 1094 modules transformed.
- `git diff --check`: PASS.

The service package's unmodified main baseline cannot compile all tests because these unrelated tests are stale after prior scheduler/cache changes:

- `openai_admission_first_output_wiring_test.go`: unused `ctx` and undefined `c` after T100 removal.
- `openai_sticky_reference_test.go`: cache stub lacks `GetReasoningContent`.
- With `unit` tags, `gateway_forward_as_chat_completions_test.go`: missing `context` import.

Focused service verification used a temporary Go overlay to exclude only those three files. The overlay was deleted and is not part of the candidate.

## Change Safety

- SQL migrations: none.
- Schema columns or generated Ent changes: none.
- Configuration changes: none.
- Production account/group/data writes: none.
- Deployment or acceptance-station action: none.
- Expected deployment downtime: false, subject to the release-chain preflight when this task reaches the release lane.
- Rollback: revert the T95 commits. Existing `accounts.extra` keys are additive and ignored by the previous code, so code rollback does not require data rollback.
