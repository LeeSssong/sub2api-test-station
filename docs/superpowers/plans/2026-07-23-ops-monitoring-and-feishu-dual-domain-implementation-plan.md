# 运维监测页与飞书双区监控实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 复用 Sub2API 原生聚合和动态 active && schedulable 账号集合，完成管理员 /ops 的站内运行双区、账号质量投影、飞书双区日报以及 15 分钟告警/恢复闭环。

**Architecture:** 扩展现有 Sub2API reader 的只读查询参数，新增纯数据 opsmetrics 聚合层供 HTTP 页面、日报和告警共同使用；沿用 incidents.Machine 做去重、确认窗口和恢复；沿用现有 Interactive Card App Bot/Webhook 发送器。Sub2API 原生 /monitor 不改，relay-ops 不执行任何写操作。

**Tech Stack:** Go 1.24, net/http, embedded HTML/CSS/JS, Sub2API v0.1.161 Admin API, existing incident store, Feishu Interactive Card.

## Global Constraints

- 当前上游只由 Sub2API active && schedulable 账号集合定义，不写死供应商、账号或 Base URL。
- /monitor 保持 Sub2API 原生实现；/ops 和 /relay-ops/api/ops-view 继续隐藏管理员鉴权，匿名返回 404。
- relay-ops 保持 read_only，飞书命令保持 dry_run；不修改路由、倍率、价格、余额、Key、候选、probe 或数据库业务数据。
- 15 分钟窗口少于 20 个请求显示“样本不足”；错误率 >=5% 和 TTFT P95 恶化 >=30% 且绝对值 >3s 均需连续两个窗口。
- 完全失败、暂停调度、倍率变化和明确 balance_exhausted 立即告警；恢复需一个完整健康窗口。
- 账号质量证据沿用现有 20 分钟过期门限；缺失值显示明确状态，不显示伪造的 0%/0ms。
- 正常巡检不逐条发送飞书消息，不制造合成故障，不删除去重记录。
- 不读取、输出或持久化生产密钥、Cookie、模型响应正文或完整用户身份。

---

### Task 1: Extend Native Sub2API Account-Scoped Queries

**Files:**
- Modify: relay-ops-service/internal/sub2api/types.go
- Modify: relay-ops-service/internal/sub2api/client.go
- Test: relay-ops-service/internal/sub2api/client_test.go

**Interfaces:**
- OpsQuery.AccountID int64 and UsageQuery.AccountID int64 are optional positive filters.
- HTTPReader.GetOpsSnapshot and GetUsageStats send account_id only when AccountID > 0.

- [ ] Step 1: Write a failing fake-server test that calls both methods with AccountID 42 and asserts the recorded query value equals 42.
```go
func TestReaderAddsAccountIDFilterToNativeAggregates(t *testing.T) {
    // fake server returns minimal valid snapshot/stats and records r.URL.Query().Get("account_id")
    // call GetOpsSnapshot(..., OpsQuery{TimeRange: "15m", AccountID: 42})
    // call GetUsageStats(..., UsageQuery{Period: "24h", AccountID: 42})
    // assert both recorded values are "42"
}
```

- [ ] Step 2: Run the focused test and verify RED.

Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/sub2api -run TestReaderAddsAccountIDFilterToNativeAggregates -count=1'
```
Expected: FAIL because account_id is absent.

- [ ] Step 3: Add the fields and set URL parameters only for positive IDs:
```go
if query.AccountID > 0 {
    values.Set("account_id", strconv.FormatInt(query.AccountID, 10))
}
```
Keep existing schema checks and headers unchanged.

- [ ] Step 4: Run focused and package tests:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/sub2api -count=1'
```

- [ ] Step 5: Commit the three files with message feat: query native ops metrics by account.

### Task 2: Build Shared Site Runtime Projection

**Files:**
- Create: relay-ops-service/internal/opsmetrics/snapshot.go
- Test: relay-ops-service/internal/opsmetrics/snapshot_test.go
- Modify: relay-ops-service/internal/http/server.go
- Modify: relay-ops-service/internal/http/sources.go
- Test: relay-ops-service/internal/http/sources_test.go

**Interfaces:**
- opsmetrics.Reader consumes ListGroups, ListAccounts, and GetOpsSnapshot.
- Collect(ctx, reader, now) returns stable public-group and active/schedulable-account rows.
- Each row contains request count, error rate, SLA, TTFT P95, duration P95, Status, ErrorCode, and evidence hash.
- Per-object aggregate failure becomes read_failed; account/group list failure remains a source error.
- DatabaseOpsSource exposes the result as OpsView.SiteRuntime.

