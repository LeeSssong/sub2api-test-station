# T19 Monitor V2 缓存命中率有效样本口径修正交接

## Candidate

- 状态：`READY_FOR_ROOT_REVIEW`
- 最新根主线基线：`main@40315c7651714a0ec86b3664efed0f325786fb6e`
- 功能实现提交：`0f9ef38f2a0621d9afe5b5c965da025161dba399`
- 刷新合并提交：`948974db28378e5a42fad97a7cfc41f59166d0f3`
- 分支：`codex/t19-monitor-v2-cache-eligibility`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t19-monitor-v2-cache-eligibility`
- 预期发布属性：`downtime_required=false`
- 当前边界：未合并根 `main`、未推送、未预检、未部署、未线上验收

## Scope

- `monitor_v2_repo.go` 的缓存统计分子与分母统一限定为 `actual_cost > 0` 且具备文本 Token Prompt Cache 语义的流水。
- 接受显式 `billing_mode='token'`，以及历史空计费模式且图片/视频字段全零的兼容流水。
- 排除图片、视频、按请求计费和零成本失败占位；API、前端、账务、价格、倍率及缓存策略不变。
- 无迁移、无配置、无生产数据写入、无 GitHub Actions 变化。

## Verification

- RED：更新 sqlmock 查询合同后，旧 SQL 因缺少资格谓词按预期失败。
- `go test ./internal/repository -run 'TestMonitorV2RepositoryGetCacheStats' -count=1`：通过。
- `go test ./internal/service -run 'MonitorV2|monitor_v2' -count=1`：通过。
- `go test ./cmd/... -run '^$'`：通过。
- `go build ./cmd/...`：通过。
- `gofmt`、`git diff --check`：通过。

## Remaining Root Work

- T15 发布暂停期间不得进入整合/发布车道；T18 仍排在 T19 前。
- 轮到 T19 时，若 `main` 再次漂移则先刷新并重跑上述直接相关门禁，再由根总控合并、推送和运行既有本地/宿主预检。
- 发布后对 24 小时与 7 天窗口做只读 SQL/API 交叉核对；异常时回滚上一活动槽/镜像。
