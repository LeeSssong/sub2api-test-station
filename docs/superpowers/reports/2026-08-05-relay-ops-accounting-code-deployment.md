# relay-ops 账务代码部署前资格

**日期：** 2026-08-05
**范围：** Task 2 — relay-ops 不可变镜像生产发布准备；本报告不是生产发布记录。
**部署权限：** 仅协调代理可推送并执行生产发布；本实施代理没有执行推送、镜像构建/上传、迁移、生产配置变更或线上验证。

## 部署前资格

- 基线已核对为 `7f7093be83a56116a123ef978b7dfcee199f3657`，开始及专项验证后工作树均无未提交改动；该基线为 `origin/main` 当前提交。
- 当前 canonical tree 的 accounting/reconciliation/app/config、Compose 与发布控制器不需要修复，因此没有制造运行时代码空改动。
- `infra/compose.yaml` 将 `RELAY_OPS_ACCOUNTING_ENABLED` 的默认值固定为 `false`。实际生产 secret env 必须继续显式保持 `RELAY_OPS_ACCOUNTING_ENABLED=false`；本任务未读取、写入或激活生产 secret。
- `config.Load` 在未设置该变量时解析为 disabled；`configuredAccountingService` 在 disabled 时返回 `nil`；应用测试覆盖 `/relay-ops/accounting` 在 disabled 时返回 `404`，因而不挂载账务页面或写入端点。
- 对账运行时接口在应用组成中存在，但所有 `/relay-ops/api/reconciliation/*` 路由都由管理员认证包装；HTTP 测试覆盖未认证请求返回 `401`。生产线上仍须由协调代理按 Task 2 Step 4 再次验证认证边界。
- 已运行且通过 Task 2 允许的专项命令：

```text
cd relay-ops-service && go test ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store -count=1
cd relay-ops-service && go vet ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store
bash tests/operations/release_relay_ops_test.sh
bash tests/operations/deploy_relay_ops_host_test.sh
```

- 发布控制器测试覆盖 0600 证据、干净工作树、不可变 digest、严格 SSH 主机校验及预加载归档校验；主机执行器测试覆盖 release-state 与共享容器身份契约。迁移集合哈希为 `044838bdb56aabb0ec779e3224936a3d56c5071bf516880a945a3103238035f5`。
- 为协调代理预留的 0600 证据绝对路径为 `/private/var/tmp/relay-ops-task2-evidence.4BcvMC/relay-ops-test-evidence.json`。证据在本报告提交后生成，字段将绑定该提交的 `source_commit`、`tested_tree` 和上述 migrations hash；协调代理须在未改变树的情况下使用该文件调用发布脚本。

## 协调代理后续唯一动作

在确认生产 secret 仍为 disabled、使用上述证据并保持工作树不变后，协调代理才可执行：

```bash
ops/release-relay-ops.sh --mode production --evidence /private/var/tmp/relay-ops-task2-evidence.4BcvMC/relay-ops-test-evidence.json
```

随后只验证镜像身份、`/healthz`、`/readyz`、账务/对账路由认证，以及 PostgreSQL、Redis、Caddy、Sub2API 容器身份未变，并停在用户验收门禁。不得由本报告推定生产已部署或已生效。
