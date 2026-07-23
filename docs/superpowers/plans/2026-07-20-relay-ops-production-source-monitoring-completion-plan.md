# relay-ops Production Source Monitoring Completion Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the missing production-upstream monitoring path so relay-ops continuously watches every customer-visible production group, records upstream price/multiplier changes, exposes actionable evidence in `/ops`, and can deliver deduplicated Feishu/Agent alerts without duplicating Sub2API native monitoring.

**Architecture:** Keep Sub2API as the source of truth for groups, channels, Ops, Usage, and `/monitor`. Add a general upstream registry in relay-ops: production sources use Sub2API native monitor IDs and public price/usage URLs without a second probe key; candidates retain dedicated low-quota probe keys and six-hour bounded probes. A shared collector stores append-only normalized snapshots and semantic diffs, while the operations view reads materialized source, incident, and Agent evidence from relay-ops PostgreSQL.

**Tech Stack:** Go 1.24, PostgreSQL 18, pgx/v5, net/http, html/template, existing pricing extractor, incident state machine, Feishu client, OpenAI-compatible Agent client, Docker Compose, Caddy, Ruby V2 evaluator, Playwright.

## Global Constraints

- Do not read, print, or persist `infra/.env`, API keys, passwords, JWTs, cookies, Admin API keys, Feishu webhooks, or Agent credentials.
- Do not read or write Sub2API PostgreSQL. All Sub2API access stays in the existing GET-only HTTP adapter.
- Production sources never receive a relay-ops probe key; their quality samples remain owned by Sub2API Channel Monitor.
- Candidate probes remain exactly one sync plus one SSE every six hours when `RELAY_OPS_MODE=probe`; `read_only` sends no paid candidate requests.
- No automatic route, price, balance, recharge, key, or account changes.
- Only semantic price/model/multiplier changes, incident transitions, recovery, new evidence above threshold, and daily summaries notify. Repeated unchanged observations stay silent.
- `/pricing` remains anonymous and customer-safe. `/ops` remains administrator-only and may show upstream names, acquisition multipliers, incidents, and Agent analyses.
- Server deployment starts in `read_only`. Agent and Feishu secrets remain optional and independent.

---

### Task 1: General Production Upstream Registry

**Files:**
- Create: `relay-ops-service/internal/upstreams/service.go`
- Create: `relay-ops-service/internal/upstreams/service_test.go`
- Modify: `relay-ops-service/internal/store/migrations/001_init.sql`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`

**Interfaces:**
- Produces: `upstreams.Source`, `upstreams.SourceInput`, `upstreams.Service.CreateProduction`, `upstreams.Service.List`, `upstreams.Service.Disable`.
- Production input contains `name`, HTTPS `base_url`, HTTPS `pricing_url`, optional HTTPS `usage_url` and `performance_url`, one or more Sub2API `group_ids`, and optional native `monitor_id`.
- Candidate registration continues through `candidates.Service` and continues to require a dedicated probe-key file.

- [x] Write and pass tests proving production sources accept no probe key, reject private/non-HTTPS URLs, require at least one customer-visible group ID, persist group links, expose no secret field, and remain unique by name/base URL.
- [x] Run focused upstream/store tests and the PostgreSQL suite; production sources and existing candidates coexist without changing candidate semantics.
- [x] Add `relay_ops.upstream_public_groups(upstream_id, group_id)` and repository methods that transact source, links, and audit event together.
- [x] Implement service validation by reusing the existing safe remote-URL policy and administrator identity requirements.

### Task 2: Shared Price/Multiplier Collector and Scheduling

**Files:**
- Create: `relay-ops-service/internal/collection/pricing.go`
- Create: `relay-ops-service/internal/collection/pricing_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/e2e_test.go`
- Modify: `relay-ops-service/internal/scheduler/scheduler_test.go`

**Interfaces:**
- Produces: `collection.PricingCollector.Run(ctx, source, allowPaidProbe)`.
- Consumes: append-only pricing snapshots, `pricing.Diff`, incident state machine, Agent service, Feishu client, and optional candidate `ProbeRunner`.
- Production collection runs every five minutes after native Sub2API synchronization; candidate collection runs every six hours. Failure of one source is recorded and does not prevent other sources from running.

- [x] Write and pass tests for production five-minute collection, candidate six-hour collection, unchanged-hash silence, multiplier/price/model semantic diffs, no production probe calls, and isolated per-source errors.
- [x] Extract the existing candidate price logic into the shared collector; emit a stable incident key per source/change class and persist every changed normalized snapshot.
- [x] Wire active production sources into `Scheduler.Production` while preserving readiness after a successful native Sub2API read.
- [x] Rerun focused tests and PostgreSQL E2E; `0.07x -> 0.10x` notifies once, repeated content stays silent, and production performs zero API probes.

### Task 3: Actionable Administrator Operations View

**Files:**
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/sources.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/app.css`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/http/sources_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`

**Interfaces:**
- Adds administrator APIs `GET/POST /relay-ops/api/upstreams` and `POST /relay-ops/api/upstreams/{id}/disable`.
- Extends `OpsView` with production sources, latest pricing observation, semantic change summary, native monitor reference, incidents, and Agent reports.
- Keeps candidate controls and public `/pricing` behavior unchanged.

- [x] Write and pass handler/source tests for source registration, CSRF/origin checks, customer-visible group links, source status, latest price change, incident/Agent evidence, stale state, and secret-field absence.
- [x] Implement repository projections and administrator handlers; render compact production/candidate tables and event rows without exposing keys, raw HTML, usage sessions, or response bodies.
- [x] Run handler tests and render the administrator page; confirm no overlap, clipping, blank sections, or authentication leakage.

### Task 4: Feishu and Read-Only Agent Acceptance Path

**Files:**
- Create: `relay-ops-service/internal/notify/delivery.go`
- Create: `relay-ops-service/internal/notify/delivery_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `docs/runbooks/relay-ops-monitoring.md`

