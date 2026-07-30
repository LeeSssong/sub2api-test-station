# Monitor Reliability And Admin Visibility Design

**Date:** 2026-07-30
**Status:** Approved by user instruction “按你推荐方案修复”

## Problem

Three production behaviors are misleading:

1. New API multiplier measurement reads `total_used` immediately after a
   charged completion. Eventually consistent upstream counters can still show
   the old value, which is persisted as `failed`.
2. Monitor V2 always filters exclusive groups, even for administrators.
3. Channel monitors are associated with groups by free-form `group_name`.
   The production values `GPT-PLUS-内测` and `GPT PLUS 内测分组` therefore do
   not match, leaving the card unconfigured with zero models.

## Goal

- Tolerate bounded upstream quota-counter propagation delay.
- Retry failed automatic multiplier measurements sooner than successful ones.
- Keep ordinary-user Monitor V2 limited to active public groups.
- Show administrators every active group, including exclusive groups.
- Associate channel monitors with groups by nullable database `group_id`,
  while retaining `group_name` as display and legacy fallback data.

## Non-goals

- Do not show inactive or deleted groups.
- Do not expose account, credential, user, request, or raw upstream data.
- Do not change group pricing, routing, scheduling, or account membership.
- Do not remove `group_name` compatibility in this release.

## Design

### Multiplier measurement

After each completion, poll `/api/usage/token/` with bounded backoff until
`total_used > before`. Stop on context cancellation, request failure, or the
poll limit. A counter that never advances still fails safely.

Successful measurements remain fresh for 24 hours. Failed measurements use a
short retry interval so normal account-monitor runs can recover without a
manual database edit or a 24-hour wait. Persist a sanitized failure code in
the private account `extra` snapshot for operator diagnosis, but do not add it
to the public/admin projection.

### Monitor V2 visibility

The authenticated handler reads the role already stored by JWT middleware.
It passes an explicit scope to the service:

- `public`: active, non-exclusive groups;
- `admin`: every active group.

The response remains aggregate-only. Shared UI copy explains both scopes so
the existing response contract does not need identity fields.

### Stable monitor-group association

Add nullable `channel_monitors.group_id` with an indexed foreign key to
`groups(id)` using `ON DELETE SET NULL`. CRUD DTOs accept and return
`group_id`. The administrator form uses a group selector and continues to
send the selected group name for readable legacy clients.

Monitor V2 resolves probes by `group_id` first. Rows without `group_id` fall
back to the existing normalized `group_name` match. Production monitor 13 is
backfilled to group 16 after deployment.

## Failure handling

- Quota polling is bounded by the existing measurement request context.
- Missing or deleted group IDs fall back to legacy name data without exposing
  the monitor endpoint or key.
- A non-admin can never request the admin scope directly; scope comes only
  from trusted middleware context.
- Migration and production backfill are idempotent.

## Acceptance criteria

- A delayed positive `total_used` value produces a valid sample.
- A counter that never advances still fails after bounded attempts.
- Failed snapshots become eligible for automatic retry before 24 hours.
- Ordinary users receive only active non-exclusive groups.
- Administrators receive all six currently active production groups.
- A probe with `group_id=16` configures `GPT-PLUS-内测` regardless of its
  legacy `group_name`.
- Existing name-only monitors continue to work.
- Backend tests, frontend tests, typecheck, lint, build, migration, and
  production smoke checks pass.
