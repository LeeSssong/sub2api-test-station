# T99 Monitor V4 分组缓存 P95 验证报告

日期：2026-08-31

## Scope

- 候选：`codex/t99-monitor-v4-group-cache-p95`
- 初始基线：`main@e9db36d4b5cf789ac85bbabdfb82aa2c4beb7479`
- 刷新基线：`main@5d77271b32990076b8b0344a3f1909c62192abc6`
- 实现提交：`2e9a4b76c`；刷新合并提交：`c2ec433ae`
- 范围仅为 Monitor V4 分组缓存读取 token P95 的 repository、service、handler、前端契约与现有卡片；无迁移、配置、依赖或生产数据写入。

## Results

- Go 1.27.0/Linux arm64，repository V4 定向测试：通过。
- Go 1.27.0/Linux arm64，完整 production service 源码 + 自包含 Monitor V4 测试：通过。
- Go 1.27.0/Linux arm64，完整 production handler 源码 + Monitor V4 handler 合同测试：通过。
- Go 1.27.0/Linux arm64，`go build -p 1 -o /tmp/sub2api-server ./cmd/server`：通过。
- Monitor V4 前端测试：2 files，11/11 通过。
- `vue-tsc --noEmit`、`vue-tsc -b`：通过。
- Vite production build：通过，1094 modules transformed。
- `gofmt`、`git diff --check`：通过；`pnpm-lock.yaml` 无变化。

## Contract Evidence

- 成功真实请求使用 `COALESCE(usage_logs.cache_read_tokens, 0)`，因此 0 值进入样本。
- `PERCENTILE_CONT(0.95)` 只过滤 `successful AND source = 'real'`，失败请求和主动探测不进入缓存 P95。
- repository scan、service projection、handler JSON、前端严格校验和卡片展示均覆盖数值与 `null/0` 两种状态。
- 成功率、TTFT、总耗时和主动探测兜底 SQL 未改变。

## Baseline Limits

- repository 全包默认 `go test` 会先被既有 `internal/repository/usage_log_repo_stats.go:1004` 的 `fmt.Sprintf` vet 错误阻断；T99 定向测试使用 `-vet=off`，完整 server build仍通过。
- service 全包测试当前有既有编译错误：`openai_admission_first_output_wiring_test.go` 的 `ctx/c` 变量问题，以及 `openai_sticky_reference_test.go` stub 缺少 `GetReasoningContent`。本报告通过“完整 production 源码 + T99 自包含测试文件”隔离验证，不把这些无关错误算作 T99 失败。
- 未在真实 PostgreSQL 大数据集上测量 7d/30d 查询耗时；上线后需要观察 Monitor V4 端点延迟。
- 未运行验收站或主站发布预检；`downtime_required` 由根线程合并后确认。
