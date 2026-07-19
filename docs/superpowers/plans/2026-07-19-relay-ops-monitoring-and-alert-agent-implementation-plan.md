# relay-ops Monitoring and Alert Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and deploy a read-only `relay-ops` server service that reuses Sub2API native monitoring and operations data, tracks production price/multiplier changes, probes candidate upstreams every six hours, compares evidence, publishes a public pricing view, and sends deduplicated Feishu/Agent analysis.

**Architecture:** Add one Go 1.24 service with an isolated PostgreSQL `relay_ops` schema. It reads Sub2API through existing HTTP APIs, invokes the existing Ruby V2 evaluator only through a bounded candidate-watch/qualification interface, stores candidate and change evidence, and serves `/pricing` plus an admin-only `/ops`; Sub2API keeps `/monitor`, production monitor history, Ops, Usage, and channel pricing ownership. The service starts in `read_only` mode, has no Sub2API write methods, and never passes secrets or response bodies to the Agent.

**Tech Stack:** Go 1.24, `net/http`, `html/template`, `embed`, `pgx/v5`, PostgreSQL 18, Ruby 3.3 for the existing V2 runner, `goquery` for structured HTML extraction, Docker Compose, Caddy 2.10.2, Feishu webhook JSON, OpenAI-compatible Agent API, Ruby/Go/Bash tests, and Playwright for browser verification.

## Global Constraints

- Sub2API `v0.1.161` remains the source of truth for production Channel Monitor history, `/monitor`, Ops, Usage, groups, and channel pricing.
- `relay-ops` must not read or write Sub2API PostgreSQL and must not implement Sub2API price, route, balance, or channel writes.
- Production/public monitor keys remain encrypted in Sub2API; `relay-ops` stores only native monitor IDs.
- Candidate probe keys are dedicated low-quota upstream API keys mounted from server-only secret files; only secret references, fingerprints, and last four characters enter PostgreSQL.
- Production price/multiplier pages are fetched every 5 minutes; unchanged content hashes do not trigger parsing or notifications.
- Every candidate upstream runs one bounded collection cycle every 6 hours. Page changes do not insert extra paid probes.
- Full V2 qualification runs only when an administrator requests promotion or when a public group has a major model/protocol change.
- Candidate-watch reuses V2 discovery, sync, SSE, TTFT, latency, usage, redaction, and HTTP contracts but does not run concurrency/RPM ladders.
- `/pricing` is public and contains only customer-visible groups/models/prices; `/monitor` stays inside Sub2API; `/ops` requires a Sub2API administrator session.
- Agent input excludes API keys, cookies, passwords, JWTs, prompts, response bodies, user email/IP, raw HTML, and arbitrary URLs.
- Agent, Feishu, billing-session, candidate-probe, and Sub2API admin credentials are separate secret files.
- No automatic upstream switch, price change, route change, recharge, refund, balance adjustment, destructive cleanup, or production restart.
- Default deployment mode is `RELAY_OPS_MODE=read_only`; real paid probes require the explicit `probe` mode after low-cost acceptance.
- All timestamps are stored in UTC; schedules and human reports use `Asia/Shanghai`.

---

## File Map

**Create:**

- `relay-ops-service/go.mod`, `relay-ops-service/go.sum`
- `relay-ops-service/cmd/relay-ops/main.go`
- `relay-ops-service/internal/config/config.go`
- `relay-ops-service/internal/domain/model.go`
- `relay-ops-service/internal/store/postgres.go`, `relay-ops-service/internal/store/migrations/001_init.sql`
- `relay-ops-service/internal/sub2api/client.go`, `relay-ops-service/internal/sub2api/types.go`
- `relay-ops-service/internal/candidates/service.go`
- `relay-ops-service/internal/pricing/fetcher.go`, `relay-ops-service/internal/pricing/extractor.go`
- `relay-ops-service/internal/probes/v2.go`
- `relay-ops-service/internal/billing/session.go`, `relay-ops-service/internal/billing/cost.go`
- `relay-ops-service/internal/compare/service.go`
- `relay-ops-service/internal/incidents/state_machine.go`
- `relay-ops-service/internal/notify/feishu.go`
- `relay-ops-service/internal/agent/client.go`, `relay-ops-service/internal/agent/contract.go`
- `relay-ops-service/internal/scheduler/scheduler.go`
- `relay-ops-service/internal/http/server.go`, `relay-ops-service/internal/http/templates/*.html`, `relay-ops-service/internal/http/static/app.css`
- `relay-ops-service/internal/testsupport/fake_sub2api.go`, `relay-ops-service/internal/testsupport/fake_upstream.go`
- focused `*_test.go` files beside each package
- `config/upstream-benchmarks/candidate-watch-v2.yaml`
- `infra/Dockerfile.relay-ops`
- `tests/relay_ops/validate_relay_ops_contract.sh`
- `docs/runbooks/relay-ops-monitoring.md`

