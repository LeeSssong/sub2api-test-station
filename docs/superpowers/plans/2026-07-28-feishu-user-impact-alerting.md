# Feishu User-Impact Alerting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver severity-aware Feishu alerts with responsible-person mentions, P0 urgency, acknowledgement, bounded escalation, stable incident occurrence identity, and production noise reduction.

**Architecture:** Extend the existing incident state machine and PostgreSQL store instead of adding a second alert system. Keep renderers pure, make the delivery boundary return auditable Feishu results, expose acknowledgement through the existing hidden-admin surface, and run escalation as one database-claimed scheduler job.

**Tech Stack:** Go 1.24, PostgreSQL 18, Feishu OpenAPI interactive cards, server-rendered HTML with vanilla JavaScript, Docker Compose, Ruby/shell operations tests.

## Global Constraints

- Do not change Sub2API request routing, pricing, billing, accounts, or credentials.
- Monitoring read failures remain fail-safe and must not create business incidents.
- P0: zero available capacity or a complete public-group request failure; mention configured recipients and apply Feishu in-app urgency.
- P1: partial capacity loss, active-but-unschedulable account, explicit balance exhaustion, sustained public-group error rate, or sustained public-group TTFT degradation; mention configured recipients.
- P2 and recovery messages do not mention, apply urgency, or escalate.
- P0 escalation is bounded to 5 and 15 minutes; P1 escalation is bounded to 15 minutes.
- Every new failure after recovery increments `occurrence_no`; delivery idempotency includes occurrence and transition.
- Alert cards and persisted payloads retain current redaction and 30 KiB limits.
- Production deployment recreates only relay-ops.

---

### Task 1: Incident Occurrence And Persistence

**Files:**
- Create: `relay-ops-service/internal/store/migrations/004_user_impact_alerting.sql`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/incidents/state_machine.go`
- Modify: `relay-ops-service/internal/incidents/state_machine_test.go`

**Interfaces:**
- Produces: `incidents.Record.OccurrenceNo int64`
- Produces: `incidents.Transition.OccurrenceNo int64`
- Produces: `Store.AcknowledgeIncident(context.Context, incidents.Acknowledgement) error`
- Produces: `incidents.ErrOccurrenceConflict` and `incidents.ErrNotActive`

- [ ] **Step 1: Write failing state-machine tests**

Add literal assertions proving first failure is occurrence 1, repeated failing samples remain occurrence 1, recovery retains occurrence 1, and a new failure after recovery becomes occurrence 2. Add acknowledgement error tests against a small in-memory repository contract.

- [ ] **Step 2: Run the state-machine tests and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/incidents -run 'Occurrence|Acknowledg' -count=1
```

Expected: compile failure because occurrence and acknowledgement contracts do not exist.

- [ ] **Step 3: Implement occurrence transitions**

Add:

```go
type Record struct {
    Key, Severity, State, EvidenceHash, CurrentValue string
    SampleCount int
    OccurrenceNo int64
}

type Transition struct {
    State, Kind, RelatedKey string
    Notify bool
    OccurrenceNo int64
}
```

Default missing historical occurrences to 1. When a recovered record fails again, preserve the record and increment `OccurrenceNo`; do not reset to an indistinguishable record.

- [ ] **Step 4: Add the idempotent migration and real-store tests**

The migration adds the exact incident and delivery columns from the spec with `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`. Append the migration to `initialMigration`. Extend `Store.Get` and `Store.Put` to persist occurrence and clear acknowledgement/escalation fields on a new occurrence or recovery.

Add a real PostgreSQL test that runs `Migrate` twice, persists occurrence 2, acknowledges occurrence 2, rejects occurrence 1, and rejects acknowledgement after recovery.

- [ ] **Step 5: Run focused tests and verify GREEN**

```bash
cd relay-ops-service
go test ./internal/incidents ./internal/store -run 'Occurrence|Acknowledg|Migrate' -count=1
```

Expected: PASS; store tests may SKIP only when `RELAY_OPS_TEST_DATABASE_URL` is absent.

- [ ] **Step 6: Commit**

```bash
git add relay-ops-service/internal/incidents relay-ops-service/internal/store
git commit -m "feat: track alert incident occurrences"
```

### Task 2: Auditable Feishu Delivery And Strong Reminder Transport

