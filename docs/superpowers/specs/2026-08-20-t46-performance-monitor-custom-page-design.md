# T46 性能监测自定义页面挂载设计

## 1. 问题证据与当前行为

- Sub 原生前端已有 `MonitorV2RouteView.vue`，负责官方/自建模式选择、登录态 `/api/v1/monitor-v2` 快照加载、失败回退和组件挂载。
- `MonitorV2View.vue`、`MonitorV2GroupCard.vue`、`MonitorV2Timeline.vue` 已承载时间线柱状图、Tooltip、窗口切换、刷新重试及 T41/T42/T44 的视觉稳定性修复。
- 用户侧导航在 `AppSidebar.vue` 中仍固定输出 `/monitor` 的“渠道状态”，自定义菜单只支持 iframe/Markdown URL，`CustomPageView.vue` 不能直接挂载 Vue 原生页面。
- 前序任务明确批准：隐藏原生 `/monitor` 固定入口；新入口命名为“性能监测”；不做旧 `/monitor` 重定向兼容；保留原生监控数据与竞品式乐观展示。

## 2. 目标与非目标

### 目标

1. 新增独立路由 `/custom/performance-monitor`，使用统一 `AppLayout` 和登录鉴权。
2. 页面内部直接挂载原生 `MonitorV2RouteView`，不使用 iframe，不复制 API、统计或时间线事实源。
3. 用户导航隐藏固定 `/monitor`，只展示“性能监测”菜单项。管理员的个人区和普通用户均可访问。
4. 页面标题、菜单标签和中英文文案可由 i18n 驱动。
5. `/custom/:id` 现有 iframe/Markdown 行为保持不变。

### 非目标

- 不保留 `/monitor` 兼容重定向。直接访问旧路径按现有路由行为处理，但不再出现在导航。
- 不修改 Monitor V2 后端 API、数据字段、统计口径、刷新间隔、探测执行器或时间线几何。
- 不新增数据库表、迁移、配置键、生产数据写入或第二套监控事实源。
- 不引入外部网站、iframe、跨域登录态或新的依赖。

## 3. 方案比较与选择

### 方案 A（采用）：专用原生页面路由

增加轻量 `PerformanceMonitorView.vue`，组件内直接渲染 `MonitorV2RouteView`；侧边栏固定添加 `/custom/performance-monitor`。优点是入口稳定、鉴权清晰、无需修改自定义菜单数据结构，且原生页面可完整复用。

### 方案 B：扩展 CustomMenuItem 增加 native target

为后端/前端菜单 JSON 增加 `target_type=native` 与白名单路由，并让 `CustomPageView` 分派原生组件。优点是后台可配置；缺点是需要修改设置校验、DTO、管理表单、CSP 和迁移/初始化，范围明显大于当前目标。

### 方案 C：iframe 加载现有 `/monitor`

实现最少，但会产生嵌套 AppLayout、跨域登录态、CSP 和主题同步问题，无法满足“Sub 原生独立页面”。

选择方案 A，原因是它在不扩大自定义菜单数据合同的前提下，满足独立页面和原生复用要求。菜单项是产品固定入口，不把它误建成可编辑的外部 URL。

## 4. 端到端数据与控制流

1. `AppSidebar` 构造个人导航时省略固定 `/monitor`，插入 `/custom/performance-monitor`，标签由 `nav.performanceMonitor` 提供。
2. 用户点击菜单后，Vue Router 命中 `PerformanceMonitor` 路由；路由 `meta.requiresAuth=true`，沿用现有登录守卫。
3. `PerformanceMonitorView.vue` 渲染 `MonitorV2RouteView`。后者按当前 `channel_monitor_mode` 选择官方 `ChannelStatusView` 或自建 `MonitorV2View`，继续使用原生鉴权 API、窗口切换、刷新和失败回退。
4. 页面卸载时由原生组件中止请求与定时器；无额外状态持久化。
5. `resolveRouteDocumentTitle` 对新路由使用 `nav.performanceMonitor` 的翻译标题。

## 5. 接口与字段契约

- 新路由：`GET /custom/performance-monitor`（SPA history fallback），不新增 HTTP API。
- 路由元数据：`requiresAuth: true`、`requiresAdmin: false`、`titleKey: 'nav.performanceMonitor'`。
- 菜单 NavItem：`path='/custom/performance-monitor'`、`label=t('nav.performanceMonitor')`、复用 `SignalIcon`。
- 原生监控组件输入/输出合同完全不变；`MonitorV2RouteView` 仍读取现有公开设置和 `/api/v1/monitor-v2`。

## 6. 失败与安全语义

- 未登录用户由既有路由守卫送往登录页。
- Monitor V2 API 失败继续使用现有 fallback/错误展示和 5 秒重试，不在新页面捕获或改写错误。
- 旧 `/monitor` 不新增重定向或旁路权限；隐藏仅针对导航入口。
- 不接受用户提供的 URL、HTML 或脚本，不扩大 CSP。

## 7. 兼容性与迁移

- iframe/Markdown 自定义菜单和后台设置合同不变。
- 不新增迁移、配置或生产数据。
- 旧书签 `/monitor` 仍由现有路由解析，但不承诺新入口重定向兼容；后续若需清理旧路由另立任务。

## 8. 场景化验收矩阵

| 场景 | 期望 |
|---|---|
| 普通用户导航 | 显示“性能监测”，不显示“渠道状态”固定项 |
| 管理员个人导航 | 同样显示“性能监测”，不重复显示固定 `/monitor` |
| 点击新入口 | URL 为 `/custom/performance-monitor`，页面渲染原生 Monitor V2 |
| 时间线交互 | T41/T42/T44 的柱体、Tooltip、窗口切换、刷新重试保持不变 |
| 主题/语言 | AppLayout、Monitor V2 和页面标题遵循现有主题与 locale |
| 自定义 iframe/Markdown | 原路径和渲染逻辑不受影响 |
| 未登录直达 | 触发现有认证守卫，不泄露监控 API |

## 9. 测试策略

- 先写失败的路由合同测试，断言新路由元数据和组件；再写 AppSidebar/Nav 合同测试，断言 `/monitor` 缺席且新入口存在。
- 为 `PerformanceMonitorView` 增加最小挂载测试，断言 `MonitorV2RouteView` 被渲染。
- 运行受影响 Vitest 文件、`pnpm typecheck`、`pnpm build`、`git diff --check`。
- 不运行无关后端全仓测试；无迁移故不运行迁移测试。

## 10. 发布、线上验证与回滚

- 候选完成直接相关前端测试、类型检查、构建和 diff 检查后进入 `READY_FOR_ROOT_REVIEW`。
- 根合并后的发布预检必须输出 `downtime_required`; 预检为 `false` 时按既有蓝绿链直接推送、部署和线上验收。
- 线上验收：登录态确认新菜单、页面 URL、监控卡片/时间线、1440px 与 390px 无整页溢出；公网 `/healthz`、`/readyz`、`/health` 均 200。
- 回滚：恢复上一已验证蓝绿槽或回退 T46 合并提交；无数据库回滚。

## 11. 用户批准记录

前序“还原性能监测页面”任务已明确批准方案：隐藏原生 `/monitor` 固定入口，新增独立“性能监测”入口，采用原生 Monitor V2 组件与竞品式时间线交互；本规格将该批准收敛为专用原生页面路由方案。