**Modify:**

- `ops/upstream-benchmark-v2.rb`: add a bounded `watch` command that reuses existing V2 components.
- `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`: prove watch-mode discovery/sync/SSE/redaction and absence of capacity ladders.
- `infra/compose.yaml`: add `relay-ops`, a schema bootstrap path, secret mounts, healthcheck, and no host port.
- `infra/Caddyfile`: route `/pricing`, `/ops`, and `/relay-ops/api/*`; keep `/monitor` and all unspecified routes on Sub2API.
- `infra/.env.example`: add non-secret relay-ops settings and secret-file references.
- `tests/infra/validate-baseline.sh`: include relay-ops isolation assertions.
- `docs/project/current-state.md`, `docs/project/llm-handoff.md`: update only after verified milestones.

---

### Task 1: Service Skeleton, Configuration, and PostgreSQL Ledger

**Files:** Create the module, config, domain, migration, store, main package, and focused tests listed above.

**Interfaces:**

```go
type MicroUSD int64
type MultiplierBPS int64
type UpstreamID int64
type AdminActor struct { UserID int64 }

type Config struct {
    Mode string
    ListenAddress string
    Timezone *time.Location
    DatabaseURLFile string
    Sub2APIBaseURL string
    Sub2APIAdminKeyFile string
    FeishuWebhookFile string
    AgentBaseURL string
    AgentAPIKeyFile string
    AgentModel string
    ProductionPageInterval time.Duration
    CandidateInterval time.Duration
}

func Load(env func(string) string) (Config, error)
func Open(ctx context.Context, databaseURLFile string) (*Store, error)
func (s *Store) Migrate(ctx context.Context) error
```

- [x] Write failing tests for exact modes (`read_only`, `probe`, `closed`), fixed 5-minute and 6-hour intervals, secret-file permissions, UTC persistence, migrations, unique candidate names/base URLs, append-only snapshots, and incident deduplication.
- [x] Run `docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24-alpine go test ./internal/config ./internal/domain ./internal/store -count=1`; expect failure because the module and packages do not exist.
- [x] Implement the module with `github.com/jackc/pgx/v5@v5.7.6`, decimal money as integer micro-USD, embedded migrations, connection timeouts, and schema-qualified tables: `upstreams`, `secret_refs`, `public_groups`, `pricing_snapshots`, `probe_runs`, `metric_refs`, `comparison_windows`, `cost_observations`, `candidate_comparisons`, `incidents`, `notification_deliveries`, `auth_sessions`, `agent_analyses`, and `audit_events`.
- [x] Run the focused tests against `postgres:18-alpine`; expect all Task 1 tests to pass and migrations to be idempotent.
- [x] Commit only Task 1 files with `git commit -m "feat: scaffold relay ops service"`.

### Task 2: Read-Only Sub2API Adapter and Native Metric References

**Files:** Create `internal/sub2api/*`, fake server support, and tests.

**Interfaces:**

```go
type Reader interface {
    ListChannels(context.Context) ([]Channel, error)
    ListGroups(context.Context) ([]Group, error)
    ListChannelMonitors(context.Context) ([]ChannelMonitor, error)
    GetChannelMonitorHistory(context.Context, int64, string, int) ([]MonitorHistory, error)
    GetOpsSnapshot(context.Context, OpsQuery) (OpsSnapshot, error)
    GetUsageStats(context.Context, UsageQuery) (UsageStats, error)
}

type MetricRef struct {
    SourceKind string
    ExternalID string
    WindowStart time.Time
    WindowEnd time.Time
    PayloadHash string
    SchemaVersion string
}
```

