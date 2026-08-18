# T23 自购账号独立采购成本与人民币利润模型

## 现状取证与原生复用
- 原生 `accounts` 已有 `procurement_cost_cny`、`estimated_usable_quota_usd`、`procurement_cost_effective_at` 投影及管理员更新校验。
- 原生 `usage_logs.actual_cost` 是站内实际消耗收入；标准 Token 总量由 input/output/cache token 字段组成；渠道 USD 经营页由 `AccountProfitabilityService` 维护。
- 原生管理员审计由 `AuditLog`/`AccountFinancialAudit` 提供；本任务复用其记录，不改变用户扣费、usage_logs 或渠道 USD 报表。

## 目标/非目标
目标：新增 expand-only 版本台账；首次配置从 `accounts.created_at` 生效、后续从修改时刻生效；成本按标准额度消耗、倍率和真实采购价封顶；失效确认结算剩余待摊成本为采购损失；独立人民币 API/UI 视图；幂等、并发保护与审计。
非目标：历史回填、删除/重写 usage_logs、汇率、改变渠道口径、普通用户接口、定时汇总、生产数据修改、GitHub Actions。

## 方案比较
1. 扩展旧 USD `AccountProfitabilityService`：改动最少但会混合币种、破坏现有契约，拒绝。
2. 新建独立 procurement ledger + service + endpoint（推荐）：事实源隔离、可审计、可逐步展开，复用原生账号/usage_logs 查询。
3. 写入 usage_logs 派生字段：查询简单但污染原始事实，拒绝。

## 数据模型与流转
新增 `account_procurement_cost_versions`：account_id、cost_cny、estimated_quota_usd、effective_at、ended_at、settled_at、loss_cny、status、actor_user_id、request_id、created_at、updated_at、version_no；唯一 `(account_id, version_no)` 与活动版本部分唯一索引。新增 `account_procurement_settlements` 以 request_id 唯一保证确认失效幂等（或同表状态转换）。管理员账号更新在事务中关闭活动版本、追加新版本并更新 accounts 投影；首次版本 effective_at 使用 account.created_at。

自购识别：`type='oauth'` 或已有采购投影且非 API key 的账号。标准消耗=usage_logs token 总量折算为 USD 的原生 `total_cost`（不使用 account_cost）；已确认成本=min(标准消耗*cost/quota, 采购成本)；待摊=max(采购成本-已确认成本,0)；失效损失=待摊（仅 confirmed_expired/administrator_confirmed_expired）；临时错误、暂停、可恢复禁用不结算。人民币营收=sum(actual_cost)。净利润=营收-已确认成本-损失；利润率=净利润/营收（营收为 0 显示 null）。未配置状态为 `cost_pending`，不按 0 成本。

额度变更：新版本记录剩余成本/剩余额度；已结算成本不回溯。版本结束时间作为旧版本 usage 截止；新版本从修改时间生效。

## API/UI 契约
- `GET /admin/operations/self-purchased-profitability?start_date&end_date&timezone` 返回 summary、rows、currency=CNY。
- `POST /admin/accounts/:id/procurement/settle` 接收 `{request_id, reason}`，仅允许失效状态，返回幂等结果。
- 经营页新增“自购账号”独立视图/标签，展示采购成本、预计额度、标准消耗、利用率、确认成本、待摊、采购损失、人民币营收、净利润、利润率、状态；渠道 USD 区块保持原样。桌面和 390px 使用堆叠卡片/可滚动表格避免横溢。

## 失败/兼容/迁移
请求缺少正数额度、负成本、未知 settlement reason 返回 400；重复 request_id 返回原结果；并发更新依赖事务锁和唯一活动索引，冲突返回 409。迁移仅新增表/索引，expand-only、可重复执行，无历史回填。

## 验收矩阵
覆盖 account_cost、created_at fallback、版本切换、剩余成本/额度、封顶、失效结算、临时状态不结算、未配置、渠道隔离、审计幂等、并发、API 与 390px UI 无横溢出。

## 批准记录
2026-08-18：依据根总控转达的用户已批准业务规则与代审授权，发布总控可代审本规格；本任务不等待额外产品决策。
