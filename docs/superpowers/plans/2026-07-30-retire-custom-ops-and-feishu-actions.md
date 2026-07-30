# Retire Custom Ops and Feishu Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redirect the retired `/ops` entry to Sub2API `/admin/ops` and make Feishu a strictly outbound reminder channel with no acknowledgement, command, callback, or state-write path.

**Architecture:** Caddy owns the legacy-path redirect and exposes only the relay service's still-supported public pages. `relay-ops` keeps monitoring, incident detection, periodic reminders, recovery messages, daily reports, deduplication, and outbound Feishu delivery, while its browser ops projection and all Feishu inbound command wiring are removed. Incident reminder selection becomes based on active occurrences rather than acknowledgement state; historical acknowledgement columns remain untouched in PostgreSQL but leave the application data flow.

**Tech Stack:** Caddy, Docker Compose, Go 1.x standard library, PostgreSQL, Bash contract tests, Go unit/integration tests.

## Global Constraints

- `GET /ops`, `/ops/`, and `/ops/*` must return HTTP `302` with `Location: /admin/ops`.
- `/relay-ops/api/ops-view`, `/relay-ops/api/incidents/ack`, and `/relay-ops/api/feishu/events` must not be publicly or internally mounted.
- New Feishu messages must not contain acknowledgement, takeover, assignee, or pending-confirmation actions or copy.
- New Feishu cards must retain one navigation-only button targeting `/admin/ops`.
- Monitoring collection, alert detection, recovery detection, daily reports, reminder cadence, and notification deduplication must remain operational.
- Historical acknowledgement columns and rows remain in PostgreSQL; do not create destructive cleanup migrations.
- Do not stage or modify the user's existing `监控日报-2026-07-28.md` change.

---

## File Structure

### Routing boundary

- Modify `infra/Caddyfile` — legacy `/ops` redirect and public relay surface.
- Modify `tests/infra/validate-sub2api-update-routing.sh` — static routing contract.
- Modify `tests/infra/audit-public-links.sh` — deployed redirect acceptance.

### Retired browser control plane

- Modify `relay-ops-service/internal/http/server.go` — remove ops page/API/ack routes and dependencies.
- Modify `relay-ops-service/internal/http/server_test.go` — replace page/ack tests with explicit `404` contracts.
- Delete `relay-ops-service/internal/http/templates/ops.html` — retired self-built page.
- Delete `relay-ops-service/internal/http/static/ops.js` — retired bootstrap.
- Delete `relay-ops-service/internal/http/static/ops-admin.js` — retired authenticated UI.
- Delete `relay-ops-service/internal/http/ops_account_quality_test.go` — it tests only the retired HTTP projection.
- Keep `relay-ops-service/internal/opsmetrics/**` and account-quality readers because alerts and reports still consume them.

### Reminder-only cards and incident lifecycle

- Modify `relay-ops-service/internal/notify/delivery.go` — stop injecting acknowledgement actions.
- Modify `relay-ops-service/internal/notify/feishu.go` — remove acknowledgement action builder and use `/admin/ops` links.
- Modify `relay-ops-service/internal/notify/user_impact.go` — remove takeover copy; retain periodic reminder language.
- Modify `relay-ops-service/internal/notify/group_alert.go` — native ops link.
- Modify `relay-ops-service/internal/notify/digest_v2.go` — native ops link.
- Modify related tests under `relay-ops-service/internal/notify/*_test.go` and `relay-ops-service/internal/app/e2e_test.go`.
- Modify `relay-ops-service/internal/store/postgres.go` and relevant tests — active reminder claims must no longer depend on acknowledgement columns.
- Modify `relay-ops-service/internal/incidents/state_machine.go` — remove the retired acknowledgement value type while preserving incident occurrence identity.

### Feishu outbound-only runtime

