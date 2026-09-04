# T91-A 额度账务基础交接

日期：2026-09-01

状态：READY_FOR_ROOT_REVIEW（已刷新）

## 范围

本交接只覆盖 schema、Ent、expand-only migration、Decimal 边界和只读 reconciliation 基础。T91-B（额度发放/订单/代充值）、T91-C（API 扣费拆分）、T91-D（退款 Saga/worker/管理员扣减）和 T91-E（历史切换/双写/cutover）均未实现。

## 已交付

- `backend/ent/schema/user_quota_grant.go`
- `backend/ent/schema/user_quota_adjustment.go`
- `backend/ent/schema/quota_decimal.go`
- 相关 payment/order/promo/redeem/user schema 字段与 Ent 生成代码
- `backend/migrations/233_quota_accounting_foundation.sql`
- `backend/internal/quota/amount.go`
- `backend/internal/quota/reconciliation/reconciliation.go`
- `backend/internal/quota/reconciliation/sql.go`（只读 PostgreSQL 快照加载）
- `backend/cmd/quota-reconciliation/`

对账快照现同时读取 `delta_usd` 与 `attribution_status`；对 `exact` attribution，
校验 paid/gift allocation 总额是否等于 signed delta 的绝对值，空 allocation 的非零
exact 记录也会报告 `usage_delta_mismatch`。

迁移仅新增表、字段、索引和外键，不删除旧表/旧列，不 truncate，不回填历史事实；保留历史 opening 负值语义。`attempted_quota_usd` 表示应扣额度，`paid_quota_delta_usd`/`gift_quota_delta_usd` 表示实际落账额度。

## 验证证据

通过：

```text
go test ./ent/schema ./migrations ./internal/quota ./internal/quota/reconciliation ./cmd/quota-reconciliation -count=1
```

通过内容包括迁移合同、Decimal 适配器、Ent Decimal/numeric 映射、reconciliation 差异分类、只读 SQL 查询（sqlmock）、JSON 十进制字符串解析和差异非零退出码。

`go vet` 对上述直接相关包及 `git diff --check` 也通过。

全仓 `go test ./... -run '^$'` 仍失败于既有无关编译问题：

- `internal/pkg/apicompat/responses_client_tools_test.go` 缺少 `responsesClientToolNames`
- `cmd/server/wire_gen_test.go` 的 `provideCleanup` 参数数量不匹配
- `internal/handler/handler_wiring_test.go` 的 `ProvideHandlers` 参数数量不匹配
- `internal/handler/openai_gateway_handler_test.go` 缺少 `openAIAccountScheduleModel`

真实 PostgreSQL 集成迁移测试（通过 Colima Docker socket）：

```text
DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock \\
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \\
go test -tags=integration ./internal/repository -run 'TestQuotaAccountingFoundationMigrationSchemaAndReadonlySnapshot|TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate' -count=1
```

通过，真实 PostgreSQL 容器中迁移全集可应用并重跑；T91-A 新表、`numeric(20,8)` 字段、关键索引和只读快照加载均通过。首次未设置 Docker 环境变量的尝试曾报 `rootless Docker not found`，不影响已显式配置 socket 后的通过结果。

最近一次刷新后，以上定向测试、`go vet`、`git diff --check` 以及 Colima PostgreSQL
集成迁移/只读快照测试均重新通过；新增 exact signed-delta 对账测试亦通过。

## 未执行与风险

- 未在验收站执行 migration；未执行生产 migration、部署或业务数据写入。
- reconciliation 支持脱敏 JSON 快照和 PostgreSQL `READ ONLY` 事务加载；命令不会自动修改余额或账务事实。生产 DSN 连接和真实 schema 执行仍未在本任务中运行。
- acceptance 运行态不足矩阵、真实退款与 provider 回调仍由 T91-D/E 或专门验收任务负责。
- 当前工作区包含其他任务和用户未提交修改；交接时不得整体清理或覆盖。

## 回滚

T91-A 未进入远端或部署车道。若根总控拒绝，可丢弃本窗口新增 T91-A 文件；若迁移未来已应用，只能按后续批准的兼容性撤销方案处理，不直接删除事实表或旧列。

## 根总控下一步

审查 SQL 与 Ent 字段逐项映射、确认 migration runner 可识别 `233_...sql`，然后决定是否进入 T91-B；在明确授权前不执行 acceptance/生产 migration、双写、cutover 或部署。
