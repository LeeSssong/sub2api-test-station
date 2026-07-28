# Feishu Notification Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a server-policy-gated Feishu notification system that correlates active production evidence into one human-readable public-group incident, while delivering pricing changes and daily reports as non-incident one-shot notifications.

**Architecture:** A strict JSON notification policy enables only the approved production families. `group-availability` and the Sub2API synchronizer persist bounded, fresh capacity and native-monitor signals; `site-monitor` combines them with real-traffic snapshots and drives one `group:<id>:user-impact` incident. Pricing and digest messages use a separate one-shot delivery store and never enter the incident lifecycle.

**Tech Stack:** Go 1.24, PostgreSQL/pgx v5, Feishu App interactive cards, Docker Compose, shell contract tests.

## Global Constraints

- Every proactive Feishu message requires a configured transport, an explicitly enabled family, a configured and fresh source, an allowed object, and a lifecycle transition that says notify.
- Missing or invalid policy fails closed; no code path becomes active merely because it exists.
- Keep Sub2API's official image read-only; modify only relay-ops, its database schema, configuration, and tests.
- Preserve `RELAY_OPS_MODE=read_only` and do not enable Feishu commands.
- Do not change routes, account scheduling, multipliers, prices, balances, credentials, or Keys.
- Candidate, Usage Session, synthetic acceptance, command-result, model-publication, and GitHub release messages are not active notification families.
- P0/P1 severity and copy are deterministic; Agent output cannot change severity, cause, action, delivery, escalation, or recovery.
- Card payloads remain redacted and below 30 KB.
- Do not deploy or push to production as part of this implementation.

---

### Task 0: Establish the implementation baseline in a new worktree

**Files:**
- Consume: `docs/superpowers/specs/2026-07-29-feishu-notification-consolidation-design.md`
- Consume: `docs/superpowers/plans/2026-07-29-feishu-notification-consolidation-implementation-plan.md`

**Interfaces:**
- Consumes: design branch `codex/feishu-notification-consolidation-design` and deployed alert branch `codex/feishu-user-impact-alerting`
- Produces: clean implementation branch `codex/feishu-notification-consolidation` containing current `origin/main`, the deployed alert implementation, and the approved spec/plan

- [ ] **Step 1: Create the sibling worktree**

```bash
git worktree add ../sub2api-feishu-notification-consolidation \
  -b codex/feishu-notification-consolidation \
  codex/feishu-notification-consolidation-design
```

- [ ] **Step 2: Merge the deployed alert baseline**

```bash
git merge --no-ff codex/feishu-user-impact-alerting \
  -m "merge: integrate deployed Feishu alert baseline"
```

Expected: no content conflicts. The pre-plan audit found zero files modified by both sides since merge base `a2a853f68`.

- [ ] **Step 3: Verify the merged baseline**

```bash
cd relay-ops-service
go mod download
go build ./...
go vet ./...
go test ./... -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
```

Expected: all commands exit 0 before feature edits begin.

- [ ] **Step 4: Record the baseline**

```bash
git status --short
git log -3 --oneline
```

Expected: clean worktree; merge commit is the branch tip.

### Task 1: Add a strict server-side notification policy

**Files:**
- Create: `relay-ops-service/internal/notificationpolicy/policy.go`
- Create: `relay-ops-service/internal/notificationpolicy/policy_test.go`
- Create: `config/relay-ops/notification-policy.example.json`
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Modify: `infra/compose.yaml`
- Modify: `infra/.env.example`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`

**Interfaces:**
- Produces:

```go
type Family string
type DeliveryMode string

const (
    FamilyGroupRuntime          Family = "group_runtime"
    FamilyGroupCapacity         Family = "group_capacity"
    FamilyAccountImpact         Family = "account_impact"
    FamilyNativeMonitorEvidence Family = "native_monitor_evidence"
    FamilyPricingNotice         Family = "pricing_notice"
    FamilyDailyDigest           Family = "daily_digest"
    FamilyIncidentEscalation    Family = "incident_escalation"

    ModeDisabled DeliveryMode = "disabled"
    ModeShadow   DeliveryMode = "shadow"
    ModeEnabled  DeliveryMode = "enabled"
)

type Policy struct {
    Version int          `json:"version"`
    Mode    DeliveryMode `json:"delivery_mode"`
    Feishu  FeishuPolicy `json:"feishu_notifications"`
}

func Load(path string) (Policy, error)
func (p Policy) Enabled(family Family) bool
func (p Policy) ShouldDeliver(family Family) bool
```

- `config.Config` produces `NotificationPolicyFile string` and `NotificationPolicy notificationpolicy.Policy`.

- [ ] **Step 1: Write failing policy parser tests**

```go
func TestLoadRejectsUnknownAndMissingFamilies(t *testing.T) {
    path := writePolicy(t, `{"version":1,"feishu_notifications":{"group_runtime_enabled":true,"typo":true}}`)
    if _, err := Load(path); err == nil {
        t.Fatal("unknown policy field accepted")
    }
}

