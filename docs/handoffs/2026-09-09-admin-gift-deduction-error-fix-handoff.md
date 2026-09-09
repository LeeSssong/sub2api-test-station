# 管理员赠送额度扣除报错修复交接

- 任务：管理员赠送额度扣除报错修复
- 基线：`origin/main@ac033ebac0515161e80f501d3b2dd9e6f51226b6`
- 候选分支：`codex/admin-gift-deduction-error-fix`
- 作者：Codex
- 时间：2026-09-09
- 状态：本地实现完成，待根总控审查；未合入、未推送、未部署

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
- 生产/测试站数据：未触碰。
- 凭据：未读取或写入。
- 主站授权：未取得，未部署。
- 测试站发布：未执行；当前约束仅允许对独立测试站只读核对，禁止使用旧发布链或直接执行数据清除。
- 回滚：代码回退到基线 commit；本次无数据迁移，不需要数据回滚。
- 未验证：尚未进行真实管理员登录态扣除操作；需在允许的测试站发布流程恢复后验证“空备注足额扣除成功、赠送额度不足提示、充值额度保持不变、重复请求不重复扣除”。
- 数据清除：用户要求清除内部测试用户的非账户数据，但该不可逆操作没有受控清理入口且项目约束禁止直接删除测试站数据，本候选未执行。
