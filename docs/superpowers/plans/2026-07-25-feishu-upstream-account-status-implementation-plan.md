# Feishu Upstream Account Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make relay-ops and Feishu consume the native Sub2API account-monitor projection and publish a beautiful, read-only “上游账号情况” card with deterministic “B 账号综合更佳” recommendations.

**Architecture:** Add a strict native projection reader to relay-ops, pass that projection through a versioned deterministic analyzer, and render it with the existing Feishu interactive-card renderer. The analyzer is also available as a server-side Ruby script for scheduled/manual inspection; neither path has a write-capable Sub2API dependency.

**Tech Stack:** Go, existing relay-ops scheduler/Feishu card renderer, Ruby 3 standard library, JSON, Docker build contracts, Go/Ruby tests.

## Global Constraints

- Read the same native Sub2API account-monitor result used by the admin page.
- Never run a second account probe from relay-ops.
- Consider only `active + schedulable` accounts bound to the relevant group.
- Use weights: stability 40%, TTFT/total latency 25%, multiplier 20%, usage/recent-load headroom 15%.
- Require fresh evidence, sufficient samples, compatible model, and a minimum improvement margin.
- Recommendations are read-only and never call route, account, price, multiplier, key, or balance mutation APIs.
- Feishu cards use native interactive-card JSON, preserve escaping, abnormal-row priority, and the existing 30 KiB limit.
- Do not include secrets, raw upstream responses, Base URLs, generated text, or local `sub` data.

---

### Task 1: Add the native account-monitor projection reader

**Files:**
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go` with projection
  decoding and rejection cases for the new reader contract.

**Interfaces:**
- Add `Reader.ListAccountMonitors(context.Context) (AccountMonitorProjection, error)`.
- Add `AccountMonitorProjection` with schema version, observed time, stale flag,
  settings, groups, and secret-free account rows.
- Add `AccountMonitorAccount` fields:
  `AccountID`, `Name`, `Status`, `Schedulable`, `GroupIDs`, `GroupNames`,
  `ModelID`, `LatestStatus`, `ErrorCode`, `SampleCount`, `SuccessRate`,
  `TTFTP50MS`, `TTFTP95MS`, `LatencyP95MS`, `Multiplier`,
  `RequestCount`, `ErrorCount`, `UsageWindows`, `CheckedAt`, `Stale`.

- [ ] **Step 1: Write failing HTTP reader tests**

Serve a fixture for `GET /api/v1/admin/account-monitors` and assert the client
decodes the exact projection, preserves nullable metrics, rejects missing
schema/version fields, rejects unknown secret-shaped keys, and returns a
schema mismatch error for missing `accounts`.

- [ ] **Step 2: Run reader tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/sub2api -run 'TestHTTPReader.*AccountMonitor' -count=1
```

Expected: FAIL because the reader method and projection types are absent.

- [ ] **Step 3: Implement strict projection decoding**

Add a bounded response body, `json.Decoder.DisallowUnknownFields`, explicit
version validation, UTC timestamp parsing, and a secret-field scan matching the
existing account-quality reader policy. Use the existing `HTTPReader.get`
authentication and timeout path.

- [ ] **Step 4: Run reader tests**

```bash
cd relay-ops-service
go test ./internal/sub2api -run 'TestHTTPReader.*AccountMonitor' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/sub2api/types.go \
  relay-ops-service/internal/sub2api/client.go \
  relay-ops-service/internal/sub2api/client_test.go
git commit -m "feat: read native account monitor projection"
```

### Task 2: Implement deterministic account recommendation analysis

**Files:**
- Create: `ops/analyze-account-monitor.rb`
- Create: `tests/operations/analyze_account_monitor_test.rb`
- Create: `relay-ops-service/internal/accountrecommendation/service.go`
- Create: `relay-ops-service/internal/accountrecommendation/service_test.go`

**Interfaces:**
- Ruby CLI: `ruby ops/analyze-account-monitor.rb analyze --input <json> --output <json>`.
- Go: `accountrecommendation.Analyze(AccountMonitorProjection) Result`.
- Result fields: `GroupID`, `GroupName`, `CurrentAccountID`, `CandidateAccountID`,
  `Decision`, `ScoreDelta`, `Reasons`, `EvidenceState`.

- [ ] **Step 1: Write failing analyzer tests**

Cover:

```ruby
assert_equal "candidate_better", result.fetch("groups").first.fetch("decision")
assert_includes result.fetch("groups").first.fetch("reasons"), "稳定性更高"
```

Also cover current-account retention, stale evidence, insufficient samples,
incompatible model, inactive/unschedulable candidate, no current account,
tie within the minimum margin, multiplier normalization, and stable output
ordering.

- [ ] **Step 2: Run analyzer tests and verify RED**

```bash
ruby tests/operations/analyze_account_monitor_test.rb
cd relay-ops-service
go test ./internal/accountrecommendation -count=1
```

Expected: FAIL because the script and Go package do not exist.

- [ ] **Step 3: Implement the shared scoring rules**

Use a deterministic normalized score:

```text
total =
  0.40 * stability_score +
  0.25 * performance_score +
  0.20 * multiplier_score +
  0.15 * headroom_score
```

Require fresh evidence, at least three samples, a compatible model, no recent
consecutive failure, and `score_delta >= 0.05` before returning
`candidate_better`. Emit `current_ok` or `insufficient_evidence` otherwise.
Keep reasons in Chinese and include the compared metrics.

- [ ] **Step 4: Implement the Ruby CLI contract**

