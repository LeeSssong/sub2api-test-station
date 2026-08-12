# T04 账号监控仅使用原生 Sub 数据生产验证

验证日期：2026-08-12（Asia/Shanghai）

## 发布身份

- 候选分支：`codex/t04-account-monitor-native-only`
- 最终候选：`63019434684a53b7b856a6acea5605e3e8b4aede`
- 合并提交：`be9e124d65c7457477fbe6d3435a9468b1ec1f4c`
- tested tree：`d3feb28c1dca0fb12b405ea3ddc1483599ea0cfe`
- 迁移哈希：`f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`
- 测试证据：`/private/tmp/t04-account-monitor-release.BudXcd/test-evidence.json`，权限 `0600`
- 发布记录：`/var/lib/sub2api/release-records/20260812T124618Z-production-1146130.json`
- 发布方式：既有本地/宿主预加载蓝绿链；未使用 GitHub Actions

## 发布结果

- 发布记录：`result=succeeded`、`state=promoted`、`rolled_back=false`
- `downtime_required=false`
- 活动槽：`blue`
- 活动上游：`sub2api-blue:8080`
- 生产镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-be9e124d65c7457477fbe6d3435a9468b1ec1f4c-04ebb3a9a7b228f7c2006f80e4f14b6bc9e08c02a0b40d1e5c3f4b3849cdd9f0`
- API 与 worker 均运行上述镜像、健康且重启次数为 0。
- PostgreSQL、Redis、Caddy 容器 ID 与发布前一致；Caddy 仅执行平滑 reload，未重建共享服务。
- 公网 `/healthz`、`/readyz`、`/health` 均返回 HTTP 200，分别为 `alive`、`ready`、`ok`。

## 本地最小 MVP 验证

- `pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts`：28/28 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `git diff --check`：通过。
- 仅有既有非致命 pnpm、Browserslist、Vite 动态导入及分块警告。

## 登录态浏览器线上验收

- 页面：`https://api.xingqiaolab.top/admin/accounts/monitor`
- 页面标题：`账号监控 - 星桥AI`
- 初载显示 `全站 78` 及原有账号卡片。
- 初载监控数据请求为 `/api/v1/admin/accounts/monitor?range=24h`。
- 点击“7 天”后仅请求 `/api/v1/admin/accounts/monitor?range=7d`；按钮最终处于激活状态，页面显示“7 天调用”。
- 初载、页面 reload 和时间窗切换的网络记录中，路径以 `/xingqiao/` 开头的请求数量均为 0。
- 页面不显示“控制面暂时不可用”“完整性：”或读模型状态条；未新增替代常驻状态。
- 未点击“立即刷新全部”或执行其他会触发上游探测、写入配置或改变账号状态的操作。

## 结论与回滚

T04 已完成推送、无停机生产部署和线上专项验收。账号监控页面只使用原生 Sub 监控接口，外部控制面状态和调用已从该页面移除，现有卡片、布局和主要交互保持不变。

若后续发现 T04 回归，可从保留的前一活动 `green` 镜像恢复，或反向 T04 合并提交后重新走既有蓝绿发布链；本任务无数据库迁移或配置回滚。
