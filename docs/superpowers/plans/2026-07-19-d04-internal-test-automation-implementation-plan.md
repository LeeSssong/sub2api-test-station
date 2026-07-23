# D04 Internal Test Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an unattended internal-test service that enforces a 15-user registration cap, lets internal users issue multiple invitation codes, grants cumulative `$20` check-in and per-invitee `$5` rewards through Sub2API's idempotent Admin API, and protects one configurable total budget.

**Architecture:** Add a single `internal-test-service` Go container with a persistent SQLite database. It owns the join page, invitation issuance, registration gate, check-in endpoint, unified credit-grant ledger, usage cursor, budget reservation, reconciliation, daily reports, and Feishu alerts. It calls Sub2API only through documented HTTP APIs and never writes PostgreSQL. Caddy routes all account-creating paths through the service or blocks them, while normal login and gateway traffic continue to Sub2API.

**Tech Stack:** Go 1.24, standard `net/http`, `database/sql`, `embed`, `httptest`, pure-Go `modernc.org/sqlite` pinned to `v1.39.1`, Docker Compose, Caddy 2.10.2, JSON/HTTP Admin API, and existing Ruby/Bash contract tests. The host has no Go binary, so all Go tests run in `golang:1.24-alpine` through Docker.

## Global Constraints

- The initial internal-test limit is `D04_MAX_USERS=15`; the limit must be checked atomically at the registration submission boundary.
- `GPT-内测` remains `1.0x`, Neko-only, per-user RPM `3`, and current Neko account concurrency `3`.
- Each Shanghai calendar day grants at most one `$20` check-in event per user; check-in and referral grants enter one cumulative balance and do not expire during D04.
- Each referred user can trigger exactly one `$5` reward after its first successful billable request; a referrer can earn one reward per distinct referred user with no personal cap.
- A successful referred registration reserves `$5 * 0.07` budget before the first request; existing reservations remain payable when the total budget is reached.
- `D04_TOTAL_BUDGET_USD` is a total budget. New check-ins, invitation generation, and registrations stop when projected occupancy reaches it; existing balances remain usable.
- Invitation codes have no fixed time expiry while D04 is open and are invalidated when D04 closes. Multiple unused codes may coexist.
- The first invitation is a one-time administrator bootstrap; after that, every authenticated D04 user can issue invitations without administrator approval.
- All account-creation paths except email/password registration through the gate are disabled during D04, including OAuth completion/create-account routes.
- No direct Sub2API PostgreSQL access, private Sub2API fork, payment, public registration, or production credential in Git, logs, tests, or documentation.
- The Admin API key is administrator-level, mounted read-only from a server-only file, and never passed to an LLM Agent.
- Any uncertain Admin API write result is reconciled by idempotency key before retry; balance disagreement enters read-only mode and sends an alert.

---

## File Map

**Create:**

- `internal-test-service/go.mod`, `internal-test-service/go.sum`
- `internal-test-service/cmd/internal-test-service/main.go`
- `internal-test-service/internal/config/config.go`
- `internal-test-service/internal/domain/model.go`
- `internal-test-service/internal/store/sqlite.go`, `internal-test-service/internal/store/schema.sql`
- `internal-test-service/internal/sub2api/client.go`, `internal-test-service/internal/sub2api/types.go`
- `internal-test-service/internal/credits/service.go`
- `internal-test-service/internal/registration/service.go`
- `internal-test-service/internal/http/server.go`, `internal-test-service/internal/http/join.html`
- `internal-test-service/internal/ops/scheduler.go`, `internal-test-service/internal/ops/report.go`, `internal-test-service/internal/ops/alert.go`
- `internal-test-service/internal/testsupport/fake_sub2api.go`
- `internal-test-service/internal/**/*_test.go`
- `infra/Dockerfile.internal-test`
- `tests/internal_test/validate_internal_test_contract.sh`

**Modify:**

- `infra/compose.yaml`: add the fifth service and SQLite volume; do not publish its port.
- `infra/Caddyfile`: route `/internal-test/*` and `POST /api/v1/auth/register` to the service; block OAuth account creation routes; keep normal Sub2API reverse proxy unchanged.
- `infra/.env.example`: add non-secret D04 configuration and symbolic secret-file paths.
- `tests/infra/validate-baseline.sh`: assert the service has no host ports, Caddy owns 80/443, and account-creation routes are gated.
- `docs/runbooks/operations-and-incident-response.md`: add D04 read-only, budget, and closure procedures.
- `docs/project/current-state.md`, `docs/project/llm-handoff.md`: update after each verified milestone.

