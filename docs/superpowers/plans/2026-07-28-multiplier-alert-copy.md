# Multiplier Alert Copy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make multiplier cards describe billing changes and subsequent stability accurately, and remove the unsupported operations-console drill-down.

**Architecture:** Extend the shared incident view with optional current and comparison labels that preserve existing defaults. Specialize only the multiplier branch in `opsmonitor` with billing language, previous-record wording, and no `/ops` action.

**Tech Stack:** Go, standard library testing, existing relay-ops Feishu card renderer.

## Global Constraints

- Do not change multiplier collection, comparison, incident state, or delivery.
- Do not change routing, scheduling, pricing, account configuration, or credentials.
- Preserve all non-multiplier card copy and actions.
- Use real rendered card output in tests.

---

### Task 1: Correct multiplier-change card semantics

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/opsmonitor/service.go`
- Test: `relay-ops-service/internal/opsmonitor/service_test.go`

**Interfaces:**
- Consumes: `notify.RenderFeishu(event notify.IncidentView) notify.FeishuMessage`
- Produces: optional `IncidentView.CurrentLabel` and `IncidentView.BaselineLabel` fields; blank values preserve the existing `影响` and `基线` labels.

- [ ] **Step 1: Write the failing multiplier-card test**

Add a focused test that renders a confirmed multiplier change and asserts the real card:

```go
func TestRenderMultiplierChangeUsesBillingLanguageWithoutUnsupportedOpsLink(t *testing.T) {
	message := render(
		object{kind: "account", id: 21, name: "Plus-TK-极速"},
		"multiplier", "0.07x", "0.08x", 1,
		time.Date(2026, 7, 28, 1, 35, 4, 0, time.UTC), "confirmed",
	)
	text := message.RenderedText()
	for _, want := range []string{
		"账号计费倍率变更：Plus-TK-极速",
		"当前倍率** 0.07x",
		"上次记录** 0.08x",
		"确认是否符合上游价格或账号配置变化",
		"不执行路由或账号写入",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	for _, forbidden := range []string{"上游账号质量告警", "**基线**", "查看上游账号质量证据"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("found %q in %q", forbidden, text)
		}
	}
	if len(message.Card.Elements) != 1 {
		t.Fatalf("multiplier card has unsupported action: %#v", message.Card.Elements)
	}
}
```

Update existing table expectations so only multiplier uses `账号计费倍率变更`.

Add a second focused test for the subsequent stable observation. It must use
`账号计费倍率记录已稳定`, state that this record matches the previous record,
avoid all health-recovery language, and contain no `/ops` action.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd relay-ops-service
go test ./internal/opsmonitor -run 'TestRenderMultiplierChangeUsesBillingLanguageWithoutUnsupportedOpsLink|TestRenderAccountMetricsUseDisplayName|TestRenderSeparatesRuntimeAndAccountQualityEvidenceInCardJSON' -count=1
```

Expected: FAIL because the current card still says `上游账号质量告警`, renders `基线`, and includes the `/ops` action.

- [ ] **Step 3: Implement optional incident field labels**

Add these fields to `notify.IncidentView`:

```go
CurrentLabel  string
BaselineLabel string
```

In `RenderAlert`, render `CurrentLabel` with default `影响`, and `BaselineLabel` with default `基线`. Blank fields must produce byte-for-byte equivalent visible labels for existing callers.

- [ ] **Step 4: Specialize multiplier rendering**

In `renderWithEvidence`, use multiplier-specific values:

```go
domain := "账号计费倍率"
currentLabel := "当前倍率"
baselineLabel := "上次记录"
change := "计费倍率已更新"
focus := "确认是否符合上游价格或账号配置变化"
links := []notify.Link(nil)
```

Use `Sub2API 原生账号监控倍率投影` as the source, describe the operation as comparing this record with the previous record, and retain the statement that no routing or account write occurred. Keep the existing account-quality branch for `balance_exhausted`.

In `renderRecovery`, specialize the multiplier transition as a stable record
rather than a health recovery. Use `当前倍率`, `稳定确认`, and
`本次记录与上次记录一致`, and omit the `/ops` action. Preserve recovery
language and actions for every other metric.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run:

```bash
cd relay-ops-service
go test ./internal/opsmonitor ./internal/notify -count=1
```

Expected: PASS.

- [ ] **Step 6: Run the relay-ops test suite**

Run:

```bash
cd relay-ops-service
go test ./... -count=1
```

Expected: PASS with no warnings or failures.

- [ ] **Step 7: Commit the implementation**

```bash
git add relay-ops-service/internal/notify/feishu.go \
  relay-ops-service/internal/opsmonitor/service.go \
  relay-ops-service/internal/opsmonitor/service_test.go \
  docs/superpowers/plans/2026-07-28-multiplier-alert-copy.md
git commit -m "fix: clarify multiplier change alerts"
```
