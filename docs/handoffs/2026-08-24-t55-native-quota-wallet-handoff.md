# T55 原生额度钱包与手动充值退款交接

## 状态

`READY_FOR_ROOT_REVIEW`

基线：`main@1af258ba776a9ff6e72248f36cb685d1e5a4a4a3`

候选提交包含钱包 schema/迁移、额度 coordinator、既有余额写入适配、管理员额度 API 与用户余额/流水 UI；为支持本次仅测试站发布，候选同时让 admin-lab 发布链构建并传输 T55 后端镜像，不复用主站后端镜像。不得把本候选合并或发布到根 `main`/主站。

## API

- `GET /api/v1/admin/users/:id/quota-summary`
- `POST /api/v1/admin/users/:id/quota-ledger`，必须带 `Idempotency-Key`，支持 `recharge` / `refund`
- `GET /api/v1/admin/users/:id/quota-ledger?page&page_size&type`
- 旧 `POST /api/v1/admin/users/:id/balance` 保持兼容，但其写入已路由至 wallet coordinator。

## 验证

- `go test ./internal/service -run '^TestQuotaWallet' -count=1` 通过。
- `go test ./internal/repository -run '^TestQuotaWalletRepository' -count=1` 通过。
- `go test ./internal/handler/admin -run 'QuotaWalletHandler' -count=1` 通过。
- `go build ./cmd/server` 通过。
- 用户页面相关 `UsersView` 测试通过。
- `pnpm typecheck` 通过。
- `pnpm build` 通过。
- `git diff --check` 通过。
- `bash -n ops/release-admin-lab.sh ops/deploy-admin-lab-host.sh` 通过。
- admin-lab 后端镜像构建通过：`sub2api-admin-lab-backend:test-t55`（Linux AMD64）。

## 数据与发布边界

- 使用 expand-only 钱包/额度流水迁移；保留 `users.balance`、`frozen_balance`、`payment_orders`、`usage_logs`。
- 充值、退款和消费写入由 coordinator 在事务中同步钱包、兼容余额和流水；没有第二支付事实源。
- 预期涉及数据库迁移，最终 `downtime_required` 必须以根发布预检为准；若返回 `true`，在停机/迁移前暂停等待授权。
- 尚未推送、合并、部署或线上验收；本次只允许走隔离 admin-lab 发布链。

## 剩余风险

- 生产迁移前必须验证现有数据库的 `users.balance` 初始化、重复迁移安全和应用/worker 启动顺序。
- 真实支付履约、兑换码、返利和异步消费路径需要在合并后的 `main` 上做专项回归，确认所有余额写入都经过 coordinator。
- T57 经营分析依赖本任务的只读字段，T55 线上稳定后 T57 必须刷新基线并重新验证。