- [ ] Step 1: Write a fake-reader test with two public groups and three accounts: one disabled, one unschedulable, one active. Return 20 requests for one object, 10 for another, and an error for one account. Assert only the public groups and active account are present, sample_insufficient is used below 20, read_failed is used for the account error, and each row has an evidence hash.
- [ ] Step 2: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/opsmetrics -run TestCollect -count=1'
```
Expected: FAIL because the package does not exist.
- [ ] Step 3: Implement GroupRuntime, AccountRuntime, Snapshot, and Collect. Call GetOpsSnapshot with TimeRange 15m and GroupID or AccountID. Map fewer than 20 requests to sample_insufficient; successful rows to ok; errors to read_failed without copying error text.
- [ ] Step 4: Add OpsView.SiteRuntime and call the collector from DatabaseOpsSource.Snapshot while preserving D04 readiness and account-quality projections.
- [ ] Step 5: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/opsmetrics ./internal/http -run "TestCollect|TestDatabaseOpsSource" -count=1'
```
- [ ] Step 6: Commit with message feat: project native site runtime metrics.

### Task 3: Update the Admin /ops View

**Files:**
- Modify: relay-ops-service/internal/http/templates/ops.html
- Modify: relay-ops-service/internal/http/static/app.css
- Modify: relay-ops-service/internal/http/templates/ops-bootstrap.html
- Test: relay-ops-service/internal/http/server_test.go
- Test: relay-ops-service/internal/http/model_release_test.go

**Interfaces:**
- Template consumes OpsView.SiteRuntime and never renders credentials or write controls.
- /monitor remains a link to Sub2API native page.
- Auto-refresh keeps existing 30-second behavior and hidden 404 behavior.

- [ ] Step 1: Add failing HTML assertions requiring 站内运行、公开分组、当前调度账号、错误率、TTFT P95、总耗时 P95 and 读取失败, and forbidding Base URL, API Key, 切换上游 and 确认切换.
- [ ] Step 2: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/http -run "TestOps|Test.*HTML" -count=1'
```
Expected: FAIL because the current template has no site-runtime section.
- [ ] Step 3: Insert a full-width 站内运行 section before D04 readiness. Render group and account tables with explicit empty/error/sample statuses. Keep the existing account-quality section as the separate 上游账号质量 section and keep technical details collapsed.
- [ ] Step 4: Add scoped responsive table styles and status colors for ok, sample_insufficient, read_failed and paused; do not alter Sub2API /monitor assets.
- [ ] Step 5: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/http -count=1'
node --check relay-ops-service/internal/http/static/ops.js
node --check relay-ops-service/internal/http/static/ops-admin.js
```
Expected: PASS and anonymous API remains hidden 404.
- [ ] Step 6: Commit with message feat: show site runtime metrics in admin ops.

### Task 4: Implement the Dual-Domain Feishu Digest

**Files:**
- Modify: relay-ops-service/internal/notify/feishu.go
- Test: relay-ops-service/internal/notify/feishu_test.go
- Modify: relay-ops-service/internal/dailyreport/service.go
- Modify: relay-ops-service/internal/dailyreport/service_test.go
- Modify: relay-ops-service/internal/app/app.go

**Interfaces:**
- Add notify.OperationsDigestView and RenderOperationsDigest.
- dailyreport.Service builds the digest from the shared runtime collector and existing account-quality source; candidate/incident/analysis evidence remains a compact footer.
- Existing alert/recovery/command renderers remain backward compatible.

- [ ] Step 1: Add a failing card test constructing one site group, one active account and one quality row. Assert CardJSON contains 站内运行, 公开分组, 当前调度账号, 上游账号质量, 倍率, TTFT P95 and 运维后台 in order; assert Base URL, api_key, model response text and full user identity never appear.
- [ ] Step 2: Run focused RED:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/notify -run TestRenderOperationsDigest -count=1'
```
- [ ] Step 3: Add separate card elements for the two domains, explicit sample-insufficient/read-failed/stale labels, stable order, /ops action, and existing 30 KB limit. Card serialization failure must not send a text fallback.
- [ ] Step 4: Add the shared runtime collector and account-quality source to dailyreport.Service. Replace the current long Markdown-only body with RenderOperationsDigest while preserving Shanghai-date identity, incident deduplication, Agent fallback, and one notification per date.
- [ ] Step 5: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/notify ./internal/dailyreport -count=1'
```
- [ ] Step 6: Commit with message feat: send dual-domain Feishu operations digest.

