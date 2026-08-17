# T18 渠道状态官方聚合/自建监控可切换生产收口

## 发布身份

- 发布源：已推送根 `main@80e5fe2a66a5eef11ad220ff280c7e3796dbb2d7`
- source/tested tree：`ee5de8f58b2695e641b66b2a9ea83589b70d02a0`
- 迁移哈希：`bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951`（无变化）
- 0600 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-80e5fe2a6-t18-channel-status-toggle-v1.json`
- 宿主记录：`/var/lib/sub2api/release-records/20260817T180044Z-production-2367549.json`
- 结果：`succeeded/promoted`，`rolled_back=false`，`downtime_required=false`
- 活动槽：`blue`，活动上游 `sub2api-blue:8080`
- 不可变镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-80e5fe2a66a5eef11ad220ff280c7e3796dbb2d7-ff2f86f4ca3f07ebedbfd3203ab8bd1980b7f72e3e4ea05a4fc0312e6d90fbae`

## 验证

- 刷新候选与根合并树均通过 `MonitorV2RouteView` 3/3、`pnpm typecheck`、`pnpm build` 和 `git diff --check`。
- 公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。
- 生产公开设置为 `channel_monitor_enabled=true`、`channel_monitor_mode=v2`，无需额外配置写入。
- 管理员登录态 `/monitor` 显示官方“渠道监控”聚合页面，包含 90m/24h/7d/30d、平台/分组/模型筛选、色块矩阵/折线图、综合/错误率/首 Token/缓存率、成功率/首 Token/每秒 Token/缓存率/RPM 和模型/错误原因/用户排行。
- 页面资源记录中 `/api/v1/monitor-v2` 请求为 0，证明 v2 模式直接渲染官方聚合页并跳过自建快照接口。
- API/worker 使用同一不可变镜像；PostgreSQL、Redis、Caddy 容器身份保持不变。

## 回滚与边界

- 功能回滚参数为 `channel_monitor_mode=v1`，无需代码回滚；应用回滚仍可使用上一 green 槽和不可变镜像。
- 本任务未修改后端、迁移、配置 schema、账务、缓存策略、生产数据或 GitHub Actions。
