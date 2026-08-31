# T104 Monitor V4 持久化快照验证报告

日期：2026-08-31（Asia/Shanghai）
状态：`READY_FOR_ROOT_REVIEW`

## 范围

T104 将 Monitor V4 从 HTTP 请求触发的全窗口统计改为 PostgreSQL 派生快照：singleton worker 每 5 分钟以同一截点生成 `24h/7d/30d`，页面只读最近成功快照。同步修正缺失已结算探测终态，使其按最新用户口径作为一次失败 `0/1` 进入分母。

初始基线为 `main@5e6ccee143f07ee34017c25e75979b74b6bcfc77`；候选在验证前无冲突刷新到 `main@fde3ece1b6e20a9e0b6a7ff47bf1e0be03213178`，生成 merge `09942e3f6a43222b46833db1d5ac1a9caa364dd7`。根总控应以最终文档收口后的分支 HEAD 作为候选 SHA。

## 实现文件

- `upstream/sub2api/backend/migrations/232_monitor_v4_snapshots.sql`
- `upstream/sub2api/backend/migrations/monitor_v4_snapshots_migration_test.go`
- `upstream/sub2api/backend/internal/repository/monitor_v4_snapshot_repo.go`
- `upstream/sub2api/backend/internal/repository/monitor_v4_snapshot_repo_test.go`
- `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- `upstream/sub2api/backend/internal/service/monitor_v4.go`
- `upstream/sub2api/backend/internal/service/monitor_v4_test.go`
- `upstream/sub2api/backend/internal/service/monitor_v4_snapshot_adapter_test.go`
- `upstream/sub2api/backend/internal/service/monitor_v4_snapshot_service_test.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_runner.go`
- `upstream/sub2api/backend/internal/service/account_monitor_runner_test.go`
- `upstream/sub2api/backend/internal/service/wire.go`
- `upstream/sub2api/backend/cmd/server/wire_gen.go`
- `upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go`
- `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts`
- `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts`
- T104 规格、计划、报告和交接文档

## 新鲜验证结果

以下命令在刷新合并 `09942e3f6` 上执行，之后仅改文档：

```text
cd upstream/sub2api/backend
go test -vet=off -count=1 -run 'TestMonitorV4|TestAccountMonitorRepositoryProjectMonitorV4|TestMonitorV4Snapshots' ./internal/repository ./internal/service
ok github.com/Wei-Shaw/sub2api/internal/repository 0.975s
ok github.com/Wei-Shaw/sub2api/internal/service 2.098s

go test -vet=off -count=1 -run 'TestAccountMonitorRunner|TestMonitorV4SnapshotRunner' ./internal/service
ok github.com/Wei-Shaw/sub2api/internal/service 2.819s

go test -vet=off -count=1 -run '^TestMonitorV4SnapshotsMigration$' ./migrations
ok github.com/Wei-Shaw/sub2api/migrations 0.476s
```

`git diff --check` 通过。`ops/assert-native-openai-concurrency-only.sh` 通过，T104 影响文件没有新增 admission/slow-session 代码。

## 功能覆盖

- 原子替换：三窗口、多分组、共享 UUID、插入失败 rollback。
- 读取完整性：无快照、窗口/时间/版本/UUID 不一致、非法计数、missing terminal 关系均 fail-closed。
- 读路径：保留当前用户专属分组裁剪和当前组元数据，读取持久化 `generated_at`，不调用 native 全窗口 projection。
- 刷新路径：同一分钟截点计算 24h/7d/30d；projection/store 失败不发布；一个 UUID 一次 replace。
- 指标：真实请求优先、探测桶一次化、缺失终态 `0/1`、成功样本截尾平均、成功真实请求缓存命中率保持合同。
- worker：启动立即刷新、5 分钟 ticker、非重入、peer leader skip、Stop 后不再刷新、nil refresher 不启动 loop。
- API/UI 合同测试代码：保持 response 字段集合，覆盖三窗口和失败时保留上次成功窗口。

## 阻断与未运行项

- Handler 聚焦测试被既有测试编译错误阻断：`ProvideHandlers` 参数不足，及 `openAIAccountScheduleModel` 未定义；目标 handler 测试未执行。
- 前端 `node_modules` 缺失；离线 frozen install 又被现有 lockfile/override mismatch 阻断，Vitest 未执行。
- 用户要求仅功能覆盖，因此未运行完整 server/frontend build、typecheck、全包回归和性能压测。
- 没有真实 PostgreSQL migration execute-twice 或生产数据复算证据。

## 迁移与发布属性

- 新增 expand-only migration 232；无回填、删除、生产业务数据写入或配置变更。
- `downtime_required=unverified`，以根合并后的发布预检为准。
- 候选未合并根 `main`、未推送、未部署、未触碰生产。

## 结论

后端核心功能覆盖在刷新候选上通过；handler/frontend 两组合同测试因已记录的仓库/环境阻断未执行，不能宣称通过。候选满足本轮用户要求的“实施完成后仅功能覆盖测试”范围，可交根总控审阅；任何合并、迁移预检和发布仍由根总控串行决定。
