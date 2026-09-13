# 管理员赠送额度扣除报错修复交接

- 任务：管理员赠送额度扣除报错修复
- 基线：`origin/main@ac033ebac0515161e80f501d3b2dd9e6f51226b6`
- 候选分支：`codex/admin-gift-deduction-error-fix`
- 作者：Codex
- 时间：2026-09-09
- 状态：已合入并推送根 `main`，独立测试站已部署并完成内部测试用户数据清理；主站未部署

## 变更

- 管理员 `subtract` 仍严格调用 gift-only deduction；充值额度不会被扣除。
- 空白备注自动使用“管理员扣除赠送额度”，显式备注保留并去除首尾空白，满足账务审计表的非空约束。
- 保留已有 `GIFT_QUOTA_INSUFFICIENT` 错误码、前端错误提示和 `Idempotency-Key` 逻辑。
- 未新增迁移、配置或账务事实源。

## 变更文件

- `upstream/sub2api/backend/internal/service/quota_accounting.go`
- `upstream/sub2api/backend/internal/service/quota_accounting_test.go`

## 测试

- `pnpm vitest run src/api/__tests__/admin.users.spec.ts src/components/admin/user/UserBalanceModal.spec.ts src/components/admin/user/UserBalanceHistoryModal.spec.ts`：14/14 通过。
- `pnpm typecheck`：通过。
- `go test -tags unit ./internal/service ./internal/handler/admin -run 'Test(AdminService_UpdateUserBalance|QuotaWallet)' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `git diff --check`：通过。

测试输出存在既有 pnpm/依赖弃用提示、Browserslist 数据过期提示和 Vite 动态导入提示，不影响本次结果。

## 发布与风险

- migration：无。
- 配置：无。
- 生产数据：未触碰。测试站仅清理内部测试用户 `1098808377@qq.com`（`id=2`）的关联业务数据，保留用户账户行。
- 凭据：未读取或写入。
- 主站授权：未取得，未部署。
- 测试站发布：已从根 `main@944bbadf` 成功发布；release `/opt/sub2api-test-station/releases/944bbadfb0ec9268d7ab600bbd01d52ac427d0e8`，source tree `b476ef25d9ca991c8b86091e4a2851ea88119e31`，image digest `6540d6911b182b1225e1f96f0ccbc5676f38ea4d3ea360e99585e23a5f6e6d6e`；`/health` 与 `/readyz` 通过。
- 回滚：代码回退到基线 commit；本次无数据迁移，不需要数据回滚。
- 未验证：尚未进行真实管理员登录态扣除操作；代码级直接相关测试已通过，需管理员在测试站执行最终 UI 验收。
- 数据清除：已在单事务中完成；用户账户行保留，所有 `user_id=2`/`target_user_id=2` 业务关联记录验证为 0，余额、冻结余额和累计充值归零；其他用户仅指向该测试用户的操作人/邀请人引用置空。
