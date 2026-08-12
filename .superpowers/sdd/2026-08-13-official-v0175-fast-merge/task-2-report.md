# Task 2 Report: Official v0.1.175 Semantic Merge

Status: the initial independent review was `REJECT` with three Important
blockers. Fix round 1 implementation and required local validation are now
complete; the new candidate HEAD remains `进行中` and must receive scoped
independent re-review before root authorization, merge, push, deployment, or
online verification.

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
tool/side effects. A replay-safe request may switch to a healthy account after
the configured same-account retry limit is exhausted, with call sequence
`[primary, primary, fallback]`. A request containing tools or function output
consumes the retry decision's `NoRetry` result before the next-account branch,
so its call sequence remains `[primary]`. Existing post-output no-replay
behavior remains intact.

## Independent Review Fix

The initial independent review rejected the candidate on three Important
items:

1. Responses requests carrying tools/function side-effect risk could skip the
   configured same-account retry but still fall through to next-account replay
   after a pool 401/403.
2. Messages lacked the symmetric safe-switch and unsafe no-replay regression,
   leaving the same cross-account replay gap unproven.
3. The healthy fallback fixture returned a completed Responses object without
   semantic output. Official `v0.1.175` treats an empty `response.completed`
   stream as a failover condition, so that fixture could not establish a real
   successful fallback.

The fix introduces the same `poolReplaySafe` gate in Responses and Messages.
Only an account-configured pool retry signal with no client output and no
tools/function side-effect risk may exhaust same-account retries and then
switch accounts. Unsafe `NoRetry` decisions terminate before the next-account
branch. The healthy fallback fixtures now use protocol-appropriate Responses
JSON or Responses SSE and include an assistant `output_text` value of `ok`;
both endpoint tests assert that semantic text reaches the client.

### TDD RED

The pre-existing production draft was saved to mode-0600 patches under
`/private/tmp`, then `openai_gateway_handler.go` was restored to the candidate
HEAD while retaining the new tests.

- Command: `GOFLAGS=-mod=mod go test ./internal/handler -run 'TestOpenAI(Responses|Messages)_APIKeyPassthroughPoolAuthFailureWithToolsNeverReplays$' -count=1`
- Expected and observed failure: all Responses/Messages 401/403 tools cases
  called `[9910, 9911]` instead of the required `[9910]`; the failure was a
  compiled behavioral assertion, not a build error.
- Command: `GOFLAGS=-mod=mod go test -tags unit ./internal/service -run '^TestOpenAIResponsesEmptyCompleted(FailsOver|WithOutputSucceeds)$' -count=1 -v`
- Semantic proof: `EmptyCompletedFailsOver` returned the official 502 failover
  behavior, while `EmptyCompletedWithOutputSucceeds` passed only with real
  semantic output. Therefore an empty completed fixture cannot represent a
  healthy fallback.

### TDD GREEN

- Command: `GOFLAGS=-mod=mod go test ./internal/handler -run 'TestOpenAI(Responses|Messages)_APIKeyPassthroughPoolAuthFailure(RetriesThenSwitchesToHealthyAccount|WithToolsNeverReplays)$' -count=1 -v`
- Result: PASS for Responses and Messages, 401 and 403. Safe requests called
  `[9910, 9910, 9911]`; tools requests called `[9910]`; both healthy endpoint
  responses contained text `ok`.
- Command: `GOFLAGS=-mod=mod go test ./internal/handler -run 'TestOpenAIResponses_PostOutputFailureNeverReplays$' -count=1 -v`
- Result: PASS; existing post-output no-replay behavior remains intact.
- Command: `GOFLAGS=-mod=mod go test ./internal/handler -count=1`
- Result: PASS (`38.775s`).
- Commands: `gofmt -w internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go`, `gofmt -d ...`, and `git diff --check`.
- Result: PASS; `gofmt -d` and `git diff --check` produced no output.

The resulting new HEAD is pending independent re-review; this report does not
mark Task 2 complete.

## Official Delta and Identity

- Official changed paths from `v0.1.173` to `v0.1.175`: 114.
- Candidate covers all 114 official paths; the only additional source path is
  the required Xingqiao provenance record.
- `upstream/sub2api/XINGQIAO_UPSTREAM.md` now records `v0.1.175` and the locked
  source/tag identities.
- `upstream/sub2api/backend/cmd/server/VERSION` is `0.1.175`.
- Official migration delta under `backend/migrations`: none.
- Deployment/host configuration changes made by this task: none.

## Candidate Verification

- Fix-round `GOFLAGS=-mod=mod go test ./internal/handler -count=1`: PASS.
- Fix-round official empty-completed semantic tests: PASS.
- Fix-round pool auth and post-output no-replay tests: PASS.
- Pre-review `GOFLAGS=-mod=mod go test ./internal/service -count=1`: PASS.
- Pre-review `GOFLAGS=-mod=mod go vet ./internal/handler ./internal/service`: PASS.
- `pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts`:
  pre-review PASS, 2 files and 39 tests.
- Pre-review `pnpm typecheck`: PASS.
- Fix-round `git diff --check`: PASS.
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