func TestEnabledMapsEveryApprovedFamily(t *testing.T) {
    policy := mustLoadPolicy(t, allEnabledPolicy)
    for _, family := range ApprovedFamilies() {
        if !policy.Enabled(family) {
            t.Fatalf("%s disabled", family)
        }
    }
}

func TestDeliveryModeRejectsUnknownValueAndShadowNeverDelivers(t *testing.T)
```

- [ ] **Step 2: Verify the parser tests fail**

Run:

```bash
cd relay-ops-service
go test ./internal/notificationpolicy -run 'TestLoad|TestEnabled' -count=1
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement strict JSON loading**

Use `json.Decoder.DisallowUnknownFields`, require exactly one JSON value, require `version == 1`, accept only `disabled`, `shadow`, or `enabled`, and represent all seven booleans explicitly. `Enabled` reports the family flag; `ShouldDeliver` additionally requires `delivery_mode=enabled`. Do not add a YAML dependency.

- [ ] **Step 4: Add failing config integration tests**

Cover:

```go
func TestLoadRequiresNotificationPolicyForConfiguredAlertTransport(t *testing.T)
func TestLoadAcceptsNoPolicyWhenNoAlertTransportExists(t *testing.T)
func TestLoadRejectsInvalidNotificationPolicyPermissions(t *testing.T)
```

Configured App alerts or Webhook without `RELAY_OPS_NOTIFICATION_POLICY_FILE` must fail. No transport means an all-disabled zero policy.

- [ ] **Step 5: Wire config, Compose, and contract fixtures**

Mount:

```yaml
- ${RELAY_OPS_NOTIFICATION_POLICY_HOST_FILE:-/dev/null}:/run/relay-ops/notification-policy.json:ro
```

Set:

```yaml
RELAY_OPS_NOTIFICATION_POLICY_FILE: ${RELAY_OPS_NOTIFICATION_POLICY_FILE:-}
```

The example JSON contains exactly the seven approved fields and no candidate/release/usage/synthetic family.
It uses `delivery_mode: "shadow"` so installing the example cannot page the production group.

- [ ] **Step 6: Run focused and contract tests**

```bash
cd relay-ops-service
go test ./internal/notificationpolicy ./internal/config -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
```

- [ ] **Step 7: Commit**

```bash
git add relay-ops-service/internal/notificationpolicy \
  relay-ops-service/internal/config \
  config/relay-ops/notification-policy.example.json \
  infra/compose.yaml infra/.env.example \
  tests/relay_ops/validate_relay_ops_contract.sh
git commit -m "feat: gate Feishu notifications with server policy"
```

### Task 2: Add persistence for group signals, one-shot messages, decisions, and baselines

**Files:**
- Create: `relay-ops-service/internal/store/migrations/006_notification_consolidation.sql`
- Create: `relay-ops-service/internal/store/notification_consolidation.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Create: `relay-ops-service/internal/notify/one_shot.go`
- Create: `relay-ops-service/internal/notify/one_shot_test.go`

**Interfaces:**
- Produces:

```go
// package store
type GroupSignal struct {
    GroupName       string
    SourceKind      string
    SourceKey       string
    Payload         json.RawMessage
    SourceObservedAt time.Time
    ExpiresAt       time.Time
}

type OneShotReservation struct {
    NotificationKey string
    Family          string
    PolicyVersion   int
    SourceKind      string
    DedupKey        string
    MessageHash     string
    Payload         []byte
}

type DecisionRecord struct {
    DecisionKey  string
    Family       string
    PolicyVersion int
    SourceKind   string
    Decision     string
    Reason       string
    Details      json.RawMessage
    ObservedAt   time.Time
}

type Baseline struct {
    Key          string
    CurrentValue string
    EvidenceHash string
    UpdatedAt    time.Time
}
```

- Store methods:

```go
UpsertGroupSignal(context.Context, GroupSignal) error
ListFreshGroupSignals(context.Context, string, time.Time) ([]GroupSignal, error)
ReserveOneShot(context.Context, notify.OneShotReservation) (int64, bool, error)
FinishOneShot(context.Context, int64, notify.DeliveryOutcome) error
RecordNotificationDecision(context.Context, DecisionRecord) error
GetOperationalBaseline(context.Context, string) (Baseline, bool, error)
PutOperationalBaseline(context.Context, Baseline) error
SupersedeLegacyNotificationIncidents(context.Context, time.Time) (int64, error)
```

- `notify.OneShotSender` consumes:

```go
type OneShotRepository interface {
    ReserveOneShot(context.Context, OneShotReservation) (int64, bool, error)
    FinishOneShot(context.Context, int64, DeliveryOutcome) error
}
```

- [ ] **Step 1: Write failing PostgreSQL contract tests**

Tests must prove:

```go
func TestGroupSignalsReplaceByIdentityAndExpire(t *testing.T)
func TestOneShotReservationIsIdempotentAndRetriesFailure(t *testing.T)
func TestNotificationDecisionUpsertsLastSeenAndCount(t *testing.T)
func TestOperationalBaselineRoundTrips(t *testing.T)
```

- [ ] **Step 2: Verify the persistence tests fail**

```bash
cd relay-ops-service
go test ./internal/store -run 'TestGroupSignals|TestOneShot|TestNotificationDecision|TestOperationalBaseline' -count=1
```

Expected: FAIL with missing migration/types/methods.

- [ ] **Step 3: Add migration 006**

The migration must:

```sql
ALTER TABLE relay_ops.incidents
    ADD COLUMN IF NOT EXISTS family TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS policy_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS recovery_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS material_hash TEXT,
    ADD COLUMN IF NOT EXISTS latest_payload JSONB;

