# T35 采购审计 PostgreSQL 参数类型热修交接报告

## 任务状态

- 状态：`READY_FOR_ROOT_REVIEW`。
- 分支：`codex/t35-procurement-audit-type-hotfix`。
- 精确基线：刷新后 `main@f71e1195e8b3ddbb019dcf4285715b0788bb53aa`。
- 最终候选 HEAD：`d31b5c3c70ee5895f0cf23155bb91bdc726dcc4d`。
- 最终 tree：`a23ff0f9764a307347b0aa3693023cfee319db16`。
- 工作区：干净；未合并、未推送、未部署、未触碰生产。
- 根总控仍拥有合并、推送、发布预检、部署和线上验收权限。

## 提交清单

1. `5dca4b13104bad06658b39acd7d5caf79d5b38be` — T35 正式规格。
2. `c914a35e77b14d2c0fb98379fb061d71e5a38099` — T35 实施计划。
3. `df1f532642ad771d2805e8f89f1c35b2db07bd02` — 精确 SQL casts 合同与三条审计失败回滚 unit 测试。
4. `021f0c29a8d21ba2ec5503fb685b3f36eb8c70e2` — 真实 PostgreSQL 生产 service RED/GREEN integration 测试。
5. `3a51693b78198de3d3e3d6c7935d2b1923013707` — 修复基线 repository integration 包已有的 `stringPtr` 重定义，使 testcontainers 包可编译；仅重命名测试 helper 调用和声明，不改变业务代码。
6. `f5f13635b80d533818c6501cacf75a01d6dda7fe` — 将结算 integration GREEN 断言对齐既有无流水全额 `loss_cny` 公式。
7. `b3e3259b671ed65252bafa6d6bb6916971565c07` — 三条生产审计 SQL 的显式 PostgreSQL casts。

## 实现范围

生产文件 `upstream/sub2api/backend/internal/service/account_procurement_profitability.go` 仅增加：

- 结算：`$4::bigint`、`$5::text`；
- 清空：`$4::bigint`；
- 录入/修改：`$4::bigint`、`$5::double precision`、`$6::double precision`。

`LEFT($3,64)`、参数顺序、事务边界、幂等、版本台账、账号投影、审计键、成本公式和 handler/frontend 合同均保持不变。

## RED 证据

- T35 unit RED：精确 casts 合同在生产 SQL 未修改时失败，证明测试能捕获缺失 casts；三条 sqlmock 审计失败测试确认错误发生后调用 `Rollback`。
- 真实 PostgreSQL RED：

```text
DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock \
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock CI=1 \
go test -tags integration ./internal/repository \
  -run 'TestAccountProcurementAuditParametersUsePostgreSQLTypes' -count=1 -v
```

在未加 casts 的生产代码上，PostgreSQL 18 testcontainers 实际启动 PostgreSQL/Redis，录入、清空、结算三个子测试均收到 `pq: could not determine data type of parameter $4`，并证明版本/投影/审计没有局部残留；随后以预期 `require.NoError` 失败结束 RED。

## GREEN 证据

同一真实 PostgreSQL testcontainers 命令在 casts 提交后通过：

```text
PASS
ok github.com/Wei-Shaw/sub2api/internal/repository 3.643s
--- PASS: TestAccountProcurementAuditParametersUsePostgreSQLTypes
    --- PASS: save_rolls_back_parser_failure_before_casts
    --- PASS: clear_rolls_back_parser_failure_before_casts
    --- PASS: settle_rolls_back_parser_failure_before_casts
```

GREEN 断言覆盖：

- 录入：账号投影、active 版本和 audit JSON 原子写入；`account_id/cost_cny/quota_usd` 为 JSON number。
- 清空：旧版本结束、新 `cost_pending` 版本、投影清空和 `cleared=true` audit 原子提交。
- 结算：版本变为 settled、settlement request id/loss 保留既有公式、`account_id` 为 number 且 reason 为 string。

直接 focused 验证通过：

```text
go test ./internal/service -run 'Test(UpdateProcurementConfig|SettleProcurement|ProcurementAudit)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 2.017s

go test ./internal/handler/admin -run 'TestAccountHandler.*Procurement' -count=1
ok github.com/Wei-Shaw/sub2api/internal/handler/admin 0.486s

go build ./cmd/server
```

`gofmt -d` 无输出，`git diff --check` 通过；禁止目录 diff（migrations、Ent schema、frontend、`.github/workflows`）通过零差异检查。

## 刷新后验证

候选已在同一 worktree 无冲突整合最新 `main@f71e1195e8b3ddbb019dcf4285715b0788bb53aa`，刷新合并提交为 `d31b5c3c70ee5895f0cf23155bb91bdc726dcc4d`。刷新后重新执行并通过：

```text
go test ./internal/service -run 'Test(UpdateProcurementConfig|SettleProcurement|ProcurementAudit)' -count=1
go test ./internal/handler/admin -run 'TestAccountHandler.*Procurement' -count=1
go build ./cmd/server
gofmt -d ...
git diff --check
```

本机 Docker/testcontainers 实际可用，使用 PostgreSQL 18.1、Redis 8.4 容器并以 `CI=1` fail-closed 重跑真实 integration；录入、清空、结算三个子测试均 `PASS`。刷新后相对新基线的范围仍仅为 T35 规格、计划、交接、service/repository 直接测试和测试 helper 修正；迁移、配置、schema、frontend 与 GitHub Actions 无变化。

## 全量验证说明

按 finishing gate 执行了 `go test ./...`。全量结果包含与 T35 无关的既有中文错误投影/网关文案断言失败，主要集中于：

- `internal/handler`：image model、gateway fallback、body limit、Codex model、image concurrency 文案期望；
- `internal/server/routes`：billing/simple mode 和 alpha-search 错误文案期望；
- `internal/service`：既有上游错误文案期望。

这些失败不触及采购 service、采购 handler、repository T35 integration 或三条 SQL；T35 直接相关验证均为 GREEN。全量命令已结束，无残留测试进程。

## 迁移、配置与发布

- 数据库迁移：零；迁移文件和迁移集合未修改。
- schema/Ent：零。
- 配置、前端、GitHub Actions：零。
- 历史回填、生产数据写入：零。
- 预期 `downtime_required=false`；最终以根总控合并后发布预检为准。
- 回滚：代码提交反向应用并沿既有蓝绿链恢复上一镜像；无数据库逆向迁移。已成功写入的业务事实不做删除，修复前失败事务已由 PostgreSQL 原子回滚。

## 未验证项与根总控下一步

- 未执行合并后 `main` 重测、推送、发布预检、部署或线上验收，按任务边界留给根总控。
- 全量 Go suite 的既有文案失败未在 T35 范围内修复。
- 根总控可在候选 HEAD `d31b5c3c70ee5895f0cf23155bb91bdc726dcc4d` 上复核直接证据，随后决定是否发出 `AUTHORIZE_MERGE_TO_MAIN`。