- Modify `relay-ops-service/cmd/relay-ops/main.go` — serve the application handler directly; do not attach a callback handler or command worker.
- Delete `relay-ops-service/internal/app/feishu_commands.go`.
- Delete `relay-ops-service/internal/app/feishu_commands_test.go`.
- Delete `relay-ops-service/internal/app/command_control_test.go`.
- Delete `relay-ops-service/internal/commands/**`.
- Delete `relay-ops-service/internal/feishuevents/**`.
- Delete `relay-ops-service/internal/routingcontrol/**` — its production caller is the removed Feishu command runtime.
- Delete `relay-ops-service/internal/store/feishu_commands.go` and its focused tests if no caller remains.
- Modify `relay-ops-service/internal/config/config.go` and `config_test.go` — retain outbound App ID/App Secret validation; remove command/callback configuration.
- Modify `infra/compose.yaml` — remove callback-only environment variables and mounts.
- Keep `relay-ops-service/internal/feishuapi/**` because outbound App Bot delivery uses it.
- Keep historical database migrations for Feishu command and acknowledgement tables/columns.

### Documentation

- Modify `docs/project/current-state.md`.
- Modify `docs/project/llm-handoff.md`.
- Modify relevant current operational verification scripts or examples that still assert `/ops` returns `200`.
- Do not rewrite historical dated reports.

---

### Task 1: Redirect `/ops` at the Edge and Close Relay Admin Routes

**Files:**
- Modify: `tests/infra/validate-sub2api-update-routing.sh`
- Modify: `tests/infra/audit-public-links.sh`
- Modify: `infra/Caddyfile`

**Interfaces:**
- Consumes: Caddy site routing and the existing `sub2api:8080` fallback.
- Produces: HTTP `302` for every legacy `/ops` path and no public reverse proxy for relay admin/callback APIs.

- [ ] **Step 1: Add failing static routing assertions**

Add these exact routing checks to `tests/infra/validate-sub2api-update-routing.sh`:

```bash
require_fixed '@legacy_ops path /ops /ops/*' infra/Caddyfile
require_fixed 'redir @legacy_ops /admin/ops 302' infra/Caddyfile

if rg -n -F 'path /relay-ops/api/feishu/events' infra/Caddyfile; then
  fail 'Feishu inbound callback must not be publicly routed'
fi
if rg -n -F 'reverse_proxy @relay_ops_admin relay-ops:8100' infra/Caddyfile; then
  fail 'retired relay ops APIs must not be publicly routed'
fi
```

Replace the old positive assertion for `reverse_proxy @relay_ops_admin` with the negative contract above.

- [ ] **Step 2: Run the routing contract and verify RED**

Run:

```bash
bash tests/infra/validate-sub2api-update-routing.sh
```

Expected: `FAIL` because `@legacy_ops` and its redirect do not exist, while relay admin/callback routes are still present.

- [ ] **Step 3: Add a deployed redirect audit**

Extend `tests/infra/audit-public-links.sh` with a non-following fetch for `/ops`, separate from its existing `curl --location` helper:

```bash
ops_headers="$TEMP_DIR/ops.headers"
ops_status=$(curl --silent --show-error --max-redirs 0 \
  --output /dev/null \
  --dump-header "$ops_headers" \
  --write-out '%{http_code}' \
  "${BASE_ORIGIN}/ops")
[[ "$ops_status" == '302' ]] || fail "/ops must return 302, got $ops_status"
grep -Eiq '^location:[[:space:]]*/admin/ops([[:space:]]|$)' "$ops_headers" || \
  fail '/ops must redirect to /admin/ops'
```

- [ ] **Step 4: Implement the Caddy redirect and remove inbound proxies**

Insert this route before `@sub2api_html` and before the generic Sub2API proxy:

```caddyfile
	@legacy_ops path /ops /ops/*
	redir @legacy_ops /admin/ops 302
```

Keep the relay public surface:

```caddyfile
	@relay_ops_public {
		path /pricing /relay-ops/static/*
	}
	reverse_proxy @relay_ops_public relay-ops:8100
```

Delete both the `@relay_ops_feishu_command` and `@relay_ops_admin` matcher/proxy blocks.

- [ ] **Step 5: Run routing tests and verify GREEN**

Run:

```bash
bash tests/infra/validate-sub2api-update-routing.sh
caddy validate --config infra/Caddyfile
```