CREATE TABLE IF NOT EXISTS relay_ops.group_impact_signals (...);
CREATE TABLE IF NOT EXISTS relay_ops.notification_messages (...);
CREATE TABLE IF NOT EXISTS relay_ops.notification_decisions (...);
CREATE TABLE IF NOT EXISTS relay_ops.operational_baselines (...);
```

`notification_messages.notification_key` and `dedup_key` are unique. Payload columns are JSONB. Retry columns match the existing bounded `1m, 2m, 5m, 10m`, maximum-five-attempt policy.

- [ ] **Step 4: Implement store methods in the focused file**

Use transactions for reservations and outcome updates. Validate identifiers, timestamps, JSON, family, and policy version before SQL.

- [ ] **Step 5: Write a failing one-shot sender test**

```go
func TestOneShotSenderDoesNotAddAcknowledgementOrIncidentIdentity(t *testing.T) {
    message := RenderPricingNotice(PricingNoticeView{Upstream: "Neko"})
    err := sender.SendOneShot(ctx, OneShotIdentity{
        Key: "pricing:7:hash", Family: "pricing_notice",
        PolicyVersion: 1, SourceKind: "public_pricing",
    }, message)
    // Assert one reservation, one send, no acknowledgement action.
}
```

- [ ] **Step 6: Implement `OneShotSender`**

```go
type OneShotIdentity struct {
    Key           string
    Family        string
    PolicyVersion int
    SourceKind    string
}

type OneShotSender struct {
    Client     MessageClient
    Repository OneShotRepository
}

func (s OneShotSender) SendOneShot(
    ctx context.Context,
    identity OneShotIdentity,
    message FeishuMessage,
) error
```

It must render, hash, reserve, send, and finish without calling `WithAcknowledgementAction`.

- [ ] **Step 7: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/store ./internal/notify -count=1
cd ..
git add relay-ops-service/internal/store \
  relay-ops-service/internal/notify/one_shot.go \
  relay-ops-service/internal/notify/one_shot_test.go
git commit -m "feat: persist notification signals and one-shot delivery"
```

### Task 3: Make incident evidence updates silent and recovery confirmed

**Files:**
- Modify: `relay-ops-service/internal/incidents/state_machine.go`
- Modify: `relay-ops-service/internal/incidents/state_machine_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`

**Interfaces:**
- Extends:

```go
type Observation struct {
    Key                 string
    Family              string
    PolicyVersion       int
    SourceKind          string
    Severity            string
    Failing             bool
    EvidenceHash        string
    MaterialHash        string
    CurrentValue        string
    LatestPayload       []byte
    ConfirmationWindows int
    RecoveryWindows     int
}

type Record struct {
    // existing fields...
    Family        string
    PolicyVersion int
    SourceKind    string
    RecoveryCount int
    MaterialHash  string
    LatestPayload []byte
}
```

- Produces transition kinds: `confirmed`, `escalated`, `progressed`, `recovered`. `new_evidence` is no longer emitted.

- [ ] **Step 1: Add failing state-machine tests**

```go
func TestEvidenceChangeUpdatesStateWithoutNotification(t *testing.T)
func TestMaterialChangeProducesProgressedNotification(t *testing.T)
func TestRecoveryRequiresConfiguredHealthyWindows(t *testing.T)
func TestFailureResetsRecoveryCount(t *testing.T)
func TestSeverityEscalationStillNotifies(t *testing.T)
```

- [ ] **Step 2: Run tests and confirm the old behavior fails**

```bash
cd relay-ops-service
go test ./internal/incidents -run 'TestEvidenceChange|TestMaterialChange|TestRecoveryRequires|TestFailureResets|TestSeverityEscalation' -count=1
```

- [ ] **Step 3: Implement minimal state transitions**

Rules:

```text
same severity + new evidence + same material => persist, Notify=false
same severity + new material                 => Kind=progressed, Notify=true
P1 -> P0                                     => Kind=escalated, Notify=true
healthy observation below RecoveryWindows   => persist recovery_count, Notify=false
healthy observation reaching RecoveryWindows=> Kind=recovered, Notify=true
new failure                                  => recovery_count=0
```

- [ ] **Step 4: Extend Store `Get`/`Put` atomically**

