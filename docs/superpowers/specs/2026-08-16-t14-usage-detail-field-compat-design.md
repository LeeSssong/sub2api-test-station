# T14 用量详情上游扣费/利润字段兼容热修规格

**状态：** 规格已获根总控代审批准，待规格自审后进入计划批准流程
**任务包：** T14
**基线：** `main@f5b14808f`（根总控已确认生产发布源 `e91504e51` 已推送并部署）
**工作区/分支：** `codex/t14-usage-detail-field-compat` / `/Users/gongtengxinwen/.codex/worktrees/bd76/sub2api搭建`

## 1. 问题证据与当前行为

生产已确认 `GET /admin/usage/:id/upstream-cost` 响应包含 PascalCase 字段，例如 `NormalizedCostCNY`、`EvidenceStatus`。当前前端 `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue` 的管理员详情逻辑只读取 `normalized_cost_cny` 与 `evidence_status`，因此上游实际扣费和利润计算得到 `null`，界面显示 `-`，不可用原因提示也不会按状态出现。

代码核验结果：

- `upstream/sub2api/frontend/src/api/admin/usage.ts` 的 `getCostEvidence()` 直接请求 `/admin/usage/:id/upstream-cost`，没有响应归一化层。
- `upstream/sub2api/frontend/src/types/index.ts` 的 `UsageCostEvidenceDetail` 是 snake_case 合同。
- `UsageDetailDialog.vue` 通过 `adminCostDetail.normalized_cost_cny` 计算上游扣费与利润，并通过 `adminCostDetail.evidence_status` 选择不可用原因文案。
- 后端 `UsageHandler.GetUpstreamCost` 只转发 service 结果；本热修不改变后端账务计算、持久化、错误码或响应生产逻辑。

## 2. 目标与非目标

### 目标

在前端 API 边界把 PascalCase 与既有 snake_case 响应归一化为同一个 `UsageCostEvidenceDetail`，使管理员用量详情在生产 PascalCase 响应下恢复显示上游实际扣费、利润及不可用原因，同时保持已有 snake_case 响应兼容。

### 非目标

- 不改变账务口径、利润公式、聚合、持久化、历史数据或生产数据。
- 不修改后端 DTO、序列化策略、错误码、认证/权限行为或 API 路径。
- 不做迁移、历史回填、配置变更、重试、估算、补查或相邻页面重构。
- 不触碰 `T12` 或其他任务包，也不删除历史成本证据/复核能力。

## 3. 方案比较与选择

### 方案 A（推荐）：API 响应边界归一化

在 `getCostEvidence()` 收到响应后，将 PascalCase 与 snake_case 同名字段映射为 snake_case 结构，再返回现有类型。至少覆盖：`UsageLogID/usage_log_id`、`Source/source`、`EvidenceStatus/evidence_status`、`ReasonCode/reason_code`、`NormalizedCostCNY/normalized_cost_cny`、`ReviewID/review_id`、`ReviewCostCNY/review_cost_cny`。snake_case 优先保留，PascalCase 仅作为缺失时的兼容回退。

优点是兼容逻辑集中、详情组件和其他消费者无需重复判断、API 类型与运行时结构一致；不会改变后端合同或财务语义。代价是 API 层增加一个小型纯函数及对应测试。

### 方案 B：详情组件 computed 层回退

在 `UsageDetailDialog.vue` 的 computed 中分别读取 PascalCase 或 snake_case。改动文件少，但兼容逻辑绑定单一组件，API 返回类型仍与运行时不一致，未来其他消费者容易再次出现同类故障。

### 方案 C：后端显式 DTO/json tags 强制 snake_case

修改后端响应 DTO/序列化，使服务端始终输出 snake_case。合同更单一，但会改变生产 API 序列化行为，超出前端热修范围，需额外后端测试与发布风险，不适合本任务。

**选择：方案 A。** 它是最小、可回滚、对账务与后端零侵入的兼容修复。

## 4. 端到端数据流与接口契约

1. `UsageDetailDialog.vue` 继续调用 `adminUsageAPI.getById(id)` 获取本站用量详情。
2. 管理员范围继续调用 `adminUsageAPI.getCostEvidence(id)` 获取 `/admin/usage/:id/upstream-cost`。
3. API 层对 JSON 对象做一次字段归一化：snake_case 值存在时优先使用；否则读取对应 PascalCase 值；`null`、`undefined` 和空字符串保持原有语义，不转换为估算值或零值。
4. API 层返回 `UsageCostEvidenceDetail`，组件继续只读取 snake_case：
   - `normalized_cost_cny` 有数值时显示上游实际扣费，并按 `site actual_cost - upstream actual cost` 显示利润。
   - `evidence_status === 'unavailable'` 时按既有 `reason_code` 显示不可用原因。
   - 缺失/空值时保持 `-`，不伪造金额。

