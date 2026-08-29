# 星桥 Q 额度、预收入与账务规则 v2 Implementation Plan

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

Goal: 在 Sub2API 原生支付、Q 余额和使用记录基础上，落地现金付费/非现金/赠送 Q 归因、预收入、聚合管理员账务退款和只读经营分析。

Architecture: 保留 users.balance 作为唯一对外 Q 总余额，使用内部来源投影区分 cash_backed_paid_q、non_cash_paid_q 和 gift_q。真实支付继续由 payment_orders 管理；所有额度发放进入统一发放记录；消费通过内部事件和只读归因投影关联 usage_logs；管理员退款按现金付费额度聚合池处理，不按订单分摊。

Tech Stack: Go、PostgreSQL、Ent schema、Gin、Vue 3、TypeScript、Vitest、现有 Sub2API payment/quota/usage billing 链路。

Spec: docs/superpowers/specs/2026-08-29-xingqiao-quota-preincome-accounting-v2-design.md

## Global Constraints

- 用户账户对外只展示一个 Q 总余额；人民币不进入用户余额，也不参与 API 消费校验。
- 页面额度统一显示 Q，不得显示为美元或 USD。
- 实收入 = cash_paid_consumed_q；charged_quota_q 是本次总扣费。
- 真实支付必须关联 payment_orders；非支付来源不得伪造支付渠道字段。
- usage_logs 不可变，只能增加只读额度归因投影。
- v1 退款是管理员账务退款，不要求关联 payment_orders，不按充值订单分摊。
- 返利默认进入 gift_q；管理员账务退款默认不清零赠送额度。
- 技术层保留追加式账务事件记录，但产品层不把它作为第二余额事实源。
- 不使用 GitHub Actions；涉及迁移、账务写入、停机或主站发布必须遵守项目全局授权门禁。
- 每个任务包必须从届时最新干净 main 建立独立 worktree、规格/交接文件、直接测试和单独发布。

## File Map

- upstream/sub2api/backend/internal/service/quota_wallet.go：Q 来源状态、充值/消费/兼容调账接口。
- upstream/sub2api/backend/internal/repository/quota_wallet_repo.go：钱包锁、Q 状态更新、内部事件写入和幂等。
- upstream/sub2api/backend/internal/repository/usage_billing_repo.go：原生扣费事务与 usage_billing_dedup 边界。
- upstream/sub2api/backend/internal/service/gateway_usage_billing.go：生产扣费编排和逻辑请求 ID。
- upstream/sub2api/backend/internal/service/payment_fulfillment.go：真实支付订单履约和充值码发放。
- upstream/sub2api/backend/internal/service/redeem_service.go：兑换码消费。
- upstream/sub2api/backend/internal/repository/affiliate_repo.go、internal/service/affiliate_service.go：返利转入。
- upstream/sub2api/backend/internal/handler/admin/user_handler.go：管理员余额、Q 摘要和额度操作 API。
- upstream/sub2api/backend/internal/service/business_overview.go：经营总览读模型。
- upstream/sub2api/backend/ent/schema/payment_order.go、user_wallet.go、user_quota_ledger_entry.go：Ent schema。
- upstream/sub2api/backend/migrations/227_user_quota_wallet_ledger.sql：已部署钱包/事件表迁移。
- upstream/sub2api/frontend/src/api/admin/users.ts：管理员用户、Q 摘要和额度接口类型。
- upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue：手工余额/Q 操作弹窗。
- upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue：用户余额/额度历史。
- upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue：经营总览页面。

---

### Task 1: 统一额度发放记录与真实支付来源

Deliverable: 每次 paid_q 或 gift_q 发放都可追溯；真实支付发放直接关联 payment_orders；非支付来源不污染真实支付统计。

Files:

- Create: upstream/sub2api/backend/migrations/<next>_quota_issuance_records.sql
- Create: upstream/sub2api/backend/ent/schema/quota_issuance_record.go
- Modify: upstream/sub2api/backend/internal/service/payment_fulfillment.go
- Modify: upstream/sub2api/backend/internal/service/redeem_service.go
- Modify: upstream/sub2api/backend/internal/repository/affiliate_repo.go
- Modify: upstream/sub2api/backend/internal/service/quota_wallet.go
- Modify: upstream/sub2api/backend/internal/repository/quota_wallet_repo.go
- Modify: upstream/sub2api/backend/internal/handler/admin/user_handler.go
- Test: payment_fulfillment_test.go、redeem_service_redeem_test.go、affiliate_repo_test.go，以及新的 issuance service/repository/handler focused tests。