Read/write all new columns. `state='recovered'` and a new occurrence reset recovery count, acknowledgement, escalation lease, and pending incident-delivery retries.

- [ ] **Step 5: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/incidents ./internal/store -count=1
cd ..
git add relay-ops-service/internal/incidents \
  relay-ops-service/internal/store/postgres.go \
  relay-ops-service/internal/store/postgres_test.go
git commit -m "feat: distinguish incident progress from evidence updates"
```

### Task 4: Build the pure public-group impact evaluator and human-language cards

**Files:**
- Create: `relay-ops-service/internal/groupimpact/types.go`
- Create: `relay-ops-service/internal/groupimpact/evaluate.go`
- Create: `relay-ops-service/internal/groupimpact/evaluate_test.go`
- Create: `relay-ops-service/internal/notify/user_impact.go`
- Create: `relay-ops-service/internal/notify/user_impact_test.go`

**Interfaces:**
- Produces:

```go
type Snapshot struct {
    GroupID       int64
    GroupName     string
    ObservedAt    time.Time
    Runtime       *RuntimeEvidence
    Capacity      *CapacityEvidence
    NativeMonitor []NativeMonitorEvidence
}

type Impact struct {
    Failing      bool
    Severity     string
    Primary      string
    EvidenceHash string
    MaterialHash string
    Headline     string
    UserImpact   string
    Current      []Fact
    Clues        []Fact
    Action       string
    ObservedAt   time.Time
}

type ReminderSnapshot struct {
    GroupName  string    `json:"group_name"`
    Headline   string    `json:"headline"`
    LatestFact string    `json:"latest_fact"`
    Capacity   string    `json:"capacity,omitempty"`
    ObservedAt time.Time `json:"observed_at"`
}

func Evaluate(Snapshot) Impact
func RenderUserImpact(UserImpactView) FeishuMessage
func RenderUserImpactRecovery(UserImpactRecoveryView) FeishuMessage
func RenderUserImpactReminder(UserImpactReminderView) FeishuMessage
```

- [ ] **Step 1: Write failing evaluator table tests**

Cover exact boundaries:

```go
{name: "P0 zero capacity", available: 0, total: 2, wantSeverity: "P0", wantPrimary: "no_available_accounts"}
{name: "P0 all requests failed", requests: 31, success: 0, wantSeverity: "P0", wantPrimary: "all_requests_failed"}
{name: "P1 errors", requests: 20, errorRate: .05, wantPrimary: "partial_request_failures"}
{name: "P1 redundancy", available: 1, total: 3, wantPrimary: "lost_redundancy"}
{name: "P1 ttft", requests: 20, ttft: 3900, baseline: 3000, wantPrimary: "ttft_degraded"}
```

Also prove priority order:

```text
no available accounts > all failed > partial failures > lost redundancy > TTFT
```

- [ ] **Step 2: Verify evaluator tests fail**

```bash
cd relay-ops-service
go test ./internal/groupimpact -count=1
```

- [ ] **Step 3: Implement pure evaluation and hashes**

`EvidenceHash` changes with current metrics. `MaterialHash` changes only when severity, primary impact, affected scope, confirmed cause category, or action changes.

- [ ] **Step 4: Write failing renderer tests**

Assert titles and forbidden terms:

```go
for _, forbidden := range []string{
    "group #", "new_evidence", "error_rate", "ttft_p95",
    "active but paused", "balance_exhausted",
} {
    if strings.Contains(message.RenderedText(), forbidden) {
        t.Fatalf("technical term leaked: %s", forbidden)
    }
}
```

- [ ] **Step 5: Implement deterministic cards**

Use the approved section order: `发生了什么`, `用户影响`, `当前容量/当前证据`, `已知原因/已知线索`, `建议处理`, time, actions.

- [ ] **Step 6: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/groupimpact ./internal/notify -count=1
cd ..
git add relay-ops-service/internal/groupimpact \
  relay-ops-service/internal/notify/user_impact.go \
  relay-ops-service/internal/notify/user_impact_test.go
git commit -m "feat: evaluate and render public group user impact"
```

### Task 5: Turn capacity and native Monitor jobs into evidence producers

**Files:**
- Create: `relay-ops-service/internal/groupimpact/signals.go`
- Create: `relay-ops-service/internal/groupimpact/signals_test.go`
- Modify: `relay-ops-service/internal/app/group_availability.go`
- Modify: `relay-ops-service/internal/app/group_availability_test.go`
- Modify: `relay-ops-service/internal/sub2api/sync.go`
- Modify: `relay-ops-service/internal/sub2api/sync_test.go`
- Modify: `relay-ops-service/internal/nativealerts/service.go`
- Modify: `relay-ops-service/internal/nativealerts/service_test.go`
- Modify: `relay-ops-service/internal/dailyreport/health.go`
- Modify: `relay-ops-service/internal/accounthealth/classify.go`

**Interfaces:**
- `group-availability` produces `source_kind=capacity`, `source_key=current`.
- native observer produces `source_kind=native_monitor`, `source_key=<monitor-id>:<model>`.
- `MonitorObserver` becomes:

