# T30 OAuth 自购全量报表与真实保存闭环规格书

## 1. 问题证据与当前行为

- T23/T28 已提供原生 `PUT /admin/operations/accounts/:id/procurement`、`account_procurement_cost_versions` 台账、`accounts.procurement_cost_cny` / `accounts.estimated_usable_quota_usd` 投影，以及账号监控成本弹窗与会话幂等键。
- `GetSelfPurchasedReport` 当前从 `account_procurement_cost_versions` 与“至少一个采购字段非空”的 legacy projection 构造 `versions`，再 `JOIN versions`；没有采购版本或字段的 OAuth 账号因此不会进入 CNY 报表。
- CNY 表当前只有结算入口，没有逐账号“录入成本/编辑成本”入口；账号监控已有 `AccountMonitorCostDialog` 和同一保存 API。
- T28 handler 成功后已在采购-only PUT 中回读账号，但报表刷新失败仍可能被前端概括为加载失败；服务错误未提供可识别状态/最新账号数据契约。

## 2. 目标

1. 定位并修复采购成本保存仍显示 internal error 的真实失败链路；错误反馈明确，保存后刷新保持。
2. `GetSelfPurchasedReport` 候选集固定为 `accounts.deleted_at IS NULL AND accounts.type='oauth'` 全部账号；无采购配置账号生成 `cost_pending` 行，成本与预计额度为 null，标准流水/收入为 0 时仍显示 0。
3. CNY 表每行提供“录入成本/编辑成本”入口，复用 `AccountMonitorCostDialog`、同一 PUT 与台账；默认预计额度 60 USD；保存/清空后 CNY 表与账号监控同步刷新。
4. 失败语义可诊断：事务失败返回明确错误；已保存但页面刷新失败时保留成功状态与最新响应数据；重复提交保持幂等，不吞错、不假成功。

## 3. 非目标

- 不新增采购字段、迁移、第二事实源、第二计算逻辑或第二保存 API。
- 不改动 USD 经营页口径、usage_logs、结算算法或普通用户 DTO。
- 不执行生产部署、推送、合并或全局队列/总账修改；候选只停 `READY_FOR_ROOT_REVIEW`。

## 4. 方案比较与选择

### 方案 A（推荐）：报表 SQL 改为 OAuth 驱动的 LEFT JOIN + 共享成本弹窗

以 OAuth 账号为主表，采购版本按时间窗口 LEFT JOIN；无版本/无 projection 生成 synthetic `cost_pending`。CNY 行组件直接挂载已有 `AccountMonitorCostDialog`，通过父视图调用 `adminAPI.accounts.updateProcurementCost`，成功后刷新 CNY 报表并发出共享采购更新事件；账号监控继续从同一原生 API/投影读取。优点是保持原生事实源和最小改动；风险是 SQL 聚合需覆盖无版本行。

### 方案 B：先新增“账号候选查询”服务，再拼接现有报表

把 OAuth 候选单独查询后在 Go 层合并现有版本结果。可读性较好，但增加一次查询和 DTO 合并复杂度，容易出现两套过滤逻辑。

### 方案 C：CNY 表跳转账号监控编辑

只提供跳转链接，不在表内打开弹窗。实现快但不满足逐行入口与保存后双向刷新体验。

选择方案 A：单一 SQL 候选事实、复用已有原生弹窗/API，避免第二入口和第二事实源。

## 5. 端到端数据/控制流

1. CNY 视图加载 `GET /admin/operations/self-purchased-profitability`。
2. 服务以 OAuth 未删除账号为驱动，按日期窗口 LEFT JOIN 采购版本和 usage_logs；无成本账号返回 `cost_status=cost_pending`、成本/额度 null、流水字段 0 或实际值。
3. 行内点击“录入成本/编辑成本”打开 `AccountMonitorCostDialog`，初始成本取行值，额度取行值或默认 60 USD。
4. 保存/清空调用 `PUT /admin/operations/accounts/:id/procurement`，request id 与 `Idempotency-Key` 复用账号监控会话策略。
5. PUT 成功后返回最新账号采购投影；前端以响应更新行并刷新 CNY 报表。若后续刷新失败，保留保存成功提示与返回数据，单独显示刷新失败并允许重试。
6. 重复 request id 返回同一结果，不重复写版本。

