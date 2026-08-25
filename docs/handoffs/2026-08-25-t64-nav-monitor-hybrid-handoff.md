# T64 用户导航与账号监控混合证据交接

- 基线：`main@fbe32c725`
- 候选分支：`codex/t64-nav-monitor-hybrid`
- 范围：隐藏用户“我的订阅”菜单；将性能监测统一命名为“分组性能监控”；分组性能监控默认 24 小时；移除管理员“经营分析-账号盈利”菜单挂载但保留路由、页面和 API；账号监控质量数据合并真实调用与主动探测证据。
- 迁移：无
- 配置：无
- 生产数据写入：无
- `downtime_required`：预期 `false`，以根发布预检为准
- 回滚：保留候选 worktree；必要时回退至 `main@fbe32c725`

## 实现要点

- 账号监控 `sample_count`、成功率、TTFT 和延迟质量字段使用真实 `usage_logs` 请求与主动探测样本的混合证据；真实请求指标优先，缺失时回退主动探测指标。
- `probe_*` 兼容字段、最新状态、可用性和新鲜度继续由主动探测门控，避免只有真实请求而没有近期探测时误报账号可用。
- 混合证据标记为 `hybrid`；无主动探测时保持 `stale` 语义。
- 分组性能监控卡片中的首字 P95、总耗时 P95 将后端毫秒值转换为秒，按四舍五入保留两位小数并显示 `s` 单位。

## 已运行验证

- 后端：`go test ./internal/service -run 'TestAccountMonitor' -count=1`
- 后端：`go test ./internal/repository -run 'TestAccountMonitor'`
- 后端：`go build ./cmd/server`
- 前端定向 Vitest：7 个文件、29 个测试通过
- 追加分组性能监控卡片测试：3 个文件、11 个测试通过；覆盖毫秒到秒及两位小数展示
- 前端：`pnpm typecheck`
- 前端：`pnpm build`
- `git diff --check`

## 未验证与剩余风险

- 尚未合并到 `main`、推送、部署或线上验收。
- 未执行真实生产数据库查询计划检查；本次只调整只读投影与菜单挂载，不新增迁移或配置。
- 构建输出包含既有 Vite 动态导入提示和 Browserslist 数据过期提示，不影响构建退出码。
