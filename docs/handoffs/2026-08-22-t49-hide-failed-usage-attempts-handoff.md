# T49 失败尝试从正常流水列表隔离交接

## 状态

`READY_FOR_ROOT_REVIEW`。候选分支 `codex/hide-failed-usage-attempts`，worktree `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/hide-failed-usage-attempts`，基线 `main@bf3815c1e`。

## 变更

- 在原生 usage repository 增加 `COALESCE(usage_completeness, 'complete') <> 'unknown'` 共享谓词。
- 管理员 `ListWithFilters`、`GetStatsWithFilters` 及其 inbound/upstream/path endpoint breakdown 排除无 usage 的 failover 失败 attempt。
- `complete`、`partial`、历史 `NULL` 保留；不改变写入、扣费、重试、失败审计、清理和详情读取。
- 无迁移、无配置变更、无生产数据写入；未新增 GitHub Actions。

## 验证

- `go test ./internal/repository -count=1`：通过。
- `go test ./internal/handler/admin -run 'TestAdminUsage' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `gofmt` 与 `git diff --check`：通过。
- RED 证据：过滤回归在实现前因实际 SQL 缺少 `WHERE COALESCE(...)` 失败；GREEN 后通过。

## 发布边界

- 预计 `downtime_required=false`，但以根发布预检为准。
- 只能从根 `main` 合并后走既有本地/宿主蓝绿发布；回滚使用上一已验证蓝绿槽/镜像。
- 候选尚未合并、推送、部署或线上验收；根总控需先授权合并并在合并后的 `main` 做发布门禁。

## 残余风险

其他非管理员读模型（例如历史账号统计/趋势接口）保持原有口径，避免本次热修扩大到相邻报表；失败 attempt 仍保留在数据库和审计链路，可通过后续专用审计入口核查。
