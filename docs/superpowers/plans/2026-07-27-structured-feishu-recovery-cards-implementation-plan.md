# Structured Feishu Recovery Cards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render every Feishu recovery notification as a structured conclusion, metric matrix, evidence section, and action instead of one long Markdown paragraph.

**Architecture:** Add one recovery-specific view and renderer in `internal/notify`, including the official Feishu `div.fields` wire shape. Recovery producers translate domain values into business-language metrics before rendering; alert and legacy recovery rendering remain compatible.

**Tech Stack:** Go 1.24, existing relay-ops notification types, Feishu Interactive Card JSON, Go `testing`.

## Global Constraints

- Cover site runtime, account quality, native monitor, group availability, usage-session, and synthetic acceptance recovery notifications.
- Do not change alert, command, digest, or pricing-change card structure.
- Do not parse semicolon-delimited strings to recover structured fields.
- Render only two to four meaningful metrics; never add blank fields or `无` placeholders.
- Preserve redaction, relative-link filtering, and the 30 KiB card limit.
- Keep relay-ops read-only and do not deploy production.

---

### Task 1: Add The Shared Structured Recovery Renderer

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/feishu_test.go`

**Interfaces:**
- Produces: `RecoveryMetric`, `RecoveryCardView`, `CardField`, and `RenderRecoveryCard(RecoveryCardView) FeishuMessage`.
- Preserves: `RenderRecovery(IncidentView) FeishuMessage` as the legacy prose renderer.

- [ ] **Step 1: Write failing renderer contract tests**

Add tests that exercise the real JSON and flattened text:

```go
func TestRenderRecoveryCardSeparatesSummaryMetricsEvidenceAndAction(t *testing.T) {
    message := RenderRecoveryCard(RecoveryCardView{
        Title:   "站内运行已恢复：特惠-SHUAI",
        Summary: "已恢复正常调度",
        Detail:  "调度状态已回到健康基线",
        Metrics: []RecoveryMetric{
            {Label: "当前状态", Value: "正常调度"},
            {Label: "健康确认", Value: "1 个完整窗口"},
            {Label: "观测窗口", Value: "15 分钟 / 24 小时"},
            {Label: "证据时间", Value: "05:15 UTC"},
        },
        Basis:  []string{"当前与基线均为可用、可调度"},
        Source: "Sub2API 原生站内运行快照",
        Focus:  "在运维后台查看站内运行证据",
        Links:  []Link{{Label: "运维后台", URL: "/ops"}},
    })

    if got := len(message.Card.Elements); got != 4 {
        t.Fatalf("elements=%d, want summary, metrics, evidence, action", got)
    }
    fields := message.Card.Elements[1].Fields
    if len(fields) != 4 {
        t.Fatalf("fields=%#v", fields)
    }
    for _, field := range fields {
        if !field.IsShort || field.Text.Tag != "lark_md" || strings.TrimSpace(field.Text.Content) == "" {
            t.Fatalf("invalid field=%#v", field)
        }
    }
    for _, want := range []string{"已恢复正常调度", "当前状态", "正常调度", "判断依据", "数据来源", "后续观察"} {
        if !strings.Contains(message.RenderedText(), want) {
            t.Fatalf("missing %q in %q", want, message.RenderedText())
        }
    }
}

func TestRenderRecoveryCardOmitsBlankMetricsAndRedactsValues(t *testing.T) {
    message := RenderRecoveryCard(RecoveryCardView{
        Title: "恢复", Summary: "已恢复",
        Metrics: []RecoveryMetric{
            {Label: "当前状态", Value: "正常"},
            {Label: "", Value: ""},
            {Label: "证据", Value: "x-api-key: secret"},
        },
    })
    fields := message.Card.Elements[1].Fields
    if len(fields) != 2 || strings.Contains(message.RenderedText(), "secret") || !strings.Contains(message.RenderedText(), "[已脱敏]") {
        t.Fatalf("fields=%#v text=%q", fields, message.RenderedText())
    }
}
```

The production change these tests catch is collapsing structured fields back into one prose `div`, emitting blank cells, or bypassing redaction.

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/notify -run 'TestRenderRecoveryCard' -count=1
```

Expected: compile failure because `RecoveryCardView`, `RecoveryMetric`, `CardField`, and `RenderRecoveryCard` do not exist.

- [ ] **Step 3: Implement the minimal renderer**

Add the types:

```go
type RecoveryMetric struct {
    Label string
    Value string
}

type RecoveryCardView struct {
    Title      string
    Summary    string
    Detail     string
    Metrics    []RecoveryMetric
    Basis      []string
    Source     string
    Focus      string
    Links      []Link
    Suppressed bool
}

type CardField struct {
    IsShort bool     `json:"is_short"`
    Text    CardText `json:"text"`
}
```