## 6. 接口与字段契约

### PUT `/admin/operations/accounts/:id/procurement`

请求：`{ procurement_cost_cny: number|null, estimated_usable_quota_usd: number|null, request_id: string }`，成本与额度必须同时为空或同时为非负/正数；request id 非空。

成功 200：沿用原生账号响应，包含 `procurement_cost_cny`、`estimated_usable_quota_usd`、`procurement_cost_effective_at`。

输入错误/账号不存在/幂等冲突分别保留 400/404/409。真正内部写入错误返回中文 message、`reason=procurement_update_failed`、account_id/request_id。台账已提交但账号回读失败返回 HTTP 202、`reason=procurement_saved_readback_failed`、request_id 与采购投影；API 层把 interceptor rejection 归一为 `procurement_readback_status=failed` 判别结果。

重复 request id 不新增版本；服务提交空事务后重试回读，仍按 200 或可识别的 202 partial-success 返回。

### GET `/admin/operations/self-purchased-profitability`

每个未删除 OAuth 账号恰好一行；`account_count == rows.length == oauth_count`。无成本行 `cost_status='cost_pending'`、`procurement_cost_cny=null`、`estimated_quota_usd=null`、`confirmed/pending/loss/net_profit` 按既有口径为 0 或 null，`standard_consumed_usd` 与 `revenue_cny` 仍显示实际/0。

## 7. 失败语义

- DB/服务不可用：返回稳定错误码与 HTTP 5xx，前端保留旧数据并显示可重试错误。
- 保存成功、后续刷新失败：前端标记“保存成功但刷新失败”，保留 PUT 返回投影，不回滚、不假成功。
- 重复提交：幂等 replay，不新增版本。
- 页面刷新：重新 GET 必须反映数据库投影；若旧 bundle/缓存，验收记录版本标识和强制刷新结果。

## 8. 兼容性与迁移

无迁移、无新字段、无配置变化。复用 `account_procurement_cost_versions` 和现有 accounts 投影；SQL 仅改变候选过滤与空值投影。

## 9. 验收矩阵

| 场景 | 期望 |
|---|---|
| 全部 OAuth（含无成本） | 行数等于未删除 OAuth 数；无成本显示“成本待录入”、流水 0 |
| 逐行录入 | 打开共享弹窗，默认 60 USD，PUT 成功，CNY/监控同步刷新 |
| 编辑/清空 | 同一 API/台账，清空后成本待录入 |
| 台账成功但 readback 失败 | HTTP 202 + `procurement_saved_readback_failed` + request_id/采购投影；前端显示“已保存但刷新失败” |
| 重复提交 | 同 request id 幂等 replay，无重复版本 |
| 页面刷新/缓存 | 刷新后数据库值一致，记录 bundle/version/cache 证据 |
| DOM/响应 | 逐行入口可见，表格无横向溢出，状态文案稳定 |

## 10. 测试策略

- Go：`GetSelfPurchasedReport` 全 OAuth 候选、无成本行、零流水、partial-success readback、真实 interceptor 形状与幂等 handler/service 测试。
- Vitest：CNY 行入口、默认 60、保存/清空调用同一 API、成功后双刷新、错误契约和 DOM 顺序。
- typecheck/build 与 `git diff --check`。

## 11. 发布、线上验证与回滚

候选不执行合并/推送/部署。根总控合并后按既有本地/宿主蓝绿链预检；预期 `downtime_required=false`。回滚为恢复上一已验证 main/镜像并保留候选 worktree；无迁移回滚负担。

## 12. 待决项与批准记录

- 不要求额外真实生产写入专项；根总控后续明确安排时再执行。
- 规格书由 T30 顶层任务按任务包授权完成；根总控代审批准记录：2026-08-19，批准进入实施计划与实现阶段，范围不扩大。
