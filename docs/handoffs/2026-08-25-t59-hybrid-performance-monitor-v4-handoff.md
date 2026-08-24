# T59 混合性能监控 V4 交接

- 基线：`main@2df788282`
- 候选分支：`codex/t59-hybrid-monitor-v4`
- 状态：`READY_FOR_ROOT_REVIEW`
- 迁移：无
- 配置：无新增配置；复用 `channel_monitor_mode`，旧 `v1`/`v2` 保持兼容
- 生产写入：无；投影只读 `account_monitor_results` 与 `usage_logs`
- `downtime_required`：预期 `false`，以根发布预检为准
- 回滚：将 `channel_monitor_mode` 恢复为 `v1` 或 `v2`，或回滚候选提交

## 交付内容

四模式注册与设置入口、混合主动探测/真实请求五分钟聚合、`GET /api/v1/monitor-v4`、前端模式分发、P95 圆环卡片和呼吸动效。

## 已运行验证

- 后端：`go test ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -run 'MonitorV4|ChannelMonitor|channelMonitor' -count=1`
- 前端：Monitor V2 路由、V4 合同、V4 卡片、设置模式 Vitest 全部通过
- 前端：`vue-tsc --noEmit` 通过
- 前端：`pnpm run build` 通过
- `git diff --check` 通过

## 未验证与剩余风险

尚未执行真实数据库上的长窗口查询计划检查；V4 SQL 使用现有两张事实表，30 天窗口并发访问量较高时需要根发布前关注查询耗时。候选未合并、未推送、未部署。
