# Official Sub2API Image Migration Design

**Date:** 2026-07-24 (Asia/Shanghai)
**Status:** Approved

## Goal

Move the production Sub2API core from the current Xingqiao custom build to an immutable official Sub2API image without losing production data or required Xingqiao behavior. After migration, Sub2API upgrades must be reproducible, health-checked, and reversible from the deployment layer rather than by replacing the binary inside a running container.

## Product Decisions

- The project will not submit changes to the upstream Sub2API repository.
- The project will not maintain a long-lived Sub2API source fork.
- A required Xingqiao behavior must be externalized into configuration, Caddy, the homepage, internal-test-service, relay-ops, or another independently released component before the official-image cutover.
- A custom change that cannot be externalized will be retired. It cannot remain as an undocumented patch inside the official image.
- Production will use an official Sub2API image pinned to an explicit version and immutable image digest. Floating `latest` tags are not allowed.
- PostgreSQL, Redis, application data, Caddy state, and independent Xingqiao services remain outside the Sub2API image lifecycle.
- The application must not replace its own executable in production. Image deployment owns upgrade and rollback.

## Why Deployment Owns Updates

The running artifact must be derivable from version-controlled deployment configuration. Sub2API's in-application updater replaces the binary inside a container writable layer, which makes the running binary differ from the declared image. Recreating the container can then restore an older binary, while image inspection no longer proves what code is running.

The supported update path is therefore a single operator command backed by Compose automation:

1. Validate the requested official version and digest.
2. Verify current service health and available storage.
3. Create and validate a PostgreSQL backup.
4. Pull the exact image.
5. Recreate only the Sub2API service against the existing data services and volumes.
6. Wait for container and public health checks.
7. Run authenticated, non-mutating smoke checks for the critical admin and gateway surfaces.
8. Keep the new image when all checks pass; otherwise recreate the prior pinned image.

This is the project's "one-click update": one controlled deployment action, not in-process mutation.

## Customization Inventory

Before changing production, compare the current custom Sub2API source and embedded frontend with the official version on which it was based. Every delta must be recorded with exactly one disposition:

| Disposition | Meaning |
|---|---|
| Externalize | Reimplement the required behavior outside the Sub2API image with a stable interface. |
| Configuration | Express the behavior using supported Sub2API, Compose, or Caddy configuration. |
| Retire | Remove the behavior because it is no longer required. |
| Blocked | The behavior requires a core patch; official-image cutover cannot proceed until the requirement changes. |

Upstream submission and indefinite fork maintenance are intentionally absent from the allowed dispositions.

The inventory must include frontend navigation and contact entry changes, backend handlers and DTOs, embedded assets, build metadata, security defaults, operational endpoints, and any behavior currently tested only against the custom image.

## Target Architecture

### Official core

- Sub2API runs from the official repository image pinned by tag and digest.
- The container filesystem is treated as disposable.
- No host bind mount overlays the Sub2API executable or embedded frontend.
- The deployed version is obtained from both the image reference and the application's version endpoint and must agree.

### Persistent state

- PostgreSQL remains the authority for users, accounts, groups, prices, balances, usage, and application settings.
- Redis remains replaceable cache and coordination infrastructure according to its existing persistence policy.
- `/app/data` remains explicitly mapped to its current persistent storage where still required.
- The migration must preserve the existing production Compose project storage. A project-name change may not implicitly create empty replacement volumes.
- No deployment or rollback command may use `down -v`, delete volumes, or initialize a new database over the production endpoint.

### Xingqiao-owned capabilities

- Public homepage and brand presentation remain in the independent homepage/Caddy deployment.
- Contact and support destinations are owned by an external Xingqiao surface or configuration, not an embedded Sub2API frontend patch.
- D04 behavior remains in internal-test-service.
- Read-only operations projection, reporting, quality monitoring, and Feishu behavior remain in relay-ops and its existing scheduler boundary.
- Caddy continues to own public routing and may expose only explicitly approved paths.

## In-Application Update Guard

Caddy will reject production requests to Sub2API's binary mutation endpoints before proxying them:

- `POST /api/v1/admin/system/update`
- `POST /api/v1/admin/system/rollback`