Interfaces:

- Produce QuotaIssuanceRecord with source_type、amount_cny、paid_q、gift_q、cash_backed_q、payment_order_id、source reference、operator、policy snapshot 和 status。
- Produce CreateIssuance(ctx, input) returning record and error；服务端生成 cash_backed_q、operator 和 confirmation status。
- source_type=payment 必须有 payment_order_id；manual_recharge、redeem_code、affiliate、admin_gift 不得写 provider 字段。
- Payment fulfillment、redeem、affiliate 保持现有状态机；只在成功事务内增加发放记录。

Steps:

- [ ] Step 1: 写失败 schema/source contract 测试：支付来源必须关联 payment_order；非支付来源 payment_order_id 为空；同一 source reference 不能重复确认。
- [ ] Step 2: 运行 go test ./internal/service ./internal/repository ./internal/handler/admin -run 'Issuance|PaymentFulfillment|Redeem|Affiliate' -count=1，确认新断言失败。
- [ ] Step 3: 添加迁移和 Ent schema：decimal 金额、来源引用索引、确认唯一约束，不改变 payment_orders 生命周期。
- [ ] Step 4: 实现发放服务/仓储：decimal 校验、来源字段校验、操作人、幂等和事务。
- [ ] Step 5: 在 PaymentService.doBalance 成功履约后创建 source_type=payment、payment_order_id=o.ID 的发放记录。
- [ ] Step 6: 兑换码创建无现金 paid_q 发放；affiliate 创建 gift_q 发放；均不创建 payment_order。
- [ ] Step 7: 将管理员操作拆为 manual_recharge 和 admin_gift；拒绝用 amount_cny=0 隐式表达赠送。
- [ ] Step 8: 运行服务、仓储、handler 和迁移 runner 测试，确认真实支付状态机不回归。
- [ ] Step 9: 提交本任务包，提交信息为 feat: record quota issuance sources。

### Task 2: 三池 Q 状态和只读使用归因

Deliverable: 用户只有一个总 Q；内部按现金付费、非现金 paid、赠送三池消费；usage_logs 只读关联出实收入和赠送消耗。

Files:

- Modify: upstream/sub2api/backend/migrations/<next>_quota_source_pools.sql
- Modify: upstream/sub2api/backend/ent/schema/user_wallet.go
- Modify: upstream/sub2api/backend/ent/schema/user_quota_ledger_entry.go
- Modify: upstream/sub2api/backend/internal/service/quota_wallet.go
- Modify: upstream/sub2api/backend/internal/repository/quota_wallet_repo.go
- Modify: upstream/sub2api/backend/internal/repository/usage_billing_repo.go
- Modify: upstream/sub2api/backend/internal/service/gateway_usage_billing.go
- Create: upstream/sub2api/backend/internal/service/usage_quota_attribution.go
- Create: upstream/sub2api/backend/internal/repository/usage_quota_attribution_repo.go
- Modify: upstream/sub2api/backend/internal/handler/admin/usage_handler.go
- Test: quota_wallet_test.go、usage_billing_repo_integration_test.go 和新的 attribution focused tests。

Interfaces:

- QuotaWallet exposes CashBackedPaidQ、NonCashPaidQ、GiftQ、TotalQ。
- ConsumeUsage returns CashPaidConsumedQ、NonCashPaidConsumedQ、GiftConsumedQ。
- GetUsageQuotaAttribution(ctx, usageLogID) is read-only。
- 消费顺序为 cash_backed_paid_q -> non_cash_paid_q -> gift_q。
- usage_billing_dedup 仍是唯一扣费幂等边界。

Steps:

- [ ] Step 1: 写失败测试：全现金、全赠送、现金 2Q + 赠送 1Q、兑换码 paid Q，以及归因和 charged_quota_q 守恒。
- [ ] Step 2: 运行 go test ./internal/service ./internal/repository -run 'QuotaWallet|UsageBilling|Attribution' -count=1，确认失败。
- [ ] Step 3: 扩展 wallet 状态和迁移，不增加用户侧人民币余额；保持 users.balance = total_q。
- [ ] Step 4: 实现三池扣减，原生余额、来源池和内部事件同一事务。
- [ ] Step 5: 按逻辑请求 ID 记录三类消耗，不更新 usage_logs。
- [ ] Step 6: 实现只读归因投影，校验守恒；历史无法关联时返回 legacy_unattributed。
- [ ] Step 7: 接入统一 billing 和 legacy fallback，确保所有成功 actual_cost 扣费都经过协调器。
- [ ] Step 8: 运行 service、repository、handler 和迁移测试，确认历史 usage_logs 未修改。
- [ ] Step 9: 提交本任务包，提交信息为 feat: attribute usage by quota source。

### Task 3: 聚合管理员账务退款

Deliverable: v1 只支持订单无关的管理员账务退款，按现金付费额度聚合池校验。

Files:

- Create: upstream/sub2api/backend/migrations/<next>_admin_refund_records.sql
- Create: upstream/sub2api/backend/ent/schema/admin_refund_record.go
- Create: upstream/sub2api/backend/internal/service/admin_refund.go
- Create: upstream/sub2api/backend/internal/repository/admin_refund_repo.go
- Modify: upstream/sub2api/backend/internal/handler/admin/user_handler.go 或创建独立 handler。
- Modify: upstream/sub2api/backend/internal/server/routes/admin.go
- Preserve behavior: upstream/sub2api/backend/internal/service/payment_refund.go 的支付渠道退款状态机。

Interfaces:

- GetRefundableQuota(ctx, userID) returns refundableQ and error。
- CreateAdminRefund(ctx, AdminRefundInput) returns AdminRefundResult and error。
- refundable_q = cash_backed_issued_q - cash_paid_consumed_q - confirmed_admin_refund_q。
- payment_order_id optional，仅作说明，不参与退款资格。
- 默认只减少现金付费 Q，gift Q 不变。

Steps:

- [ ] Step 1: 写失败测试：四笔充值 1100Q、现金消耗 100Q、退款 600Q；超限、并发退款、幂等重放、不同 payload 冲突和事务回滚。
- [ ] Step 2: 运行 go test ./internal/service ./internal/repository ./internal/handler/admin -run 'AdminRefund|Refundable' -count=1，确认失败。
- [ ] Step 3: 添加退款表和索引，包含 CNY、退款 Q、状态、原因、操作人、凭证、可选 payment_order_id 和幂等键。
- [ ] Step 4: 在用户锁内聚合已确认发放、现金付费消耗和历史退款，负值只用于显示修正，非法写入必须拒绝。
- [ ] Step 5: 同事务扣减现金付费 Q、写退款记录、写内部事件并更新 users.balance。
- [ ] Step 6: 增加管理员 endpoint 和稳定错误码。
- [ ] Step 7: 运行退款测试及既有 payment_refund 测试，确认 provider refund 行为不变。
- [ ] Step 8: 提交本任务包，提交信息为 feat: add aggregate admin accounting refunds。

### Task 4: Q 管理员 UI、财务记录和经营总览

Deliverable: 管理员可分别查看真实支付/预收入、额度发放、管理员账务退款和使用归因；所有额度显示 Q。

Files:

- Modify: upstream/sub2api/frontend/src/api/admin/users.ts
- Modify: upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue
- Modify: upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue
- Modify: upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.spec.ts
- Modify: upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.spec.ts
- Modify: upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue
- Modify: upstream/sub2api/frontend/src/views/admin/__tests__/BusinessOverviewView.spec.ts
- Modify: upstream/sub2api/backend/internal/service/business_overview.go
- Modify: upstream/sub2api/backend/internal/handler/admin/dashboard_handler.go
- Modify: upstream/sub2api/frontend/src/api/admin/businessOverview.ts
- Modify: relevant zh/en admin locale files。

Interfaces:

- Q summary returns total_quota_q、cash_backed_paid_q、non_cash_paid_q、gift_q、refundable_q、refundable_cny。
- UI modes are manual_recharge、admin_gift、admin_refund。
- Business overview returns native total consumption plus cash_paid_consumed_q、non_cash_paid_consumed_q、gift_consumed_q、legacy_unattributed_q。
- Payment order page remains source for real payment/pre-income lifecycle。