- [x] Write failing `httptest` cases for `/api/v1/admin/channels`, `/groups`, `/channel-monitors`, monitor history, `/ops/dashboard/snapshot-v2`, and `/usage/stats`; assert timeouts, 401/403 classification, response size caps, schema drift errors, and no response-body logging.
- [x] Run `go test ./internal/sub2api ./internal/testsupport -count=1`; expect missing package failures.
- [x] Implement a GET-only client that reads its admin key from a `0600`/`0640` secret file, sends `x-api-key`, redacts errors, normalizes existing Sub2API response wrappers, and exposes no mutation method.
- [x] Implement public-group synchronization: active customer-visible groups link to channel IDs and native monitor IDs; production raw histories are not copied, only `MetricRef` rows and comparison materializations are stored.
- [x] Run focused tests and the fake Sub2API contract suite; expect all calls to be GET and all secret/body leakage assertions to pass.
- [x] Commit with `git commit -m "feat: read native sub2api operations data"`.

### Task 3: Candidate Registry, Admin Authentication, and Secret References

**Files:** Create `internal/candidates/service.go`, candidate/admin HTTP handlers, and tests.

**Interfaces:**

```go
type CandidateInput struct {
    Name string
    BaseURL string
    PricingURL string
    UsageURL string
    PerformanceURL string
    ProbeKeyFile string
}

func (s *Service) Create(ctx context.Context, actor AdminActor, in CandidateInput) (Upstream, error)
func (s *Service) Disable(ctx context.Context, actor AdminActor, upstreamID int64) error
func (a *Authenticator) RequireAdmin(next http.Handler) http.Handler
```

- [x] Write failing tests for minimum input, HTTPS-only URLs, SSRF/private-host rejection, canonical Base URL uniqueness, probe-key fingerprint/last-four storage, absence of key plaintext in SQL/log/API, and Sub2API admin-session verification.
- [x] Run `go test ./internal/candidates ./internal/http -run 'Candidate|Admin' -count=1`; expect failures.
- [x] Implement candidate create/list/disable APIs. Read the probe key only when a probe starts; persist its server file reference, fingerprint, and last four characters. Write audit events for every administrator action.
- [x] Implement `/ops` authentication by forwarding the presented bearer token to Sub2API `/api/v1/auth/me` and requiring an administrator role; do not create a second password database.
- [x] Run focused tests; expect unauthorized users, private URLs, writable secret files, and duplicate candidates to be rejected.
- [x] Commit with `git commit -m "feat: register candidate upstreams safely"`.

### Task 4: Bounded V2 Candidate Watch and Qualification Orchestration

**Files:** Modify the Ruby V2 CLI/tests; create `candidate-watch-v2.yaml`, Go executor, and tests.

**Interfaces:**

```ruby
UpstreamBenchmarkV2::CandidateWatchRunner.new(client:, profile:).run(channel_id:)
```

```go
type V2Runner interface {
    Watch(context.Context, Candidate) (ProbeRun, error)
    Qualify(context.Context, Candidate, QualificationRequest) (QualificationResult, error)
}
```

- [x] Add failing Ruby tests proving `watch` discovers all models, chooses configured representative text models, performs one sync and one SSE per chosen model, records TTFT/total latency/usage/`[DONE]`, redacts secrets, and never invokes concurrency/RPM probes.
- [x] Run `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`; expect watch-command failures.
- [x] Implement `watch --channel ID --key-env NAME --profile PATH` using existing `Registry`, `HttpClient`, `ModelCatalog`, request metrics, and redactor. The key remains in an environment variable created from the mounted secret file and never appears in argv or output.
- [x] Add Go executor tests for deadline, output cap, malformed JSON, non-zero exit, expense estimate, and environment scrubbing; implement `exec.CommandContext` with a fixed Ruby/script path and allowlisted environment.
- [x] Run Ruby and Go tests; expect watch mode to pass with no capacity fields and full qualification to remain backward compatible.
- [x] Commit with `git commit -m "feat: add bounded candidate watch mode"`.

