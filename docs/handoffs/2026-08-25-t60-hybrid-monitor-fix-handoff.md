# T60 第四套混合性能监测配置与中文渲染修复交接

- 基线：`main@b545167bb`
- 候选分支：`codex/t60-hybrid-monitor-fix`
- 范围：hybrid 管理页保留主动探测配置入口；第四模式页面和卡片改用 `channelMonitorV2.hybrid.*`；后端不再用 V2 `enabled` 阻断 hybrid 分组快照
- 迁移：无
- 生产数据写入：无
- `downtime_required`：预期 `false`，以根发布预检为准
- 回滚：切回 `v1`/`v2`，或回滚 T60 提交

## 验证

- `go test ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -run 'MonitorV4|ChannelMonitor|channelMonitor' -count=1`
- `go build ./cmd/server`
- V4、设置模式、Monitor V2 路由和管理员配置 Vitest：11/11 通过
- `pnpm exec vue-tsc --noEmit`
- `pnpm run build`
- `git diff --check`

## 剩余风险

未执行真实数据库长窗口查询计划检查；本修复不改变 V4 SQL，只恢复配置页入口、中文命名空间和 hybrid 分组投影门控。