Add `Fields []CardField `json:"fields,omitempty"` to `CardElement`. Update `FeishuMessage.RenderedText` to append every `field.Text.Content` in field order.

Implement `RenderRecoveryCard` with:

- a green header;
- a summary `div`;
- one metrics `div` containing at most four non-empty `CardField` entries;
- an evidence `div` with basis, source, optional focus, and optional suppression note;
- the existing safe action builder behavior.

Use `safeValue` for every title, summary, detail, label, value, evidence, source, and focus. Extract only a small `appendCardActions` helper if needed to share the existing action assembly without changing its filtering semantics.

- [ ] **Step 4: Run notify tests and verify GREEN**

Run:

```bash
go test ./internal/notify -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the renderer**

```bash
git add relay-ops-service/internal/notify/feishu.go relay-ops-service/internal/notify/feishu_test.go
git commit -m "feat: add structured Feishu recovery renderer"
```

---

### Task 2: Migrate Site, Account-Quality, And Native-Monitor Recovery

**Files:**
- Modify: `relay-ops-service/internal/opsmonitor/service.go`
- Modify: `relay-ops-service/internal/opsmonitor/service_test.go`
- Modify: `relay-ops-service/internal/nativealerts/service.go`
- Modify: `relay-ops-service/internal/nativealerts/service_test.go`

**Interfaces:**
- Consumes: `notify.RenderRecoveryCard(notify.RecoveryCardView)`.
- Produces: business-language recovery messages for runtime metrics, account-quality metrics, and native Channel Monitor status.

- [ ] **Step 1: Write failing producer tests**

Replace the old recovery prose assertion with table-driven observable behavior:

```go
func TestRenderRecoveryUsesBusinessLanguageAndStructuredMetrics(t *testing.T) {
    now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
    tests := []struct {
        name    string
        message notify.FeishuMessage
        wants   []string
        forbids []string
    }{
        {
            name: "runtime scheduling",
            message: render(object{kind: "account", id: 10, name: "特惠-SHUAI"},
                "paused", "active && schedulable", "active && schedulable", 1, now, "recovered"),
            wants: []string{"已恢复正常调度", "当前状态", "正常调度", "1 个完整窗口", "Sub2API 原生站内运行快照"},
            forbids: []string{"**恢复结果**", "指标：paused", "active && schedulable"},
        },
        {
            name: "account balance",
            message: render(object{kind: "account", id: 10, name: "特惠-SHUAI"},
                "balance_exhausted", "passed", "not_balance_exhausted", 1, now, "recovered"),
            wants: []string{"余额状态已恢复", "余额可用", "余额耗尽", "账号质量定时巡检结果"},
            forbids: []string{"balance_exhausted", "not_balance_exhausted"},
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            text := tt.message.RenderedText()
            for _, want := range tt.wants {
                if !strings.Contains(text, want) {
                    t.Fatalf("missing %q in %q", want, text)
                }
            }
            for _, forbidden := range tt.forbids {
                if strings.Contains(text, forbidden) {
                    t.Fatalf("found %q in %q", forbidden, text)
                }
            }
        })
    }
}
```

Extend the existing native-monitor recovery test to require `运行正常`, the monitored model, latency, and `Sub2API 原生 Channel Monitor 最新状态`.

These tests catch producers continuing to pass raw enum prose into the legacy renderer.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/opsmonitor ./internal/nativealerts -run 'Recovery' -count=1
```

Expected: FAIL because recovery messages still use `RenderFeishu(IncidentView)` and expose legacy raw values.

- [ ] **Step 3: Implement explicit producer mappings**

In `opsmonitor`, keep alert construction unchanged. For `transition == "recovered"`, build `RecoveryCardView` directly. Add pure helpers that map:

```go
paused            -> 调度状态 / 正常调度 / 已恢复正常调度
availability      -> 可用性 / 可用 / 可用性已恢复
error_rate        -> 错误率 / existing numeric values / 错误率已恢复
ttft_p95          -> 首字延迟 P95 / existing millisecond values / 首字延迟已恢复
multiplier        -> 倍率 / existing multiplier values / 倍率已恢复
balance_exhausted -> 余额耗尽 / 余额可用 / 余额状态已恢复
```

Runtime metrics use the native 15-minute/24-hour snapshot source. Account-quality metrics use the scheduled-patrol source and state that no new probe was started. Format the evidence time as `15:04 UTC`.

In `nativealerts`, keep alert rendering unchanged. Recovery uses:

- summary `原生监控已恢复`;
- current state `运行正常`;
- model name;
- latency only when positive;
- checked time when parseable;
- source `Sub2API 原生 Channel Monitor 最新状态`.

- [ ] **Step 4: Run focused packages and verify GREEN**

Run:

```bash
go test ./internal/opsmonitor ./internal/nativealerts -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit producer migration**

```bash
git add relay-ops-service/internal/opsmonitor/service.go relay-ops-service/internal/opsmonitor/service_test.go relay-ops-service/internal/nativealerts/service.go relay-ops-service/internal/nativealerts/service_test.go
git commit -m "feat: structure operational recovery notifications"
```