```go
ObserveMonitor(context.Context, sub2api.Group, sub2api.ChannelMonitor, sub2api.MonitorHistory) error
```

- [ ] **Step 1: Add failing capacity producer tests**

Prove:

- it upserts one fresh signal per visible group;
- an active but unschedulable account counts as unavailable;
- `balance_exhausted` becomes Chinese cause text in the signal;
- source failure records a stable suppression and sends no card.

- [ ] **Step 2: Remove incident sending from `runGroupAvailability`**

Replace `groupIncidentObserver` and `groupAlertSender` with:

```go
type groupSignalSink interface {
    UpsertGroupSignal(context.Context, store.GroupSignal) error
    RecordNotificationDecision(context.Context, store.DecisionRecord) error
}
```

The job still reads the one-hour account window but only persists evidence.

- [ ] **Step 3: Add failing native Monitor producer tests**

Prove:

- enabled Monitor plus visible group produces evidence;
- unbound Monitor never reaches the observer;
- abnormal Monitor stores a clue but sends no P1 card;
- policy disabled records `suppressed/policy_disabled`.

- [ ] **Step 4: Pass the matched group through the synchronizer**

Change `collectMonitorRef` to receive the current visible group and call the new observer signature.

- [ ] **Step 5: Implement the evidence-only native observer**

Keep the `nativealerts` package name for a small migration diff, but delete Agent and incident/notifier dependencies from its `Service`. Its only outputs are fresh group signals and decision audit.

- [ ] **Step 6: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/accounthealth ./internal/dailyreport \
  ./internal/app ./internal/sub2api ./internal/nativealerts ./internal/groupimpact -count=1
cd ..
git add relay-ops-service/internal/accounthealth \
  relay-ops-service/internal/dailyreport/health.go \
  relay-ops-service/internal/app/group_availability.go \
  relay-ops-service/internal/app/group_availability_test.go \
  relay-ops-service/internal/sub2api \
  relay-ops-service/internal/nativealerts \
  relay-ops-service/internal/groupimpact
git commit -m "refactor: collect capacity and monitor evidence without paging"
```

### Task 6: Drive one correlated group incident from `site-monitor`

**Files:**
- Create: `relay-ops-service/internal/groupimpact/service.go`
- Create: `relay-ops-service/internal/groupimpact/service_test.go`
- Modify: `relay-ops-service/internal/opsmonitor/service.go`
- Modify: `relay-ops-service/internal/opsmonitor/service_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`

**Interfaces:**
- Produces:

```go
type Service struct {
    Reader      RuntimeReader
    Signals     SignalRepository
    Incidents   IncidentObserver
    Notifier    IncidentSender
    Policy      notificationpolicy.Policy
    Decisions   DecisionRecorder
    Now         func() time.Time
}

func (s Service) Run(context.Context) error
```

The service-local interfaces are:

```go
type RuntimeReader interface {
    ListGroups(context.Context) ([]sub2api.Group, error)
    GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error)
}

type SignalRepository interface {
    ListFreshGroupSignals(context.Context, string, time.Time) ([]store.GroupSignal, error)
}

