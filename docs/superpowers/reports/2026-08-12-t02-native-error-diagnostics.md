# T02 Native Error Diagnostics Handoff Report

## Status

- Task: T02 native error translation and administrator diagnostics MVP
- Base: `74f139243a0d402674713f218c8079232ad6465d`
- Candidate: `codex/t02-native-error-diagnostics`
- Handoff state: `READY_FOR_ROOT_REVIEW` after the final implementation commit and clean-worktree check
- Ledger state: 进行中; not merged, pushed, deployed, or production-verified

## Delivered Scope

- Deterministic persisted-record projection for `local_limit`, `upstream_overloaded`, `upstream_failed`, and `upload_interrupted`.
- Existing administrator detail DTO/modal receives stage, ownership, selected/not-selected upstream account state, optional account/group identity, and re-sanitized upstream status/message/detail.
- User list/detail DTOs expose only safe Chinese meaning and suggestion plus ordinary owned-request metadata. Raw upstream status/body, platform, group, account, endpoint/model internals, credentials, and sensitive request bodies are absent.
- HTTP and SSE transport behavior is unchanged: no status, type, code, body, retry, scheduler, or routing modification. Their persisted records share the same diagnosis projection.

## TDD Evidence

### Backend diagnosis RED

```text
go test ./internal/service -run 'Test(ProjectNativeErrorDiagnosis|AttachNativeErrorDiagnosis)' -count=1
```

Failed as expected because `ProjectNativeErrorDiagnosis`, `AttachNativeErrorDiagnosis`, and `Diagnosis` did not yet exist.

### Backend diagnosis GREEN

```text
ok github.com/Wei-Shaw/sub2api/internal/service 1.056s
```

### User DTO RED

```text
go test ./internal/service -run 'Test(ToUserErrorRequest|ListUserErrorRequests|GetUserErrorRequestDetail)' -count=1
```

Failed as expected because the safe `ErrorClass`, `Meaning`, and `Suggestion` projection did not yet exist. The first GREEN attempt exposed a list-only local-limit classification gap because list records omit `is_business_limited`; the persisted `phase=request/type=rate_limit_error` evidence was then covered and implemented.

### User privacy RED

```text
go test ./internal/service -run 'TestToUserErrorRequest' -count=1
pnpm vitest run src/components/user/__tests__/UserErrorRequestsTable.spec.ts
```

Failed as expected because user JSON/table still exposed effective status, group, and platform fields. These fields and the corresponding filter/columns were removed from the user projection.

### Frontend RED

```text
pnpm vitest run \
  src/components/user/__tests__/UserErrorDetailModal.spec.ts \
  src/views/admin/ops/components/__tests__/OpsErrorDetailModal.spec.ts
```

Failed as expected because the user modal rendered legacy raw message/body/status and the administrator diagnosis renderer did not exist. A partial `vue-i18n` mock correction was made before evaluating component behavior.

## Fresh Final Verification

```text
go test ./internal/service -run 'Test(ProjectNativeErrorDiagnosis|AttachNativeErrorDiagnosis|ToUserErrorRequest|ListUserErrorRequests|GetUserErrorRequestDetail)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 2.665s
```

```text
go test ./internal/service ./internal/handler ./internal/repository
ok github.com/Wei-Shaw/sub2api/internal/service 105.120s
ok github.com/Wei-Shaw/sub2api/internal/handler 38.418s
ok github.com/Wei-Shaw/sub2api/internal/repository 5.044s
```

```text
go vet ./internal/service ./internal/handler ./internal/repository
go build ./cmd/server
# exit 0
```

```text
pnpm vitest run \
  src/components/user/__tests__/UserErrorRequestsTable.spec.ts \
  src/components/user/__tests__/UserErrorDetailModal.spec.ts \
  src/views/admin/ops/components/__tests__/OpsErrorDetailModal.spec.ts \
  src/views/user/__tests__/UsageView.spec.ts
# 4 files passed, 19 tests passed

pnpm typecheck
# exit 0

pnpm build
# exit 0; 1044 modules transformed
```

The frontend build emitted only existing Browserslist age, dynamic/static import, and chunk-size warnings.

## Release Properties

- Migrations: none
- Configuration changes: none
- GitHub Actions: none
- `downtime_required=false`
- Rollback: revert the T02 documentation and implementation commits; no data/configuration rollback is required.

## Review / Remaining Risk

- No live browser smoke test, production deployment, or online verification was performed in this candidate task.
- Classification is intentionally conservative: non-target or unknown evidence produces no diagnosis, and account selection is never inferred without a positive persisted account ID.
- Root review should confirm the strict user DTO allowlist and administrator-only evidence boundary before merge.

## Independent Review Follow-up

The follow-up review identified and fixed three important boundaries without expanding T02 scope:

- `is_business_limited` is now carried by the base persisted error record and selected by the list repository query, so list and detail projections use the same canonical native flag. Text fallback also covers the existing native policy, API-key quota, subscription, platform quota, and model/policy rejection markers.
- `upstream_overloaded` now requires a positive selected account ID. Pre-selection capacity/overload wording conservatively falls back to `upstream_failed`; selected-account list status still recognizes effective 429/529.
- Administrator evidence redaction now covers non-JSON `x-api-key`, `api_key`, `access_token`, `refresh_token`, `client_secret`, `authorization`/Bearer, Cookie, quoted values, and key-like prefixes with bounded output. User DTO behavior is unchanged.

A subsequent review removed the former catch-all `upstream_failed` projection. Authentication failures, generic routing failures, required-model validation, oversized request bodies, and a bare `is_business_limited=true` flag now produce `diagnosis=nil`. The administrator diagnosis section is omitted, while the user record keeps only a credential-sanitized, bounded native-safe message with no four-class code or suggestion. `upstream_failed` is emitted only with explicit upstream/network/account-auth, selected-account, upstream status, or upstream evidence fields.

Follow-up TDD evidence:

```text
go test ./internal/service ./internal/repository -run 'Test(ProjectNativeErrorDiagnosis(ListAndDetail|MatchesCanonical|DoesNotCallPreselection|SanitizesCommon)|OpsErrorListSelectsCanonical)' -count=1
ok internal/service; ok internal/repository
```

The quoted non-JSON credential case first failed as expected, then passed after the regex correction.

Fresh post-review verification:

```text
go test ./internal/service ./internal/handler ./internal/repository
ok internal/service 103.937s
ok internal/handler 39.685s
ok internal/repository 2.463s

go vet ./internal/service ./internal/handler ./internal/repository
go build ./cmd/server
# exit 0

pnpm vitest run ...
# 4 files passed, 19 tests passed
pnpm typecheck
# exit 0
pnpm build
# exit 0; 1044 modules transformed
```
