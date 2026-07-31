# Monitor V2 卡片信息精简 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从 Monitor V2 服务卡片和快照接口中移除样本数量与模型信息，同时保留服务分组的汇总指标、P95、可用率和趋势。

**Architecture:** 将接口契约升至 v4，并从服务层、处理器和前端解析类型中移除 `models`。卡片只渲染可操作的性能信息；其详情保留 P95，不再承担模型列表展示。

**Tech Stack:** Go、Gin、Vue 3、TypeScript、Vitest、Vue Test Utils。

## Global Constraints

- 卡片统计仍以服务分组的全部调用样本汇总，不转为按模型平均。
- 不展示或传输任何模型名称、模型状态、模型数量或样本数量。
- 保留 TTFT P50、输出 TPS、总延迟 P50、缓存命中率、有效调用、P95 和趋势。
- 任何本地实现都只能在项目总账中保持“进行中”，直到推送、部署和生产验证完成。

---

### Task 1: 收敛 Monitor V2 v4 快照与服务卡片

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/types.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/api.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/api.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`

**Interfaces:**

- Consumes: `MonitorV2Service.Snapshot`, `monitorV2SnapshotFromService`, and `getMonitorV2Snapshot`.
- Produces: v4 snapshot groups that contain group metrics, availability and timeline only; no `models` key.

- [ ] **Step 1: Write the failing tests**

Add a Go handler assertion that a non-empty group response serializes `contract_version` as `"4"` and lacks a `models` key. Update each TypeScript snapshot fixture to version `"4"` without `models`, then add a contract test that appends `models: []` and expects `MonitorV2ContractError`. Update the card-view test so it expects no `个样本`, `个模型`, `查看模型`, `收起模型`, model names, or model statuses while preserving the displayed P50/P95 values and availability evidence.

- [ ] **Step 2: Run the target tests to verify they fail**

Run:

```bash
cd upstream/sub2api/backend && go test ./internal/handler -run 'TestMonitorV2.*' -count=1
cd upstream/sub2api/frontend && pnpm vitest run src/features/monitor-v2/__tests__/api.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts
```

Expected: failures identify the existing v3 version, `models` serialization/parser acceptance, and visible sample/model content.

- [ ] **Step 3: Implement the smallest contract and rendering change**

Set `MonitorV2ContractVersion` and `MONITOR_V2_CONTRACT_VERSION` to `"4"`. Remove model structs/fields, service inputs and model aggregation from the Go snapshot path; remove models parsing and type definitions from TypeScript. Render metric values without the sample `<dd>`, and remove the `details` block which contains the P95 and model list; render the two P95 values as compact card content so the retained metrics remain visible without model disclosure. Remove unused model and sample translation keys from the English and Chinese dashboard locale modules.

- [ ] **Step 4: Run the target tests to verify they pass**

Run the same Go and Vitest commands from Step 2.

Expected: all selected tests pass with the v4 model-free response and sample-free card.

- [ ] **Step 5: Run integration verification**

Run:

```bash
cd upstream/sub2api/backend && go test ./internal/service ./internal/handler -count=1
cd upstream/sub2api/frontend && pnpm vitest run src/features/monitor-v2
cd upstream/sub2api/frontend && pnpm build
```

Expected: Go unit suites, Monitor V2 frontend suites and production frontend build all exit 0.

- [ ] **Step 6: Commit**

```bash
git add docs/project/project-progress.md docs/superpowers/specs/2026-07-31-monitor-v2-card-simplification-design.md docs/superpowers/plans/2026-07-31-monitor-v2-card-simplification-implementation-plan.md upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/internal/service/monitor_v2_test.go upstream/sub2api/backend/internal/handler/monitor_v2_handler.go upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go upstream/sub2api/frontend/src/features/monitor-v2/types.ts upstream/sub2api/frontend/src/features/monitor-v2/api.ts upstream/sub2api/frontend/src/features/monitor-v2/__tests__/api.spec.ts upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts
git commit -m "refactor: simplify monitor v2 cards"
```