---

### Task 1: Scaffold the Go Service and SQLite Domain Ledger

**Files:**

- Create: `internal-test-service/go.mod`, `internal-test-service/go.sum`
- Create: `internal-test-service/internal/config/config.go`
- Create: `internal-test-service/internal/domain/model.go`
- Create: `internal-test-service/internal/store/schema.sql`, `internal-test-service/internal/store/sqlite.go`
- Test: `internal-test-service/internal/store/sqlite_test.go`, `internal-test-service/internal/domain/model_test.go`

**Interfaces:**

- `config.Load(env func(string) string) (config.Config, error)` validates `D04_MAX_USERS`, `D04_TOTAL_BUDGET_USD`, timezone, Sub2API base URL, Admin API key file, and Feishu webhook file without reading `infra/.env` in tests.
- `store.Store` exposes `WithTx(ctx, fn)`, `GetSetting`, `SetSetting`, `RegisterInvitation`, `ListInvitationUses`, `RegisterUser`, `CountRegisteredUsers`, `CreateGrant`, `FindGrantByIdempotencyKey`, `ListGrants`, `RecordUsage`, `GetUsageCursor`, `SetUsageCursor`, `GetUserBalanceSnapshot`, and `SetReadOnlyReason`.
- Domain constants are `GrantCheckin`, `GrantReferral`, `TaskPending`, `TaskSucceeded`, `TaskUncertain`, and `ModeReadOnly`.

- [x] **Step 1: Write failing domain and schema tests.** Define tests for Shanghai dates, one check-in per `(user_id, grant_date)`, one referral grant per `invitee_user_id`, unique idempotency keys, and a transaction that serializes registration-cap checks.

- [x] **Step 2: Run the focused tests and verify they fail.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/domain ./internal/store -count=1`

Expected: FAIL because the module, schema, and store methods do not exist.

- [x] **Step 3: Implement the schema and store.** Initialize the module with `go mod init example.invalid/internal-test-service` and `go get modernc.org/sqlite@v1.39.1`. Use SQLite WAL mode, foreign keys, and busy timeout. Create `settings`, `internal_users`, `invitations`, `credit_grants`, `usage_cursors`, `usage_records`, `jobs`, and `audit_events`. Store only numeric user IDs and redacted metadata. Make `credit_grants.idempotency_key`, `credit_grants.invitee_user_id` for referral rows, and `(user_id, grant_date)` unique.

- [x] **Step 4: Implement configuration and domain validation.** Reject `D04_MAX_USERS < 1`, non-positive total budget, non-`Asia/Shanghai` timezone, missing Admin API key file, or a writable key file. Parse money as decimal strings or integer microdollars; never use binary floating point for ledger arithmetic.

- [x] **Step 5: Run focused tests and verify they pass.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/domain ./internal/store -count=1`

Expected: PASS with tests covering duplicate check-in, duplicate referral, restart persistence, and transaction rollback.

- [ ] **Step 6: Commit the isolated scaffold.** Deferred because this shared worktree already contains unrelated/untracked project state; no commit was requested.

Run: `git add internal-test-service && git commit -m "feat: scaffold internal test ledger service"`

---

### Task 2: Implement the Sub2API HTTP Client and Fake Provider

**Files:**

- Create: `internal-test-service/internal/sub2api/types.go`, `internal-test-service/internal/sub2api/client.go`
- Create: `internal-test-service/internal/testsupport/fake_sub2api.go`
- Test: `internal-test-service/internal/sub2api/client_test.go`, `internal-test-service/internal/testsupport/fake_sub2api_test.go`

**Interfaces:**

- `sub2api.Client` methods: `GetCurrentUser(ctx, bearer string)`, `GenerateInvitation(ctx, count int, expiresAt *time.Time, idem string)`, `ListInvitationCodes(ctx)`, `ExpireInvitation(ctx, codeID int64, idem string)`, `AddBalance(ctx, userID int64, amount MicroUSD, idem, note string)`, `GetBalance(ctx, userID int64)`, `ListUsage(ctx, userID int64, afterID int64)`, and `GetUser(ctx, userID int64)`.
- `sub2api.Fake` implements the same behavior, including `Idempotency-Key`, timeout-after-commit, used invitation codes, successful usage records, and balance history.

- [x] **Step 1: Write failing contract tests.** Cover Admin API headers, JSON decoding, non-2xx error bodies, pagination, idempotent replay, timeout-after-commit recovery, and prohibition on logging key material or bearer tokens.

