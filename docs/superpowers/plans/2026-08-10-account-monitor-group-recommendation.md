# 账号监控卡片“推荐分组” Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有账号监控投影和卡片同行状态信息中增加只读“推荐分组”，让测试组账号获得人工迁移建议，且不自动修改账号配置。

**Architecture:** 在后端新增纯函数式推荐评估器，使用固定滚动 7 天主动探测聚合、当前账号成本/余额/调度状态和规范分组配置，生成可选的 `group_recommendation` 投影。`ListWindow` 只负责装载 7 天探测证据并把推荐对象附加到账号行；前端只渲染后端目标/原因码，不重算业务规则，使用现有 Tooltip 和当前卡片 header 的同行元信息位置。

**Tech Stack:** Go 1.26、Sub2API account monitor service/repository、Vue 3 + TypeScript、Vitest、Go testing/testify。

## Global Constraints

- 推荐质量证据只使用主动探测；不得使用真实请求覆盖成功率、TTFT 或完整响应延迟。
- 推荐固定使用滚动 `7d` 主动探测窗口；页面选择的 `24h/7d/30d` 只影响原有展示指标。
- `GPT-Pro` 与 `【专属】GPT-Pro` 共用一个质量档位和账号池语义；推荐不得硬编码生产分组 ID。
- Pro/Plus 最低利润 `0.05x`，特惠最低利润 `0.02x`，专属 Pro 最低利润 `0.02x`；利润门槛先于质量评分。
- 正式组只分析 `status=active`、`schedulable=true` 且能够正常调度的账号；没有明确其他正式推荐档位时不返回推荐对象。
- 测试组始终返回推荐/继续观察/暂缓迁入/暂不建议中的一种可解释结果；自测不是推荐目标。
- Codex Auth 自购账号正常时默认推荐 Pro；测试组出现认证、余额、配额、模型或持续探测异常时保留 Pro 目标但动作是 `hold`。
- 不修改账号 `group_ids`、全局 `priority`、调度器、评分权重、数据库 schema 或定时器；不新增第二套探测脚本。
- 卡片只在现有平台/当前分组/调度状态同行追加字段；不得新增独立指标区块或自动迁移按钮。

---

## 文件与边界

| 文件 | 责任 |
| --- | --- |
| `upstream/sub2api/backend/internal/service/account_monitor_types.go` | 推荐目标、状态、原因码和 JSON 投影类型；递增 schema version |
| `upstream/sub2api/backend/internal/service/account_monitor_recommendation.go` | 规范组名映射、硬门槛、Codex Auth 规则和单账号纯评估函数 |
| `upstream/sub2api/backend/internal/service/account_monitor_service.go` | 读取固定 7 天探测聚合、调用评估器、按正式组/测试组投影语义附加推荐 |
| `upstream/sub2api/backend/internal/service/account_monitor_service_test.go` | 推荐规则、窗口来源、正式组过滤和故障降级测试 |
| `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts` | `group_recommendation` TypeScript 合同和原因码类型 |
| `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue` | 现有 header 元信息同行的最小增量渲染与 Tooltip |
| `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts` | 测试组/正式组/异常/键盘 Tooltip/旧数据兼容测试 |
| `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts` | 投影传递和窗口切换不影响推荐字段的回归测试（仅必要时修改） |
| `docs/project/project-progress.md` | 任务状态和验证证据持续登记 |

## Data Contract

后端类型使用以下可选对象；正式组无明确迁移目标时为 `nil`，旧客户端缺失时前端保持原样：

```go
type AccountMonitorGroupRecommendation struct {
	Status       string    `json:"status"`
	Target       string    `json:"target"`
	TargetName   string    `json:"target_name"`
	Action       string    `json:"action"`
	ReasonCodes  []string  `json:"reason_codes"`
	SampleCount  int       `json:"sample_count"`
	ObservedAt   time.Time `json:"observed_at"`
	Source       string    `json:"source"`
}
```

Allowed values:

- status: `recommended`, `observe`, `blocked`, `not_recommended`
- target: `gpt_pro`, `gpt_plus`, `gpt_special`, or empty
- action: `keep`, `migrate`, `hold`, `none`
- source: `monitor_probe`

---

### Task 1: Add typed recommendation contract and pure evaluator

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Create: `upstream/sub2api/backend/internal/service/account_monitor_recommendation.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_recommendation_test.go`

