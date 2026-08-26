# T74 交接：原生扣费与充值额度账本一致性

## 状态

`READY_FOR_ROOT_REVIEW`。候选基线为 `main@6d30ac9ef5526aa6860969baf6806ddd604bf8bc`；候选提交见本交接提交后的 Git SHA。无迁移、无配置/依赖变更、无生产数据写入。

## 实现

- `usage_billing_repo.go`：在成功 claim 原生 `usage_billing_dedup` 后，保留原 `deductUsageBillingBalance` 原子扣费，在同一 `*sql.Tx` 内锁定/初始化钱包、按 paid-first/gift-second 投影消费，并写 `usage_consumption`。技术透支落到 paid 负额，gift 保持非负，投影总额与原生新 `users.balance` 逐次校验。
- 对 T67 已产生的钱包高估：下一次该用户的可计费请求会在同一事务内先插一条 `migration_projection` 校准到本请求扣费前的原生余额，再插入真实本请求 `usage_consumption`；不会静默覆盖，也不改变用户的真实余额。
- 两个管理员余额弹窗打开时均刷新 `/admin/users/:id/quota-summary`；不再用列表中旧 `AdminUser.balance` 作为实时余额后备值。
- 生产只读影响报告：`docs/superpowers/reports/2026-08-26-t74-wallet-drift-readonly-impact.md`。17:16 时 8 个用户存在钱包高估，合计 `74.73037899`。

## 验证

- `go test -tags=unit ./internal/repository -run 'Test(DeductUsageBillingBalance|ApplyUsageBillingEffects|ProjectUsageBillingWallet)' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `pnpm vitest run src/components/admin/user/UserBalanceModal.spec.ts src/components/admin/user/UserBalanceHistoryModal.spec.ts`：3/3 通过。
- `pnpm typecheck`、`pnpm build`：通过。
- `git diff --check`：通过。
- integration test `TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling` 未能启动：本机 testcontainers 报 `panic: rootless Docker not found`。这与 T67 已记录的环境限制一致。
- 全包 repository 单测另有两项基线失败，已在未改动的主线复现：`TestApplyRedeemBalanceAdjustment_UsesAtomicFloor`、`TestApplyRedeemAdjustment_MissingUser`；它们属于 T72 兑换码测试夹具/实现边界，不由 T74 修改。

## 发布与回滚

- 仅可在根合并后的 `main` 完成直接相关门禁和发布预检；预检若返回 `downtime_required=true`，切换前必须等待用户确认。
- 正常蓝绿发布可回退到发布前 `main` 源。T74 没有迁移或直接生产数据写入；钱包投影仅随新的原生扣费事务产生可审计流水。
- 历史静态漂移的批量修复不在本包内。需在临近执行时重新只读核算，并取得用户对可审计对账写入的明确授权。
