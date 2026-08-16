# T14 用量详情上游扣费字段兼容实施计划

> **For agentic workers:** 使用 `superpowers:subagent-driven-development` 逐任务执行；每个实现任务先用 `superpowers:test-driven-development`，完成后由独立 reviewer 只读复审。

**Goal:** 在管理员用量详情 API 边界兼容 PascalCase 与 snake_case 上游成本字段，使现有详情弹窗恢复显示上游扣费、利润和不可用原因；不改变后端账务或 API 生产逻辑。

**Baseline:** `main@4f31ec3dd010dc3d2b6c5caaacadddce1adb84a2`，候选分支 `codex/t14-usage-detail-field-compat`；计划提交前已合入该最新 main。

**Scope:** 只改前端 `getCostEvidence()` 归一化及直接相关 API/组件测试；不改后端 DTO、账务公式、持久化、迁移、配置、权限、其他页面或全局账本。

## 门禁

- 规格 `11d62dc3c` 已获根总控方案与书面规格批准；本计划由根总控代为批准后才能实现。
- 候选只维护自己的规格、计划、实现、测试、复审和 handoff；不得修改 `docs/project/project-progress.md` 或 `docs/project/native-sub-task-package-queue.md`。
- 实现完成后依次 fresh task review、finding 修复与 scoped re-review，再 fresh whole-branch review；两级批准后才交付 `READY_FOR_ROOT_REVIEW`。
- 只运行直接相关的前端 contract/component 测试、typecheck/build、`git diff --check` 和范围/禁区扫描；不跑全仓或无关回归。

## Task 1: API 响应归一化与 contract tests

**Files:**

- Modify `upstream/sub2api/frontend/src/api/admin/usage.ts`。
- Modify or create the existing focused API test under `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`.

1. 先阅读现有 `getCostEvidence()`、`UsageCostEvidenceDetail` 类型和 mock/fixture 约定，写表驱动 RED 测试覆盖：纯 PascalCase、纯 snake_case、两种命名并存时 snake_case 优先、`null`/空字段保留缺失语义、非对象/空响应安全降级。
2. 新增 API 层私有归一化函数。每个规范字段只在 snake_case 缺失时回退到对应 PascalCase：`usage_log_id`/`UsageLogID`、`source`/`Source`、`evidence_status`/`EvidenceStatus`、`reason_code`/`ReasonCode`、`normalized_cost_cny`/`NormalizedCostCNY`、`review_id`/`ReviewID`、`review_cost_cny`/`ReviewCostCNY`。
3. 保留数字、字符串、空值和精度，不做单位转换、估算、默认零值或利润计算；网络错误继续原样抛给调用方。
4. 让 `getCostEvidence()` 返回既有 snake_case 类型，组件无需增加兼容分支。
5. 运行 API focused test，记录 RED/GREEN 和 diff 范围。

## Task 2: 详情组件行为回归与最小验证

**Files:**

- Modify/create the existing direct test for `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue` according to repository test placement.
- Do not modify the component unless a test proves a non-API contract defect; the approved design expects no component logic change.

1. 增加 PascalCase fixture 的组件测试，确认上游金额、利润和 `confirmed` 状态展示与既有 snake_case fixture 相同。
2. 增加 `unavailable`/`record_not_found` 和金额缺失测试，确认显示既有不可用文案或 `-`，不转成 0。
3. 确认普通用户/非管理员路径不调用管理员 upstream-cost 端点；沿用现有权限 mock，不新增权限。
4. 运行直接相关 API+组件测试；按现有前端脚本执行必要 `pnpm typecheck` 与 `pnpm build`。
5. 运行 `git diff --check`、允许路径 guard、禁区 guard（后端、迁移、配置、Actions、其他页面无命中），并扫描任务文档邮箱/凭据。

## Task 3: 证据、自审与提交

**Files:**

- Create `docs/superpowers/reports/2026-08-16-t14-usage-detail-field-compat-implementation.md`。

1. 报告基线、commit/tree、精确变更文件、测试结果、未验证项、无迁移/配置变化、`downtime_required=false` 预期和上一生产提交回滚方式。
2. 对照规格检查 snake_case 优先、缺失值安全降级、后端/账务/权限未变、T12 未触碰、生产数据未改。
3. 提交实现与报告；保持工作区 clean。
4. 由独立 task reviewer 复审本任务；发现项修复后 scoped re-review；随后 fresh whole-branch reviewer 复审全部差异。只有全部批准才写最终 handoff 并声明 `READY_FOR_ROOT_REVIEW`。

## 根总控后续

根总控收到候选后，确认候选基线未漂移且是当前唯一发布候选，授权合入最新 `main`；在合并后的 `main` 运行同一最小验证、创建 0600 发布证据、预检、推送、蓝绿部署和管理员登录态即时验收。预期无停机；失败则保留候选与证据并在原候选修复。回滚为上一已验证生产提交，不涉及数据回滚。
