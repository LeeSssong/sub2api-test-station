# T02 Native Error Diagnostics Design

## Goal

Project persisted native error fields into one deterministic four-class diagnosis used by user error records and both existing administrator error-detail entry points.

## Scope

- `local_limit`: local RPM, concurrency, quota, or other business rejection before account selection.
- `upstream_overloaded`: selected upstream returned overload/capacity evidence (429/529, overload or upstream rate-limit semantics).
- `upstream_failed`: other upstream/provider failure, including the conservative unknown fallback.
- `upload_interrupted`: ingress/request-body read interruption before account selection.

No transport error code/body changes are made. HTTP and SSE clients keep existing protocol-compatible responses; persisted records are the shared semantic seam.

## Projection Contract

`OpsErrorLogDetail.diagnosis` contains stable `class` and `code`, stage, ownership, `upstream_account_selected`, optional selected account/group identity, and sanitized original upstream status/message/detail. Account selection is true only when `account_id > 0`. Pre-selection local limits and upload interruptions therefore explicitly project as not selected.

Classification order is upload interruption, explicit local business-limit evidence, selected-account upstream overload, then explicit generic upstream-failure evidence. Records outside these four evidenced classes have no diagnosis; administrator JSON omits the diagnosis section and the user projection preserves only a sanitized, bounded native-safe message without inventing `upstream_failed`. The persisted `is_business_limited` flag is supporting canonical evidence but never classifies a record by itself.

The user DTO exposes only `error_class`, a concise Chinese-safe `meaning`, and an actionable `suggestion`. Its existing `message` is replaced by the safe meaning for compatibility with current table rendering. User detail no longer serializes raw error body or upstream status.

## Sanitization

Administrator evidence is re-sanitized at read projection time using the existing native storage sanitizers and bounded lengths. User DTOs never receive administrator evidence, account/group internals, upstream endpoint/model, credentials, full API keys, or request bodies.

## UI

- User error detail shows meaning and suggestion, not raw upstream evidence.
- Existing admin usage and ops modals already share `OpsErrorDetailModal`; it gains an “管理员诊断” section from `diagnosis` and keeps the existing response evidence section for administrators.
- No new page or route.

## Verification

Focused Go tests cover all four classes, selected/not-selected semantics, sanitization, unknown fallback, user JSON whitelist, and equivalent HTTP/SSE persisted projections. Vitest covers user raw-evidence removal and the shared administrator diagnosis renderer/type contract. Run frontend typecheck/build plus focused backend tests, vet/build, diff/secret review.

## Non-goals

No migration, new table, retry/scheduler changes, attempt-chain UI, tracing, root-cause inference, billing, external control plane, GitHub Actions, deployment, or production changes.