### Task 5: Structured Price and Multiplier Collection

**Files:** Create `internal/pricing/*` and tests.

**Interfaces:**

```go
type FetchResult struct { URL string; FetchedAt time.Time; ContentHash string; ContentType string; Body []byte }
type Evidence struct { Models []ModelPrice; AdvertisedMultiplier *domain.MultiplierBPS; SourceURL string; Confidence string }
type Extractor interface { Match(FetchResult) bool; Extract(FetchResult) (Evidence, error) }
```

- [x] Write failing tests with Sub2API/NewAPI/generic HTML and JSON fixtures for model prices, tier prices, `0.1x`/`0.05x` multiplier extraction, gzip/body limits, redirect/SSRF rejection, unchanged hashes, parser failure, and stale snapshots.
- [x] Run `go test ./internal/pricing -count=1`; expect missing implementation failures.
- [x] Implement HTTP fetching with DNS/IP validation before every redirect, 10-second timeout, 2 MiB cap, structured JSON parsing, `goquery` DOM text extraction, adapter-specific selectors, normalized evidence JSON, and append-only snapshots.
- [x] Diff consecutive snapshots into added/removed models, price changes, multiplier changes, and unparseable-field events. A hash change without a semantic diff remains informational and sends no alert.
- [x] Run focused tests; expect exact `0.07 -> 0.10` and model-price fixtures to produce semantic diffs without storing HTML.
- [x] Commit with `git commit -m "feat: detect upstream pricing changes"`.

### Task 6: Upstream Usage Sessions and Auxiliary Cost Evidence

**Files:** Create `internal/billing/session.go`, `internal/billing/cost.go`, and tests; extend candidate configuration with optional auth-session metadata.

**Interfaces:**

```go
type SessionConfig struct {
    UpstreamID domain.UpstreamID
    UsageURL string
    LoginURL string
    AuthMode string
    SecretRef string
}

func (s *SessionReader) ReadUsage(ctx context.Context, cfg SessionConfig) (UsageEvidence, error)
func EstimateEffectiveMultiplier(standard, actual domain.MicroUSD) (domain.MultiplierBPS, error)
```

- [x] Write failing tests for bearer/cookie secret files, permission checks, redirects/SSRF, 401/session-expired classification, usage-page schema changes, optional standard/actual cost extraction, login URL retention, and absence of passwords/cookies in SQL, logs, Agent input, or Feishu.
- [x] Run `go test ./internal/billing -count=1`; expect missing implementation failures.
- [x] Implement read-only usage collection with a server-side cookie/token file populated during initial administrator authorization. Persist only session status, expiry/last-success timestamps, failure reason, and secret reference; do not store account passwords.
- [x] Store valid standard/actual cost as auxiliary `cost_observations`. Missing or stale evidence must display “按公开定价估算” and must not block routing, probing, or public availability.
- [x] Emit one session-expired incident with the exact upstream login link, retry once after 401, suppress repeated reminders for 24 hours, and recover automatically after the secret file is refreshed.
- [x] Run focused tests; expect an expired session to pause only cost reconciliation while quality and public-price collection continue.
- [x] Commit with `git commit -m "feat: track upstream usage sessions"`.

### Task 7: Comparison Windows and Incident State Machine

**Files:** Create `internal/compare/*`, `internal/incidents/*`, and tests.

**Interfaces:**

```go
func Materialize(production NativeMetrics, candidate ProbeMetrics) (ComparisonWindow, error)
func Classify(window ComparisonWindow, policy Policy) CandidateComparison
func (m *Machine) Observe(ctx context.Context, observation Observation) (Transition, error)
```

