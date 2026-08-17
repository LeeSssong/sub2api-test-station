# T17 用量详情“上游扣费/利润”统一 Sub 原生有效账号成本口径热修规格

- 任务包：T17
- 日期：2026-08-17
- 基线：main@d27ceba5278c957a8901e3ffb2108353375139fb
- worktree：/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t17-effective-account-cost-hotfix
- 分支：codex/t17-effective-account-cost-hotfix
- 规格状态：已完成自审；由唯一发布总控依据既定队列代审授权批准进入计划阶段

## 1. 问题证据与当前行为

使用记录列表和账号利润/经营页的 Sub 原生成本事实源为：

`effective_account_cost = COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`

管理员用量详情当前同时读取：

- `GET /api/v1/admin/usage/:id`：返回 `AdminUsageLog`，已经包含 `account_cost`、`account_stats_cost`、`account_rate_multiplier`、`total_cost`、`actual_cost`；
- `GET /api/v1/admin/usage/:id/upstream-cost`：返回历史 `usage_upstream_cost_evidence` 的严格账单核验状态/原因及可选 `normalized_cost_cny`。

`UsageDetailDialog.vue` 当前把 `normalized_cost_cny` 当作“上游扣费”主金额，并以它计算利润；当 `evidence_status=unavailable` 时显示 `-`。因此已存在的 `account_cost` 没有进入详情主金额，造成列表、详情和经营页不一致。

生产样本已确认：

- 125444：`actual_cost=0.0055241000`，`account_cost=0.0033144600`，利润应为 `0.0022096400`；
- 125509：`actual_cost=0.0097092000`，`account_cost=0.0058255200`，利润应为 `0.0038836800`；
- 125512：`actual_cost=0.0100099000`，`account_cost=0.0060059400`，利润应为 `0.0040039600`；
- 账号 214 当日 518 笔均有 `account_cost`，严格 evidence 均 unavailable，但账号成本合计和利润页已使用原生口径。

两条详情接口均 HTTP 200；根因是详情选错事实源，不是 API 失败。T14 只增加 PascalCase/snake_case 兼容，没有改变事实源。

## 2. 原生能力盘点与边界

已核对并复用的原生能力：

- 后端 `service.UsageLog` 已保留四个成本字段；
- `dto.AdminUsageLog` 和 `UsageLogFromServiceAdmin` 已向管理员详情输出上述字段；
- `backend/internal/repository/account_cost_sql.go` 已集中表达原生 fallback SQL；
- 经营页/统计查询已使用相同 SQL 公式；
- 前端 `AdminUsageLog` 类型已包含上述字段；
- `adminUsageAPI.getCostEvidence` 已兼容 PascalCase 与 snake_case；
- 详情组件已有严格 evidence 辅助状态/原因提示和错误隔离测试。

因此不新增服务、数据库字段、API endpoint、第二账务源或 evidence 表语义。

## 3. 方案比较与选择

### 方案 A：前端复用现有详情字段计算（推荐）

新增一个纯函数 `effectiveAccountCost`，按原生公式使用 nullish 语义（`0` 是有效值），详情主金额直接来自 `AdminUsageLog`；利润为 `actual_cost - effectiveAccountCost`。严格 evidence 请求仍保留，仅更新辅助状态/原因。

优点：改动最小、与现有 API/经营页事实源一致、无迁移和无接口扩张；每种历史 fallback 可直接测试。风险：公式需在前端与后端同步，使用单一 helper 与公式回归降低漂移。

### 方案 B：后端详情 DTO 新增 `effective_account_cost`

由后端统一计算并返回派生字段，前端只渲染该字段。

优点：公式集中。缺点：扩大 DTO/API 契约、需要后端 mapper/handler/前后端类型同步，且仍需保证已有四字段与经营页一致；对本次热修过度。

### 方案 C：继续以 evidence 为主，evidence unavailable 时回退

保留当前主事实源，只在 evidence 不可用时回退到 `account_cost`。

拒绝原因：同一流水在 evidence confirmed 时仍可能与 Sub 原生有效账号成本不一致，无法满足“唯一主金额口径”；也继续把严格账单核验事实错误地当作财务主事实源。

选择方案 A。

## 4. 端到端数据与控制流

1. 管理员打开详情，`adminUsageAPI.getById(id)` 获取现有 `AdminUsageLog`。
2. `effectiveAccountCost(detail)` 按以下顺序计算：
   - 有限的 `account_cost`（包括 0）直接返回；
   - 否则取有限的 `account_stats_cost`，否则取 `total_cost`；
   - 乘以有限的 `account_rate_multiplier`，缺失/null 时按 1；
   - 没有任何有限成本时返回 null，显示 `-`，不伪造 0。
3. “上游扣费”显示该 helper 的金额；利润显示 `actual_cost - effective_account_cost`。
4. 并行读取 `getCostEvidence(id)`，其 `source/evidence_status/reason_code/normalized_cost_cny/review_*` 仅用于严格账单核验辅助区和不可用原因提示：
   - `evidence_status=unavailable` 不隐藏主金额、不改变利润；
   - evidence 请求失败也不清空已从详情事实源计算出的主金额；
   - PascalCase/snake_case 兼容保持不变。