---

### Task 3: Migrate Remaining Recovery Sources

**Files:**
- Modify: `relay-ops-service/internal/acceptance/service.go`
- Modify: `relay-ops-service/internal/acceptance/service_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`
- Modify: `relay-ops-service/internal/notify/group_alert.go`
- Modify: `relay-ops-service/internal/notify/group_alert_test.go`
- Modify: `relay-ops-service/internal/app/group_availability_test.go`

**Interfaces:**
- Consumes: `notify.RenderRecoveryCard(notify.RecoveryCardView)`.
- Produces: structured synthetic-acceptance, usage-session, and group-availability recovery cards.

- [ ] **Step 1: Write failing recovery-source tests**

Extend acceptance assertions:

```go
recoveryText := notifier.messages[1].RenderedText()
for _, want := range []string{"合成事件已恢复", "重复事件", "已抑制", "真实服务影响", "无", "合成验收"} {
    if !strings.Contains(recoveryText, want) {
        t.Fatalf("missing %q in %q", want, recoveryText)
    }
}
```

Add an app-level helper contract:

```go
func TestRenderUsageSessionRecoveryUsesStructuredBusinessCopy(t *testing.T) {
    message := renderUsageSessionRecovery("wawazz")
    for _, want := range []string{"用量读取会话已恢复", "会话状态", "正常", "消费核对", "已恢复", "上游用量页面"} {
        if !strings.Contains(message.RenderedText(), want) {
            t.Fatalf("missing %q in %q", want, message.RenderedText())
        }
    }
}
```

Strengthen `TestRenderGroupAlertRecovery` to assert separate fields for `可用账号`, `3 / 3`, `当前状态`, and `可用性正常`, plus the shared evidence section. Keep the existing regression that recovery omits the prior down-account list.

These tests catch any remaining recovery path using a single prose block or exposing stale failure rows.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/acceptance ./internal/app ./internal/notify -run 'Recovery|Acceptance' -count=1
```

Expected: FAIL because these producers still use legacy recovery rendering.

- [ ] **Step 3: Implement the remaining mappings**

- Synthetic acceptance: summary `合成事件已恢复`; metrics for current state, duplicate suppression, and real-service impact; source `relay-ops 合成验收（未访问上游）`; keep `Suppressed: true`.
- Usage session: extract `renderUsageSessionRecovery(upstream string)` and call it from the existing recovered transition; metrics for session state and resumed cost verification; source `上游用量页面只读读取结果`.
- Group availability: keep the alert branch byte-for-byte behaviorally equivalent. In the recovery branch, call `RenderRecoveryCard` with availability `available / total`, current state `可用性正常`, confirmation `1 个完整窗口`, and source `Sub2API 账号监控分组快照`.

- [ ] **Step 4: Run focused packages and verify GREEN**

Run:

```bash
go test ./internal/acceptance ./internal/app ./internal/notify -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit remaining sources**

```bash
git add relay-ops-service/internal/acceptance/service.go relay-ops-service/internal/acceptance/service_test.go relay-ops-service/internal/app/app.go relay-ops-service/internal/app/app_test.go relay-ops-service/internal/notify/group_alert.go relay-ops-service/internal/notify/group_alert_test.go relay-ops-service/internal/app/group_availability_test.go
git commit -m "feat: unify Feishu recovery notifications"
```

---

### Task 4: Full Verification And Closeout

**Files:**
- Modify only files required to fix regressions caused by Tasks 1–3.

**Interfaces:**
- Consumes: the complete structured recovery implementation.
- Produces: verified repository state ready to merge.

- [ ] **Step 1: Format modified Go files**

```bash
gofmt -w relay-ops-service/internal/notify/feishu.go relay-ops-service/internal/notify/feishu_test.go relay-ops-service/internal/notify/group_alert.go relay-ops-service/internal/notify/group_alert_test.go relay-ops-service/internal/opsmonitor/service.go relay-ops-service/internal/opsmonitor/service_test.go relay-ops-service/internal/nativealerts/service.go relay-ops-service/internal/nativealerts/service_test.go relay-ops-service/internal/acceptance/service.go relay-ops-service/internal/acceptance/service_test.go relay-ops-service/internal/app/app.go relay-ops-service/internal/app/app_test.go relay-ops-service/internal/app/group_availability_test.go
```

- [ ] **Step 2: Run the complete relay-ops verification**

```bash
cd relay-ops-service
go build ./...
go vet ./...
go test ./... -count=1
```

Expected: all commands exit 0.

- [ ] **Step 3: Run repository contract and infrastructure validation**

```bash
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
```

Expected: both scripts exit 0.

- [ ] **Step 4: Inspect the final diff**

```bash
git diff --check
git status --short
git log --oneline main..HEAD
```

Expected: no whitespace errors; only the planned files are modified or committed.