Expected: both commands pass. If `caddy` is unavailable locally, run the repository's containerized Caddy validation command and record that substitution.

- [ ] **Step 6: Commit the edge boundary**

```bash
git add infra/Caddyfile \
  tests/infra/validate-sub2api-update-routing.sh \
  tests/infra/audit-public-links.sh
git commit -m "ops: redirect legacy dashboard to native monitoring"
```

---

### Task 2: Remove the Self-Built Ops HTTP Surface

**Files:**
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Delete: `relay-ops-service/internal/http/templates/ops.html`
- Delete: `relay-ops-service/internal/http/static/ops.js`
- Delete: `relay-ops-service/internal/http/static/ops-admin.js`
- Delete: `relay-ops-service/internal/http/ops_account_quality_test.go`

**Interfaces:**
- Consumes: `httpserver.Dependencies` with pricing and retained public services.
- Produces: relay HTTP handler where `/pricing` remains available and all retired ops/ack paths naturally return `404`.

- [ ] **Step 1: Replace legacy positive tests with failing retirement tests**

In `relay-ops-service/internal/http/server_test.go`, replace tests that expect `/ops`, `ops-view`, ops static scripts, or incident ack to work with:

```go
func TestRetiredOpsAndAcknowledgementRoutesAreNotMounted(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/ops"},
		{http.MethodGet, "/relay-ops/api/ops-view"},
		{http.MethodPost, "/relay-ops/api/incidents/ack"},
		{http.MethodGet, "/relay-ops/static/ops.js"},
		{http.MethodGet, "/relay-ops/static/ops-admin.js"},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d want=404", tt.method, tt.path, recorder.Code)
		}
	}
}
```

Retain the anonymous `/pricing` and content-leak tests.

- [ ] **Step 2: Run the focused HTTP test and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/http -run 'TestRetiredOpsAndAcknowledgementRoutesAreNotMounted' -count=1 -v
```

Expected: FAIL because the routes are currently mounted.

- [ ] **Step 3: Remove ops and acknowledgement routes from `NewServer`**

Change the server dependency validation so it no longer requires `Auth`, `Ops`, or `IncidentAcknowledgements` solely for the retired page. Remove:

```go
mux.HandleFunc("GET /relay-ops/static/ops.js", s.opsScript)
mux.HandleFunc("GET /relay-ops/static/ops-admin.js", s.opsAdminScript)
mux.HandleFunc("GET /ops", s.opsBootstrap)
mux.Handle("GET /relay-ops/api/ops-view", ...)
mux.Handle("POST /relay-ops/api/incidents/ack", ...)
```

Remove `ackIncident`, `ops`, `opsBootstrap`, `opsScript`, and `opsAdminScript` when they have no callers. Remove template/static fields that exist only for the retired page. Keep CSS/template loading needed by `/pricing`.

- [ ] **Step 4: Remove application acknowledgement injection**

Delete the `incidentAcknowledgements` adapter from `internal/app/app.go`, remove `IncidentAcknowledgements` and the browser-only `Ops` dependency from the `httpserver.Dependencies` construction, and leave monitoring repositories wired to schedulers and notification services.

- [ ] **Step 5: Delete retired page assets and page-only tests**

Delete the three retired page files and `ops_account_quality_test.go`. Preserve account-quality parser/service tests outside `internal/http`.

- [ ] **Step 6: Run focused HTTP and app tests**

Run:

```bash
cd relay-ops-service
go test ./internal/http ./internal/app -count=1
```

Expected: PASS with `/pricing` still covered and retired routes returning `404`.

- [ ] **Step 7: Commit the browser control-plane retirement**

```bash
git add relay-ops-service/internal/http \
  relay-ops-service/internal/app/app.go