type IncidentObserver interface {
    Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type IncidentSender interface {
    SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type DecisionRecorder interface {
    RecordNotificationDecision(context.Context, store.DecisionRecord) error
}
```

- [ ] **Step 1: Write failing service tests**

Required cases:

```go
func TestServiceCorrelatesRuntimeCapacityAndMonitorIntoOneIncident(t *testing.T)
func TestServiceUsesStableGroupIDKeyAndCurrentGroupName(t *testing.T)
func TestServiceStoresEvidenceChangeWithoutSending(t *testing.T)
func TestServiceSendsProgressOnlyForMaterialChange(t *testing.T)
func TestServiceRequiresTwoHealthyObservationsForRecovery(t *testing.T)
func TestServiceFailsSafeWhenRuntimeIsUnavailable(t *testing.T)
```

- [ ] **Step 2: Verify service tests fail**

```bash
cd relay-ops-service
go test ./internal/groupimpact -run TestService -count=1
```

- [ ] **Step 3: Implement group iteration and correlation**

For every `CustomerVisible()` group:

1. read 15-minute runtime and 24-hour baseline;
2. read fresh stored signals by current `Group.Name`;
3. evaluate one `Impact`;
4. render and serialize the latest deterministic card;
5. call the incident machine with key `group:<id>:user-impact`, family `group_runtime`, recovery windows 2;
6. send only when `transition.Notify`.

In `delivery_mode=shadow`, use key `shadow:group:<id>:user-impact`, run the complete state machine, record `shadow_would_deliver`, and never call the Feishu sender or schedule escalation. Switching to `enabled` starts the normal key with a fresh occurrence and leaves shadow rows inactive.

- [ ] **Step 4: Restrict old `opsmonitor.Service`**

Delete direct group runtime, paused-account, and balance-exhausted incident sends. Keep multiplier collection temporarily behind the pricing family until Task 7 moves it to one-shot delivery.

- [ ] **Step 5: Wire the scheduler**

`site-monitor` runs:

```go
errors.Join(groupImpactService.Run(ctx), multiplierWatcher.Run(ctx))
```

`group-availability` remains the capacity evidence producer. `production-collection` remains the native Monitor and public-pricing producer.

- [ ] **Step 6: Run focused tests and commit**

```bash
cd relay-ops-service
go test ./internal/groupimpact ./internal/opsmonitor ./internal/app -count=1
cd ..
git add relay-ops-service/internal/groupimpact \
  relay-ops-service/internal/opsmonitor \
  relay-ops-service/internal/app
git commit -m "feat: correlate one user impact incident per public group"
```

### Task 7: Convert pricing and multiplier changes to one-shot P2 events

**Files:**
- Create: `relay-ops-service/internal/pricingevents/service.go`
- Create: `relay-ops-service/internal/pricingevents/service_test.go`
- Create: `relay-ops-service/internal/notify/pricing_notice.go`
- Create: `relay-ops-service/internal/notify/pricing_notice_test.go`
- Modify: `relay-ops-service/internal/collection/pricing.go`
- Modify: `relay-ops-service/internal/collection/pricing_test.go`
- Modify: `relay-ops-service/internal/upstreampricing/resolver.go`
- Modify: `relay-ops-service/internal/upstreampricing/resolver_test.go`
- Modify: `relay-ops-service/internal/app/app.go`

**Interfaces:**
- Public pricing and multiplier services consume:

```go
type EventSender interface {
    SendOneShot(context.Context, notify.OneShotIdentity, notify.FeishuMessage) error
}
```

- Multiplier watcher consumes the operational baseline store and an explicit account-to-pricing-source resolution:

```go
type Resolution struct {
    PricingURL string
    Multiplier float64
}

func (r *Resolver) Resolve(context.Context, string) (Resolution, bool)
```

- [ ] **Step 1: Add failing pricing renderer tests**

Assert:

- title is `价格变更｜<生产上游> 公开定价发生变化`;
- body says what changed, what the system did not change, and what to review;
- severity is P2;
- no acknowledgement action, escalation wording, Agent copy, or recovery language.

- [ ] **Step 2: Convert public pricing notification tests**

Replace incident expectations with:

```go
identity.Key == "pricing:<upstream-id>:<semantic-hash>"
identity.Family == "pricing_notice"
identity.SourceKind == "public_pricing"
```

HTML-only changes and unparseable evidence must not send.
Shadow mode records `shadow_would_deliver` with the semantic notification key and sends nothing.

- [ ] **Step 3: Refactor `collection.Collector`**

Remove `IncidentMachine` and `AnalysisRunner` from pricing-change delivery. Keep snapshot persistence. On a reliable semantic diff, gate by policy and call `EventSender`.

- [ ] **Step 4: Add multiplier watcher tests**

Prove:

- first trustworthy value only creates a baseline;
- a changed value sends one P2 event and advances the baseline;
- unchanged value sends nothing;
- an explicitly mapped account whose production upstream already has public pricing records `covered_by_production_pricing` and does not create a duplicate event;
- an unmapped trustworthy multiplier change can send one standalone event.

- [ ] **Step 5: Implement multiplier watcher and app wiring**

Move multiplier logic out of the incident state machine. Store baselines in `operational_baselines`, send through `OneShotSender`, and invoke it from `site-monitor`.

- [ ] **Step 6: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/collection ./internal/pricingevents \
  ./internal/upstreampricing ./internal/notify ./internal/app -count=1
cd ..
git add relay-ops-service/internal/collection \
  relay-ops-service/internal/pricingevents \
  relay-ops-service/internal/upstreampricing \
  relay-ops-service/internal/notify/pricing_notice.go \
  relay-ops-service/internal/notify/pricing_notice_test.go \
  relay-ops-service/internal/app/app.go
git commit -m "refactor: deliver pricing changes as P2 events"
```

### Task 8: Convert the daily report to an idempotent digest

**Files:**
- Modify: `relay-ops-service/internal/dailyreport/service.go`
- Modify: `relay-ops-service/internal/dailyreport/service_test.go`
- Modify: `relay-ops-service/internal/dailyreport/health.go`
- Modify: `relay-ops-service/internal/dailyreport/health_test.go`
- Modify: `relay-ops-service/internal/notify/digest_v2.go`
- Modify: `relay-ops-service/internal/notify/digest_v2_test.go`
- Modify: `relay-ops-service/internal/store/notification_consolidation.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/app/app.go`

**Interfaces:**
- Removes `CandidateReader`, `IncidentStateRunner`, and `AnalysisRunner` from `dailyreport.Service`.
- Adds:

```go
type DigestSummaryReader interface {
    ReadDailyNotificationSummary(context.Context, time.Time, time.Time) (DailyNotificationSummary, error)
}

type Service struct {
    Reader    opsmetrics.Reader
    Summary   DigestSummaryReader
    Notifier  EventSender
    Policy    notificationpolicy.Policy
    Timezone  *time.Location
    Now       func() time.Time
    Fallback  func(context.Context, string) *float64
}
```

- [ ] **Step 1: Write failing service tests**

Prove:

- notification identity is `daily-digest:<date>`;
- no incident observation occurs;
- same date is idempotent;
- policy disabled records suppression;
- policy shadow records `shadow_would_deliver` and sends nothing;
- candidate reader and Agent are absent from the service.

- [ ] **Step 2: Write failing digest renderer tests**

Require sections:

```text
一句话结论
用户侧运行
需要处理
经营情况
监控完整性
```

Forbid:

```text
候选
当前账号
候选账号
只读分析
调整建议
```

- [ ] **Step 3: Remove candidate recommendations**

Delete `RecommendationLine`, `Recommendations`, `buildRecommendations`, and the `accountrecommendation` dependency from the daily notification path. Do not remove unrelated historical packages in this task.

- [ ] **Step 4: Implement digest summary query and rendering**

Summary includes active P0/P1 group incidents, recovered occurrences in the previous business day, delivered pricing events, and source completeness. It contains no private group name or internal incident key.

- [ ] **Step 5: Send through `OneShotSender`**

Use family `daily_digest`, source kind `daily_report`, policy version 1, and the Shanghai business date key.

- [ ] **Step 6: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/dailyreport ./internal/notify ./internal/store ./internal/app -count=1
cd ..
git add relay-ops-service/internal/dailyreport \
  relay-ops-service/internal/notify/digest_v2.go \
  relay-ops-service/internal/notify/digest_v2_test.go \
  relay-ops-service/internal/store \
  relay-ops-service/internal/app/app.go
git commit -m "refactor: deliver daily operations as a digest"
```

### Task 9: Render reminders from the latest group snapshot

**Files:**
- Modify: `relay-ops-service/internal/alerting/escalator.go`
- Modify: `relay-ops-service/internal/alerting/escalator_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/notify/user_impact.go`
- Modify: `relay-ops-service/internal/notify/user_impact_test.go`

**Interfaces:**
- `alerting.Incident` adds `CurrentValue string`.
- `CurrentValue` for group incidents is bounded JSON encoded from `groupimpact.ReminderSnapshot`, defined in Task 4.

- [ ] **Step 1: Write failing reminder tests**

Assert:

- reminder title starts `再次提醒｜`;
- body contains duration, unacknowledged status, latest fact, and capacity;
- body does not clone the complete initial card;
- no `第 1 次提醒`;
- P0/P1 schedules remain 5/15 and 15 minutes respectively.

- [ ] **Step 2: Select the current incident value**

`ClaimDueEscalation` reads `incidents.current_value`, not the latest delivered card as the source of operational facts. The original delivery is still used only to prove that the occurrence was delivered.

- [ ] **Step 3: Render the concise reminder**

Decode `ReminderSnapshot` and call `RenderUserImpactReminder`. Invalid or legacy snapshots fail safely and remain retryable; they must not leak raw JSON into Feishu.

- [ ] **Step 4: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/alerting ./internal/store ./internal/notify -count=1
cd ..
git add relay-ops-service/internal/alerting \
  relay-ops-service/internal/store/postgres.go \
  relay-ops-service/internal/store/postgres_test.go \
  relay-ops-service/internal/notify/user_impact.go \
  relay-ops-service/internal/notify/user_impact_test.go
git commit -m "fix: render escalation reminders from latest evidence"
```

### Task 10: Archive legacy notification states and close inactive send paths

**Files:**
- Modify: `relay-ops-service/internal/store/migrations/006_notification_consolidation.sql`
- Modify: `relay-ops-service/internal/store/notification_consolidation.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`
- Modify: `relay-ops-service/internal/acceptance/service.go`
- Modify: `relay-ops-service/internal/acceptance/service_test.go`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`

**Interfaces:**
- Produces `state='superseded'` for active legacy notification incidents.
- Superseded incidents have:

```text
next_escalation_at = NULL
escalation_claim_token = NULL
escalation_claimed_at = NULL
```

- [ ] **Step 1: Write failing migration tests**

Seed active rows for:

```text
daily-report:
native-monitor:
site:account:*:paused
site:account:*:balance_exhausted
site:group:*:availability
site:group:*:error_rate
site:group:*:ttft_p95
upstream:*:pricing
candidate / quality / synthetic / usage_session
```

Run supersession and assert:

- rows are preserved;
- state becomes `superseded`;
- escalation is cleared;
- no notification delivery is inserted.

- [ ] **Step 2: Remove inactive notifier injection**

Do not pass the production notifier to candidate fast/quality paths, Usage Session handling, or synthetic acceptance. These historical capabilities may keep read-only/local behavior but cannot send proactive Feishu messages.

- [ ] **Step 3: Add forbidden-path contract checks**

The shell contract must fail if inactive packages regain `SendIncident` or `SendOneShot` wiring from `app.New`.

- [ ] **Step 4: Exclude superseded rows everywhere**

Incident summaries, retry claim, escalation claim, acknowledgement, and recovery handling must treat `superseded` as inactive.

- [ ] **Step 5: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/store ./internal/app ./internal/acceptance ./internal/alerting ./internal/notify -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
git add relay-ops-service/internal/store \
  relay-ops-service/internal/app \
  relay-ops-service/internal/acceptance \
  tests/relay_ops/validate_relay_ops_contract.sh
git commit -m "chore: retire inactive Feishu notification paths"
```

### Task 11: Add retry coverage for one-shot messages and end-to-end contracts

**Files:**
- Modify: `relay-ops-service/internal/notify/retry.go`
- Modify: `relay-ops-service/internal/notify/retry_test.go`
- Modify: `relay-ops-service/internal/store/notification_consolidation.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/e2e_test.go`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`

**Interfaces:**
- Retry service claims both:

```go
type RetryDelivery struct {
    Kind            string // "incident" or "one_shot"
    ID              int64
    IncidentKey     string
    NotificationKey string
    Severity        string
    OccurrenceNo    int64
    Transition      string
    Payload         []byte
}
```

- [ ] **Step 1: Add failing retry tests**

Prove:

- failed P0/P1 retries keep occurrence and transition;
- failed one-shot retries keep notification key and never add acknowledgement;
- delivered/expired/max-attempt one-shot rows are not claimed;
- acknowledged/recovered/superseded incidents are not claimed.

- [ ] **Step 2: Extend claim and finish paths**

Prefer due incident deliveries first, then due one-shot messages. Both keep the existing five-attempt ceiling and backoff.

- [ ] **Step 3: Add end-to-end fake transport tests**

One test run must exercise:

```text
capacity signal + runtime evidence
-> one P1 group incident
-> silent numeric evidence update
-> one concise reminder
-> two healthy observations
-> one recovery
```

A second run must exercise:

```text
reliable production pricing diff
-> one P2 one-shot
-> repeated diff
-> zero additional delivery
```

- [ ] **Step 4: Run tests and commit**

```bash
cd relay-ops-service
go test ./internal/notify ./internal/store ./internal/app -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
git add relay-ops-service/internal/notify \
  relay-ops-service/internal/store \
  relay-ops-service/internal/app \
  tests/relay_ops/validate_relay_ops_contract.sh
git commit -m "test: cover consolidated notification delivery lifecycle"
```

### Task 12: Document configuration and perform final verification

**Files:**
- Modify: `docs/project/current-state.md`
- Create: `docs/superpowers/reports/2026-07-29-feishu-notification-consolidation-local-verification.md`
- Modify: `config/relay-ops/notification-policy.example.json` if verification finds a schema drift

**Interfaces:**
- Produces a local-only verification report. It must explicitly say “not deployed” and “production unchanged”.

- [ ] **Step 1: Update current-state without claiming deployment**

Record:

```text
local implementation complete
production image unchanged
production policy file not installed
no Feishu message manufactured
no route/account/price/balance/key write
```

- [ ] **Step 2: Run the full required verification**

```bash
cd relay-ops-service
go build ./...
go vet ./...
go test ./... -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

- [ ] **Step 3: Audit forbidden notification terms and paths**

```bash
rg -n 'new_evidence|group #[0-9]+|error_rate|ttft_p95' relay-ops-service/internal/notify
rg -n 'SendIncident|SendOneShot' \
  relay-ops-service/internal/acceptance \
  relay-ops-service/internal/candidates \
  relay-ops-service/internal/qualityreports \
  relay-ops-service/internal/billing
```

Expected: no user-visible forbidden term and no inactive package wired to proactive delivery. Test fixtures may contain forbidden terms only in explicit negative assertions.

- [ ] **Step 4: Review production safety diff**

```bash
git diff codex/feishu-notification-consolidation-design...HEAD -- \
  infra relay-ops-service config tests docs
git status --short
```

Confirm no deployment command, production secret, route write, account write, price write, or generated credential was added.

- [ ] **Step 5: Write the local verification report**

Include exact commands, exit codes, test packages, commit SHA, dirty status, and the statement that no production push/deploy occurred.
List the unexecuted production gates explicitly: install a reviewed policy in `shadow` for at least 48 hours, review would-deliver decisions, switch to `enabled` only with separate approval, then observe real notifications for at least 72 小时.

- [ ] **Step 6: Commit documentation**

```bash
git add docs/project/current-state.md \
  docs/superpowers/reports/2026-07-29-feishu-notification-consolidation-local-verification.md
git commit -m "docs: record local Feishu consolidation verification"
```

- [ ] **Step 7: Stop before production**

Do not run `git push`, Docker image publishing, SSH deployment, Compose recreation, database migration, policy-file installation, or real Feishu acceptance. Hand off the clean local branch and verification evidence for review.