- [x] **Step 2: Run the tests and verify they fail.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/sub2api ./internal/testsupport -count=1`

Expected: FAIL because the client and fake server are absent.

- [x] **Step 3: Implement the client.** Use an `http.Client` with explicit connect, header, and body timeouts. Read the Admin key once from the configured file, send it only as `x-api-key`, attach stable `Idempotency-Key` for every write, and redact URLs, headers, and bodies in errors. Implement the verified v0.1.161 paths: `/api/v1/admin/redeem-codes/generate`, `/api/v1/admin/redeem-codes`, `/api/v1/admin/redeem-codes/:id/expire`, `/api/v1/admin/users/:id/balance`, `/api/v1/admin/usage?user_id=:id&sort_by=id&sort_order=asc`, `/api/v1/admin/users`, and `/api/v1/auth/me`. Use `/api/v1/admin/users/:id/usage` only for summaries in reports, never as the usage cursor source.

- [x] **Step 4: Implement the fake provider.** Keep all state in memory behind a mutex, expose the endpoints required by the client, and add test controls for delayed response, timeout-after-commit, malformed JSON, and injected balance mismatch.

- [x] **Step 5: Run the tests and verify they pass.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/sub2api ./internal/testsupport -count=1`

Expected: PASS, including timeout-after-commit proving the same idempotency key returns one effect.

- [ ] **Step 6: Commit the provider contract.** Deferred for the same shared-worktree reason.

Run: `git add internal-test-service && git commit -m "feat: add sub2api client contract and fake provider"`

---

### Task 3: Build the Registration Gate and Self-Service Invitations

**Files:**

- Create: `internal-test-service/internal/registration/service.go`
- Modify: `internal-test-service/internal/store/sqlite.go`
- Test: `internal-test-service/internal/registration/service_test.go`, `internal-test-service/internal/registration/registration_concurrency_test.go`

**Interfaces:**

- `registration.Service.CreateInvitation(ctx, issuerUserID int64) (JoinLink, error)` issues one native invitation code and stores its provider code ID plus a private-volume display value and issuer ID. The raw code is never logged, documented, or returned by status APIs except to the join-link page; the SQLite volume remains server-only and is not exposed to reports or agents.
- `registration.Service.JoinState(ctx, joinID string) (JoinState, error)` returns `open`, `full`, `budget_full`, or `closed`; it never returns the raw code in logs.
- `registration.Service.GateRegistration(ctx, body []byte, headers http.Header) (status int, responseHeaders http.Header, responseBody []byte, err error)` serializes cap checks and forwards allowed email/password registrations.
- `registration.Service.Close(ctx)` disables invitation issuance, invalidates unused codes, and records an audit event.

- [x] **Step 1: Write failing tests for multiple codes and hard cap.** Verify two users can issue codes concurrently, codes remain usable while D04 is open, the 15th registration succeeds, the 16th returns `409 INTERNAL_TEST_FULL`, and five simultaneous registration submissions create at most the remaining slots.

- [x] **Step 2: Run the tests and verify they fail.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/registration -count=1`

Expected: FAIL because invitation and registration services are absent.

- [x] **Step 3: Implement invitation creation.** Require a valid JWT-derived internal user, D04 open mode, `registered_count < D04_MAX_USERS`, and projected budget room for the `$5 * 0.07` referral reservation. Call `GenerateInvitation` with no expiry, use `d04-invite-<issuer>-<random-id>` as idempotency key, and return an opaque join ID. Multiple IDs are allowed.

- [x] **Step 4: Implement the registration gate.** Acquire a process-wide mutex and SQLite transaction, read `invite_code` from the JSON registration body or `X-D04-Invitation-Code`, remove that field before forwarding, count distinct non-null `used_by` users among invitation codes issued by D04, reject capacity or budget exhaustion with stable JSON error codes and Chinese user-facing messages, forward only allowed email/password registration bodies to Sub2API, then persist the resulting user and referral reservation. Never log the body or response.

- [x] **Step 5: Implement close behavior.** Set D04 mode to `closed`, reject new invitation and registration requests, and call the provider expiry endpoint for every unused D04 invitation code. Record failures as retryable jobs and alert instead of claiming closure succeeded.

- [x] **Step 6: Run concurrency and restart tests.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/registration -race -count=1`

Expected: PASS with no race and no more than `D04_MAX_USERS` successful registrations.

- [ ] **Step 7: Commit the registration gate.** Deferred for the same shared-worktree reason.