git commit -m "refactor: retire relay ops browser control plane"
```

---

### Task 3: Render Reminder-Only Feishu Cards

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu_test.go`
- Modify: `relay-ops-service/internal/notify/user_impact_test.go`
- Modify: `relay-ops-service/internal/notify/group_alert_test.go`
- Modify: `relay-ops-service/internal/notify/digest_v2_test.go`
- Modify: `relay-ops-service/internal/notify/retry_test.go`
- Modify: `relay-ops-service/internal/notify/one_shot_test.go`
- Modify: `relay-ops-service/internal/app/e2e_test.go`
- Modify: `relay-ops-service/internal/notify/delivery.go`
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/user_impact.go`
- Modify: `relay-ops-service/internal/notify/group_alert.go`
- Modify: `relay-ops-service/internal/notify/digest_v2.go`

**Interfaces:**
- Consumes: existing `FeishuMessage`, `CardElement`, `CardAction`, and outbound `AppClient`.
- Produces: cards with no acknowledgement semantics and one navigation-only `/admin/ops` action.

- [ ] **Step 1: Add a reusable card boundary assertion in tests**

Add this helper to an existing notify test file:

```go
func assertReminderOnlyCard(t *testing.T, message FeishuMessage) {
	t.Helper()
	text := message.RenderedText()
	for _, forbidden := range []string{
		"确认并接手", "尚未有人确认接手", "接手状态", "已接手", "处理人",
		"ack_incident", "ack_occurrence",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("card contains retired interaction %q: %s", forbidden, text)
		}
	}
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if strings.Contains(body, "ack_incident") || strings.Contains(body, "ack_occurrence") {
		t.Fatalf("card payload contains acknowledgement query: %s", body)
	}
	if !strings.Contains(body, `"url":"/admin/ops"`) {
		t.Fatalf("card payload missing native ops link: %s", body)
	}
}
```

Use it for initial alerts, progress alerts, reminders, recoveries, group availability alerts, and health digests.

- [ ] **Step 2: Add a failing delivery test**

Replace `TestWithAcknowledgementActionOnlyAddsCurrentP0P1Button` with:

```go
func TestDeliverySenderDoesNotInjectAcknowledgementAction(t *testing.T) {
	message := WithDeliveryIdentity(RenderUserImpact(UserImpactView{
		GroupName: "GPT-PLUS-内测", Severity: "P1", Headline: "部分请求持续失败",
	}), 3, "confirmed")
	repository := &recordingDeliveryRepository{}
	client := &recordingMessageClient{}
	sender := DeliverySender{Client: client, Repository: repository}
	if err := sender.SendIncident(context.Background(), "group:1", "evidence", message); err != nil {
		t.Fatal(err)
	}
	assertReminderOnlyCard(t, client.message)
}
```

Use the repository/client fakes already present in the notify test package.

- [ ] **Step 3: Run notify tests and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/notify -run 'TestDeliverySenderDoesNotInjectAcknowledgementAction|TestRenderUserImpact|TestRenderGroupAlert|TestRenderHealthDigest' -count=1 -v
```

Expected: FAIL because delivery injects the acknowledgement button, reminder copy mentions takeover state, and navigation actions still target `/ops`.

- [ ] **Step 4: Remove acknowledgement action injection**

In `delivery.go`, delete:

```go
message = WithAcknowledgementAction(message, incidentKey, occurrenceNo)
```

Delete `WithAcknowledgementAction` from `feishu.go` and remove imports used only for its query construction.

- [ ] **Step 5: Change every retained navigation action to native ops**

Use exactly:

```go
MultiURL: &CardURL{URL: "/admin/ops"}
```

Update `operationsAction`, group alerts, health digest, structured alert/recovery links, quality-report links, and truncation copy. Replace text such as:

```text
其余对象请在 /ops 查看
```

with:

```text
其余对象请在原生运维后台查看
```

- [ ] **Step 6: Remove takeover copy from periodic reminders**

Replace:

```go
"**接手状态**：尚未有人确认接手。",
```

with reminder-only evidence copy:

```go
"**提醒状态**：该异常仍在持续。",
```

Do not add assignee, acknowledgement, or completion-state wording elsewhere.

- [ ] **Step 7: Run notify and E2E tests and verify GREEN**

Run:

```bash
cd relay-ops-service
go test ./internal/notify ./internal/app -count=1
```