**Files:**
- Modify: `relay-ops-service/internal/feishuapi/client.go`
- Modify: `relay-ops-service/internal/feishuapi/client_test.go`
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/feishu_test.go`
- Modify: `relay-ops-service/internal/notify/delivery.go`
- Modify: `relay-ops-service/internal/notify/delivery_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`

**Interfaces:**
- Produces: `feishuapi.Client.UrgentMessage(context.Context, string, []string) (int, error)`
- Produces: `notify.SendResult{MessageID string, ResponseCode int, Payload []byte, UrgentStatus string, UrgentResponseCode int}`
- Produces: `notify.FeishuMessage.Severity string`
- Produces: `notify.AppClient.RecipientOpenIDs []string`
- Changes: `notify.MessageClient.Send(context.Context, notify.FeishuMessage) (notify.SendResult, error)`

- [ ] **Step 1: Write failing Feishu API and AppClient tests**

Test the real HTTP boundary:

- `PATCH /open-apis/im/v1/messages/msg-1/urgent_app?user_id_type=open_id`
- JSON body `{"user_id_list":["ou-a","ou-b"]}`
- P0 card contains both `<at id=ou-a></at>` and `<at id=ou-b></at>`
- P0 calls urgency after `SendMessage` returns `msg-1`
- P1 mentions but does not call urgency
- P2 and recovery do neither
- message success plus urgency failure returns a successful message result with `UrgentStatus == "failed"`

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/feishuapi ./internal/notify -run 'Urgent|Mention|SendResult' -count=1
```

Expected: compile failure for missing result, severity, recipient, and urgency contracts.

- [ ] **Step 3: Implement urgency and message-result contracts**

Use Feishu application urgency only; do not add SMS or phone urgency. Deduplicate and bound recipient Open IDs to 20 before rendering or sending. Clone the card before inserting a mention element so renderers remain pure and repeat sends do not accumulate mentions.

- [ ] **Step 4: Write failing delivery audit tests**

Test that:

- dedup identity includes incident key, occurrence, transition, and evidence;
- occurrence 1 delivered evidence does not suppress occurrence 2;
- the actual outbound card, Feishu message ID, response code, urgent status, and urgent response code are persisted;
- delivered message plus failed urgency is not retried as a duplicate first message.

- [ ] **Step 5: Implement delivery persistence**

Replace positional reserve/finish parameters with structs:

```go
type Reservation struct {
    IncidentKey, DedupKey, MessageHash, Transition string
    OccurrenceNo int64
}

type DeliveryOutcome struct {
    Status, MessageID, UrgentStatus string
    ResponseCode, UrgentResponseCode int
    Payload []byte
}
```

Keep failed-message retry behavior. Treat `MessageID != ""` as delivered even when urgency failed.

- [ ] **Step 6: Run focused and real-store tests**

```bash
cd relay-ops-service
go test ./internal/feishuapi ./internal/notify ./internal/store -run 'Urgent|Mention|Delivery|Notification' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add relay-ops-service/internal/feishuapi relay-ops-service/internal/notify relay-ops-service/internal/store
git commit -m "feat: audit and strengthen Feishu delivery"
```

### Task 3: Severity, Stable Group Events, And Runtime Noise Reduction

**Files:**
- Modify: `relay-ops-service/internal/app/group_availability.go`
- Modify: `relay-ops-service/internal/app/group_availability_test.go`
- Modify: `relay-ops-service/internal/notify/group_alert.go`
- Modify: `relay-ops-service/internal/notify/group_alert_test.go`
- Modify: `relay-ops-service/internal/opsmonitor/service.go`
- Modify: `relay-ops-service/internal/opsmonitor/service_test.go`

**Interfaces:**
- Produces: `notify.GroupAlertView.Severity string`
- Uses: `incidents.Transition.OccurrenceNo`
- Uses: occurrence-aware `notify.DeliverySender.SendIncident`

- [ ] **Step 1: Write failing group-severity and recurrence tests**

Add tests proving:

- `0/N` is P0 with one confirmation window;
- partial capacity below the existing threshold is P1 with two windows;
- group evidence is stable inside and across hours;
- failure, recovery, and an identical second failure all produce three deliveries with occurrence numbers 1, 1, and 2;
- cards start with `P0｜` or `P1｜`, show available/total capacity, and carry matching severity metadata.