**Interfaces:**
- Produces durable `notification_deliveries` records keyed by incident transition/evidence hash.
- Sends Feishu only after the delivery row is reserved; retries failed deliveries without duplicating successful ones.
- Agent analysis remains one analysis per incident evidence event and receives only `relay-ops-incident-v1` structured input.

- [x] Write and pass tests for reserve/send/mark-delivered, duplicate suppression, failed-delivery retry, no-secret payload, deterministic fallback, and zero-cost notifier paths.
- [x] Implement transactional delivery reservation and status updates, then wire semantic-change notifications through it.
- [x] Inspect captured notifier/Agent contracts for secrets and PII; real credentials remain an external gate.

### Task 5: Read-Only Server Deployment and Current Source Enrollment

**Files:**
- Modify: `infra/compose.yaml`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Create: `docs/superpowers/reports/2026-07-20-relay-ops-production-source-monitoring-verification.md`

**Interfaces:**
- Deploys a pinned `sub2api-relay-ops:<revision>` image with the existing non-root/read-only/no-host-port contract.
- Enrolls Neko as production source for `GPT-Pro` and Wawazz as production source for `GPT-Plus` through relay-ops administrator HTTP APIs.
- Keeps `RELAY_OPS_MODE=read_only` until Feishu is working and a candidate low-quota Key has been installed.

- [x] Run Go tests, PostgreSQL E2E, `go vet`, Compose/Caddy contracts, `git diff --check`, and secret-pattern checks; focused and full container test suites pass.
- [x] Build and smoke the pinned relay-ops image as UID `10002` with a read-only root filesystem.
- [x] Deploy only relay-ops and the required secret-directory mount; Sub2API, Redis, and PostgreSQL were not restarted.
- [x] Enroll Neko/Wawazz public sources without storing production keys, run a five-minute cycle, and verify `/ops` source evidence while `/pricing` and native `/monitor` remain unchanged.
- [x] Record absent Feishu/Agent/session secrets as an external gate without weakening the implemented path.
- [x] Update state, handoff, runbook, and implementation status with image digest, health checks, source enrollment, and remaining gates.

### Task 6: Zero-Cost Alert Acceptance Endpoint

**Files:**
- Create: `relay-ops-service/internal/acceptance/service.go`
- Create: `relay-ops-service/internal/acceptance/service_test.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `docs/runbooks/relay-ops-monitoring.md`
- Modify: `docs/project/current-state.md`, `docs/project/llm-handoff.md`

- [x] Add an admin/origin-protected POST endpoint that accepts only `{}` and emits a fixed redacted incident.
- [x] Reuse the existing state machine, Agent contract, and durable Feishu sender; repeated calls do not repeat analysis or notifications.
- [x] Prove optional Agent/Feishu absence and delivery failures degrade without upstream access or production writes.
- [x] Run `go test ./...` in the pinned Go 1.24 container and deploy only the AMD64 relay-ops container; real authenticated trigger and webhook acceptance remain the next external gate.

### Task 7: Administrator Acceptance Control

**Files:**
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/static/app.css`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Modify: `docs/runbooks/relay-ops-monitoring.md`

**Interfaces:**
- Consumes: the existing `POST /relay-ops/api/acceptance/synthetic` endpoint and browser-held Sub2API `auth_token`.
- Produces: an administrator-only button with `idle`, `running`, `delivered`, `degraded`, and `failed` feedback without accepting event content or exposing the token.

- [x] Add a handler/static-contract test requiring the button, live result region, endpoint path, same-origin credentials, Bearer header, and no token logging or request-controlled event fields.
- [x] Run `go test ./internal/http` and `bash tests/relay_ops/validate_relay_ops_contract.sh`; confirm the new assertions fail before implementation.
- [x] Add an unframed “告警链路验收” section to `/ops` with one command button, a concise safety boundary, and a live result region. The button must remain disabled while a request is running.
- [x] Call the endpoint with JSON `{}`, `credentials: same-origin`, and the existing authorization helper. Redirect 401/403 to `/login?redirect=%2Fops`; render `external_upstream`, `agent_status`, and `notification` in plain Chinese without inserting HTML.
- [x] Run HTTP tests, the relay-ops contract, `go test ./...`, `go vet ./...`, and `git diff --check`.
- [x] Render `/ops` at desktop and mobile widths, verify keyboard focus, wrapping, no overlap, and no visible “内测” wording.
- [x] Build an AMD64 image on the production server, update only the relay-ops image tag, run Compose config validation, recreate only relay-ops, and verify healthy state plus unauthenticated `401` for the acceptance endpoint.
- [x] Update the runbook, current state, handoff, and verification report with the deployed digest and the remaining Feishu/Agent/session credential gates.

## Completion Gate

This increment is complete when current production sources are enrolled and their public price/multiplier pages are collected on the server, unchanged observations are silent, semantic changes create durable incident evidence, `/ops` displays source/incident/Agent state, production quality still comes only from Sub2API native monitor, all tests pass, and missing external credentials are explicitly separated from implemented functionality. A real candidate probe remains a separate operational acceptance only when a candidate and its dedicated low-quota Key actually exist.