Run: `git add internal-test-service && git commit -m "feat: enforce internal test registration cap"`

---

### Task 4: Implement the Unified Credit Ledger, Check-In, and Referral Rewards

**Files:**

- Create: `internal-test-service/internal/credits/service.go`
- Modify: `internal-test-service/internal/store/sqlite.go`, `internal-test-service/internal/domain/model.go`
- Test: `internal-test-service/internal/credits/service_test.go`, `internal-test-service/internal/credits/budget_test.go`

**Interfaces:**

- `credits.Service.CheckIn(ctx, userID int64, now time.Time) (GrantResult, error)` creates one `$20` event for the Shanghai date and calls `AddBalance` with `d04-checkin-<user>-<date>`.
- `credits.Service.ProcessUsage(ctx, userID int64) (UsageSyncResult, error)` consumes provider usage records after the cursor, records successful debits, and updates referral first-use state.
- `credits.Service.ReconcileUser(ctx, userID int64) (Reconciliation, error)` evaluates `opening_balance + grants - successful_usage == provider_balance`.
- `credits.Service.Budget(ctx) (BudgetSnapshot, error)` returns actual cost, current balances, pending referral reservations, occupancy, and remaining total budget.

- [x] **Step 1: Write failing tests.** Cover Shanghai midnight without clearing balance, duplicate check-in, unified balance math, first successful referred usage, failed usage not rewarding, duplicate usage polling, referral reservation conversion, budget-full check-in rejection, and balance mismatch entering read-only mode.

- [x] **Step 2: Run the tests and verify they fail.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/credits -count=1`

Expected: FAIL because the credit service is absent.

- [x] **Step 3: Implement monetary arithmetic and budget projection.** Use integer micro-USD. Compute `occupancy = actual_cost + sum(provider_balances) * 0.07 + pending_referral_count * 5 * 0.07`; reject new grants/registrations if the projected amount exceeds `D04_TOTAL_BUDGET_USD`.

- [x] **Step 4: Implement check-in.** Validate internal membership and Shanghai date, insert the unique grant event, call the Admin balance endpoint with a stable key, and recover uncertain results by reading balance history before retrying. Store the event only after a confirmed provider effect.

- [x] **Step 5: Implement usage and referral processing.** Poll only successful billable usage records, advance the cursor transactionally, and for each referred user with no prior reward create exactly one `d04-referral-<invitee>` grant. Convert its reservation to actual balance without adding a second budget charge.

- [x] **Step 6: Implement reconciliation and read-only mode.** Compare the provider balance with the unified ledger equation; on unexplained drift, set `read_only_reason`, stop all writes for that user/service, preserve evidence, and emit an alert.

- [x] **Step 7: Run focused tests; commit deferred.** Tests passed; commit is deferred for the shared-worktree reason.

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/credits -race -count=1`

Expected: PASS with all idempotency, budget, midnight, and mismatch tests.

Commit: `git add internal-test-service && git commit -m "feat: add cumulative checkin and referral credits"`

---

### Task 5: Add JWT-Protected HTTP Routes and the Join Page

**Files:**

- Create: `internal-test-service/internal/http/server.go`, `internal-test-service/internal/http/join.html`
- Test: `internal-test-service/internal/http/server_test.go`

**Interfaces:**

- `GET /healthz` returns a fixed non-sensitive health response.
- `GET /internal-test/join/{join_id}` renders the current count, cap state, and copyable invitation code when open. The code is intentionally available to anyone holding the opaque join link; it is never written to logs or ordinary reports.
- `GET /internal-test/api/join/{join_id}` returns only `state`, `registered_count`, `max_users`, `aff_code`, and a redacted invitation display; the authenticated join-link owner can obtain the one-time code from the creation response. Never expose Admin API keys or provider IDs.
- `POST /internal-test/api/invitations` and `POST /internal-test/api/checkin` require the Sub2API bearer token and call the registration/credit services.
- `POST /api/v1/auth/register` is the internal proxy target used by Caddy; it is not exposed as a second public route.

- [x] **Step 1: Write failing handler tests.** Test missing/invalid JWT, valid internal user, full join page copy, multiple join links, check-in idempotency, JSON error codes, CSP headers, and absence of raw invitation codes from structured logs.