- [ ] **Step 2: Run group tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/app ./internal/notify -run 'Group.*(P0|P1|Occurrence|Hour)' -count=1
```

Expected: FAIL because the current evidence contains an hour bucket and every group alert is fixed P1.

- [ ] **Step 3: Implement stable group incidents**

Remove the hour from `EvidenceHash`. Set severity and confirmation windows from current capacity. Use occurrence-aware delivery evidence for confirmed, new-evidence, escalated, and recovered transitions.

- [ ] **Step 4: Write failing runtime-noise tests**

Use a reader returning one public group and eight active accounts with the same bad window. Assert one public-group error-rate incident is notified and no `site:account:*:error_rate`, `availability`, or `ttft_p95` incident is created. Assert paused, balance exhaustion, and multiplier account incidents still work.

- [ ] **Step 5: Remove duplicate account runtime alerts**

Retain account status and quality checks, but stop calling `snapshots`/`evaluateRuntime` for accounts. Public-group runtime remains unchanged except severity: complete failure is P0; error rate and TTFT remain P1.

- [ ] **Step 6: Run focused tests and verify GREEN**

```bash
cd relay-ops-service
go test ./internal/app ./internal/notify ./internal/opsmonitor -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add relay-ops-service/internal/app/group_availability* relay-ops-service/internal/notify/group_alert* relay-ops-service/internal/opsmonitor
git commit -m "fix: prioritize user impact in Feishu alerts"
```

### Task 4: Authenticated Acknowledgement

**Files:**
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/http/static/ops.js`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/feishu_test.go`
- Modify: `relay-ops-service/internal/app/app.go`

**Interfaces:**
- Produces: `IncidentAcknowledgementService.Acknowledge(context.Context, incidents.Acknowledgement) error`
- Produces: `POST /relay-ops/api/incidents/ack`
- Produces: `notify.WithAcknowledgementAction(message, incidentKey string, occurrenceNo int64) FeishuMessage`

- [ ] **Step 1: Write failing renderer and HTTP tests**

Assert P0/P1 cards include a single “确认并接手” URL button whose query has URL-encoded incident key and literal occurrence number. Assert P2/recovery cards do not include it.

For HTTP, use the real `adminauth.RequireHiddenAdmin` boundary and test:

- valid bearer + exact Origin + current active occurrence returns 204 and records admin user ID;
- missing bearer is hidden as 404;
- wrong Origin returns 403;
- stale occurrence and recovered incident return 409;
- unknown JSON fields and oversized bodies return 400.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/notify ./internal/http -run 'Acknowledg|IncidentAck' -count=1
```

Expected: compile failure for missing action and acknowledgement service.

- [ ] **Step 3: Implement the hidden-admin endpoint**

Register:

```go
mux.Handle(
    "POST /relay-ops/api/incidents/ack",
    adminauth.RequireHiddenAdmin(dependencies.Auth, http.HandlerFunc(s.ackIncident)),
)
```

Reuse `validMutation`, enforce a 1 KiB body limit, and map occurrence/not-active conflicts to 409.

- [ ] **Step 4: Implement the card-to-ops flow**

The button URL is:

```text
/ops?ack_incident=<urlencoded-key>&ack_occurrence=<positive-int>
```

`ops.js` and `ops-admin.js` preserve the query, read the existing bearer token, POST the acknowledgement once, remove the query with `history.replaceState`, and show an `aria-live` success/failure status. Never put the token in the URL.

- [ ] **Step 5: Run focused tests and verify GREEN**

```bash
cd relay-ops-service
go test ./internal/notify ./internal/http ./internal/app -run 'Acknowledg|IncidentAck|NewServer' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add relay-ops-service/internal/http relay-ops-service/internal/notify relay-ops-service/internal/app/app.go
git commit -m "feat: acknowledge Feishu incidents from ops"
```

### Task 5: Bounded Unacknowledged Escalation

**Files:**
- Create: `relay-ops-service/internal/alerting/escalator.go`
- Create: `relay-ops-service/internal/alerting/escalator_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/scheduler/scheduler.go`
- Modify: `relay-ops-service/internal/scheduler/scheduler_test.go`
- Modify: `relay-ops-service/internal/app/app.go`