Expected: PASS; every new card is reminder-only and links to `/admin/ops`.

- [ ] **Step 8: Commit card behavior**

```bash
git add relay-ops-service/internal/notify \
  relay-ops-service/internal/app/e2e_test.go
git commit -m "feat: make Feishu cards reminder only"
```

---

### Task 4: Remove Acknowledgement State from Active Reminder Selection

**Files:**
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`
- Modify: `relay-ops-service/internal/alerting/escalator.go`
- Modify: `relay-ops-service/internal/alerting/escalator_test.go`
- Modify: `relay-ops-service/internal/incidents/state_machine.go` if the acknowledgement value type becomes unused

**Interfaces:**
- Consumes: active incident occurrence, most recent delivered notification, escalation cadence.
- Produces: periodic reminder claims for active incidents regardless of historical acknowledgement columns.

- [ ] **Step 1: Add a failing historical-acknowledgement independence test**

Add a store integration test that:

1. Creates an active incident occurrence.
2. Directly sets its historical `acknowledged_occurrence`, `acknowledged_at`, and `acknowledged_by` columns.
3. Advances `next_escalation_at` into the past.
4. Calls `ClaimDueEscalation`.
5. Expects the active occurrence to be returned.

The core assertion must be:

```go
if claim == nil || claim.Key != observation.Key {
	t.Fatalf("active reminder claim = %#v, want incident %q", claim, observation.Key)
}
```

- [ ] **Step 2: Run the focused store test and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/store -run 'TestActiveReminderClaimIgnoresHistoricalAcknowledgement' -count=1 -v
```

Expected: FAIL because the current SQL filters out acknowledged occurrences.

- [ ] **Step 3: Preserve the existing active-reminder repository API**

Keep the existing interface:

```go
ClaimDueEscalation(context.Context, time.Time) (*alerting.Incident, error)
```

Rename only test names or local variables that describe claims as “unacknowledged”; use “active” or “due reminder” instead. Keep occurrence number, transition, first delivery time, claim lease, and next escalation time unchanged.

- [ ] **Step 4: Remove acknowledgement predicates from reminder SQL**

Remove conditions equivalent to:

```sql
acknowledged_occurrence IS NULL
OR acknowledged_occurrence <> occurrence_no
```

from initial escalation scheduling, active reminder claims, counts, and notification retry-selection queries. Stop scanning acknowledgement columns solely to decide whether an active reminder is eligible.

Remove the incident-upsert `CASE` expressions that reset `acknowledged_occurrence`, `acknowledged_at`, or `acknowledged_by` on a new occurrence. Historical values then remain inert and are never consulted.

Do not drop or rewrite the database columns.

- [ ] **Step 5: Remove the acknowledgement writer**

Delete `Store.AcknowledgeIncident`, the `incidents.Acknowledgement` type, and focused tests that validate acknowledgement writes. Preserve incident occurrence persistence and notification delivery persistence.

- [ ] **Step 6: Run incident, alerting, and store tests**

Run:

```bash
cd relay-ops-service
go test ./internal/incidents ./internal/alerting ./internal/store -count=1
```

Expected: PASS; reminders still obey cadence and deduplication but no longer read or write acknowledgement state.

- [ ] **Step 7: Commit state-machine simplification**

```bash
git add relay-ops-service/internal/store \
  relay-ops-service/internal/alerting \
  relay-ops-service/internal/incidents
git commit -m "refactor: decouple reminders from acknowledgement state"
```

---

### Task 5: Remove Feishu Inbound Command Runtime and Callback Configuration

