# Native P0/P1 Feishu Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Forward every newly produced Sub2API native P0/P1 ops alert to Feishu with deduplicated trigger, recovery/manual-close, silence-aware suppression, and recurring severity-specific reminders.

**Architecture:** relay-ops polls the existing Sub2API Admin API for event discovery and status refresh, persists a source-event ledger plus cursor in its own schema, and queries only `public.ops_alert_silences` through a separate least-privilege PostgreSQL connection. A new `nativeopsalerts` service maps source events to the existing incident and notification-delivery lifecycle, while one-shot delivery handles alerts that begin and end between polls.

**Tech Stack:** Go 1.24, pgx v5, PostgreSQL 18, Docker Compose, existing relay-ops incident/notification infrastructure, Sub2API Admin API, Feishu App/Webhook transports.

## Global Constraints

- Do not modify or rebuild the official Sub2API image or any file under `upstream/sub2api`.
- Only P0 and P1 enter the bridge; P2/P3 are audited as suppressed.
- Poll every minute.
- P0 reminders: 5 minutes, 15 minutes, then every 30 minutes.
- P1 reminders: 15 minutes, then every 60 minutes.
- Existing group user-impact alerts remain unchanged and use different family/source/dedup identities.
- Source events come only from the Admin API.
- The secondary database account may query only `public.ops_alert_silences`.
- Silence lookup failures are fail-closed for triggers and reminders.
- Do not log or render Admin keys, database URLs, Feishu secrets, raw HTTP bodies, or unknown event dimensions.
- All behavior changes use test-first red-green-refactor cycles.
- Each task is implemented by a fresh subagent and independently reviewed before the next task.

---

## File Map

**Create**

- `relay-ops-service/internal/nativeopssilence/postgres.go` — restricted silence lookup client.
- `relay-ops-service/internal/nativeopssilence/postgres_test.go` — silence matching and permission-safe error tests.
- `relay-ops-service/internal/nativeopsalerts/service.go` — polling and lifecycle orchestration.
- `relay-ops-service/internal/nativeopsalerts/service_test.go` — service lifecycle tests.
- `relay-ops-service/internal/nativeopsalerts/identity.go` — canonical dimensions and incident-key construction.
- `relay-ops-service/internal/nativeopsalerts/identity_test.go` — canonicalization/security tests.
- `relay-ops-service/internal/notify/native_ops_alert.go` — dedicated Feishu card renderer.
- `relay-ops-service/internal/notify/native_ops_alert_test.go` — card content, bounds, and redaction tests.
- `relay-ops-service/internal/store/migrations/007_native_ops_alert_bridge.sql` — source cursor and source-event ledger.
- `docs/superpowers/reports/2026-07-30-native-p0-p1-feishu-bridge-verification.md` — verification and deployment handoff.

**Modify**

- `relay-ops-service/internal/sub2api/types.go` — native alert DTOs and reader interface.
- `relay-ops-service/internal/sub2api/client.go` — list/detail Admin API calls.
- `relay-ops-service/internal/sub2api/client_test.go` — API pagination and strict validation tests.
- `relay-ops-service/internal/notificationpolicy/policy.go` — `native_ops_alerts_enabled` family.
- `relay-ops-service/internal/notificationpolicy/policy_test.go` — required policy field tests.
- `config/notification-policy.json` — production policy field.
- `relay-ops-service/internal/config/config.go` — read-only silence database secret path.
- `relay-ops-service/internal/config/config_test.go` — conditional configuration validation.
- `relay-ops-service/internal/store/postgres.go` — migration embed and source-ledger methods.
- `relay-ops-service/internal/store/postgres_test.go` — transaction/idempotency tests.
- `relay-ops-service/internal/incidents/state_machine.go` — explicit suppression/close controls if required by source lifecycle.
- `relay-ops-service/internal/incidents/state_machine_test.go` — cancellation/state tests.
- `relay-ops-service/internal/alerting/escalator.go` — infinite post-threshold reminder cadence and native reminder rendering.
- `relay-ops-service/internal/alerting/escalator_test.go` — P0/P1 schedule tests.
- `relay-ops-service/internal/store/postgres.go` — cancellation of escalation/retry when source is silenced or terminal.
- `relay-ops-service/internal/scheduler/scheduler.go` — one-minute native alert sync job.
- `relay-ops-service/internal/scheduler/scheduler_test.go` — cadence and closed-mode tests.
- `relay-ops-service/internal/app/app.go` — dependency wiring and secondary pool lifecycle.
- `relay-ops-service/internal/app/app_test.go` — wiring/configuration tests.
- `infra/compose.yaml` — secret mount and environment variable.
- `infra/env.example` — host secret path example, if present.
- `infra/production-deployment-runbook.md` — least-privilege role and rollout instructions, if present; otherwise update the existing relay-ops deployment runbook located during the task.