**Interfaces:**
- Consumes: `Account`, `AccountMonitorGroup`, `AccountMonitorQualityEvidence`, `AccountMonitorLatest`, `AccountMonitorMultiplier`, current group names, and `time.Time`.
- Produces: `EvaluateAccountMonitorGroupRecommendation(account Account, currentGroupNames []string, groups []AccountMonitorGroup, evidence AccountMonitorQualityEvidence, latest AccountMonitorLatest, now time.Time) *AccountMonitorGroupRecommendation`; no repository or network access.

- [ ] **Step 1: Write failing table-driven tests for group normalization and target selection.** Cover exact names `GPT-Pro`, `【专属】GPT-Pro`, `GPT-Plus`, `GPT-特惠`, `GPT-特惠分组`, `GPT-测试分组`; assert IDs are never required and Pro/special Pro resolve to one target.
- [ ] **Step 2: Write failing tests for hard gates.** Assert sample count `<20`, stale evidence, unavailable multiplier, cost above `group.RateMultiplier-margin`, fatal latest errors (`401/403/402`, `auth`, `balance_exhausted`, `quota`, `billing`) and missing latency samples block a test-group recommendation; assert a formal-group account with no alternate target returns `nil`.
- [ ] **Step 3: Write failing tests for quality tiers.** Use evidence at 98%/95%/70% boundaries and configured TTFT/latency limits; assert highest passing tier wins, a failed Pro falls through to Plus, and failed all tiers returns `not_recommended` only for the test group.
- [ ] **Step 4: Write failing tests for Codex Auth.** Use the existing `Account.IsOpenAIAgentIdentity`/`IsOpenAIPersonalAccessToken` semantics (OpenAI OAuth plus the stored auth mode), assert healthy accounts target `gpt_pro`, and fatal/stale test-group accounts retain `gpt_pro` with `action=hold`.
- [ ] **Step 5: Implement constants, stable group-name normalization, margin calculation, quality predicates, and the pure evaluator.** Use `group.RateMultiplier - margin` for the cost ceiling; use the existing normalized score weights and limits; never infer cost from account names.
- [ ] **Step 6: Run the focused Go test package.**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountMonitorGroupRecommendation' -count=1`

Expected: PASS with coverage for every rule above.

- [ ] **Step 7: Commit the typed contract and evaluator.**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_types.go \
  upstream/sub2api/backend/internal/service/account_monitor_recommendation.go \
  upstream/sub2api/backend/internal/service/account_monitor_recommendation_test.go
git commit -m "feat: evaluate account group recommendations"
```

### Task 2: Integrate fixed 7-day probe evidence into the monitor projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes: Task 1 `EvaluateAccountMonitorGroupRecommendation` and existing `ListAggregates`, `ListLatest`, `ListGroups` repository methods.
- Produces: each `AccountMonitorAccount.GroupRecommendation` populated or nil without changing existing score/rank/evidence fields.

- [ ] **Step 1: Add a failing service test proving recommendation uses a fixed seven-day aggregate even when `ListWindow("24h")` is requested.** Extend the repository stub to record aggregate `since` values; return distinct 24-hour and 7-day probe data and assert the recommendation follows 7-day data while the existing card metrics remain 24-hour data.
- [ ] **Step 2: Add failing tests for projection scope.** Assert test-group accounts receive `recommended`, `observe`, `blocked`, or `not_recommended`; formal-group disabled/unschedulable accounts receive nil; formal-group healthy accounts receive nil when current tier matches; formal-group healthy accounts receive `action=migrate` only when another target tier is explicit.
- [ ] **Step 3: Add failing test that recommendation evaluation failure is isolated to one row.** Use a malformed group configuration or evaluator guard and assert the page still returns all accounts with only that row's recommendation nil and a warning log.
- [ ] **Step 4: Implement a fixed `recommendationSince := observedAt.Add(-7*24*time.Hour)` aggregate load.** Reuse the requested aggregate only when the requested range is 7d; otherwise issue one additional `ListAggregates` call. Preserve the existing probe-only quality projection and never pass usage-log aggregates to the evaluator.
- [ ] **Step 5: Attach recommendations after groups are normalized and base rows are built.** Determine whether the account is in test or formal target by normalized names; pass only normal/schedulable formal accounts to the evaluator; suppress formal outputs unless the returned target differs from the current tier. Keep Pro and special Pro equivalent.
- [ ] **Step 6: Increment `AccountMonitorSchemaVersion`, run focused service tests, all monitor service tests, and `go vet`.**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'AccountMonitor' -count=1 && go vet ./internal/service`