- [x] **Step 2: Run the tests and verify they fail.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/http -count=1`

Expected: FAIL because routes and embedded page are absent.

- [x] **Step 3: Implement JWT forwarding and handlers.** Forward the incoming bearer token to Sub2API `/api/v1/auth/me`, reject non-D04 users, set `Cache-Control: no-store`, use a strict CSP, and return Chinese messages for full/budget/read-only states.

- [x] **Step 4: Implement the join page.** Embed a small static HTML page with no third-party script. It calls the join-state endpoint, displays the invitation code with a copy button, and links to `/register?aff_code=<affiliate-code>`; the user pastes the invitation code into the native Sub2API form. The page endpoint may include the raw code because possession of the join link is the invitation capability; JSON status and logs remain redacted.

- [x] **Step 5: Run handler tests; commit deferred.** Tests passed; commit is deferred for the shared-worktree reason.

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/http -race -count=1`

Expected: PASS with no credential or code leakage in logs.

Commit: `git add internal-test-service && git commit -m "feat: expose internal test join and checkin routes"`

---

### Task 6: Add Background Workers, Reports, and Feishu Alerts

**Files:**

- Create: `internal-test-service/internal/ops/scheduler.go`, `internal-test-service/internal/ops/report.go`, `internal-test-service/internal/ops/alert.go`
- Modify: `internal-test-service/internal/store/sqlite.go`
- Test: `internal-test-service/internal/ops/scheduler_test.go`, `internal-test-service/internal/ops/report_test.go`

**Interfaces:**

- `ops.Scheduler.Run(ctx)` starts one process-local scheduler; it must not run duplicate jobs after restart.
- `ops.Reporter.Daily(ctx, date time.Time) (Report, error)` returns check-ins, grants, usage, provider cost, budget occupancy, P95 latency, errors, pending jobs, and read-only reasons without secrets.
- `ops.Alerter.Send(ctx, Alert) error` posts a redacted Feishu message through a webhook file; failures are persisted as retryable jobs.

- [x] **Step 1: Write failing scheduler tests.** Use a fake clock to verify one-minute usage sync, five-minute reconciliation, hourly invitation/count/health checks, Shanghai date transition without clearing balances, daily report generation, and retry backoff.

- [x] **Step 2: Run the tests and verify they fail.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/ops -count=1`

Expected: FAIL because scheduler, report, and alert components are absent.

- [x] **Step 3: Implement scheduler and durable jobs.** Persist job keys and status in SQLite, claim jobs transactionally, retry only known-safe idempotent operations, and leave uncertain writes pending until provider history resolves them.

- [x] **Step 4: Implement reports and alerts.** Render a Chinese Markdown/text report with counts and monetary values only. Redact user emails, invitation codes, JWTs, Admin API keys, upstream keys, and full URLs. Alert on budget threshold, capacity/errors, registration full, read-only mode, failed jobs, and balance drift.

- [x] **Step 5: Run tests; commit deferred.** Tests passed; commit is deferred for the shared-worktree reason.

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./internal/ops -race -count=1`

Expected: PASS with deterministic fake-clock assertions.

Commit: `git add internal-test-service && git commit -m "feat: add unattended internal test operations"`

---

### Task 7: Integrate Compose, Caddy, and Secret Mounts

**Files:**

- Create: `internal-test-service/cmd/internal-test-service/main.go`, `internal-test-service/internal/app/app.go`, `infra/Dockerfile.internal-test`
- Modify: `infra/compose.yaml`, `infra/Caddyfile`, `infra/.env.example`, `tests/infra/validate-baseline.sh`
- Test: `tests/internal_test/validate_internal_test_contract.sh`

**Interfaces:**

- Container listens only on Docker network port `8090`, runs as non-root with read-only root filesystem, `no-new-privileges`, no extra capabilities, and writable only at `/var/lib/internal-test`.
- Environment variables are `D04_MAX_USERS`, `D04_TOTAL_BUDGET_USD`, `D04_TIMEZONE`, `D04_SUB2API_URL`, `D04_ADMIN_API_KEY_FILE`, `D04_FEISHU_WEBHOOK_FILE`, and `D04_MODE`.
- Caddy routes `/internal-test/*` and `POST /api/v1/auth/register` to `internal-test-service:8090`; all other API and web paths route to `sub2api:8080`.

- [x] **Step 1: Write the infrastructure contract test.** Assert pinned Go runtime image, no host ports on the service, read-only key/webhook mounts, healthcheck, restart policy, Caddy registration-gate matcher, blocked OAuth account-creation matchers, and preserved Caddy 80/443 ownership.

- [x] **Step 2: Run the contract and verify it fails.**