Read only JSON from the native projection, reject credentials/raw response
keys, sort groups and accounts deterministically, write JSON atomically, and
print one concise Chinese summary per group. Exit non-zero for malformed or
secret-bearing input; never mutate Sub2API state.

- [ ] **Step 5: Run analyzer tests**

```bash
ruby tests/operations/analyze_account_monitor_test.rb
cd relay-ops-service
go test ./internal/accountrecommendation -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ops/analyze-account-monitor.rb \
  tests/operations/analyze_account_monitor_test.rb \
  relay-ops-service/internal/accountrecommendation
git commit -m "feat: analyze account monitor recommendations"
```

### Task 3: Replace Feishu quality input with native account status

**Files:**
- Modify: `relay-ops-service/internal/dailyreport/service.go`
- Modify: `relay-ops-service/internal/dailyreport/service_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`
- Modify: `relay-ops-service/internal/config/config.go` to define the
  read-only analyzer command path used by the daily-report service.

**Interfaces:**
- `dailyreport.Service` consumes a native `sub2api.Reader` account-monitor
  projection and an `accountrecommendation.Analyzer`.
- The daily report keeps existing runtime/candidate/incident sections and adds
  an `UpstreamAccountStatus` field with analysis output.

- [ ] **Step 1: Write failing daily-report tests**

Assert the report calls the native reader exactly once, does not call the
legacy account-quality probe source, marks native stale/mismatched evidence as
unavailable, and carries one recommendation per eligible group into the
notification view.

- [ ] **Step 2: Run daily-report tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/dailyreport ./internal/app -run 'AccountMonitor|AccountQuality' -count=1
```

Expected: FAIL because the service has no native projection dependency.

- [ ] **Step 3: Wire the native reader and analyzer**

Keep the existing scheduler cadence and incident deduplication. Read native
account monitor data after the runtime snapshot, run the deterministic
analyzer in-process, and pass a secret-free view to the Feishu renderer.
Preserve the existing file source only for unrelated legacy paths; do not
probe accounts twice.

- [ ] **Step 4: Run daily-report and app tests**

```bash
cd relay-ops-service
go test ./internal/dailyreport ./internal/app -run 'AccountMonitor|AccountQuality' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/dailyreport \
  relay-ops-service/internal/app \
  relay-ops-service/internal/config
git commit -m "feat: feed Feishu from native account monitor"
```

### Task 4: Render the native Feishu interactive card

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/feishu_test.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html` to expose the
  configured admin account-monitor URL used by the Feishu action button.

**Interfaces:**
- Extend `notify.OperationsDigestView` with `UpstreamAccountStatus`.
- Add `notify.RenderUpstreamAccountStatus(...)` or make the existing
  `RenderOperationsDigest` render the new section through a focused helper.

- [ ] **Step 1: Write failing card tests**

Assert the serialized card contains:

```text
上游账号情况
B 账号综合更佳
成功率
TTFT
倍率
用量窗口
账号监控
```

Cover healthy/current-ok, recommendation, stale, insufficient evidence,
failed account, HTML/Markdown escaping, deterministic group order, abnormal
row retention, and the 30 KiB encoded-card bound.

- [ ] **Step 2: Run card tests and verify RED**

```bash
cd relay-ops-service
go test ./internal/notify -run 'TestRender.*Account|TestRenderOperationsDigest' -count=1
```

Expected: FAIL because the new view and card section do not exist.

- [ ] **Step 3: Implement the native card layout**

Use the existing interactive-card JSON types. Add a blue header, overall
status block, per-group current/candidate rows, green recommendation blocks,
orange/red evidence warnings, and the native admin-page action button. Keep
the existing `fitDigestSection` budget and include all abnormal/recommended
groups before ordinary groups.

- [ ] **Step 4: Run card tests and full notify regression**

```bash
cd relay-ops-service
go test ./internal/notify -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/notify/feishu.go \
  relay-ops-service/internal/notify/feishu_test.go \
  relay-ops-service/internal/http/templates/ops.html
git commit -m "feat: render native upstream account Feishu card"
```

### Task 5: Update deployment contracts and verify the full Feishu path

**Files:**
- Modify: `infra/Dockerfile.relay-ops`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Test: `relay-ops-service/internal/sub2api/*_test.go`
- Test: `relay-ops-service/internal/accountrecommendation/*_test.go`
- Test: `relay-ops-service/internal/dailyreport/*_test.go`
- Test: `relay-ops-service/internal/notify/*_test.go`
- Test: `tests/operations/analyze_account_monitor_test.rb`

- [ ] **Step 1: Write the deployment contract assertion**

Require the analyzer script to be copied into the relay-ops image, require no
new write-capable Sub2API endpoint, and require the native account-monitor API
path in the client contract fixture.

- [ ] **Step 2: Run the contract test and verify RED**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: FAIL until the Docker copy and contract assertions are updated.

- [ ] **Step 3: Update the relay-ops image contract**

Copy `ops/analyze-account-monitor.rb` into the existing `/app/ops` directory
in `infra/Dockerfile.relay-ops`. Keep secrets mounted read-only and do not add
any runtime volume that can mutate Sub2API data.

- [ ] **Step 4: Run all focused and contract tests**

```bash
cd relay-ops-service
go test ./... -count=1
cd ..
ruby tests/operations/analyze_account_monitor_test.rb
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: PASS, with existing Feishu command dry-run/read-only contracts
unchanged.

- [ ] **Step 5: Commit**

```bash
git add infra/Dockerfile.relay-ops tests/relay_ops/validate_relay_ops_contract.sh
git commit -m "test: verify native account status Feishu integration"
```