---

### Task 1: Add Strict Sub2API Native Alert API Reader

**Files:**

- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`

**Interfaces:**

- Produces:

```go
type OpsAlertReader interface {
    ListOpsAlertEvents(context.Context, OpsAlertEventCursor) ([]OpsAlertEvent, error)
    GetOpsAlertEvent(context.Context, int64) (OpsAlertEvent, error)
}

type OpsAlertEventCursor struct {
    Limit         int
    BeforeFiredAt *time.Time
    BeforeID      *int64
}

type OpsAlertEvent struct {
    ID             int64
    RuleID         int64
    Severity       string
    Status         string
    Title          string
    Description    string
    MetricValue    *float64
    ThresholdValue *float64
    Dimensions     map[string]any
    FiredAt        time.Time
    ResolvedAt     *time.Time
}
```

- Validation accepts only positive IDs, severity `P0|P1|P2|P3`, status `firing|resolved|manual_resolved`, finite numeric values, non-zero timestamps, and dimensions that fit the existing response-size ceiling.

- [ ] **Step 1: Write failing list/detail API tests**

Add tests that serve:

```json
{"data":[{"id":41,"rule_id":7,"severity":"P0","status":"firing","title":"错误率过高","description":"error_rate > 5","metric_value":80,"threshold_value":5,"dimensions":{"platform":"openai","group_id":16},"fired_at":"2026-07-30T13:48:00Z","resolved_at":null,"email_sent":false,"created_at":"2026-07-30T13:48:00Z"}]}
```

Assert `limit=500`, cursor parameters use RFC3339Nano, detail uses `/api/v1/admin/ops/alert-events/41`, and `email_sent` is ignored.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/sub2api -run 'OpsAlert' -count=1
```

Expected: compile failure because the new types and methods do not exist.

- [ ] **Step 3: Add malformed-response tests**

Cover zero IDs, invalid severity/status, NaN/Inf values, missing `fired_at`, mismatched cursor pair, detail ID mismatch, and response larger than 2 MiB. Each must return `IsSchemaMismatch(err)` or `IsResponseTooLarge(err)` as appropriate.

- [ ] **Step 4: Implement the minimal strict reader**

Use the existing `get`/`getStrict` infrastructure. Encode list query exactly:

```go
values := url.Values{"limit": {strconv.Itoa(limit)}}
if cursor.BeforeFiredAt != nil && cursor.BeforeID != nil {
    values.Set("before_fired_at", cursor.BeforeFiredAt.UTC().Format(time.RFC3339Nano))
    values.Set("before_id", strconv.FormatInt(*cursor.BeforeID, 10))
}
```

Default limit to 500 and reject values outside `1..500`.

- [ ] **Step 5: Run focused and package tests**

```bash
cd relay-ops-service
go test ./internal/sub2api -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add relay-ops-service/internal/sub2api
git commit -m "feat: read native Sub2API ops alerts"
```

---

### Task 2: Add Policy Flag and Least-Privilege Silence Reader

**Files:**