Steps:

- [ ] Step 1: 写失败 Vitest：额度标签只能是 Q；手工充值、管理员赠送、管理员退款分开；退款不要求选择订单。
- [ ] Step 2: 运行 pnpm vitest run src/components/admin/user/UserBalanceModal.spec.ts src/components/admin/user/UserBalanceHistoryModal.spec.ts src/views/admin/__tests__/BusinessOverviewView.spec.ts，确认失败。
- [ ] Step 3: 更新 API 类型和后端 DTO；兼容保留旧 *_usd 字段，但新页面不显示 USD。
- [ ] Step 4: 拆分三种 UI 操作；提交前读取摘要，成功后使用服务端摘要，不用本地浮点覆盖。
- [ ] Step 5: 将支付/预收入、发放、退款和使用归因分组展示，显示来源引用和操作人。
- [ ] Step 6: 扩展 business overview，保留原生成本/总扣费公式，新增现金付费实收入和历史未归因消耗。
- [ ] Step 7: 验证键盘、错误重试、390px 无页面横溢出和 Q 文案。
- [ ] Step 8: 运行 Vitest、pnpm typecheck、pnpm build、相关 Go 测试、go build ./cmd/server、gofmt 和 git diff --check。
- [ ] Step 9: 提交本任务包，提交信息为 feat: expose q accounting views。

### Task 5: 停止人民币钱包产品语义并收敛兼容旁路

Deliverable: cash_balance_cny 不再是用户账户或退款事实源；所有 users.balance 写入经过协调器。

Files:

- Modify: upstream/sub2api/backend/internal/repository/quota_wallet_repo.go
- Modify: upstream/sub2api/backend/internal/service/quota_wallet.go
- Modify: upstream/sub2api/backend/internal/service/admin_user.go
- Modify: upstream/sub2api/backend/internal/service/payment_fulfillment.go
- Modify: upstream/sub2api/backend/internal/service/payment_refund.go，仅兼容回滚需要时修改。
- Modify: upstream/sub2api/backend/internal/repository/user_repo.go
- Modify: upstream/sub2api/backend/internal/service/business_overview.go
- Test: quota wallet、payment fulfillment、payment refund、admin balance compatibility tests。

Interfaces:

- 不新增用户侧人民币余额。
- cash_balance_cny 在删除前只作为 deprecated 兼容字段。
- 每次 users.balance 写入都产生对应 Q 事件或显式失败。
- provider refund 与 admin accounting refund 保持两条独立链路。

Steps:

- [ ] Step 1: 用 rg -n 'UpdateBalance|SetBalance|AdjustBalance|users.balance|cash_balance_cny|Recharge|LegacyAdjust' 盘点所有余额写入。
- [ ] Step 2: 写 bypass detection 测试，确保所有支持的写入调用协调器，且退款资格不读 cash_balance_cny。
- [ ] Step 3: 收敛 admin balance、payment fulfillment rollback、redeem、affiliate 旁路，不改变公开状态机。
- [ ] Step 4: 新 DTO/UI 停止返回或展示 cash_balance_cny；旧客户端需要的兼容字段继续保留。
- [ ] Step 5: 运行直接相关 Go 测试、server build、前端 typecheck/build、迁移 checksum 和 diff check。
- [ ] Step 6: 不删除字段、不删除历史数据；另立 cleanup proposal，待线上只读证明无消费者后处理。
- [ ] Step 7: 提交本任务包，提交信息为 refactor: retire cash wallet product semantics。

## Execution Order and Release Gates

1. Task 1 先于 Task 2；Task 2 先于 Task 3。
2. Task 3 和 Task 4 可并行设计，但合并、部署和线上验证仍为单车道。
3. Task 5 最后执行；不得在同一发布中删除 cash_balance_cny。
4. 每个任务包报告基线 main SHA、变更文件、迁移/配置变化、直接测试、风险、回滚和 downtime_required。
5. 部署前把候选合入已验证 root main；生产推送和部署只能来自该 main。
6. preflight 返回 downtime_required=true 时，在停服/重启/迁移前暂停并取得授权。
7. 候选只有在推送、部署和线上专项验证成功后才能标记 DONE。