**Files:**
- Modify: `relay-ops-service/internal/config/config_test.go`
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/cmd/relay-ops/main.go`
- Modify: `infra/compose.yaml`
- Delete: `relay-ops-service/internal/app/feishu_commands.go`
- Delete: `relay-ops-service/internal/app/feishu_commands_test.go`
- Delete: `relay-ops-service/internal/app/command_control_test.go`
- Delete: `relay-ops-service/internal/commands/**`
- Delete: `relay-ops-service/internal/feishuevents/**`
- Delete: `relay-ops-service/internal/routingcontrol/**` if unused
- Delete: `relay-ops-service/internal/store/feishu_commands.go`
- Delete: `relay-ops-service/internal/store/feishu_commands_test.go`

**Interfaces:**
- Consumes: outbound Feishu App ID/App Secret, alert chat ID, recipients, notification policy.
- Produces: startup configuration that requires no verification token, encrypt key, command mode, routing file, callback handler, or command worker.

- [ ] **Step 1: Add failing outbound-only configuration tests**

Add:

```go
func TestLoadAcceptsOutboundAlertAppWithoutCallbackSecrets(t *testing.T) {
	t.Parallel()
	env := validEnv(t)
	appID, appSecret, chatID, recipients := addOutboundFeishuAlertFiles(t, env)
	policy := addNotificationPolicy(t, env)
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("outbound-only alert configuration rejected: %v", err)
	}
	if cfg.FeishuAppIDFile != appID || cfg.FeishuAppSecretFile != appSecret ||
		cfg.FeishuAlertChatIDFile != chatID ||
		cfg.FeishuAlertRecipientsFile != recipients ||
		cfg.NotificationPolicyFile != policy {
		t.Fatalf("outbound Feishu config = %#v", cfg)
	}
}
```

The helper must set only:

- `RELAY_OPS_FEISHU_APP_ID_FILE`
- `RELAY_OPS_FEISHU_APP_SECRET_FILE`
- `RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE`
- `RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE`
- `RELAY_OPS_NOTIFICATION_POLICY_FILE`

It must not create verification-token, encrypt-key, routing, or command-mode files.

- [ ] **Step 2: Run config test and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/config -run 'TestLoadAcceptsOutboundAlertAppWithoutCallbackSecrets' -count=1 -v
```

Expected: FAIL because alert chat currently requires a complete callback file set.

- [ ] **Step 3: Simplify `Config` to outbound fields**

Remove:

```go
FeishuCommandMode
FeishuVerificationFile
FeishuEncryptKeyFile
FeishuRoutingFile
```

Remove command-mode constants and validation. Validate App ID and App Secret as an all-or-none pair. Require the pair when `FeishuAlertChatIDFile` is configured. Keep the chat/recipients pairing and notification-policy requirement.

- [ ] **Step 4: Remove callback wrapping from process startup**

Change `cmd/relay-ops/main.go` from:

```go
commandRuntime, err := app.ConfigureFeishuCommandsForStore(cfg, application.Store, application.Handler)
```

to serving `application.Handler` directly. Delete command-worker startup/shutdown branches.

- [ ] **Step 5: Remove inbound packages and store adapters**

Delete the inbound command runtime, decoder/verifier, route-control command execution, callback-specific store adapter, and their tests. Retain migrations as historical schema.

Delete `routingcontrol` together with the command runtime; it has no remaining production caller.

- [ ] **Step 6: Remove callback-only Compose configuration**

Delete these environment variables and mounts from `infra/compose.yaml`:

```text
RELAY_OPS_FEISHU_COMMAND_MODE
RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE
RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE
RELAY_OPS_FEISHU_ROUTING_FILE
RELAY_OPS_FEISHU_VERIFICATION_TOKEN_HOST_FILE
RELAY_OPS_FEISHU_ENCRYPT_KEY_HOST_FILE
RELAY_OPS_FEISHU_ROUTING_HOST_FILE
```

Keep App ID/App Secret, alert chat ID, recipients, webhook, and notification policy configuration used by outbound delivery.

- [ ] **Step 7: Run configuration and process tests**

Run:

```bash
cd relay-ops-service
go test ./internal/config ./internal/app ./internal/feishuapi ./cmd/relay-ops -count=1
```

Expected: PASS without any inbound callback configuration.

- [ ] **Step 8: Commit outbound-only runtime**

```bash
git add relay-ops-service/cmd/relay-ops \
  relay-ops-service/internal \
  infra/compose.yaml
git commit -m "refactor: remove Feishu inbound command runtime"
```

---

### Task 6: Update Current Documentation and Run Full Verification

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Modify: current operational scripts that assert `/ops` returns `200`
- Create: `docs/superpowers/reports/2026-07-30-native-ops-redirect-and-reminder-only-feishu-verification.md`

**Interfaces:**
- Consumes: completed code and test results.
- Produces: current-state documentation and reproducible verification evidence.

- [ ] **Step 1: Update current-state documentation**

Document these exact current contracts:

```text
/ops is retired and returns 302 to /admin/ops.
relay-ops no longer exposes an ops browser/API control plane.
Feishu is outbound-only: alerts, recoveries, reminders, and daily reports.
Feishu cards contain only a navigation link to /admin/ops and cannot mutate state.
```

Do not alter historical reports that accurately describe earlier deployments.

- [ ] **Step 2: Update smoke expectations**

For current deployment/smoke scripts, replace assertions that `/ops` returns `200` with:

```bash
status=$(curl -sS -o /dev/null -D "$headers" -w '%{http_code}' "$BASE_URL/ops")
[[ "$status" == "302" ]]
grep -Eiq '^location:[[:space:]]*/admin/ops([[:space:]]|$)' "$headers"
```

Add unauthenticated checks that the three retired relay endpoints return `404`.

- [ ] **Step 3: Run formatting and static checks**

Run:

```bash
cd relay-ops-service
gofmt -w $(find cmd internal -type f -name '*.go')
go vet ./...
```

Expected: clean exit with no diagnostics.

- [ ] **Step 4: Run the full relay-ops test suite**

Run:

```bash
cd relay-ops-service
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 5: Run infrastructure contracts**

