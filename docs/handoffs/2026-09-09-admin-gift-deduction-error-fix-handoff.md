# 管理员赠送额度扣除报错修复交接

- 任务：管理员赠送额度扣除报错修复
- 基线：`origin/main@02792c02da54a2816a4967cfba4aa4db9c5a07d6`
- 候选分支：`codex/admin-gift-deduction-error-fix`
- 根 `main` 合并提交：`b1f56f932f7d38241849094902f5fc113a05db12`
- 根 `main` tree：`1a6f12985e9fc1fb8bf2ad769b16eb37ed17c9b3`
- 作者：Codex
- 时间：2026-09-09
- 状态：已合入并推送根 `main`，独立测试站已部署，等待管理员功能验收

## 变更

- 新增稳定业务错误码 `GIFT_QUOTA_INSUFFICIENT`，替代扣除失败时不稳定的通用错误文本。
- 管理员 `subtract` 仍严格调用 gift-only deduction；充值额度不会被扣除。
- 前端按错误码显示“赠送额度不足，不能扣除充值额度”，其他错误继续显示后端详情或通用兜底。
- 保留已有 `Idempotency-Key` 自动生成和发送逻辑，避免重复请求和后端幂等校验错误。
- 未新增迁移、配置或账务事实源。

## 变更文件

- `upstream/sub2api/backend/internal/service/user_service.go`
- `upstream/sub2api/backend/internal/service/admin_user.go`
- `upstream/sub2api/backend/internal/service/admin_service_update_balance_test.go`
- `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`
- `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.spec.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/overview.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/overview.ts`

## 测试

- `pnpm vitest run src/api/__tests__/admin.users.spec.ts src/components/admin/user/UserBalanceModal.spec.ts src/components/admin/user/UserBalanceHistoryModal.spec.ts`：14/14 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过，1095 modules transformed。
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
- 测试站发布：已执行成功。release `/opt/sub2api-test-station/releases/b1f56f932f7d38241849094902f5fc113a05db12`，image digest `fed0f609a051eee6bd6540c240982b1de5516f7d3ef1416f5a0967b4f7cd5039`；`/health` 返回 `{"status":"ok"}`，`/readyz` HTTP 200，API/worker/detector/PostgreSQL/Redis/Caddy healthy。
- 回滚：切回测试站上一已验证 release；本次无数据迁移，不需要数据回滚。
- 未验证：尚未进行真实管理员登录态扣除操作；需在测试站先验证“足额扣除成功、赠送额度不足提示、充值额度保持不变、重复请求不重复扣除”。
