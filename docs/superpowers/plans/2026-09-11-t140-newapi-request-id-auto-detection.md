# T140 NewAPI Request ID Auto-Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically capture `X-Oneapi-Request-Id` for conservatively eligible OpenAI API-key relays while preserving strict Sub2API-native-first multiplier synchronization.

**Architecture:** Extend the existing response-header selector without persisting inferred configuration. Keep NewAPI multiplier registration in the existing post-usage registrar, but require an explicit native billing `unsupported` snapshot before NewAPI may write a multiplier; native `ok`, transient failures, and unresolved states remain authoritative blockers.

**Tech Stack:** Go, `net/http`, `net/url`, existing Sub2API service models and focused Go tests.

**Spec:** `docs/superpowers/specs/2026-09-11-t140-newapi-request-id-auto-detection-design.md`

## Global Constraints

- Explicit `extra.upstream_request_id_header` always wins.
- Automatic inference applies only to OpenAI API-key accounts.
- A non-official custom BaseURL may enable request-ID capture but must not establish NewAPI billing identity.
- NewAPI multiplier writes require native billing probe status `unsupported`; `ok`, failed, unavailable, or missing snapshots block writes.
- Existing sync-enabled and manual-mode ownership gates remain unchanged.
- No database migration, bulk account update, history backfill, new API, new setting, or production write.
- Deployment and production verification are outside candidate implementation and require explicit authorization; verification uses SSH/config/log/database evidence, not page inspection.

---

### Task 1: Automatic OneAPI Response Header Selection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_request_id.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_request_id_test.go`

**Interfaces:**
- Consumes: `Account.Platform`, `Account.Type`, `Account.GetCredential("base_url")`, `newAPIRateRegistrationIdentity(*Account)`.
- Produces: `isAutomaticOneAPIRequestIDEligible(account *Account) bool`, `isCustomOpenAIBaseURL(account *Account) bool`, and updated `UpstreamRequestIDHeaderName(account *Account) string`.

- [ ] **Step 1: Write failing table tests for inferred header selection**

Add cases that call the real `UpstreamRequestIDHeaderName` and `UpstreamRequestIDFromHeaders` functions:

```go
func TestUpstreamRequestIDHeaderNameInfersOneAPIConservatively(t *testing.T) {
    tests := []struct {
        name    string
        account *Account
        want    string
    }{
        {name: "custom OpenAI API key base URL", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1"}}, want: "X-Oneapi-Request-Id"},
        {name: "official OpenAI base URL", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://api.openai.com/v1"}}},
        {name: "empty base URL", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}},
        {name: "OpenAI OAuth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://relay.example/v1"}}},
        {name: "non OpenAI platform", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1"}}},
        {name: "invalid base URL", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "://bad"}}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            require.Equal(t, tt.want, UpstreamRequestIDHeaderName(tt.account))
        })
    }
}
```

Also add focused cases proving a trusted `account_monitor_balance.source=newapi` or an `unsupported` native snapshot selects the OneAPI header, and that a configured `X-Custom-Request-Id` overrides inference.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestUpstreamRequestID(HeaderNameInfersOneAPIConservatively|FromHeaders)' -count=1
```

Expected: the custom BaseURL and trusted-identity cases fail because unconfigured accounts currently return an empty header name.

- [ ] **Step 3: Implement minimal conservative inference**

In `upstream_request_id.go`:

```go
const newAPIUpstreamRequestIDHeader = "X-Oneapi-Request-Id"

func UpstreamRequestIDHeaderName(account *Account) string {
    if account == nil {
        return ""
    }
    if explicit := strings.TrimSpace(account.GetExtraString(AccountExtraUpstreamRequestIDHeader)); explicit != "" {
        return explicit
    }
    if isAutomaticOneAPIRequestIDEligible(account) {
        return newAPIUpstreamRequestIDHeader
    }
    return ""
}
```

Implement `isCustomOpenAIBaseURL` with `url.Parse`, require `http` or `https`, a nonempty hostname, and reject hostname `api.openai.com` case-insensitively. `isAutomaticOneAPIRequestIDEligible` must require OpenAI plus API Key, then accept either `newAPIRateRegistrationIdentity(account)` or `isCustomOpenAIBaseURL(account)`.

- [ ] **Step 4: Run request-ID tests and verify GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(UpstreamRequestID|UsageUpstreamRequestIDPtr|ValidateUpstreamRequestIDHeaderExtra)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Format and commit Task 1**

```bash
gofmt -w upstream/sub2api/backend/internal/service/upstream_request_id.go upstream/sub2api/backend/internal/service/upstream_request_id_test.go
git add upstream/sub2api/backend/internal/service/upstream_request_id.go upstream/sub2api/backend/internal/service/upstream_request_id_test.go
git commit -m "feat: infer NewAPI upstream request ids"
```

### Task 2: Enforce Native Billing Resolution Before NewAPI Rate Writes

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- Test: `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
- Test: `upstream/sub2api/backend/internal/service/usage_cost_evidence_test.go`

**Interfaces:**
- Consumes: `decodeUpstreamBillingProbeSnapshot`, `UpstreamBillingProbeStatusUnsupported`, existing `newAPIRateRegistrationIdentity`, sync and manual-mode gates.
- Produces: `newAPIRateMultiplierRegistrationEligible` that returns true only when native billing is explicitly unsupported.