Run from repository root:

```bash
bash tests/infra/validate-sub2api-update-routing.sh
docker compose --env-file infra/.env -f infra/compose.yaml config >/dev/null
```

Expected: PASS.

- [ ] **Step 6: Scan for retired behavior**

Run:

```bash
rg -n -S \
  '确认并接手|尚未有人确认接手|接手状态|ack_incident|ack_occurrence|/relay-ops/api/feishu/events|/relay-ops/api/incidents/ack|/relay-ops/api/ops-view' \
  relay-ops-service infra \
  --glob '!internal/store/migrations/**'
```

Expected: no production-code or active-infrastructure matches. Historical migrations may still contain acknowledgement columns.

Also run:

```bash
rg -n -S 'URL: "/ops"|URL:"/ops"|其余对象请在 /ops 查看' relay-ops-service/internal/notify
```

Expected: no matches.

- [ ] **Step 7: Write the verification report**

Record:

- exact test commands and exit results;
- changed route behavior;
- confirmation that outbound notification tests still cover alert, reminder, recovery, daily digest, and deduplication;
- confirmation that the user's unrelated `监控日报-2026-07-28.md` remains unstaged.

- [ ] **Step 8: Commit docs and verification evidence**

```bash
git add docs/project/current-state.md \
  docs/project/llm-handoff.md \
  docs/superpowers/reports/2026-07-30-native-ops-redirect-and-reminder-only-feishu-verification.md \
  tests
git commit -m "docs: record native ops and reminder-only Feishu"
```

---

## Final Review Checklist

- [ ] `/ops`, `/ops/`, and an `/ops/*` sample are covered by the Caddy redirect matcher.
- [ ] Caddy no longer sends any relay admin or Feishu callback API to `relay-ops`.
- [ ] The relay HTTP server itself returns `404` for retired routes.
- [ ] No new Feishu payload contains acknowledgement query parameters or takeover wording.
- [ ] Every retained ops button targets `/admin/ops`.
- [ ] Active reminders still repeat on cadence until recovery, independent of historical acknowledgement fields.
- [ ] Outbound App Bot delivery starts with App ID/App Secret only; verification/encryption/routing command secrets are unnecessary.
- [ ] Full Go tests, vet, routing contracts, and Compose validation pass.
- [ ] Historical database migrations and records are preserved.
- [ ] `监控日报-2026-07-28.md` remains outside all commits.