### Task 5: Add 15-Minute Site Alert and Recovery Evaluation

**Files:**
- Create: relay-ops-service/internal/opsmonitor/service.go
- Test: relay-ops-service/internal/opsmonitor/service_test.go
- Modify: relay-ops-service/internal/scheduler/scheduler.go
- Test: relay-ops-service/internal/scheduler/scheduler_test.go
- Modify: relay-ops-service/internal/app/app.go

**Interfaces:**
- opsmonitor.Service.Run(context.Context) error reads 15-minute and 24-hour native snapshots plus account-quality result.
- It uses incidents.Machine for persistence and notify.MessageSender for only confirmed alert/recovery transitions.
- It never calls a write endpoint or sends a message for sample-insufficient windows.

- [ ] Step 1: Write evaluator tests for two-window error rate, sample-insufficient no-alert, paused account immediate alert, multiplier/balance_exhausted immediate alert, and one-window recovery. Use an in-memory incident repository and fake reader; assert notification count, incident key, severity and no write calls.
- [ ] Step 2: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/opsmonitor -run TestService -count=1'
```
Expected: FAIL because the package and scheduler hook do not exist.
- [ ] Step 3: Implement deterministic observations. For each public group and active/schedulable account read 15m and 24h snapshots; require at least 20 requests except complete failure/paused states. Compare TTFT P95 to 24h baseline. Quality rows emit immediate multiplier/balance_exhausted observations; ordinary account_test_error never implies balance exhaustion.
- [ ] Step 4: Render through RenderAlert and RenderRecovery with object, metric, current, baseline, window count, evidence timestamp and /ops link. Hash only non-secret identifiers and metric values.
- [ ] Step 5: Add Scheduler.SiteMonitor and runDue key site-monitor with 15-minute interval; wire app using the same reader, account-quality source, incident machine and notifier. Keep existing production/candidate/probe and daily schedules unchanged.
- [ ] Step 6: Run:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go test ./internal/opsmonitor ./internal/scheduler ./internal/app -count=1'
```
- [ ] Step 7: Commit with message feat: monitor site metrics with deduplicated alerts.

### Task 6: Full Verification, Documentation, and Read-Only Acceptance

**Files:**
- Modify: docs/project/current-state.md
- Modify: docs/project/llm-handoff.md
- Create: docs/superpowers/reports/2026-07-23-ops-monitoring-and-feishu-dual-domain-verification.md
- Modify tests/relay_ops/validate_relay_ops_contract.sh only if the existing contract needs new labels

- [ ] Step 1: Run all local gates serially:
```sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=1 go test ./... -race -count=1'
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24.13-bookworm \
  bash -lc 'GOMAXPROCS=2 go vet ./...'
node --check relay-ops-service/internal/http/static/ops.js
node --check relay-ops-service/internal/http/static/ops-admin.js
bash tests/relay_ops/validate_relay_ops_contract.sh
git diff --check
```
Expected: every command exits 0; if any fails, fix and rerun before continuing.

- [ ] Step 2: Inspect git diff and search changed files for api_key, base_url, Authorization, response_text and Cookie; confirm no production secret values.
- [ ] Step 3: Run read-only production checks for relay-ops image/health/restarts, modes, anonymous /relay-ops/api/ops-view, /monitor response fingerprint, active/schedulable account hash and existing route/database write evidence. Do not send synthetic events or rebuild containers.
- [ ] Step 4: Write verification report and update current-state/llm-handoff with the two-domain monitoring result and next mainline D04 controlled low-budget acceptance/internal-test opening.
- [ ] Step 5: Commit documentation only with message docs: close ops monitoring and Feishu dual-domain verification.

## Self-Review Checklist

- [ ] Every spec section maps to one or more tasks.
- [ ] No task changes Sub2API /monitor or production routing.
- [ ] Account filtering is explicit and only uses positive account_id.
- [ ] Sample-insufficient windows cannot produce false percentage or latency alerts.
- [ ] Balance exhaustion is accepted only from the explicit quality result code.
- [ ] Every production claim is backed by a fresh command in Task 6.

