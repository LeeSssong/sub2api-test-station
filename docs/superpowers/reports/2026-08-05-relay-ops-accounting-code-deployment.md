# relay-ops 账务代码生产部署与验证

**日期：** 2026-08-05
**范围：** Task 2 — relay-ops 不可变镜像生产发布，accounting 保持 disabled。
**部署权限：** 实施代理只完成资格与修复；协调代理在独立审查后完成推送、不可变发布和必要线上验证。

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

## 构建代理阻断修复

生产前的两次本地不可变镜像构建均在 `go mod download` 访问 `proxy.golang.org` 时失败，发生在归档上传和生产切换前。用户已批准将构建接口限定扩展为：Docker build 参数 `GOPROXY` 与发布控制器环境变量 `RELAY_OPS_BUILD_GOPROXY`。

- 默认值保持 `https://proxy.golang.org,direct`；只有 `https://goproxy.cn,direct` 可以作为本次协调代理的构建期覆盖。
- controller 在任何 Docker、SCP 或 SSH 副作用前拒绝非 allowlist 值；fake-Docker 回归同时覆盖默认、批准覆盖与非法覆盖。
- 覆盖只传递给镜像构建期的 `go mod download`，不进入生产运行时环境；Dockerfile 未关闭 Go checksum database。
- 此修复轮只完成本地 RED/GREEN 和 Task 2 四组专项回归。实际 push、带 `RELAY_OPS_BUILD_GOPROXY=https://goproxy.cn,direct` 的生产构建、运行时 `RELAY_OPS_ACCOUNTING_ENABLED=false` 复核，以及线上健康/认证/容器身份验证仍仅由协调代理执行。
- 先前的 `0600` 证据绑定的是修复前提交 `de739afaf`，不能用于包含本修复的发布；协调代理必须在本提交已推送且树保持不变时重新生成与新 commit/tree/migrations hash 绑定的证据。

## 生产发布结果

协调代理重新运行 Task 2 四组专项命令并生成 root-user-owned `0600` evidence；证据绑定：

- source commit：`8fef0e03c80a55ec1a1cceedabd1949bf12bfe8b`
- tested/source tree：`059bd91cd63533f1a6e98c5ab636a265f441537e`
- migrations hash：`044838bdb56aabb0ec779e3224936a3d56c5071bf516880a945a3103238035f5`
- result：`passed`，commands：4

生产构建显式使用 `RELAY_OPS_BUILD_GOPROXY=https://goproxy.cn,direct`；BuildKit 日志确认只有 `go mod download` 使用该构建参数，Go checksum database 未关闭。不可变预加载发布返回：

- result：`succeeded`
- requested image：`example.invalid/xingqiao-relay-ops:release-8fef0e03c80a55ec1a1cceedabd1949bf12bfe8b`
- previous image：`example.invalid/xingqiao-relay-ops:release-d3860531ddc59217a29513bd5fa9d057e301826c`
- image ID：`sha256:cbe11755570aacf38fb1c20510c288a37f20ab0bfb8f5e04c594e67c972bf979`
- relay-ops container：`342d58ebdec7b323e80f3d6037c301e1ca778c7ffc16a730526d66f62764175e`
- migration startup verified：`true`
- shared services unchanged：`true`
- restart count：`0`

生产 secret 为 root-owned `0600`，且唯一显式设置 `RELAY_OPS_ACCOUNTING_ENABLED=false`；修改前文件保留 root-owned `0600` 备份。线上必要验证结果：

```text
/healthz = 200, status=alive
/readyz = 200, status=ready
/relay-ops/accounting = 404 (accounting disabled, route not mounted)
/relay-ops/api/reconciliation/operations = 401 (unauthenticated)
```

PostgreSQL `2db52788ad73`、Redis `c45202c0d9e6`、Caddy `ace4a23b9650`、Sub2API blue `cfbaea1abd30`、green `9171b00cd77a`、worker `7e218a28a62d` 的容器 ID 均与发布前一致。本任务已推送、部署并完成必要线上验证，但按用户硬门禁保持“进行中 / 等待用户验收”；Task 3 未启动。
