# Sub2API Usage Record Details Design

**Date:** 2026-07-25 (Asia/Shanghai)
**Status:** Approved for specification review; implementation not started

## Goal

Add an explicit detail action to successful and failed request records on both
the administrator and ordinary-user usage pages. A detail dialog must expose
the request, token, latency, and billing information already recorded by
Sub2API while preserving the existing user/admin visibility boundary.

The implementation must reuse Sub2API's existing usage log storage, service
query, user ownership check, error-detail APIs, formatting conventions, and
dialog/table components. It must not introduce another log table or duplicate
request collection.

## Scope

### In scope

- Administrator usage records at `/admin/usage`.
- Ordinary-user usage records at `/usage`.
- Successful usage records and failed-request records on both pages.
- A shared successful-record detail dialog.
- One administrator-only endpoint for fetching a successful record by ID.
- Explicit detail actions in desktop and mobile table presentations.
- Chinese and English translations, component tests, API tests, and backend
  handler tests.

### Out of scope

- Capturing request or response bodies for successful requests.
- Persisting new usage fields or changing existing billing calculations.
- Combining the existing successful-record and error-record tabs into one
  feed.
- Exposing credentials, authorization headers, cookies, raw upstream
  responses, account secrets, or Base URLs.
- Exposing administrator-only routing or account-cost fields to ordinary
  users.

## Approved Decisions

1. Use one successful-record dialog for both administrator and user pages.
2. Select the detail API with an explicit `user` or `admin` scope prop.
3. Reuse `GET /api/v1/usage/:id` for ordinary users.
4. Add `GET /api/v1/admin/usage/:id` for administrators.
5. Reuse the existing error-detail APIs and error-detail dialogs.
6. Keep the existing error-record row-click behavior and add a visible detail
   action so the interaction is discoverable.
7. Keep user details redacted through the existing user DTO; do not rely only
   on frontend field hiding.
8. Display only values supported by persisted usage data. Do not invent a
   pricing-plan name or upstream metadata when no value exists.
9. Add the current user's own request ID to the user error-detail whitelist;
   keep it out of the error list response and keep all upstream/internal IDs
   subject to the existing redaction policy.

## Backend Design

### Existing user endpoint

`GET /api/v1/usage/:id` remains unchanged. It already:

1. Reads the authenticated user from the request context.
2. Parses the usage log ID.
3. calls `UsageService.GetByID`.
4. Verifies that `record.UserID` equals the authenticated user ID.
5. maps the result with `dto.UsageLogFromService`.

The ordinary-user dialog must use this endpoint and must not obtain data from
an administrator endpoint.

### New administrator endpoint

Add `GET /api/v1/admin/usage/:id` under the existing administrator route
group. The handler must:

1. Reject a missing, non-numeric, or non-positive ID as a bad request.
2. Call the existing `UsageService.GetByID` method.
3. Preserve existing not-found and repository error mapping through
   `response.ErrorFrom`.
4. Return `dto.UsageLogFromServiceAdmin(record)`.

The existing administrator middleware is the authorization boundary. No user
ownership restriction applies after administrator authorization succeeds.
Static `/admin/usage` routes such as `/stats`, `/search-users`, and cleanup
routes remain unchanged.

### Data and persistence

No migration or new repository is required. Both endpoints read the existing
`usage_logs` record and its already-loaded shallow relationships. Successful
record details contain no request body or generated response content because
Sub2API does not persist those values in usage logs.

### User error-detail whitelist

Keep `GET /api/v1/usage/errors/:id` and its ownership check. Extend only the
detail projection with a `request_id` field selected from the stored gateway
request ID, falling back to the stored client request ID when necessary. A
request ID identifies the user's own request and is safe only after the
existing user ownership check succeeds.

Do not add the request ID to the paginated user error list, and do not expose
account IDs, upstream endpoints, upstream request IDs, internal phases, raw
headers, or other administrator diagnostics through this projection.

## Frontend Architecture

### Shared successful-record dialog

Add `UsageDetailDialog.vue` in the shared usage component area. Its public
contract is:

- `show: boolean` controls visibility;
- `usageId: number | null` identifies the record;
- `scope: 'user' | 'admin'` selects the API and visibility mode;
- `update:show` closes the dialog.

When opened with a valid ID, the dialog fetches the complete record. Closing
the dialog clears the previous record and error state. If the ID or scope
changes while open, only the latest request may populate the dialog. The
component must ignore or cancel stale responses.

The dialog uses `BaseDialog`, existing formatting helpers, and the application
toast/clipboard behavior. It must support light mode, dark mode, narrow mobile
viewports, long request IDs, long endpoint paths, and missing historical
fields.

### Successful-record table integration

Extend `UsageTable.vue` with a `detailClick` event carrying the selected usage
log ID. Add a `detail` cell containing a text/icon action with an accessible
label. The action must stop propagation so it does not trigger unrelated row
behavior.