Run: `bash tests/internal_test/validate_internal_test_contract.sh`

Expected: FAIL because the service, Dockerfile, and Caddy routes do not exist.

- [x] **Step 3: Add the container and secret mounts.** Use a multi-stage pure-Go build, pin the base image digest, create an unprivileged UID, mount SQLite data read-write and Admin/Feishu files read-only, and add a healthcheck to `/healthz`.

- [x] **Step 4: Add Caddy route ordering.** Match the registration POST before the catch-all Sub2API proxy, match every OAuth account-creation endpoint before the catch-all and return `403 D04_REGISTRATION_METHOD_DISABLED`, and preserve `flush_interval -1` for normal gateway traffic.

- [x] **Step 5: Run Compose and infrastructure checks.**

Run: `docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet && bash tests/infra/validate-baseline.sh && bash tests/internal_test/validate_internal_test_contract.sh`

Expected: all commands exit `0`; only Caddy publishes host ports.

- [ ] **Step 6: Commit infrastructure integration.** Deferred for the same shared-worktree reason.

Run: `git add internal-test-service infra tests/infra/validate-baseline.sh tests/internal_test && git commit -m "feat: integrate internal test service with compose and caddy"`

---

### Task 8: Run Local End-to-End Tests and Prepare Read-Only Deployment

**Files:**

- Create: `internal-test-service/internal/e2e/e2e_test.go`
- Modify: `docs/runbooks/operations-and-incident-response.md`, `docs/project/current-state.md`, `docs/project/llm-handoff.md`
- Test: all `internal-test-service/**/*_test.go`, `tests/infra/validate-baseline.sh`, `tests/internal_test/validate_internal_test_contract.sh`

**Interfaces:**

- End-to-end test uses only the fake Sub2API and temporary SQLite; it must never read `infra/.env` or connect to production.
- Read-only deployment mode accepts JWT/user/usage/balance reads but rejects all Admin balance writes and invitation issuance.
- Production write mode remains disabled until formal domain/TLS and isolated low-credit acceptance are separately approved.

- [x] **Step 1: Write the end-to-end scenario.** Exercise: bootstrap invitation, two parallel invitations, 15 successful registrations, rejected 16th registration, full-page message, daily check-in across Shanghai midnight, duplicate check-in, three invitees producing one reward each, duplicate usage polling, budget reservation, Admin timeout-after-commit, service restart, balance drift, read-only mode, and closure invalidating unused codes.

- [x] **Step 2: Run the complete local suite.**

Run: `docker run --rm -v "$PWD/internal-test-service:/src" -w /src golang:1.24-alpine go test ./... -race -count=1`

Expected: PASS with no race detector findings.

- [x] **Step 3: Run repository contracts.**

Run: `bash tests/infra/validate-baseline.sh && bash tests/internal_test/validate_internal_test_contract.sh && ruby -Itests tests/upstream_benchmarks/upstream_benchmark_test.rb`

Expected: all commands exit `0`; no secret scan reports.

- [x] **Step 4: Build the read-only image locally.**

Run: `docker build --file infra/Dockerfile.internal-test --tag sub2api-internal-test:local internal-test-service`

Expected: image builds without network-time provider access and contains no source credentials.

- [x] **Step 5: Update durable handoff state.** Record the local test counts, image digest if available, and that production writes remain disabled. Do not record domain secrets, server IP, JWTs, Admin API keys, invitation codes, or webhook URLs.

- [ ] **Step 6: Commit the validated plan execution baseline.** Deferred for the same shared-worktree reason.

Run: `git add internal-test-service infra docs/runbooks/operations-and-incident-response.md docs/project/current-state.md docs/project/llm-handoff.md tests && git commit -m "test: validate internal test automation baseline"`

---

## Production Handoff Gate

Do not execute production writes until all of the following are separately confirmed:

1. `xingqiaolab.top` registration, DNS, Caddy TLS, login, join page, and API health pass.
2. Read-only service mode has run through one scheduler interval without errors.
3. One isolated D04 user completes a low-credit check-in and duplicate-check-in test.
4. One isolated referred user completes a first successful request and exactly one `$5` reward is observed.
5. Admin timeout-after-commit and balance reconciliation have been observed against the fake provider and documented.
6. The real Admin API key and Feishu webhook are installed only on the server with mode `600`; neither appears in logs or project files.
7. User explicitly approves enabling D04 writes and then enabling the first external invitation.

Until this gate passes, the service remains read-only, the public registration switch remains closed, and the production Sub2API balance is not modified.