- Modify: `relay-ops-service/internal/notificationpolicy/policy.go`
- Modify: `relay-ops-service/internal/notificationpolicy/policy_test.go`
- Modify: `config/notification-policy.json`
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Create: `relay-ops-service/internal/nativeopssilence/postgres.go`
- Create: `relay-ops-service/internal/nativeopssilence/postgres_test.go`

**Interfaces:**

- Produces:

```go
const FamilyNativeOpsAlert notificationpolicy.Family = "native_ops_alert"

type Scope struct {
    RuleID   int64
    Platform string
    GroupID  *int64
    Region   *string
}

type Reader interface {
    IsSilenced(context.Context, Scope, time.Time) (bool, error)
    Close()
}

func Open(context.Context, string) (*PostgresReader, error)
```

- Adds `Config.Sub2APIAlertReadDatabaseURLFile string`.
- Requires `RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE` only when `native_ops_alerts_enabled=true`.

- [ ] **Step 1: Write failing policy tests**

Update the valid JSON fixture with:

```json
"native_ops_alerts_enabled": true
```

Add a test proving the field is required and `Policy.ShouldDeliver(FamilyNativeOpsAlert)` follows both family flag and delivery mode.

- [ ] **Step 2: Run policy tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/notificationpolicy -count=1
```

- [ ] **Step 3: Implement the policy field**

Add the field to `FeishuPolicy`, `rawFeishuPolicy`, required-field validation, `ApprovedFamilies`, `Enabled`, and the checked-in policy JSON.

- [ ] **Step 4: Write failing config tests**

Cases:

```go
// enabled + no secret path => error
// enabled + relative path => error
// enabled + 0600 non-empty file => accepted
// disabled + no path => accepted
```

The accepted path is returned unchanged in `Config.Sub2APIAlertReadDatabaseURLFile`.

- [ ] **Step 5: Implement conditional config validation**

Use `validateSecretFile` and require an absolute path. Never read or include file contents in errors.

- [ ] **Step 6: Write failing silence reader tests**

Use the repository’s PostgreSQL test helper. Create a temporary `public.ops_alert_silences` table and assert:

```go
matched, err := reader.IsSilenced(ctx, Scope{
    RuleID: 7, Platform: "openai", GroupID: ptr(int64(16)),
}, now)
```

Cover exact NULL semantics, expired rows, different dimensions, invalid input, and query failure returning an error without the connection string.

- [ ] **Step 7: Implement the restricted reader**

Parse the URL from the secret file, configure pgx with:

```go
config.MaxConns = 2
config.MinConns = 0
config.MaxConnIdleTime = 2 * time.Minute
```

Query only:

```sql
SELECT EXISTS (
    SELECT 1
    FROM public.ops_alert_silences
    WHERE rule_id=$1
      AND platform=$2
      AND group_id IS NOT DISTINCT FROM $3
      AND region IS NOT DISTINCT FROM $4
      AND until>$5
)
```

Wrap each query with a five-second timeout.

- [ ] **Step 8: Run focused tests**

```bash
cd relay-ops-service
go test ./internal/notificationpolicy ./internal/config ./internal/nativeopssilence -count=1
```

- [ ] **Step 9: Commit**

```bash
git add config/notification-policy.json relay-ops-service/internal/notificationpolicy relay-ops-service/internal/config relay-ops-service/internal/nativeopssilence
git commit -m "feat: configure native alert silence lookup"
```

---

### Task 3: Persist Native Alert Cursor and Source-Event Ledger

**Files:**

- Create: `relay-ops-service/internal/store/migrations/007_native_ops_alert_bridge.sql`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`

**Interfaces:**

- Produces:

```go
type NativeOpsAlertCursor struct {
    FiredAt time.Time
    EventID int64
}

type NativeOpsAlertSource struct {
    SourceEventID int64
    RuleID        int64
    IncidentKey   string
    Severity      string
    SourceStatus  string
    FiredAt       time.Time
    ResolvedAt    *time.Time
    Silenced      bool
    DimensionsHash string
    LastSeenAt    time.Time
}

func (s *Store) LoadNativeOpsAlertCursor(context.Context) (NativeOpsAlertCursor, bool, error)
func (s *Store) InitializeNativeOpsAlertSync(context.Context, NativeOpsAlertCursor, []NativeOpsAlertSource) error
func (s *Store) CommitNativeOpsAlertPage(context.Context, []NativeOpsAlertSource, NativeOpsAlertCursor) error
func (s *Store) UpsertNativeOpsAlertSource(context.Context, NativeOpsAlertSource) error
func (s *Store) ListFiringNativeOpsAlertSources(context.Context, int) ([]NativeOpsAlertSource, error)
```

- [ ] **Step 1: Write failing migration and round-trip tests**

Assert migration reruns cleanly and:

```go
cursor, found, err := st.LoadNativeOpsAlertCursor(ctx)
// initially found == false

err = st.InitializeNativeOpsAlertSync(ctx, cursor41, []store.NativeOpsAlertSource{source41})
got, found, err := st.LoadNativeOpsAlertCursor(ctx)
// got == cursor41
```

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/store -run 'NativeOpsAlert' -count=1
```

- [ ] **Step 3: Add transaction rollback and idempotency tests**

Force one invalid source in a page and assert neither cursor nor valid preceding rows commit. Recommit the same event and assert one row remains. Update `firing` to `resolved` and assert it disappears from `ListFiringNativeOpsAlertSources`.

- [ ] **Step 4: Create migration**

Use the exact tables and constraints from the approved spec, plus:

```sql
CREATE INDEX native_ops_alert_events_firing_idx
ON relay_ops.native_ops_alert_events (last_seen_at, source_event_id)
WHERE source_status='firing';
```

- [ ] **Step 5: Embed migration and implement methods**

Add:

```go
//go:embed migrations/007_native_ops_alert_bridge.sql
var nativeOpsAlertBridgeMigration string
```

Append it in `init()`. Validate IDs, statuses, severity, hashes, and timestamps before starting transactions.

- [ ] **Step 6: Run store tests**

```bash
cd relay-ops-service
go test ./internal/store -count=1
```

- [ ] **Step 7: Commit**

```bash
git add relay-ops-service/internal/store
git commit -m "feat: persist native alert sync state"
```

---

### Task 4: Support Silence Cancellation and Infinite Reminder Cadence

**Files:**

- Modify: `relay-ops-service/internal/incidents/state_machine.go`
- Modify: `relay-ops-service/internal/incidents/state_machine_test.go`
- Modify: `relay-ops-service/internal/alerting/escalator.go`
- Modify: `relay-ops-service/internal/alerting/escalator_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`

**Interfaces:**

- Produces:

```go
func (m Machine) Suppress(context.Context, string) error
func (m Machine) Close(context.Context, string, string) (Transition, error)
```

Allowed close reasons are `resolved` and `manual_resolved`.

Store behavior when suppressed or terminal:

- Set incident state to `suppressed`, `recovered`, or `closed`.
- Set `next_escalation_at=NULL`.
- Clear escalation claim fields.
- Cancel pending `reserved|failed` notification deliveries for the current occurrence whose transition is not the newly emitted terminal transition.

- [ ] **Step 1: Write failing state-machine tests**

Cover:

```go
// confirmed -> Suppress => state suppressed, no occurrence increment
// suppressed + failing observation => same occurrence returns confirmed and Notify=true
// confirmed -> Close("manual_resolved") => state closed, Kind manual_resolved
// confirmed -> Close("resolved") => state recovered, Kind recovered
```

- [ ] **Step 2: Run incident tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/incidents -run 'Suppress|Close' -count=1
```

- [ ] **Step 3: Implement minimal state controls**

Keep existing observation behavior unchanged for other families. A firing observation after `suppressed` resumes the same occurrence with one notification; a new event after `recovered|closed` creates a new occurrence.

- [ ] **Step 4: Write failing reminder cadence tests**

Assert exact deadlines:

```go
P0: level 0 -> +5m, 1 -> +15m, 2 -> +45m, 3 -> +75m
P1: level 0 -> +15m, 1 -> +75m, 2 -> +135m
```

