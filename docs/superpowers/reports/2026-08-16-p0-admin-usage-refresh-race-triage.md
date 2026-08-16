# P0 Admin Usage Refresh Race Triage

Status: READY_FOR_ROOT_REVIEW

## Scope

Investigated the reported admin UsageView login redirect when several first-load
requests receive `401` after an access token expires. Production logs identify a
one-time refresh-token rotation race: one request rotates the token, while a
concurrent request presents the old token and receives `Refresh token not found,
possible reuse attack`.

No production code change is required on baseline `c42b5b8cca4b22b3974cda5500e8bd851fabd7b1`.
The required minimum safe client-side coordination is already present in that
baseline and covered by pre-existing tests.

## Evidence

`upstream/sub2api/frontend/src/api/tokenRefresh.ts` provides:

- a document-local `inFlightRefresh` promise, so parallel requests in one page
  issue one refresh request;
- Web Lock serialization across tabs when supported;
- an authenticated local-storage snapshot check that adopts only a newly
  published, unexpired token pair for the same `auth_user` identity;
- lock-free race recovery that waits through the peer's refresh request timeout
  after a 4xx refresh rejection, then retries the original protected request
  using the peer's rotated token.

`upstream/sub2api/frontend/src/api/client.ts` only clears credentials after this
coordination fails. It does not suppress arbitrary `401` responses, permit old
refresh-token reuse, or weaken the backend reuse-detection policy.

The existing regression `waits for a slow peer after losing a refresh-token race
without Web Locks` models the production race: two isolated module instances use
the old refresh token, one receives `401`, the other publishes the rotated pair
later, and both callers resolve with the new session.

The requested test-first sequence cannot honestly add a new failing test here:
the direct minimal reproduction already exists in the baseline and passes before
any change. Adding a code diff solely to force a red-green cycle would either
duplicate the existing protection or risk changing authentication behavior
outside this P0 scope.

## Verification

From `upstream/sub2api/frontend`:

```text
pnpm vitest run src/api/__tests__/tokenRefresh.spec.ts src/api/__tests__/client.spec.ts
2 files passed, 29 tests passed
```

From the worktree root:

```text
git diff --check
exit 0
```

## Delivery

- Migration changes: none.
- Configuration changes: none.
- `downtime_required`: not applicable; no deployment was performed.
- Rollback: revert this documentation-only commit if the record must be removed.
- Remaining risk: this code-level result does not establish which frontend asset
  version served the logged production incident. Root review should compare the
  deployed asset source identity with this baseline before treating the incident
  as resolved in production.
