# T92 CodexRadar 独立数据模块实施交接

## 范围

- CodexRadar 站长推荐与社区测试数据继续只读取固定的公开 CodexRadar 外部接口。
- 新增匿名 GET-only 代理：`/api/v1/public/codexradar/insights`、`/api/v1/public/codexradar/community`。
- 保留原登录态 `/api/v1/monitor-v2/codexradar-*` 兼容路由，但前端已切换到 public 路径。
- 不读取本站数据库、Redis、用户、账号、分组、usage_logs、探测、计费或 MonitorV4 数据。

## 交付内容

- 推荐响应支持 `source_status`、分类 `status`，空分类返回 `items: []` 并展示“当前暂无推荐”。
- 推荐外部响应上限提高到 2 MiB，超时提高到 10 秒，保留最近成功快照作为 stale。
- 社区软件/视觉来源并发抓取；单源失败返回 `partial`，失败 tab 独立标记，双源失败且无缓存才返回 503。
- 综合 tab 在无共同模型/effort 时返回 `NO_SHARED_MODEL_EFFORTS`，不阻断软件或视觉 tab。
- 前端支持空态、partial、stale、独立错误与重试，不再依赖登录态 Monitor 数据。

## 验证

- 后端：CodexRadar service、handler、route focused tests 通过；`go build ./cmd/server` 通过。
- 前端：4 个 CodexRadar Vitest 文件，13 个测试通过。
- `git diff --check` 通过。
- `vue-tsc --noEmit` / `vite build` 能识别 T92 代码；当前构建仍被基线中的 `AccountMonitorCard.vue` 两个未使用变量错误阻断：`statisticsCutoffLabel`、`balanceDetail`，不属于 T92 改动。

## 发布状态

- 当前 worktree：`.worktrees/t92-codexradar-independent-module`
- 分支：`codex/t92-codexradar-independent-module`
- 基线：`f489b4ba21af80ea517d9d1325e103ebccba06cb`
- 未推送、未合并、未部署；无迁移、配置或生产数据变化。
- 下一状态：`READY_FOR_ROOT_REVIEW`。根总控需在授权后合并、运行合并后门禁并按验收站约束发布。