Expected: PASS; no existing score, rank, real-request-count, or group-health regression.

- [ ] **Step 7: Commit the projection integration.**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go \
  upstream/sub2api/backend/internal/service/account_monitor_types.go
git commit -m "feat: project account group recommendations"
```

### Task 3: Expose the contract and render the minimal inline card field

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts` only if the projection fixture type requires it

**Interfaces:**
- Consumes: optional backend `group_recommendation` object from Task 2.
- Produces: a same-line `推荐：GPT-Pro/Plus/特惠` or test-group status; a warning icon with an accessible existing `HelpTooltip` only for explicit formal-group migration.

- [ ] **Step 1: Add failing Vue tests for the exact DOM contract.** Cover test-group recommended text, test-group `继续观察`/`暂缓迁入`/`暂不建议入组`, formal-group matching nil/no icon, formal-group migration text plus icon, and missing object preserving existing card DOM.
- [ ] **Step 2: Add a failing accessibility test for the warning.** Assert the icon has an accessible label/title and `HelpTooltip` content includes `推荐迁移至`, target group, primary reason, `7d` sample count and observation time; assert keyboard focus exposes the same tooltip through the existing component.
- [ ] **Step 3: Implement the TypeScript interfaces and a small local reason-code formatter.** Keep the backend target/status authoritative; the formatter only maps stable reason codes and existing evidence values to short Chinese text.
- [ ] **Step 4: Insert the field into the existing header metadata row.** The current checked production page and `main` source have diverged on whether platform/group/schedulable metadata is already present; first locate the actual row in the target implementation. Append the recommendation inline beside those existing values. If the target file truly lacks that row, add one compact metadata row containing the already-projected platform, current group, and schedulable state, with recommendation in the same row; do not create a second recommendation row or metric block.
- [ ] **Step 5: Run focused frontend tests and typecheck.**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts && pnpm typecheck`

Expected: PASS; current card shell, metrics, spacing, and mobile wrapping remain unchanged except for the inline field.

- [ ] **Step 6: Commit the frontend contract and card change.**

```bash
git add upstream/sub2api/frontend/src/api/admin/accountMonitor.ts \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts \
  upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
git commit -m "feat: show account group recommendations"
```

### Task 4: Full validation, independent review, and handoff documentation

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-10-account-monitor-group-recommendation-implementation.md`

**Interfaces:**
- Consumes: Tasks 1–3 commits and their test evidence.
- Produces: reviewable implementation report; no production deployment in this plan.

- [ ] **Step 1: Run backend focused/full validation.**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler/admin ./internal/repository -count=1 && go vet ./...`

- [ ] **Step 2: Run frontend validation.**

Run: `cd upstream/sub2api/frontend && pnpm test:run -- src/components/admin/account-monitor src/views/admin/__tests__/AccountMonitorView.spec.ts && pnpm typecheck && pnpm build`

- [ ] **Step 3: Review the diff against the design.** Confirm no migration, scheduler write, group mutation, real-request quality fallback, hardcoded production group IDs, image/Claude behavior, or automatic migration action was introduced. Verify the production account monitor timer remains the sole probe owner.
- [ ] **Step 4: Run a local browser smoke check at desktop and narrow viewport.** Confirm inline text does not increase normal card height materially, formal-group warning appears only for explicit migration, and tooltip is keyboard reachable. Do not use production write actions.
- [ ] **Step 5: Write the implementation report with commands, results, screenshots/DOM evidence, and residual risks.** Keep production deployment status explicitly “未部署/未线上验证”.
- [ ] **Step 6: Update `docs/project/project-progress.md` to implementation complete locally but still “进行中” until the user authorizes merge, push, deployment, and online verification.**
- [ ] **Step 7: Perform independent whole-branch review before any deployment request.** Review findings first; block completion on any scope, compatibility, accessibility, or source-of-truth regression.

## Execution Order and Review Gates

1. Task 1 must pass pure evaluator tests before Task 2.
2. Task 2 must pass projection/service tests before Task 3.
3. Task 3 must pass frontend tests/typecheck before Task 4.
4. Each task is committed independently and receives a fresh implementer plus an independent review under `superpowers:subagent-driven-development`.
5. Task 4 whole-branch review is required before any production merge or deployment planning.

## Rollback

Because this feature adds only optional JSON fields and read-only rendering, rollback is a code revert of the feature commits; no database rollback or account configuration repair is required. Existing clients ignore the optional object, and the frontend hides it when absent.