兼容字段映射：

| 规范字段 | PascalCase 输入 | snake_case 输入 |
| --- | --- | --- |
| `usage_log_id` | `UsageLogID` | `usage_log_id` |
| `source` | `Source` | `source` |
| `evidence_status` | `EvidenceStatus` | `evidence_status` |
| `reason_code` | `ReasonCode` | `reason_code` |
| `normalized_cost_cny` | `NormalizedCostCNY` | `normalized_cost_cny` |
| `review_id` | `ReviewID` | `review_id` |
| `review_cost_cny` | `ReviewCostCNY` | `review_cost_cny` |

不会新增字段、改变数值单位或修改组件利润公式。

## 5. 失败、安全与兼容语义

- API 请求失败仍由 `UsageDetailDialog` 现有错误处理负责；归一化异常不吞掉网络错误。
- 非对象、空响应或字段缺失返回安全的空值结构/原有 `null` 行为，详情继续显示 `-` 或既有不可用提示。
- 不记录、暴露或复制 API Key、凭据、请求体等敏感数据。
- 仅管理员 API 继续可访问上游成本；普通用户详情不调用该端点。
- 同时存在两种命名时，snake_case 优先，避免覆盖已标准化值。
- 对布尔、数字、字符串只做字段选择，不做单位/精度/符号转换。

## 6. 场景化验收矩阵

| 场景 | 输入 | 预期 |
| --- | --- | --- |
| PascalCase 成功 | `NormalizedCostCNY=0.004`、`EvidenceStatus=confirmed` | API 返回 snake_case；详情显示 `$0.004000` 与正确利润 |
| snake_case 回归 | 现有 snake_case 响应 | 行为与修复前一致 |
| 混合响应 | 同时含两种命名且值不同 | snake_case 优先 |
| PascalCase unavailable | `EvidenceStatus=unavailable`、`ReasonCode=record_not_found` | 上游金额/利润为 `-`，显示既有不可用文案 |
| 空/缺失金额 | 状态存在但金额为空 | 不估算、不转零，继续显示 `-` |
| API 失败 | HTTP 错误/网络异常 | 保持详情错误与重试流程 |
| 权限隔离 | 普通用户详情 | 不调用管理员上游成本端点 |

## 7. 测试策略

只保留直接相关验证：

- 前端 API contract test：PascalCase、snake_case、混合优先级、空字段映射。
- `UsageDetailDialog` 组件定向测试：PascalCase 数据显示上游扣费/利润；unavailable 文案与缺失值行为不回归。
- 必要的前端 typecheck/build（按现有脚本可执行范围）。
- `git diff --check` 与变更范围检查。

不运行全仓测试、无关模块回归、迁移测试或生产专项验收；后端测试不是本热修的必要变更，因为后端合同与逻辑不变。

## 8. 发布、线上验证与回滚

- 变更仅前端 API 兼容层及其直接测试；无迁移、无配置变化，`downtime_required=false` 预期不变。
- 候选完成实现、独立任务复审和最终全分支复审后只能进入 `READY_FOR_ROOT_REVIEW`，不得自行合并、推送、部署。
- 根总控授权合并后，仅在合并后的 `main` 执行直接相关验证、发布预检、推送、蓝绿部署和管理员登录态即时验收。
- 回滚方式：根总控从已验证的上一生产提交重新发布；前端 API 归一化为纯边界逻辑，不涉及数据或迁移回滚。

## 9. 变更文件与剩余风险

预期实现文件：

- `upstream/sub2api/frontend/src/api/admin/usage.ts`
- `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`
- `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue` 的直接组件测试文件（若现有测试位置不同，按仓库约定新增/扩展，不改组件逻辑）

不修改 `docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、后端 service/handler、迁移、配置或生产记录。

剩余风险：生产若未来改为第三种命名或返回非对象结构，仍会按缺失值安全降级为 `-`；该风险不在 T14 范围内。

## 10. 批准记录

- **方案批准：** 根总控于 2026-08-16 书面批准方案 A。
- **规格批准：** 待根总控对本文件自审结果书面确认后记录。