- [ ] **Step 5: Implement recurring cadence**

Use:

```go
case "P0":
    if completedLevel == 0 { return first.Add(5*time.Minute), true }
    if completedLevel == 1 { return first.Add(15*time.Minute), true }
    return first.Add(15*time.Minute + time.Duration(completedLevel-1)*30*time.Minute), true
case "P1":
    if completedLevel == 0 { return first.Add(15*time.Minute), true }
    return first.Add(15*time.Minute + time.Duration(completedLevel)*60*time.Minute), true
```

Check formula results against the asserted table before accepting GREEN.

- [ ] **Step 6: Write failing PostgreSQL cancellation tests**

Create a delivered alert, a failed retry, and a due escalation. Suppress or close the incident, then assert:

- no due escalation can be claimed;
- old alert retry cannot be claimed;
- a recovery/manual-close delivery can still be reserved;
- suppressing one incident does not affect another.

- [ ] **Step 7: Implement transactional cancellation**

Add narrow store methods used by `Machine.Suppress`/`Machine.Close`; do not globally change retry semantics for unrelated incidents.

- [ ] **Step 8: Run focused tests**

```bash
cd relay-ops-service
go test ./internal/incidents ./internal/alerting ./internal/store -count=1
```

- [ ] **Step 9: Commit**

```bash
git add relay-ops-service/internal/incidents relay-ops-service/internal/alerting relay-ops-service/internal/store
git commit -m "feat: control native alert reminder lifecycle"
```

---

### Task 5: Render Dedicated Native Ops Alert Cards

**Files:**

- Create: `relay-ops-service/internal/notify/native_ops_alert.go`
- Create: `relay-ops-service/internal/notify/native_ops_alert_test.go`

**Interfaces:**

- Produces:

```go
type NativeOpsAlertView struct {
    EventID       int64
    RuleID        int64
    Title         string
    Severity      string
    Status        string
    MetricValue   *float64
    ThresholdValue *float64
    Dimensions    map[string]string
    FiredAt       time.Time
    ResolvedAt    *time.Time
    ReminderLevel int
    SilenceExpired bool
    TerminalSummary bool
}

func RenderNativeOpsAlert(NativeOpsAlertView) FeishuMessage
func RenderNativeOpsAlertRecovery(NativeOpsAlertView) FeishuMessage
func RenderNativeOpsAlertManualClose(NativeOpsAlertView) FeishuMessage
func RenderNativeOpsAlertReminder(NativeOpsAlertView) FeishuMessage
```

- [ ] **Step 1: Write failing content tests**

Assert titles:

```text
P0｜Sub2API 原生告警｜错误率过高
已恢复｜Sub2API 原生告警｜错误率过高
人工关闭｜Sub2API 原生告警｜错误率过高
P1｜仍在告警｜Sub2API 原生告警｜错误率过高
```

Assert event/rule IDs, current value, threshold, timestamps, and public admin link are present.

