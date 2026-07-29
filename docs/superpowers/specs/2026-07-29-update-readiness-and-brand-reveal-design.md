# Update Readiness and Brand Reveal Recovery Design

## Goal

Restore the real Sub2API `0.1.168` upgrade path without presenting an
unqualified release as immediately installable, and restore visible homepage
brand particles in both light and dark themes. The production rollout updates
only the host updater surfaces and Caddy; PostgreSQL and Redis must not be
recreated or restarted.

## Current Failure

The official release endpoint reports `0.1.168`, but the production Docker
daemon does not yet contain the qualified image
`xingqiao-sub2api:upstream-0.1.168`. The updater resolver therefore rejects the
request. The updater HTTP layer converts this expected readiness state into a
generic HTTP 500 response, while the injected update UI translates that empty
service-error envelope into `更新服务不可用`.

The homepage brand reveal is deployed and its Canvas is active. Its cells are
always rendered as `#dce4fa`, however, while the light-theme footer background
is also a pale blue. The Canvas hides the static fallback after activation, so
the running particle field becomes almost invisible in light mode.

## Required Behavior

1. A production candidate is ready only when the target is still the latest
   stable official release and a local qualified image exists with matching
   version, official commit, source commit, architecture, and qualification
   labels.
2. A missing qualified image is an expected non-terminal state, not an updater
   outage.
3. An authenticated administrator can query readiness without creating an
   update operation or modifying updater state.
4. The update mutation rechecks readiness atomically before scheduling. A
   candidate that disappears or changes after the UI check is still rejected.
5. The HTTP API returns `409 UPDATE_CANDIDATE_NOT_READY` for a missing candidate
   and preserves `409 UPDATE_TARGET_CHANGED` when the requested release is no
   longer latest.
6. The update UI shows `候选版本正在准备，暂不可升级`, disables the submit
   button while the candidate is unavailable, and rechecks readiness every
   30 seconds while the dialog remains open.
7. Authentication, same-origin checks, confirmation, scheduling limits, update
   execution, rollback behavior, and update-state persistence remain unchanged.
8. The brand Canvas uses a light particle color in dark mode and a dark blue
   particle color in light mode. Switching theme updates the Canvas without
   requiring a page reload.
9. Reduced-motion users retain the static `星桥` fallback with the existing
   theme-aware CSS color.
10. The `0.1.168` candidate must pass the repository's complete release
    qualification gates and be loaded on the production Docker daemon with the
    immutable qualification labels before readiness can report `ready=true`.
11. Candidate loading does not switch the running Sub2API container.
12. Deployment records the PostgreSQL and Redis container IDs, start times, and
    restart counts before change. Only updater/Caddy surfaces may be restarted;
    all recorded PostgreSQL and Redis values must remain identical afterward.

## Architecture

### Typed candidate readiness

`UpdateResolver.Resolve` remains the single authority that checks the official
release and qualified local image. It returns a typed
`ErrCandidateNotReady` when Docker cannot find the expected qualified tag.
Qualification-label mismatches remain hard service errors because they indicate
corrupt or unsafe candidate state.

`Service.Readiness(ctx, targetVersion)` calls the resolver without persisting an
operation and returns:

```json
{
  "target_version": "0.1.168",
  "ready": false,
  "reason": "candidate_not_ready"
}
```

The new authenticated endpoint is:

```text
GET /api/v1/admin/system/host-update/readiness?target_version=0.1.168
```

The endpoint uses the same bearer-token, active-admin, UI-header,
same-origin/fetch-site, and official identity verification rules as update
status. `ready=false` is returned in a successful envelope so the UI can render
normal preparation state. Unexpected resolver failures remain HTTP 500.

The existing POST endpoint still invokes `Schedule`, which calls the resolver
again. If the candidate is no longer available, the POST returns
`409 UPDATE_CANDIDATE_NOT_READY` rather than relying on the earlier UI result.

### Update dialog state

When the dialog opens, the UI fetches official update information, current
update-operation status, and host readiness. A scheduled/running operation
continues to take precedence over readiness copy. Otherwise:

- `ready=true`: the existing confirmation and scheduling controls are enabled.
- `ready=false`: controls remain visible but submit is disabled and the dialog
  presents the preparation message in an informational warning tone.
- readiness request failure: submit remains disabled and the dialog presents a
  genuine service-unavailable message.

The dialog rechecks readiness every 30 seconds and immediately enables normal
submission after the exact target becomes ready. Closing the dialog cancels the
poll timer.

### Theme-aware brand particles

`App` passes its resolved `Theme` to `BrandReveal`. `BrandReveal` derives one
particle color per theme:

- dark: `#dce4fa`
- light: `#354e78`

The Canvas build effect depends on `theme`; a theme toggle rebuilds the pixel
field with the new color and preserves pointer disturbance, scroll parallax,
intersection suspension, and resize behavior. The static fallback already uses
the same colors through CSS.

### Candidate preparation and production loading

The existing `Sub2API release preparation` workflow is the authority for:

1. resolving the official `0.1.168` tag and commit;
2. merging upstream source onto the current trusted `main`;
3. running backend, frontend, updater, host-update, vet, build, and difference
   gates;
4. building the immutable Linux AMD64 image with qualification labels;
5. publishing the immutable image and audit branch;
6. loading and verifying the candidate through the restricted production
   candidate loader;
7. advancing the source branch only after production candidate staging
   succeeds.

No manually labelled image may be substituted for this workflow. The running
Sub2API service remains on its current image; this task restores upgrade
availability rather than performing the application upgrade.

## Error Semantics

| Condition | HTTP/API behavior | UI behavior |
|---|---|---|
| Qualified image missing | readiness `ready=false`; POST `409 UPDATE_CANDIDATE_NOT_READY` | preparation message, submit disabled |
| Requested version no longer latest | `409 UPDATE_TARGET_CHANGED` | reload official target |
| Qualification labels invalid | `500 UPDATE_SERVICE_ERROR` | service error, submit disabled |
| Updater/socket unavailable | proxy/service failure | service unavailable, submit disabled |
| Candidate ready | readiness `ready=true` | normal update controls |

## Verification

### Automated

- Resolver test proves a missing Docker image is classified as
  `ErrCandidateNotReady`.
- Service test proves readiness creates no operation or state file.
- HTTP tests cover authenticated readiness, non-admin rejection, missing
  target, ready response, candidate-not-ready response, and POST 409 mapping.
- Update UI contract tests cover disabled submission, preparation copy,
  30-second readiness polling, transition to ready, and poll cleanup.
- BrandReveal component tests inspect Canvas drawing in light and dark themes
  and retain the reduced-motion fallback test.
- Homepage browser regression captures the bottom reveal in both themes and
  verifies the Canvas remains active and visually distinct.
- Existing updater, homepage, infrastructure, and release-preparation suites
  remain green.

### Production

Before deployment, record:

- PostgreSQL and Redis container IDs;
- `.State.StartedAt`;
- restart counts;
- updater and Caddy versions/hashes;
- absence or presence of `upstream-0.1.168`.

After candidate staging and updater/Caddy rollout, verify:

- `xingqiao-sub2api:upstream-0.1.168` exists by immutable image ID;
- all qualification labels match the official and source commits;
- the readiness endpoint returns `ready=true`;
- the update dialog enables submission without initiating an update;
- light and dark homepage screenshots show visible brand particles;
- updater is active and Caddy serves the new assets;
- the running Sub2API, relay-ops, PostgreSQL, and Redis container IDs are
  unchanged;
- PostgreSQL and Redis start times and restart counts are unchanged;
- public health, homepage, admin UI, and updater status return successfully.

## Rollback

The previous updater binary, unit, host UI assets, production Compose file, and
Caddy image are retained until acceptance completes. If updater or Caddy
acceptance fails, restore only those artifacts. Candidate image loading is
non-running state and does not require a database rollback. PostgreSQL and Redis
must never be included in rollback recreation commands.

## Out of Scope

- Clicking the enabled update button or switching the running Sub2API service
  to `0.1.168`.
- Enabling Feishu delivery beyond its current `shadow` policy.
- Rebuilding relay-ops, PostgreSQL, or Redis.
- Changing account, routing, pricing, balance, credential, or API-key data.