- [x] Write failing tests for separate real/native-synthetic/candidate sources, incompatible models/windows, TTFT P95 improvement >=20%, cost improvement >=10%, no service-quality regression, P0/P1/P2 transitions, consecutive-window confirmation, duplicate suppression, escalation, recovery, and evidence changes.
- [x] Run `go test ./internal/compare ./internal/incidents -count=1`; expect failures.
- [x] Implement deterministic comparison labels and an incident state machine with stable keys. Never average real and synthetic traffic into one rate; persist the metric schema version and evidence references.
- [x] Run focused tests; expect identical repeated observations to create no new delivery and recovered events to link to the original incident.
- [x] Commit with `git commit -m "feat: compare upstreams and deduplicate incidents"`.

### Task 8: Feishu Notifications and Read-Only Alert Agent

**Files:** Create `internal/notify/*`, `internal/agent/*`, and tests.

**Interfaces:**

```go
func RenderFeishu(event IncidentView) FeishuMessage
func (c *AgentClient) Analyze(ctx context.Context, input IncidentContractV1) (AgentAnalysis, error)
func ValidateAgentOutput([]byte) (AgentAnalysis, error)
```

- [x] Write failing tests for the five message sections, notification triggers only on confirm/escalate/recover/new evidence, daily summary, auth-session-expired login link, webhook/LLM failure fallback, Agent JSON schema, secret/PII rejection, one-analysis-per-event, token/output caps, and deterministic fallback text.
- [x] Run `go test ./internal/notify ./internal/agent -count=1`; expect failures.
- [x] Implement Feishu webhook delivery from a separate secret file. Messages state what ran, results, change versus baseline, required attention, and internal/native links; normal repeated probes do not notify.
- [x] Implement an OpenAI-compatible Agent client that receives only the versioned incident contract, has no tool for external requests or secrets, validates JSON output, and falls back to the deterministic message when unavailable.
- [x] Run focused tests and scan captured requests; expect no key, cookie, prompt, response body, email, IP, or raw HTML.
- [x] Commit with `git commit -m "feat: add relay ops alerts and analysis"`.

### Task 9: Scheduler and Application Assembly

**Files:** Create `internal/scheduler/scheduler.go`, app assembly, health/readiness, and tests.

**Interfaces:**

```go
func (s *Scheduler) Run(context.Context) error
func (s *Scheduler) RunProductionCollection(context.Context) error
func (s *Scheduler) RunCandidateCycle(context.Context, int64) error
func New(ctx context.Context, cfg config.Config) (*App, error)
```

- [ ] Write failing fake-clock tests for 5-minute production pages, exactly 6-hour candidate cycles, no paid probes in `read_only`, one-cycle locks, restart-safe due times, no extra probe on page diff, daily report time, and isolated per-upstream failures.
- [ ] Run `go test ./internal/scheduler ./internal/app -count=1`; expect failures.
- [ ] Implement database-backed due times and PostgreSQL advisory locks. `read_only` fetches Sub2API/native metrics and public pages but skips candidate API calls; `probe` enables bounded V2 watch; `closed` serves historical pages and health only.
- [ ] Assemble store, Sub2API reader, candidates, pricing, probe runner, comparison, incidents, Feishu, Agent, scheduler, and HTTP server. Health checks process liveness; readiness checks PostgreSQL and last successful Sub2API read without requiring Agent/Feishu.
- [ ] Run scheduler/app tests with race detection; expect no duplicate cycles under concurrent scheduler starts.
- [ ] Commit with `git commit -m "feat: schedule relay ops collection"`.

### Task 10: Public Pricing and Administrator Operations UI

**Files:** Create embedded templates/static CSS, HTTP handlers, and browser tests.

**Interfaces:**

```text
GET  /healthz
GET  /readyz
GET  /pricing
GET  /ops
GET  /relay-ops/api/candidates
POST /relay-ops/api/candidates
POST /relay-ops/api/candidates/:id/disable
GET  /relay-ops/api/incidents
GET  /relay-ops/api/comparisons
```

