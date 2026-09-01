# T87 READY_FOR_ROOT_REVIEW Handoff

日期：2026-09-01
任务包：T87 逻辑请求错误生命周期投影
状态：READY_FOR_ROOT_REVIEW

## 基线与候选

- 基线：`main@a4580f476fbb61f89ddbcba3a8f5a5429313ec2f`
- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api-t87-logical-request-error-lifecycle`
- 候选分支：`codex/t87-logical-request-terminal-projection`
- 候选提交：`322a819219a31261e27d063aca05718ea0f41955`
- 候选 tree：`6eae8f767b2f04cd3d354411d009b51f6c9dfeaa`
- 提交前置 T87 提交：`7f7dab997`；Monitor V4 逻辑请求修订提交：`a61a81d95`、`cf361e1ab`

## 实现范围

- 复用 `usage_logs` 与 `ops_error_logs`，按 `logical_request_id` 优先、缺失时 `request_id` 聚合逻辑请求。
- 增加终态、关联质量、attempt/failover/upstream error 计数、最终状态、协议、usage 完整性和安全重放诊断字段。
- 中间上游错误最终恢复时投影为 `auto_retry_recovered`；最终用户可见错误只计一条逻辑请求；证据不足保持 `incomplete_unknown`。
- Monitor V4 真实请求投影按逻辑请求终态去重，并保留完整 usage 才可作为成功证据。
- 管理员请求详情 API 类型与详情弹窗展示终态、尝试次数、切号次数和上游错误数；普通用户错误 DTO 未扩展诊断字段。
- 无数据库迁移、无配置 schema 变化、无运行时重试/切号策略变化、无生产数据写入。

## 变更文件

- `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- `upstream/sub2api/backend/internal/repository/ops_repo_request_details.go`
- `upstream/sub2api/backend/internal/repository/ops_repo_request_details_test.go`
- `upstream/sub2api/backend/internal/repository/ops_repo_request_details_integration_test.go`
- `upstream/sub2api/backend/internal/service/ops_request_details.go`
- `upstream/sub2api/frontend/src/api/admin/ops.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/ops.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/ops.ts`
- `upstream/sub2api/frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`
- `upstream/sub2api/frontend/src/views/admin/ops/components/__tests__/OpsRequestDetailsModal.lifecycle.spec.ts`
- `docs/superpowers/handoffs/2026-09-01-t87-logical-request-error-lifecycle-handoff.md`

## 验证

- 通过：T87 repository 直接测试与 Monitor V4 逻辑请求测试。
- 通过：T87 service 直接测试。
- 通过：前端生命周期测试与 `opsLocaleKeys`，共 28 tests。
- 通过：前端 `pnpm typecheck`。
- 通过：前端 `pnpm build`，1093 modules transformed，构建完成；仅有既有动态导入、Browserslist 和 Node 警告。
- 通过：`go build ./cmd/server`。
- 通过：`git diff --check`。
- 未通过但与 T87 无关：全量 `go test ./internal/service` 触发既有 Codex seed、channel monitor URL、CodexRadar 和 probe baseline 失败；T87 直接 service 测试通过，未修改这些模块。

## 未验证项

- 未连接真实 PostgreSQL 执行 integration build tag 场景。
- 未执行验收站或主站发布，也未做线上专项验收。
- 未验证真实生产历史数据的生命周期投影分布；本任务没有读取或写入生产业务数据。

## 发布与回滚

- `downtime_required`：待根总控在干净且与 `origin/main` 一致的根 `main` 上预检；本候选未执行发布预检。
- 回滚：由根总控从上一已验证应用版本回滚；不删除、不回写历史 usage/error 记录。
- 本候选不授权合并、推送、部署或生产配置变更。

## 注意事项

- 候选 worktree 仍有一个未纳入本提交的既有脏文件：`upstream/sub2api/frontend/pnpm-lock.yaml`。这是独立的依赖锁文件重解析变更，未由 T87 产生，也未纳入任何 T87 提交；根总控合并时必须排除，不应把它作为 T87 变更带入。
- 收到带目标 `main` SHA 的 `AUTHORIZE_MERGE_TO_MAIN` 前保持等待，不自行合并、推送、部署或清理 worktree/分支。
