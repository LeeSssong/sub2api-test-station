# Task 2 Independent Review: Official v0.1.175 Semantic Merge

Review scope:

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/official-v0175-fast-merge`
- Branch: `codex/official-v0175-fast-merge`
- BASE: `75c491d5e72f7fc32e125b6060c319ef9b96fb63`
- HEAD: `29cc455caf69ab2f0b6f5dd25f3b738d926fe241`
- Review mode: independent, read-only except for this report; no merge, push,
  deployment, production access, or source modification.

## Verdict

- **SPEC COMPLIANCE: FAIL**
- **CODE/ARTIFACT QUALITY: FAIL**

The official source set, semantic conflict unions, identity files, UI
contracts, and declared test commands are otherwise substantially sound. The
candidate is blocked by one replay-safety defect in both Responses and
Messages handler loops.

## Critical

None.

## Important

### 1. Unsafe tool/side-effect requests can still fail over on non-transient pool 401/403

Affected code:

- `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go:878-912`
- `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go:1582-1610`

The merge correctly prevents the configured same-account pool retry when
`failure.HasSideEffect` is true. However, after that check fails, both loops
continue to `failoverErr.ShouldRetryNextAccount()` and then switch accounts.
For API-key passthrough 401/403, `newOpenAIUpstreamFailoverError` leaves
`NextAccountAction` at legacy retry, so `ShouldRetryNextAccount()` is true.

The exact control flow is:

1. A request containing `tools` or `function_call_output` is marked
   `requestHasSideEffects=true` by `openAIRequestHasSideEffects`.
2. `ClassifyOpenAIUpstreamFailure` classifies 401/403 as `Hard=true`,
   `Transient=false`, `SafeToReplay=false`, and `HasSideEffect=true`.
3. `decideOpenAIRetry` returns `NoRetry=true` and
   `TerminalRecovery=false` for that failure.
4. The loops consume `TerminalRecovery`, `RetrySameAccount`, and the explicit
   pool retry condition, but never consume `NoRetry` before the next-account
   branch.
5. The request is therefore replayed against another account despite the
   retained contract that tool/side-effect requests must never be replayed.

This is not covered by the passing handler package. The unit test
`TestOpenAIDecideRetry_CachePreservationModes` proves that the decision helper
returns no replay for an unsafe tool request, but there is no handler-level
assertion that the loop honors that decision. The new pool-auth regression
only exercises a side-effect-free Responses request. There is also no
Messages pool-auth 401/403 handler regression, even though the Task 2 report
claims the behavior for both loops.

Required correction before root review:

- In both Responses and Messages, stop before all same-account or
  next-account replay whenever semantic output or tool/side-effect state makes
  replay unsafe, including non-transient/hard failures.
- Preserve the intended safe-request behavior: configured pool 401/403 gets
  the configured same-account retry, then may switch to a healthy account.
- Add handler-level regressions for both endpoints covering:
  - safe pool 401/403 sequence `[primary, primary, fallback]`;
  - tool/side-effect request sequence `[primary]` with no same-account retry
    and no account switch;
  - existing post-output no-replay behavior.

## Minor

None.

## Evidence and Checks

### Official path completeness and merge construction

- Official `v0.1.173` (`29009f0b...`) to `v0.1.175`
  (`93c32fa1...`) changed path count: **114**.
- Candidate source path set covers all 114 official paths: **0 missing**.
- Candidate has one additional source path:
  `upstream/sub2api/XINGQIAO_UPSTREAM.md`, as required.
- Of the 105 non-conflict official paths, 104 are byte-identical to the
  official source commit. The only intentional content difference is
  `backend/cmd/server/VERSION`: the official repository still contains
  `0.1.173`, while the approved Task 2 identity is `0.1.175`.
- Each of the nine conflict files differs from both stage 2 (Xingqiao) and
  stage 3 (official); none is a whole-file ours/theirs resolution.
- Function-name inventories from both stages are retained in the six Go
  conflict files. Test-title inventories from both stages are retained in the
  two conflicted spec files.

### Semantic conflict review

- Pricing retains custom-rule precedence, `AccountStatsCost`/final
  `AccountCost` semantics, service-tier-aware LiteLLM fallback for priority,
  flex, and long context, and fixed image tier cost without account-rate
  multiplication.
- Scheduling retains official pre-profit exclusion diagnostics together with
  Xingqiao profit vetoes and lease-aware half-open rechecks.
- Stream-read error code retains official cancellation, deadline, canceled
  context, and oversized-body exclusion together with Xingqiao recovery
  metadata APIs.
- Usage UI retains admin success/error detail actions and dialogs while adding
  request-ID hidden-by-default visibility, rendering, and copy behavior.
- Provenance records `v0.1.175`, source commit
  `93c32fa1a2450351561abc46156d2e28cb5f74ca`, and annotated tag object
  `b898c60c422d1de059968c56aca22f6643f1fed4`.
- No official migration delta exists under `backend/migrations`.
- BASE-to-HEAD contains no `.github/workflows` changes and no identified
  T03-R1 implementation path or async upstream-cost persistence behavior.
- The project ledger remains `进行中` and does not claim deployment or online
  completion.

### Independently rerun verification

- `GOFLAGS=-mod=mod go test ./internal/handler -count=1`: **PASS**
  (`38.976s`).
- `GOFLAGS=-mod=mod go test ./internal/service -count=1`: **PASS**
  (`108.079s`).
- `GOFLAGS=-mod=mod go vet ./internal/handler ./internal/service`: **PASS**.
- `GOFLAGS=-mod=mod go test ./internal/handler -run '^TestOpenAIResponses_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyAccount$' -count=1 -v`:
  **PASS**, both 401 and 403, with the expected safe-request sequence.
- `pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts`:
  **PASS**, 2 files / 39 tests.
- `pnpm typecheck`: **PASS**.
- `git diff --check BASE HEAD`: **PASS**.
- Unmerged paths: none.
- Literal conflict markers under `upstream/sub2api`: none.

The Task 2 report's listed command results are reproducible, but those green
commands do not invalidate Important finding 1 because the unsafe handler loop
path has no end-to-end regression.