- [ ] **Step 1: Write failing eligibility tests for unresolved native states**

Refactor the existing eligibility table so each case owns its account fixture. Add explicit cases for:

```go
{name: "NewAPI balance identity without native unsupported is blocked", probeStatus: "", balanceSource: AccountMonitorBalanceSourceNewAPI, want: false}
{name: "native unsupported permits exact NewAPI record", probeStatus: UpstreamBillingProbeStatusUnsupported, want: true}
{name: "native ok blocks NewAPI", probeStatus: UpstreamBillingProbeStatusOK, want: false}
{name: "native failed blocks NewAPI", probeStatus: UpstreamBillingProbeStatusFailed, want: false}
```

Keep existing cases for missing ID, OAuth, refund, fuzzy match, manual mode, and disabled sync.

- [ ] **Step 2: Run the focused eligibility test and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestNewAPIUsageRecordEligibilityRequiresExactSuccessfulNewAPIUsage$' -count=1
```

Expected: the NewAPI balance-only case fails because current code accepts identity without requiring the native snapshot to be `unsupported`.

- [ ] **Step 3: Add the explicit native-unsupported gate**

Change `newAPIRateMultiplierRegistrationEligible` after the sync/manual checks:

```go
snapshot := decodeUpstreamBillingProbeSnapshot(usage.Account.Extra)
if snapshot == nil || snapshot.Status != UpstreamBillingProbeStatusUnsupported {
    return false
}
if !newAPIRateRegistrationIdentity(usage.Account) {
    return false
}
```

Do not treat probe failures, missing snapshots, or balance identity alone as fallback authorization.

- [ ] **Step 4: Add an integration regression for native-first registration**

In `usage_cost_evidence_test.go`, retain the existing successful `unsupported` registration test and add a sibling test whose account has a NewAPI balance snapshot but native probe status `ok` or absent. Assert the real registrar performs zero `/api/log/token` requests and produces no `CompleteNewAPIRateRefresh` call.

- [ ] **Step 5: Run rate-registration tests and verify GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(NewAPIUsageRecordEligibilityRequiresExactSuccessfulNewAPIUsage|UsageCostEvidenceRegistrarReusesNewAPILogForRateRegistration|UsageCostEvidenceRegistrar.*Native)' -count=1
```

Expected: PASS, with exactly one NewAPI lookup only in the native-unsupported case.

- [ ] **Step 6: Format and commit Task 2**

```bash
gofmt -w upstream/sub2api/backend/internal/service/sub_upstream_cost.go upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go upstream/sub2api/backend/internal/service/usage_cost_evidence_test.go
git add upstream/sub2api/backend/internal/service/sub_upstream_cost.go upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go upstream/sub2api/backend/internal/service/usage_cost_evidence_test.go
git commit -m "fix: keep native billing ahead of NewAPI rates"
```

### Task 3: Focused Verification and Candidate Handoff

**Files:**
- Create: `docs/superpowers/reports/2026-09-11-t140-newapi-request-id-auto-detection-implementation.md`
- Create: `docs/handoffs/2026-09-11-t140-newapi-request-id-auto-detection-handoff.md`

**Interfaces:**
- Consumes: completed Task 1 and Task 2 commits.
- Produces: a reviewable candidate with test evidence and no deployment action.

- [ ] **Step 1: Run focused service tests**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(UpstreamRequestID|UsageUpstreamRequestIDPtr|ValidateUpstreamRequestIDHeaderExtra|NewAPIUsageRecordEligibilityRequiresExactSuccessfulNewAPIUsage|UsageCostEvidenceRegistrar)' -count=1
```

- [ ] **Step 2: Run required compile checks**

```bash
cd upstream/sub2api/backend
go build ./internal/service
go build ./cmd/server
```

If an existing unrelated mainline compile failure appears, record the exact symbol and confirm it reproduces on the baseline; do not alter unrelated files.

- [ ] **Step 3: Run formatting and scope checks**

```bash
gofmt -w upstream/sub2api/backend/internal/service/upstream_request_id.go upstream/sub2api/backend/internal/service/upstream_request_id_test.go upstream/sub2api/backend/internal/service/sub_upstream_cost.go upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go upstream/sub2api/backend/internal/service/usage_cost_evidence_test.go
git diff --check
git diff --name-status origin/main...HEAD
git status --short
```

Confirm no migration, dependency, workflow, deployment, credential, production data, or unrelated frontend file changed.

- [ ] **Step 4: Write implementation report and handoff**

Record baseline SHA, candidate SHA/tree, changed files, RED/GREEN commands, focused tests, build results, migration/config status, `downtime_required=false`, rollback as Git revert or previous blue-green slot, and the remaining requirement for SSH-based online verification after an authorized deployment.

- [ ] **Step 5: Commit documentation**

```bash
git add docs/superpowers/specs/2026-09-11-t140-newapi-request-id-auto-detection-design.md docs/superpowers/plans/2026-09-11-t140-newapi-request-id-auto-detection.md docs/superpowers/reports/2026-09-11-t140-newapi-request-id-auto-detection-implementation.md docs/handoffs/2026-09-11-t140-newapi-request-id-auto-detection-handoff.md
git commit -m "docs: hand off NewAPI request id detection"
```

Stop at `READY_FOR_ROOT_REVIEW`. Do not merge to `main`, push, modify production accounts, or deploy without the root release controller and explicit production authorization.