The response must be a stable JSON error explaining that Docker deployments are updated through the controlled Compose workflow. Read-only version checks remain available. Blocking occurs at the deployment boundary so it continues to apply while using an unmodified official image.

The guard is defense in depth. Administrator documentation and the deployment runbook must also identify the Compose workflow as the only supported update path.

## Release And Rollback State

The deployment keeps a small, non-secret release record containing:

- previous official image reference and digest;
- requested image reference and digest;
- deployment time;
- backup artifact identifier and validation result;
- health and smoke-check result;
- final state: promoted or rolled back.

Rollback restores the previous image against the same persistent stores. Database migrations require special handling: an image is eligible for automatic rollback only when the release notes and a preflight check establish backward compatibility. Otherwise the deployment stops before mutation and requires a separately approved database restore procedure.

## Migration Sequence

### Phase 1: Freeze and inventory

- Preserve the current custom image digest and Compose configuration as the rollback baseline.
- Record the actual production container, network, bind mount, named volume, and Compose project mappings without exposing secrets.
- Produce the complete custom-delta inventory and assign every delta a disposition.

### Phase 2: Externalize and verify

- Implement only the required external replacements.
- Add contract tests showing that each replacement works without modifying the Sub2API image.
- Verify the current custom deployment still works while external components are introduced.

### Phase 3: Official-image rehearsal

- Restore a recent sanitized or controlled production backup into an isolated rehearsal stack.
- Start the pinned official image against the rehearsal state.
- Verify schema migration, authentication, administrator flows, account scheduling, model pricing, downstream gateway requests, usage recording, and the independent Xingqiao services.
- Confirm the Caddy update guard rejects mutation endpoints.

### Phase 4: Controlled production cutover

- Create and validate a fresh backup.
- Stop writes for the bounded cutover window where required by migration behavior.
- Recreate only the Sub2API service with the approved official image.
- Run health and smoke checks, then either promote or restore the prior image according to the predeclared rollback decision.

### Phase 5: Routine operation

- Use the controlled Compose update command for later releases.
- Keep the previous image and validated backup until the new release completes its observation window.
- Re-run the customization boundary check so new core patches cannot silently reappear.

## Failure Handling

- Missing or invalid image digest: stop before pulling or changing services.
- Backup creation or validation failure: stop before changing services.
- Storage mapping mismatch: stop before starting the official image.
- Official image health failure: recreate the previous image; do not alter volumes.
- Authenticated smoke-check failure: treat the release as failed even when `/health` is green.
- Database migration incompatibility: stop automatic rollback and follow the separately approved restore procedure.
- Required custom delta marked `Blocked`: do not cut over to the official image.

## Acceptance Criteria

- The custom-delta inventory has no unresolved or `Blocked` required item.
- Production Compose references an official Sub2API version and immutable digest, with no custom Sub2API build context.
- The running application's reported version agrees with the pinned official image.
- Existing administrator, user, account, group, pricing, balance, usage, and API-key records remain present after cutover.
- Existing downstream synchronous and streaming gateway smoke checks pass without creating uncontrolled spend.
- Homepage, contact/support path, D04, relay-ops, monitoring, reports, and approved Caddy routes retain their required behavior without modifying the Sub2API image.
- The Caddy guard rejects in-application update and rollback requests with the documented error.
- A rehearsed rollback recreates the previous image without deleting or replacing production storage.
- Recreating the Sub2API container does not change its reported version or remove Xingqiao-owned capabilities.
- Repository checks reject future reintroduction of a Sub2API build context, floating image tag, binary bind mount, or missing update guard.

## Non-Goals

- Contributing changes to the upstream Sub2API project.
- Maintaining a private Sub2API fork or private patched image after migration.
- Automatically applying every upstream release without review.
- Deleting historical custom source or rollback images during the migration increment.
- Changing production accounts, prices, balances, routing, registration, payment state, or upstream credentials as part of the image cutover.

## Self-Review

- The design has no placeholder or undecided implementation boundary.
- The no-upstream-submission decision is consistent with the requirement to externalize or retire every custom core change.
- The official-image cutover is explicitly blocked by any required behavior that still depends on a core patch.
- Update, rollback, persistent-state ownership, and failure handling are defined independently of the current Compose project name.
- The scope changes the Sub2API release boundary without changing business configuration or authorizing production cutover before rehearsal passes.
