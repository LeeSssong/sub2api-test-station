# T72 兑换码充值事务边界修复交接

## 状态

`READY_FOR_ROOT_REVIEW`

基线：`main@801f5a915`（T72 登记后）
候选：`codex/t72-redeem-transaction-fix`

## 根因

`RedeemService.Redeem` 已在外层 Ent 事务中标记兑换码并调用钱包协调器。`quotaWalletRepository.WithLockedWallet` 原先无条件调用 `r.client.Tx(ctx)`，在 ambient transaction 上触发 `ent: cannot start a transaction within a transaction`，外层回滚。生产兑换码 `eb1bc00840de1b7ff6d3c66d7ea1f648` 只读状态为 `id=44/type=balance/value=20/status=unused`，未发生入账。

## 实现

- `WithLockedWallet` 识别 `dbent.TxFromContext(ctx)` 并复用现有事务；仅在没有 ambient transaction 时自建、提交/回滚事务。
- 新增 `RedeemBalanceAdjuster.AdjustRedeemBalance`，负数兑换在钱包行锁内一次性扣减付费额度并封顶 0，赠送额度不变，消除旧快照覆盖并发充值的风险。
- legacy SQL fallback 测试显式构造无钱包协调器的 repository，保留旧 SQL 合同。
- 未改变正向充值、管理员退款、支付履约、返利、用量扣费的业务口径、幂等键或账本模型；无迁移、配置、依赖和生产数据变化。

## 验证

- RED：`go test ./internal/repository -run '^TestQuotaWalletRepositoryReusesAmbientTransaction$' -count=1` 在修复前因 nested `BEGIN` 失败。
- GREEN：同命令修复后通过。
- `go test ./internal/service -run 'Redeem|QuotaWallet|PaymentFulfillment|Affiliate|UsageBilling' -count=1` 通过。
- `go test ./internal/repository -run 'Redeem|QuotaWallet|PaymentFulfillment|Affiliate|UsageBilling' -count=1` 通过。
- `go test ./internal/handler ./internal/server -run 'Redeem|QuotaWallet|Payment' -count=1` 通过（handler 无匹配测试，server 无测试文件）。
- `go build ./cmd/server` 通过。
- `git diff --check` 通过。

未验证：本机 repository integration 依赖 rootless Docker/PostgreSQL；生产发布预检、部署和线上兑换专项验收尚未执行。

## 发布

无迁移、无配置变化，预期 `downtime_required=false`，以根发布预检为准。根总控须在当前唯一发布车道空闲且 T71 VERIFYING 收口后刷新候选、授权合并、从验证后的 `main` 推送并走既有蓝绿链。

## 线上验收与回滚

发布后先只读确认码 44 仍为 unused；在受控登录态兑换一次，核验码变为 used、`users.balance` 等于钱包 paid+gift、账本新增一条 `record_type=legacy_balance_adjustment/reference_type=redeem`（正向兑换服务使用 `redeem_credit`）且重复兑换不新增账本。失败沿既有蓝绿回滚；本任务无数据库回滚。

## 变更文件

- `upstream/sub2api/backend/internal/repository/quota_wallet_repo.go`
- `upstream/sub2api/backend/internal/repository/quota_wallet_repo_test.go`
- `upstream/sub2api/backend/internal/repository/user_repo.go`
- `upstream/sub2api/backend/internal/repository/user_repo_redeem_adjustment_test.go`
- `upstream/sub2api/backend/internal/service/quota_wallet.go`
- `upstream/sub2api/backend/internal/service/quota_wallet_test.go`
- `docs/superpowers/specs/2026-08-26-t72-redeem-transaction-fix-design.md`
- `docs/superpowers/plans/2026-08-26-t72-redeem-transaction-fix.md`
