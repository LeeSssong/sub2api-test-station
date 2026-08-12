# Task 2 Report: Official v0.1.175 Semantic Merge

Status: implementation and local verification complete; candidate remains
`进行中` pending independent review, root authorization, merge, push,
deployment, and online verification.

## Candidate

- Branch: `codex/official-v0175-fast-merge`
- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/official-v0175-fast-merge`
- Task 1 baseline: `75c491d5e72f7fc32e125b6060c319ef9b96fb63`
- Official release: `v0.1.175`
- Official commit: `93c32fa1a2450351561abc46156d2e28cb5f74ca`
- Annotated tag object: `b898c60c422d1de059968c56aca22f6643f1fed4`

## Semantic Merge

The nine reported conflicts were resolved function-by-function and
component-by-component. The merge retains Xingqiao attempt metadata, failed
attempt accounting, stream recovery/no-replay safeguards, lease-aware
scheduling, profit gates, custom pricing precedence, fixed image-tier cost
semantics, administrator usage details, and error-dialog contracts.

Official behavior retained includes service-tier-aware pricing, configured
pool-mode retry counts and delays, request-ID display/copy behavior, compact
keepalive terminal failure handling, pre-profit scheduling diagnostics, and
the stream read cancellation/deadline/body-size classification guard.

The Responses and Messages loops allow explicitly configured pool statuses,
including 401/403, to retry on the same account only before semantic output or
tool/side effects. After the configured retry limit is exhausted, normal
account failover continues. The regression confirms the call sequence
`[primary, primary, fallback]`.

## Official Delta and Identity

- Official changed paths from `v0.1.173` to `v0.1.175`: 114.
- Candidate covers all 114 official paths; the only additional source path is
  the required Xingqiao provenance record.
- `upstream/sub2api/XINGQIAO_UPSTREAM.md` now records `v0.1.175` and the locked
  source/tag identities.
- `upstream/sub2api/backend/cmd/server/VERSION` is `0.1.175`.
- Official migration delta under `backend/migrations`: none.
- Deployment/host configuration changes made by this task: none.

## Verification

- `GOFLAGS=-mod=mod go test ./internal/handler -count=1`: PASS.
- `GOFLAGS=-mod=mod go test ./internal/service -count=1`: PASS.
- `GOFLAGS=-mod=mod go vet ./internal/handler ./internal/service`: PASS.
- Focused pool auth regression for Responses 401/403: PASS.
- Focused Xingqiao transient retry and post-output no-replay handler tests:
  PASS.
- `pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts`:
  PASS, 2 files and 39 tests.
- `pnpm typecheck`: PASS.
- `git diff --check`: PASS.
- Unmerged paths: none.
- Literal conflict markers under `upstream/sub2api`: none.

## Release Boundary and Risks

- No T03-R1 files or behavior were changed.
- No merge to `main`, push, deployment, production access, secret access, or
  GitHub Actions work was performed.
- `downtime_required`: deferred to the root release preflight after the
  reviewed candidate is merged into the then-current `main`.
- Rollback before merge is the Task 1 baseline commit. After root-authorized
  release, rollback must use the existing reviewed blue-green host chain and
  its release evidence.
- Remaining risk is integration outside the focused handler/service and usage
  UI scopes; the root task must run its approved whole-branch review and
  merged-main release gates before promotion.