**Interfaces:**
- Produces: `alerting.Repository.ClaimDueEscalation(context.Context, time.Time) (*alerting.Incident, error)`
- Produces: `alerting.Repository.FinishEscalation(context.Context, alerting.Result) error`
- Produces: `alerting.Service.Run(context.Context) error`
- Produces: `scheduler.Scheduler.IncidentEscalation func(context.Context) error`

- [ ] **Step 1: Write failing pure policy tests**

Use a fixed clock and literal expected times:

- P0 first delivery schedules +5m;
- P0 level 1 schedules the second level at original +15m;
- P0 level 2 schedules nothing;
- P1 first delivery schedules +15m and level 1 schedules nothing;
- P2/recovery schedule nothing.

- [ ] **Step 2: Write failing service tests**

Test that the service:

- prepends `再次提醒｜` and includes elapsed duration;
- preserves severity so P0 urgency is retried;
- uses dedup evidence `occurrence:<n>:escalation:<level>`;
- does nothing after acknowledgement or recovery;
- returns a failed claim to retry in one minute without advancing permanently.

- [ ] **Step 3: Run alerting tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/alerting ./internal/scheduler -run 'Escalat' -count=1
```

Expected: compile failure because the alerting package and scheduler hook do not exist.

- [ ] **Step 4: Implement atomic escalation claims**

Claim only current active, unacknowledged occurrences whose `next_escalation_at <= now()`. Increment the claimed level atomically and load the most recent delivered non-recovery payload. On successful send, set the next bounded deadline; on failure, restore the prior level and schedule one-minute retry.

- [ ] **Step 5: Wire the one-minute scheduler job**

Add a database-claimed `incident-escalation` job with a one-minute interval. The job performs no paid probes and no Sub2API mutations.

- [ ] **Step 6: Run focused and store tests**

```bash
cd relay-ops-service
go test ./internal/alerting ./internal/scheduler ./internal/store ./internal/app -run 'Escalat|Notification' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add relay-ops-service/internal/alerting relay-ops-service/internal/scheduler relay-ops-service/internal/store relay-ops-service/internal/app/app.go
git commit -m "feat: escalate unacknowledged user-impact alerts"
```

### Task 6: Recipient Configuration And Deployment Contract

**Files:**
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Modify: `infra/compose.yaml`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`

**Interfaces:**
- Produces: `config.Config.FeishuAlertRecipientsFile string`
- Produces: `notify.LoadRecipientOpenIDs(path string) ([]string, error)`

- [ ] **Step 1: Write failing config and recipient parser tests**

Cover valid 1/20-recipient files and reject: missing file when App alert chat is enabled, empty list, 21 entries, duplicates after trimming, empty values, unknown JSON fields, oversized file, and invalid JSON.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/config ./internal/notify ./internal/app -run 'Recipient|AlertChat' -count=1
```

Expected: compile failure for the missing config field and parser.

- [ ] **Step 3: Implement strict secret loading and AppClient wiring**

Read the file once at startup, keep Open IDs only in memory, and pass them to `notify.AppClient`. Error messages must not include file contents or Open IDs.

- [ ] **Step 4: Update Compose and contract tests**

Add:

```yaml
RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE: ${RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE:-}
```

and:

```yaml
- ${RELAY_OPS_FEISHU_ALERT_RECIPIENTS_HOST_FILE:-/dev/null}:/run/secrets/feishu-alert-recipients.json:ro
```

Update the shell contract to require both lines without weakening existing checks.

- [ ] **Step 5: Run focused and infrastructure tests**

```bash
cd relay-ops-service
go test ./internal/config ./internal/notify ./internal/app -run 'Recipient|AlertChat' -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add relay-ops-service/internal/config relay-ops-service/internal/notify relay-ops-service/internal/app infra/compose.yaml tests/relay_ops/validate_relay_ops_contract.sh
git commit -m "feat: configure Feishu alert recipients"
```

### Task 7: Full Verification And Diff Review

**Files:**
- Modify only if verification exposes a defect in the files already listed.

- [ ] **Step 1: Format and run static checks**

```bash
git diff --name-only --diff-filter=ACM | awk '/^relay-ops-service\/.*\.go$/' | xargs gofmt -w
git diff --check
cd relay-ops-service
go vet ./...
```

Expected: no output from `git diff --check`; `go vet` exits 0.

- [ ] **Step 2: Run all relay-ops tests**

```bash
cd relay-ops-service
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run real PostgreSQL tests**