- [ ] **Step 2: Run renderer tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/notify -run 'NativeOpsAlert' -count=1
```

- [ ] **Step 3: Add security and size tests**

Input dimensions:

```go
map[string]string{
    "platform":"openai", "group_id":"16", "region":"us",
    "api_key":"sk-secret", "cookie":"session-secret", "unknown":"private",
}
```

Assert only `platform`, `group_id`, `region`, `model`, and `account_id` can render. Assert no secret value appears in `CardJSON()`, oversized title/description is bounded, and the card stays below the existing size limit.

- [ ] **Step 4: Implement the renderers**

Reuse existing card primitives and `WithDeliveryIdentity`; do not create a new Feishu transport. Terminal summary cards must visibly say “轮询间短时告警” and include both fired/resolved timestamps.

- [ ] **Step 5: Run notify tests**

```bash
cd relay-ops-service
go test ./internal/notify -count=1
```

- [ ] **Step 6: Commit**

```bash
git add relay-ops-service/internal/notify/native_ops_alert.go relay-ops-service/internal/notify/native_ops_alert_test.go
git commit -m "feat: render native ops alert cards"
```

---

### Task 6: Implement Native Alert Polling and Lifecycle Service

**Files:**

- Create: `relay-ops-service/internal/nativeopsalerts/identity.go`
- Create: `relay-ops-service/internal/nativeopsalerts/identity_test.go`
- Create: `relay-ops-service/internal/nativeopsalerts/service.go`
- Create: `relay-ops-service/internal/nativeopsalerts/service_test.go`

**Interfaces:**

- Consumes Task 1 API reader, Task 2 silence reader/policy, Task 3 source ledger, Task 4 incident controls, Task 5 renderers, and existing `notify.DeliverySender`/`notify.OneShotSender`.
- Produces:

```go
type SourceRepository interface {
    LoadNativeOpsAlertCursor(context.Context) (store.NativeOpsAlertCursor, bool, error)
    InitializeNativeOpsAlertSync(context.Context, store.NativeOpsAlertCursor, []store.NativeOpsAlertSource) error
    CommitNativeOpsAlertPage(context.Context, []store.NativeOpsAlertSource, store.NativeOpsAlertCursor) error
    UpsertNativeOpsAlertSource(context.Context, store.NativeOpsAlertSource) error
    ListFiringNativeOpsAlertSources(context.Context, int) ([]store.NativeOpsAlertSource, error)
    RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

type IncidentController interface {
    Observe(context.Context, incidents.Observation) (incidents.Transition, error)
    Suppress(context.Context, string) error
    Close(context.Context, string, string) (incidents.Transition, error)
}

type Service struct {
    Reader      sub2api.OpsAlertReader
    Sources     SourceRepository
    Silences    nativeopssilence.Reader
    Incidents   IncidentController
    Notifier    interface {
        SendIncident(context.Context, string, string, notify.FeishuMessage) error
        SendOneShot(context.Context, notify.OneShotIdentity, notify.FeishuMessage) error
    }
    Policy      notificationpolicy.Policy
    Clock       func() time.Time
}

func (s Service) Run(context.Context) error
```

- [ ] **Step 1: Write failing identity tests**

Prove these dimensions have the same hash:

```json
{"group_id":16,"platform":"openai","ratio":5}
{"ratio":5.0,"platform":"openai","group_id":16.0}
```

Prove different values differ. Assert incident key format is `native-ops-alert:7:<64-lowercase-hex>`. Assert display dimensions use the allowlist only.

- [ ] **Step 2: Run identity tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/nativeopsalerts -run 'Identity|Dimensions' -count=1
```

- [ ] **Step 3: Implement deterministic canonicalization**

Decode/normalize JSON numbers through `math/big.Rat` or an equivalent deterministic representation, recursively sort object keys, preserve array order, hash canonical bytes, and never return raw dimensions in an error.

- [ ] **Step 4: Write failing bootstrap and pagination tests**

Cases:

- no cursor: walk all pages, import only current firing P0/P1, set newest cursor, do not replay historical terminal events;
- existing cursor: walk newest pages until matching `(fired_at,id)`, process unseen items oldest-first;
- more than 500 unseen events: paginate without loss;
- Admin API failure: cursor remains unchanged;
- duplicate event: no duplicate notification.

- [ ] **Step 5: Write failing lifecycle tests**

Cover:

```text
new P0 firing -> one trigger
new P1 firing -> one trigger
P2/P3 -> suppressed decision
same event again -> evidence_stored only
same instance later new source event -> new occurrence
P1 source upgraded to P0 -> immediate escalation
resolved after delivered trigger -> one recovery
manual_resolved after delivered trigger -> one manual-close
terminal event first seen between polls -> one one-shot summary
terminal event summary duplicate -> no second send
```

- [ ] **Step 6: Write failing silence tests**

Cover:

- silence hit before trigger: suppress;
- delivered firing event becomes silenced: cancel escalation/retry;
- silence lookup error: no trigger, no cursor advancement for the affected page, decision reason `silence_lookup_unavailable`;
- silence expires while source remains firing: one resumed alert with `SilenceExpired=true`;
- source resolves during silence after a delivered alert: recovery still sends.

- [ ] **Step 7: Implement minimal service**

Processing order per run:

```text
1. load/init cursor
2. discover new events and stage page
3. for each P0/P1 event, compute identity and check silence
4. apply lifecycle and send/reserve notification
5. persist source rows and page cursor transactionally
6. refresh every tracked firing event by detail endpoint
7. update latest evidence without duplicate delivery
```

Join independent errors with `errors.Join`, but never advance a page cursor if any event on that page could not be safely classified.

- [ ] **Step 8: Run package tests**

```bash
cd relay-ops-service
go test ./internal/nativeopsalerts -count=1
```

- [ ] **Step 9: Commit**

```bash
git add relay-ops-service/internal/nativeopsalerts
git commit -m "feat: bridge native ops alert lifecycle"
```

---

### Task 7: Wire Scheduler, Application, Compose, and Deployment Controls

**Files:**

- Modify: `relay-ops-service/internal/scheduler/scheduler.go`
- Modify: `relay-ops-service/internal/scheduler/scheduler_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`
- Modify: `infra/compose.yaml`
- Modify: `infra/env.example` if present
- Modify: existing relay-ops deployment runbook selected during this task

**Interfaces:**

- Adds `Scheduler.NativeOpsAlerts func(context.Context) error`.
- Adds job key `native-ops-alert-sync`, interval one minute.
- App owns and closes the secondary silence reader.

- [ ] **Step 1: Write failing scheduler tests**

At `00:00:00`, `00:00:30`, and `00:01:00`, expect two runs. In `ModeClosed`, expect zero. A failure must be joined with other scheduler job failures without preventing later jobs.

- [ ] **Step 2: Run scheduler tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/scheduler -run 'NativeOpsAlert' -count=1
```

- [ ] **Step 3: Implement scheduler hook**

Call it after notification retry and before general incident escalation so newly discovered terminal/silence states can cancel stale reminders in the same tick.

- [ ] **Step 4: Write failing app wiring tests**

Test:

- policy disabled: no silence connection required and scheduler hook is nil;
- policy enabled: missing silence secret rejected by config;
- policy enabled with fake dependencies: hook is installed;
- application close closes both relay store and silence reader;
- no raw URL appears in errors.

- [ ] **Step 5: Implement application wiring**

Construct:

```go
silenceReader, err := nativeopssilence.Open(ctx, cfg.Sub2APIAlertReadDatabaseURLFile)
nativeService := nativeopsalerts.Service{...}
scheduled.NativeOpsAlerts = nativeService.Run
```

Reuse the existing `incidentMachine`, `notifier`, notification transport, database, and policy. Add an app close method only if needed; preserve existing call sites.

- [ ] **Step 6: Update Compose and example environment**

Add:

```yaml
RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE: /run/secrets/sub2api-alert-read-database-url
```

and a read-only bind mount:

```yaml
- ${RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_HOST_FILE:-/dev/null}:/run/secrets/sub2api-alert-read-database-url:ro
```

Do not add an actual URL or password to the repository.

- [ ] **Step 7: Add least-privilege runbook commands**

Document parameterized SQL:

```sql
CREATE ROLE relay_ops_alert_reader LOGIN PASSWORD '<generated-secret>';
GRANT CONNECT ON DATABASE <sub2api_database> TO relay_ops_alert_reader;
GRANT USAGE ON SCHEMA public TO relay_ops_alert_reader;
GRANT SELECT ON TABLE public.ops_alert_silences TO relay_ops_alert_reader;
```

Also document negative permission checks against `ops_alert_events` and another business table. Mark password creation and secret-file installation as operator steps, not automated repository actions.

- [ ] **Step 8: Run focused integration tests**

```bash
cd relay-ops-service
go test ./internal/scheduler ./internal/app ./internal/nativeopsalerts ./internal/nativeopssilence -count=1
cd ..
docker compose -f infra/compose.yaml config >/dev/null
```

- [ ] **Step 9: Commit**

```bash
git add relay-ops-service/internal/scheduler relay-ops-service/internal/app infra config docs
git commit -m "feat: schedule native P0 P1 Feishu alerts"
```

---

### Task 8: Full Verification, Security Review, and Handoff Report

**Files:**

- Create: `docs/superpowers/reports/2026-07-30-native-p0-p1-feishu-bridge-verification.md`

**Consumes:** All prior tasks.

- [ ] **Step 1: Run formatting and static checks**

```bash
gofmt -w \
  relay-ops-service/internal/sub2api \
  relay-ops-service/internal/notificationpolicy \
  relay-ops-service/internal/config \
  relay-ops-service/internal/nativeopssilence \
  relay-ops-service/internal/nativeopsalerts \
  relay-ops-service/internal/notify \
  relay-ops-service/internal/incidents \
  relay-ops-service/internal/alerting \
  relay-ops-service/internal/store \
  relay-ops-service/internal/scheduler \
  relay-ops-service/internal/app

cd relay-ops-service
go vet ./...
```

- [ ] **Step 2: Run the complete Go test suite**

```bash
cd relay-ops-service
go test ./... -count=1
```

Expected: zero failures.

- [ ] **Step 3: Run race tests for stateful packages**

```bash
cd relay-ops-service
go test -race ./internal/nativeopsalerts ./internal/store ./internal/scheduler ./internal/notify ./internal/alerting -count=1
```

Expected: zero races and zero failures.

- [ ] **Step 4: Validate migration and Compose**

```bash
docker compose -f infra/compose.yaml config >/dev/null
git diff --check
```

Run the repository’s PostgreSQL integration test command twice to prove migration idempotency.

- [ ] **Step 5: Perform secret and scope scan**

```bash
rg -n --hidden \
  '(sk-[A-Za-z0-9]|open\\.feishu\\.cn/open-apis/bot/v2/hook/|postgres(ql)?://[^[:space:]]+:[^[:space:]]+@|x-api-key[\"'\"']?\\s*[:=]\\s*[\"'\"'][^\"'\"']+)' \
  relay-ops-service infra config docs \
  -g '!**/*_test.go'
```

Expected: no real credentials. Review every match manually.

- [ ] **Step 6: Review requirement coverage**

Create a table in the report mapping each approved requirement to:

- implementation file;
- test name;
- verification command/result;
- any production-only acceptance step.

Explicitly include P0/P1 cadence, short terminal summary, manual close, silence expiry, fail-closed lookup, policy disabled mode, and preservation of group-impact alerts.

- [ ] **Step 7: Run independent whole-branch review**

Reviewer checks:

- API cursor correctness and losslessness;
- transaction boundaries;
- multi-instance idempotency;
- silence NULL semantics;
- cancellation of stale retries/reminders;
- no secret leakage;
- no changes under `upstream/sub2api`;
- no regression in existing notification families.

Address every actionable finding, rerun affected tests, then rerun Steps 1–5.

- [ ] **Step 8: Write verification report**

Record exact commands, exit codes, test counts where available, and remaining production-only steps. Do not claim live Feishu delivery or database grants unless they were actually executed and verified.

- [ ] **Step 9: Commit**

```bash
git add docs/superpowers/reports/2026-07-30-native-p0-p1-feishu-bridge-verification.md
git commit -m "docs: verify native P0 P1 Feishu bridge"
```

---

## Plan Self-Review Checklist

- [ ] Every requirement in the approved spec maps to at least one task and test.
- [ ] No task modifies `upstream/sub2api`.
- [ ] Interface names used by later tasks match the producing task.
- [ ] No placeholder instructions remain.
- [ ] Every behavior change starts with a failing test.
- [ ] Every task ends with focused verification and a commit.
- [ ] Final verification includes full tests, race tests, Compose validation, secret scan, and independent branch review.