5. 普通用户详情仍使用用户端点，不输出管理员字段或 evidence。

## 5. 接口与字段契约

不新增接口、不改写入逻辑。管理员详情继续提供：

- `GET /api/v1/admin/usage/:id`：`actual_cost:number`、`total_cost:number`、`account_rate_multiplier?:number|null`、`account_stats_cost?:number|null`、`account_cost?:number|null`；
- `GET /api/v1/admin/usage/:id/upstream-cost`：既有 evidence 合同和 PascalCase/snake_case 兼容。

前端内部新增纯函数只接受现有字段，返回 `number|null`；不把派生字段加入普通用户 DTO 或数据库。

## 6. 失败、安全与兼容语义

- `account_cost=0`、`account_stats_cost=0`、`account_rate_multiplier=0` 都是有效数值，不能使用真假判断丢失；
- 旧流水 `account_cost=null` 按历史公式计算；
- evidence unavailable、evidence 请求网络/解析失败不阻断详情主金额；
- 只有主详情记录缺失或成本字段全部非有限时，主金额/利润显示 `-`；
- 不发起任何新的上游账单 HTTP 请求，不保存凭据，不修改账务写入，不回填历史数据；
- 继续保留 evidence 表和既有严格核验入口。

## 7. 兼容性、迁移与配置

- 数据库迁移：0；
- 历史回填/账务重算/生产数据修改：0；
- 配置、依赖、外部控制面：0；
- GitHub Actions：0；
- API 兼容：保持现有管理员详情和 evidence 两种字段命名兼容；
- 回滚：恢复详情组件使用 evidence 主金额的上一不可变镜像；不需要数据库回滚。

## 8. 场景化验收矩阵

| 场景 | account_cost | account_stats_cost | total_cost | multiplier | evidence | 期望上游扣费 | 期望利润 |
|---|---:|---:|---:|---:|---|---:|---:|
| 新流水主快照 | 0.00331446 | 0.01 | 0.02 | 0.25 | unavailable | 0.00331446 | actual - 0.00331446 |
| 历史自定义成本 | null | 0.01 | 0.02 | 0.25 | unavailable | 0.0025 | actual - 0.0025 |
| 历史默认成本 | null | null | 0.02 | 0.25 | unavailable | 0.005 | actual - 0.005 |
| 零值有效性 | 0 | 0.01 | 0.02 | 0.25 | unavailable | 0 | actual |
| 严格证据已确认 | 0.00331446 | 0.01 | 0.02 | 0.25 | confirmed | 0.00331446 | actual - 0.00331446 |
| evidence 请求失败 | 0.00331446 | 0.01 | 0.02 | 0.25 | 无响应 | 0.00331446 | actual - 0.00331446 |
| 全部成本缺失 | null | null | null | null | 任意 | - | - |

## 9. 测试策略

直接相关最小验证：

- 前端公式 helper 单元测试：account_cost、account_stats_cost fallback、total_cost fallback、null multiplier、0 值和全空；
- `UsageDetailDialog` 回归：account_cost 主金额、历史 fallback、evidence unavailable 仍显示主金额并正确利润、evidence 请求失败不清空主金额、PascalCase evidence 兼容；
- 后端管理员详情 API 回归：四个成本字段继续序列化，普通用户详情仍不泄露管理员字段；
- 受影响前端 Vitest、typecheck、production build；
- 受影响后端 handler/dto focused tests、compile-only/build、gofmt、diff-check；
- 迁移集合和 `.github/workflows` diff 必须为 0；
- 不运行全仓、压力、soak、mutation 或无关浏览器矩阵。

## 10. 发布、线上验收与回滚

候选仅在 T17 worktree 达到 READY_FOR_ROOT_REVIEW 后交由根总控。根总控合并最新 main、执行专项门禁、推送并运行既有本地/宿主蓝绿预检。

- `downtime_required=false`：按全局规则直接发布和线上验收，不重复请求授权；
- `downtime_required=true`：在停服/迁移/重启/切换前停止并请求用户授权；
- 线上验收：健康端点、管理员自然流水详情与列表成本数学一致、evidence unavailable 时详情不显示 `-`、账号利润页同值（允许展示精度差异），不修改生产账号或制造失败；
- 失败：保留候选、发布证据和回滚镜像，原任务继续修复。

## 11. 待决事项与批准记录

- 产品口径、样本、公式、边界和目标均由用户在任务委托中明确给出；
- 本轮无需要用户补充的产品澄清问题；
- 2026-08-17 唯一发布总控完成原生代码/接口/测试盘点、方案比较和规格自审，结论为方案 A；依据项目文件中“用户离席期间代审授权”，批准该规格进入实施计划阶段；
- 自审结果：PASS。无 TBD/TODO、无范围漂移、无迁移/回填/账务重算、无未解决安全或接口问题。
