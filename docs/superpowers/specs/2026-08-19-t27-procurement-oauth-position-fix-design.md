# T27 自购账号口径、保存失败与财务页位置修复

## 现状证据

- `UpdateProcurementConfig` 查询活动版本时把可空 `cost_cny` 与 `estimated_usable_quota_usd` 直接扫描到 `float64`；从 `cost_pending` 重新录入金额会在数据库 NULL 扫描阶段失败，事务不会完成。
- `GetSelfPurchasedReport` 的台账 CTE 与 legacy projection fallback 只按采购字段识别账号，未强制原生 `accounts.type = 'oauth'`，因此非 OAuth 账号可能进入 rows、summary 和 settlement 入口。
- `AccountProfitabilityView.vue` 在原生五张财务摘要卡之前渲染自购人民币面板；页面已有 `overflow-x-hidden`，自购表已有 `overflow-x-auto`，只需移动 DOM 位置。

## 目标与非目标

目标是修复上述三条用户可见行为，同时保持 T23 的事务、版本、幂等、actor 审计、accounts 投影和财务字段语义不变。`cost_pending` 版本不参与旧消耗/剩余额度折算。自购台账、legacy fallback、rows/summary/settlement 只接受 `oauth`。

非目标：不新增迁移、不回填或删除历史数据、不改变用户扣费、渠道 USD 口径、调度、采购公式或 API 字段，不引入新页面/事实源，不改发布链或 GitHub Actions。

## 方案比较

1. 在 handler/UI 层过滤账号：覆盖不全，后台报告和结算仍可能污染，拒绝。
2. 在原生 service SQL 入口统一过滤并在 service 扫描处做可空处理（推荐）：覆盖台账、fallback、聚合和写事务，改动最小且保持契约。
3. 新增数据库视图或迁移约束：部署面和回滚面扩大，且无法修复 NULL 扫描，拒绝。

## 数据与控制流

保存请求进入既有 `UpdateProcurementConfig` 事务。活动版本用 `sql.NullInt64`/`sql.NullFloat64`/`sql.NullTime` 兼容 NULL；若旧状态为 `cost_pending`，直接关闭旧版本并用新输入创建 active 版本，不读取 usage_logs 做旧成本/额度折算。其他状态继续沿用原剩余成本/额度算法。幂等、锁、投影和审计 SQL 保持原序列。

报告查询在 `versions` 两个分支均加入 `a.type = 'oauth'`，scoped 额外保留同一条件作为防回归屏障；因此 rows、summary、settlement 可见集合一致，历史非 OAuth 数据只读保留。

前端将现有 `self-purchased-panel` 节点移动到 `summary-grid` 之后、经营维度 Tab 之前，不改变加载/错误/刷新逻辑和表格滚动容器。页面根部继续限制 390px 横向溢出。

## 失败与兼容语义

NULL 旧值不再产生泛化 internal error；非法新值、幂等冲突、锁/数据库错误仍按原错误返回并回滚。非 OAuth 报告不产生行或汇总数，也不会开放结算按钮。API JSON 与现有 UI 文案不变。

## 验收矩阵

| 场景 | 预期 |
| --- | --- |
| cost_pending 旧版本重新录入 7.7/60 | 提交成功，旧版本关闭，新 active 版本和 accounts 投影写入，旧消耗不折算 |
| active 旧版本更新 | 原剩余成本/额度算法不变 |
| 台账含 oauth 与 api_key | 仅 oauth 出现在 rows/summary |
| 仅 legacy projection 的 oauth 与 api_key | 仅 oauth fallback 出现 |
| 非 oauth settlement 请求 | 查询/写路径拒绝或无可结算版本，不产生结算副作用 |
| 财务页加载/错误/刷新 | 自购面板位于五张卡之后、Tab 之前，状态与字段保持 |
| 390px | 根页面无横溢出，自购表仍自身横向滚动 |

## 测试、发布与回滚

先补 Go sqlmock service 测试、报告 SQL 合同/过滤测试和 `AccountProfitabilityView.spec.ts` DOM 顺序/390px 测试，再实现。运行直接相关 Go service/sqlmock、handler/API、AccountMonitor/AccountProfitability 前端测试，必要 typecheck、production build、gofmt、git diff --check；不跑全仓无关回归。无迁移、无配置变更，预期 `downtime_required=false`。回滚为恢复本提交前代码；不需数据回填。

## 自审与批准记录

范围仅触及既有 service、handler 相关测试和单一财务页；没有未决产品问题、占位项或接口矛盾。2026-08-19 依据用户已批准 T27 范围及根总控代审授权通过规格自审，可进入实施计划。