Start an isolated PostgreSQL 18 container on a free local port, export
`RELAY_OPS_TEST_DATABASE_URL`, run:

```bash
cd relay-ops-service
go test ./internal/store -count=1
```

Expected: PASS with no skips for database tests. Remove only the specifically named temporary container afterward.

- [ ] **Step 4: Run repository operations checks**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
ruby -Itest tests/operations/account_quality_monitor_test.rb
ruby -Itest tests/operations/analyze_account_monitor_test.rb
```

Expected: PASS.

- [ ] **Step 5: Build the production binary and image**

```bash
docker build -f infra/Dockerfile.relay-ops -t sub2api-relay-ops:user-impact-alerting-20260728-v1 .
```

Expected: image build exits 0 and the container health endpoint returns 200 in an isolated Compose/render smoke.

- [ ] **Step 6: Review the complete diff**

Confirm:

- no unrelated dirty files are staged or changed;
- Open IDs, chat IDs, tokens, webhooks, and credentials are absent from git diff and logs;
- every P0/P1 call path carries severity and occurrence;
- every recovery clears escalation;
- all query/body limits and admin/origin checks are present;
- no loop can resend indefinitely.

- [ ] **Step 7: Commit any verification-only correction**

Stage only alerting-related files and use a focused fix commit. Do not stage pre-existing workflow, updater, or upstream lifecycle changes.

### Task 8: Production Deployment And End-To-End Acceptance

**Files:**
- Production only: `/opt/sub2api/production/compose.yaml`
- Production only: `/opt/sub2api/production/.env`
- Production only: `/opt/sub2api/production/secrets/feishu-alert-recipients.json`
- Create: `docs/superpowers/reports/2026-07-28-feishu-user-impact-alerting-production-verification.md`

- [ ] **Step 1: Resolve recipients without exposing identifiers**

Compare the configured alert chat ID with `relay_ops.feishu_command_events.chat_id` on the server. Export unique recent `sender_open_id` values from that exact chat into the root-owned recipient JSON. Abort if the matching chat has zero operators or more than 20; never print identifiers.

- [ ] **Step 2: Back up the exact deployment state**

Create a timestamped directory below `/opt/sub2api/production/backups/feishu-user-impact-alerting-*` containing Compose, `.env`, relay-ops schema-only dump, current image ID, and container inspection. Restrict permissions to root.

- [ ] **Step 3: Render and validate Compose before mutation**

Add the recipient env/mount and the new image tag to a staged Compose copy. Run `docker compose config` and verify only relay-ops configuration changes.

- [ ] **Step 4: Deploy only relay-ops**

Build or transfer the verified image, then run:

```bash
sudo docker compose -f /opt/sub2api/production/compose.yaml up -d --no-deps relay-ops
```

Expected: relay-ops becomes healthy; Sub2API, PostgreSQL, Redis, and Caddy container IDs and restart counts remain unchanged.

- [ ] **Step 5: Verify migration and scheduler state**

Confirm new columns, constraints, recipient configuration, `incident-escalation` scheduler job, zero failed delivery rows created by startup, and healthy `/healthz`, `/readyz`, `/ops`.

- [ ] **Step 6: Send one labeled P0 acceptance incident**

Use a dedicated `acceptance:feishu-user-impact-alerting:20260728` incident that cannot collide with business incidents. The card title must start `P0｜告警链路验收`, mention configured recipients, apply urgency, and contain the acknowledgement button.

- [ ] **Step 7: Acknowledge through the real ops flow**

Open the card button with an existing admin browser session. Verify HTTP 204, `acknowledged_occurrence`, `acknowledged_by`, `message_id`, and successful urgency evidence without displaying their raw values. Wait beyond the five-minute boundary and verify no escalation delivery is created.

- [ ] **Step 8: Close the acceptance incident and verify no residue**

Mark the synthetic occurrence recovered through the acceptance harness, verify exactly one green recovery card and `next_escalation_at IS NULL`, then confirm no pending synthetic incident, reservation, or scheduler retry remains.

- [ ] **Step 9: Record production evidence**

Write the verification report with image ID, migration result, container invariants, redacted delivery/urgency/ack evidence, test commands, rollback location, and remaining risks. Commit the report without secrets.
