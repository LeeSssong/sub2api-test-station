# 管理员赠送/扣除额度与用户记录弹窗交接

- 任务：管理员赠送/扣除额度与用户记录弹窗修复
- 基线：`main@481fc8e2`
- 候选分支：`codex/admin-gift-balance-history-fix`
- 作者：Codex
- 时间：2026-09-08
- 状态：本地实现完成，等待根总控审查；未合入、未推送、未部署

## 变更

- 管理员 `add/subtract` 改为现有 quota accounting 的 gift-only grant/adjustment 路径，要求管理员 ID 和幂等键。
- 用户历史接口补充 `source`、`paid_quota_delta_usd`、`gift_quota_delta_usd`，合并兑换码、支付订单、管理员赠送和管理员赠送扣除来源。
- 新增迁移 `236_remove_legacy_admin_balance_history.sql`，只删除 `redeem_codes.type='admin_balance'`，不修改钱包、用户余额、支付订单、普通兑换码或并发记录。
- 用户充值/并发记录弹窗按竖向摘要布局展示可用、付费、赠送额度，记录项展示付费/赠送增量。
- 保留管理员赠送/扣除入口，移除旧管理员充值/退款入口语义。

## 测试

- `go test -tags unit ./internal/service ./internal/handler/admin ./migrations -run 'Test(AdminService_UpdateUserBalance_UsesGiftQuotaOnly|AdminService_UpdateUserBalance_RejectsGiftDeductionShortfall|MergeBalanceHistory|RemoveLegacyAdminBalanceHistoryMigrationTargetsOnlyLegacyRows)' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `pnpm vitest run src/components/admin/user/UserBalanceHistoryModal.spec.ts`：4/4 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过，1095 modules transformed。
- `git diff --check`：通过。
- 构建警告仅为现有 pnpm 配置/依赖弃用提示和 Vite chunk 提示。

## 发布与风险

- migration：新增 236，含不可逆删除；未在任何环境执行。
- 配置：无。
- 数据：未触碰测试站、主站或任何运行数据库。
- 凭据：未读取或写入。
- 主站授权：未取得，未执行主站发布。
- 测试站发布：未执行。
- 回滚：代码可恢复上一已验证 commit；迁移删除不能依靠代码回滚恢复，发布前必须按发布总控要求保留受保护删除计数和恢复证据。
- 未验证：尚未进行真实管理员操作、真实兑换码/支付订单线上验收；统一历史查询尚未在独立数据库上执行真实 SQL 验证。
