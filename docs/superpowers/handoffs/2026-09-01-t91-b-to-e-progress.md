# T91-B～E 隔离工作区交接（2026-09-01）

## 工作区

- worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t91-quota-accounting`
- branch: `codex/t91-quota-accounting`
- HEAD: `a4580f476fbb61f89ddbcba3a8f5a5429313ec2f`
- HEAD tree: `db29b1e9cbcee31560f657783e7a43892f8e8342`
- 当前变更均未提交；根目录 `main` 未写入、未合并、未推送、未构建、未部署。

## 已完成

- Ent quota schema、生成物及 payment order quota 字段。
- Decimal/numeric(20,8) 边界与支付订单额度快照。
- quota grant 创建及 redeem/promo/affiliate/admin gift 适配器。
- paid-first/gift FIFO、opening 排除、legacy debt offset 领域规则。
- `attempted_quota_usd` / `delta_usd` 命令契约。
- refund reservation、finalize/release 领域服务入口。
- refund retry/dead-letter 判定与 SQL outbox worker store。
- `quota_refund.requested` 事件常量。
- T91-E 只读 reconciliation/cutover dry-run 命令。

## 验证

通过：

```text
go test ./internal/quota/... ./internal/service -run 'Quota|quota|Refund|refund|PaymentOrderQuota|AdminRecharge|UsageBilling' -count=1
go test ./internal/repository ./ent/schema ./migrations ./cmd/quota-cutover ./cmd/quota-reconciliation -count=1
go vet ./internal/quota/... ./internal/service ./internal/repository ./cmd/quota-cutover ./cmd/quota-reconciliation
git diff --check
```

全量 `go test ./... -run '^$'` 仍有项目既有的非 T91 编译/测试基线失败（handler/apicompat/cmd-server 及若干旧 service 测试），未将其混入本任务修复。

## 尚未完成或需授权

- 真实 payment fulfillment 与新 grant 事实源的生产双写/切换仍需在发布总控授权下接入。
- 现有 legacy refund flow 尚未切换为 reservation→provider→finalize 的完整线上 Saga。
- 历史 opening 正式回填、dual-write cutover、生产 migration、真实 provider 退款、验收站/主站发布均未执行。
- 需要后续窗口补充 command-level integration、worker callback correlation 及 PostgreSQL reservation 测试。