- [x] Write failing handler tests for anonymous `/pricing`, customer-visible filtering, tier/model prices, no upstream/cost leakage, admin-only `/ops`, CSRF/origin checks, candidate form errors, empty/loading/stale states, and no `/performance` route.
- [x] Run `go test ./internal/http -count=1`; expect failures.
- [x] Implement `/pricing` as a dense searchable table sourced from Sub2API channel pricing snapshots. Implement `/ops` as a quiet operations workspace with native Sub2API links, current public groups, price diffs, candidate status, comparisons, incidents, auth status, and Agent reports; avoid duplicating native QPS/SLA/Usage dashboards.
- [x] Add stable responsive dimensions, accessible labels, clear status colors, and no nested cards. User-facing `/pricing` contains no operator terminology; `/ops` is optimized for scanning and repeated action.
- [x] Run handler tests, start the local stack, and use Playwright at desktop/mobile sizes to verify no overlap, clipped text, auth leakage, blank states, or broken links. Confirm Sub2API `/monitor` remains untouched.
- [x] Commit with `git commit -m "feat: serve relay pricing and operations views"`.

### Task 11: Hardened Container and Reverse-Proxy Integration

**Files:** Create Dockerfile/contract test; modify Compose, Caddy, env example, and infrastructure tests.

**Interfaces:**

```text
relay-ops:8100 (internal expose only)
/pricing -> relay-ops:8100
/ops* -> relay-ops:8100
/relay-ops/api/* -> relay-ops:8100
/monitor -> Sub2API existing route
```

- [ ] Write a failing shell contract asserting no host port, read-only root filesystem, UID `10002`, dropped capabilities, `no-new-privileges`, bounded resources, PostgreSQL dependency, read-only secret mounts, healthcheck, log rotation, and exact Caddy routing.
- [ ] Run `bash tests/relay_ops/validate_relay_ops_contract.sh`; expect failure before integration.
- [ ] Build a multi-stage image with pinned Go/Ruby bases, a static Go binary, the V2 Ruby runner/profile, writable `/tmp` only, and no compiler in the runtime image. Add Compose settings with `RELAY_OPS_MODE=read_only` and schema/database secret files.
- [ ] Update Caddy so `/pricing`, `/ops*`, and `/relay-ops/api/*` route to relay-ops; all `/api/*`, `/monitor`, and other UI routes continue to Sub2API. Do not publish port 8100.
- [ ] Run Compose config, image build, non-root/read-only health smoke, existing infra/internal-test contracts, and the new relay-ops contract.
- [ ] Commit with `git commit -m "infra: deploy relay ops read-only service"`.

### Task 12: End-to-End Acceptance, Runbook, and Handoff

**Files:** Create runbook and E2E tests; update current state and handoff after evidence exists.

- [ ] Start PostgreSQL, fake Sub2API, fake candidate upstream, fake Feishu, and fake Agent. Register a candidate, advance the fake clock 6 hours, observe one sync/SSE cycle, create a multiplier diff, confirm one incident/notification/analysis, repeat unchanged data with no notification, and verify recovery.
- [ ] Run all Go tests with `-race`, Ruby V2 tests, `go vet`, `git diff --check`, image/Compose contracts, existing repository regressions, and secret scans. Record exact counts and failures in a verification report.
- [ ] Run Playwright screenshots for `/pricing` desktop/mobile and authenticated `/ops`; inspect screenshots and console/network errors.
- [ ] Deploy to the server in `read_only`, install secret files outside Git with restrictive permissions, run one native-data collection cycle, and confirm no paid candidate request occurs.
- [ ] After explicit low-cost acceptance, change only `RELAY_OPS_MODE=probe`, run one isolated candidate cycle, report its purpose/result/estimated cost in Feishu, then return to scheduled 6-hour operation.
- [ ] Update `docs/runbooks/relay-ops-monitoring.md`, `docs/project/current-state.md`, and `docs/project/llm-handoff.md` with verified production state, rollback steps, login-session recovery, and the boundary between Sub2API native pages and relay-ops.
- [ ] Commit verified documentation with `git commit -m "docs: hand off relay ops monitoring"`.

## Completion Gate

Implementation is complete only when all tasks are checked, all local and container tests pass, browser artifacts are inspected, the server is healthy in `read_only`, one approved low-cost candidate probe is reconciled, repeated unchanged probes are silent, Sub2API `/monitor` remains native, and the project handoff records the exact deployed version and rollback path.
