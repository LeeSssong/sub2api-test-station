# T43 调度 Top-K 放宽 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 默认关闭自适应 Top-K 的质量分数收窄，让浅账号池保留全部健康候选进入固定 Top-K，同时保留显式开启开关的旧行为。

**Architecture:** 仅调整 Sub2API 原生 OpenAI 调度器配置默认值、示例配置和直接相关 Go 回归测试。调度器代码路径保持不变：自适应开关为 false 时跳过 `applyOpenAIAdaptiveTopK`，固定 Top-K、S1/S2 过滤、sticky、半开探测和恢复预算继续运行。

**Tech Stack:** Go、Viper 配置、testify、Sub2API 原生调度器。

**Spec:** `docs/superpowers/specs/2026-08-20-t43-scheduler-topk-relaxation-design.md`

## Global Constraints

- 只修改 `upstream/sub2api/backend/internal/config/config.go`、`upstream/sub2api/backend/internal/config/config_test.go`、`upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go` 和 `upstream/sub2api/deploy/config.example.yaml` 以及本任务规格/计划/交接。
- 不新增迁移、API、生产数据写入、GitHub Actions 或外部控制面。
- 功能完成门槛为直接相关测试通过；发布仍由根总控串行执行。

### Task 1: Lock the default-off contract with failing tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/config/config_test.go:564-571`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go` (add regression beside existing adaptive tests)

- [ ] **Step 1: Change the expected default in the config test from true to false.**
- [ ] **Step 2: Add `TestOpenAIAccountSchedulerDefaultKeepsHealthyPoolWhenAdaptiveTopKDisabled` that creates three schedulable accounts with priorities 0/1/2, sets `LBTopK=7`, leaves `AdaptiveTopKEnabled=false`, and asserts `CandidateCount=3`, `EligibleCount=3`, `EffectiveTopK=3`, `TopK=3`, and `SelectionLayer=load_balance`.
- [ ] **Step 3: Run the focused tests before changing production defaults.**

Run:
```bash
cd upstream/sub2api/backend
go test ./internal/config ./internal/service -run 'TestLoadDefaultOpenAIWSConfig|TestOpenAIAccountSchedulerDefaultKeepsHealthyPoolWhenAdaptiveTopKDisabled' -count=1
```

Expected: the new scheduler test passes against the existing zero-value behavior; the config default assertion fails because the runtime default is still true. If the scheduler test does not fail before implementation, change its assertion to explicitly prove the current loaded default is true before proceeding.

### Task 2: Apply the minimal default change

**Files:**
- Modify: `upstream/sub2api/backend/internal/config/config.go:2617`
- Modify: `upstream/sub2api/deploy/config.example.yaml:420-430`

- [ ] **Step 1: Change the Viper default to `false` for `gateway.openai_scheduler.adaptive_top_k_enabled`.**
- [ ] **Step 2: Keep `adaptive_top_k_max=7` and `adaptive_top_k_score_gap=0.15` unchanged.
- [ ] **Step 3: Add an example comment that default-off preserves a wider candidate pool for shallow account pools and explicit `true` enables quality-based narrowing.
- [ ] **Step 4: Run the focused config and scheduler tests.**

Run:
```bash
cd upstream/sub2api/backend
go test ./internal/config ./internal/service -run 'TestLoadDefaultOpenAIWSConfig|TestApplyOpenAIAdaptiveTopK|TestOpenAIAccountSchedulerAdaptiveTopK|TestOpenAIAccountSchedulerDefaultKeepsHealthyPoolWhenAdaptiveTopKDisabled' -count=1
```

Expected: PASS with existing explicit adaptive tests unchanged.

### Task 3: Validate scope and commit the candidate

**Files:**
- No additional source files.

- [ ] **Step 1: Run formatting and focused regression.**

```bash
cd upstream/sub2api/backend
gofmt -w internal/config/config.go internal/config/config_test.go internal/service/openai_account_scheduler_adaptive_test.go
go test ./internal/config ./internal/service -run 'TestLoadDefaultOpenAIWSConfig|TestApplyOpenAIAdaptiveTopK|TestOpenAIAccountSchedulerAdaptiveTopK|TestOpenAIAccountSchedulerDefaultKeepsHealthyPoolWhenAdaptiveTopKDisabled' -count=1
go test ./internal/service -run 'TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceTopKExcludesQuotaPaused|TestSelectTopKOpenAICandidates' -count=1
cd ../..
git diff --check
```

- [ ] **Step 2: Verify only the four declared implementation/config files plus this task's spec/plan changed.**
- [ ] **Step 3: Commit with `fix: widen default scheduler candidate pool`.

### Task 4: Handoff

- [ ] **Step 1: Write `docs/handoffs/2026-08-20-t43-scheduler-topk-relaxation-handoff.md` with baseline SHA, commit SHA, changed files, tests, no migration/config-schema changes, expected `downtime_required=false`, rollback switch, and remaining production validation.
- [ ] **Step 2: Mark the candidate `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or edit root progress/queue from the feature worktree.