Both `admin/UsageView.vue` and `user/UsageView.vue` add the detail column at
the right edge and keep it always visible rather than placing it in optional
column settings. Each view owns its selected ID and dialog visibility state,
renders the shared dialog with the correct scope, and opens it from the table
event.

### Failed-request integration

Failed requests continue to use their existing data path:

- ordinary users use `GET /api/v1/usage/errors/:id` and
  `UserErrorDetailModal`;
- administrators use the existing administrator ops error-detail endpoint and
  `OpsErrorDetailModal`.

Add an explicit detail action to each error table presentation. Ordinary-user
row click remains supported. Administrator error-table behavior continues to
emit its existing `openErrorDetail` event. This change does not merge success
and error APIs or dialogs because their persisted schemas and security
policies differ.

## Detail Content

### Common successful-request summary

Show the following when present:

- request ID with a copy action;
- request time;
- inbound endpoint;
- API key display name;
- requested model;
- group display name;
- request type, including sync, stream, WebSocket, or cyber;
- service tier and reasoning effort;
- first-token latency and total duration;
- client IP and user agent where the selected DTO permits them.

### Token and media details

For token-billed requests, show input, output, cache-creation, cache-read,
five-minute cache-creation, one-hour cache-creation, and image input/output
tokens when non-zero or semantically relevant.

For image requests, reuse the existing billing-mode and image helpers to show
count, input/output size, size source, and size breakdown only when those
values are present. Other media modes show the billing mode and the common
fields currently present in the usage DTO; the feature does not add new media
metadata. Historical missing values render as `-` rather than causing a
broken layout.

### Billing details

Show billing mode, billing type, input cost, output cost, cache costs, image
input/output costs, standard total cost, group rate multiplier, long-context
billing marker, and actual charged cost as supported by the record.

An effective per-million-token price may be displayed only when the relevant
token count is greater than zero, calculated from the stored component cost.
It must be labeled as an effective unit price, not as a named pricing rule.
Currency symbols and numeric precision follow the existing Sub2API usage
formatters.

### Administrator-only details

When `scope` is `admin`, additionally show fields present in
`AdminUsageLog`:

- upstream account ID and display name;
- upstream endpoint and upstream model;
- channel ID;
- model mapping chain;
- billing tier;
- account rate multiplier;
- account statistics cost or its existing fallback calculation.

These sections are absent in user mode. The user endpoint must also omit the
underlying fields so browser inspection cannot reveal them.

### Error details

Error details retain the existing sanitized fields: request time, inbound
endpoint, key/group/model/platform metadata, status code, category, message,
and redacted response body. The ordinary-user detail additionally includes
the owned request ID described by the whitelist above. Administrator-only
error details may continue to show their existing upstream and diagnostic
fields.

## Loading and Error Behavior

- Opening a detail starts in a loading state and does not show the previously
  selected record.
- A failed request shows an inline retryable load error and leaves the page
  usable.
- A not-found or forbidden response is not replaced with list-row data.
- Missing optional values display `-` or omit the irrelevant row.
- Closing during a request prevents the late response from reopening or
  repopulating the dialog.
- Copy success and failure use existing application feedback conventions.
- Detail failures do not refresh, clear, or mutate the surrounding usage
  table.

## Testing

### Backend

- Administrator detail returns an admin DTO for a valid ID.
- Invalid and non-positive IDs return a bad request.
- Missing records preserve not-found behavior.
- The administrator route is registered under the protected route group.
- Existing ordinary-user ownership tests continue to prove cross-user access
  is forbidden.
- User error-detail mapper tests verify request-ID selection and confirm that
  internal/upstream identifiers remain omitted.
- DTO mapper tests verify that user output omits admin-only fields while admin
  output includes safe account and upstream metadata.

### Frontend

- API tests cover the new administrator detail call.
- `UsageTable` emits the correct ID and keeps the detail action accessible.
- The shared dialog selects the correct endpoint for each scope.
- User mode renders common/token/billing fields and never renders admin-only
  sections.
- Admin mode renders available upstream/account fields.
- Loading, stale-response, load-error, close, missing-field, and copy states
  are covered.
- Administrator and user usage view tests verify event wiring and dialog
  scope.
- Error table tests verify the explicit detail action without regressing
  ordinary-user row click or administrator `openErrorDetail` behavior.

## Done When

1. Successful records on both usage pages expose an explicit detail action.
2. Failed records on both pages expose an explicit detail action.
3. Successful details show request, token/media, latency, and billing sections
   using existing stored data.
4. Administrator details show safe upstream/account metadata.
5. Ordinary-user API responses and UI contain no administrator-only fields.
6. No database migration or duplicate logging path is introduced.
7. Focused backend and frontend tests pass.
8. Desktop and mobile browser checks show readable, non-overlapping dialogs
   in light and dark themes.
