# Implementation Plan: Unified Quality Account Priority And Daily Cap 50

> **For agentic workers:** Implement only this plan in `.worktrees/unified-quality-account-priority`. Do not merge, push, or deploy.

**Goal:** Unified quality ranking uses `accounts.priority` and a default daily cap of 50.

**Architecture:** Reuse the existing cold-start/daily signal functions. Change the priority source and the default cap. Native non-unified selectors stay on `accountSchedulingPriorityForGroup`.

## Task 1: Defaults

- `defaultOpenAIUnifiedQualityPriorityDailyMax = 50`
- Viper and `config.example.yaml` default `unified_quality_priority_daily_max: 50`
- Config test `TestLoad_DefaultUnifiedQualityPriorityCaps` expects 50
- Malformed global fallback test expects daily 50

## Task 2: Read `accounts.priority`

- Add `openAIUnifiedQualitySchedulingPriority(account *Account) int` returning `account.Priority` (50 if nil)
- `selectByUnifiedQualityInternal` and unified-quality `Project()` rank pool use it
- Do not change `gateway_scheduling.go` or legacy load-plan comparators

## Task 3: Tests

- Mapping helper with cap 50: priority 1 daily = 50, 50 = 0, 100 = -50
- Selector tests use `unifiedQualityPriorityTestConfig()` daily 50
- Cold-start + daily for unknown `priority=1` totals 100
- Mature `priority=1` daily 50
- Invert group-row test: `accounts.priority` wins
- Keep 0% success `priority=1` losing to healthy `priority=50`
- Projection: card priority 1 beats group-row priority 1 when accounts.priority is 100 vs 1

## Task 4: Verify

```bash
gofmt -w <changed go files>
go test ./internal/service -count=1 -run 'TestOpenAIUnifiedQuality|TestOpenAIAccountSchedulerProjectionUses|TestOpenAIGatewayService_UnifiedQualityPriorityCaps|TestLoad_DefaultUnifiedQualityPriorityCaps|TestLoad_ConfiguredUnifiedQualityPriorityCaps|TestValidateConfig_UnifiedQualityPriorityCaps'
go build ./cmd/server
git diff --check
```
