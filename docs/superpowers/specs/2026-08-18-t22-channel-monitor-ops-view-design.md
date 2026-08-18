# T22 官方 Channel Monitor V2 简洁运营视图规格

## 1. 问题证据与当前行为

T18 已将 `/monitor` 的 `channel_monitor_mode=v2` 路径切换为官方 `ChannelStatusView`，直接读取 `/channel-monitor-v2` 或 `/admin/channel-monitor-v2`，不调用自建 `/api/v1/monitor-v2`。`channel_monitor_mode=v1` 继续保留旧页面回滚能力。

当前官方 V2 页面已经提供 90m/24h/7d/30d、整体指标、分组矩阵、趋势、模型、错误和用户排行，但存在三个运营使用问题：

1. 未指定 URL 参数时默认 90m，运营日常判断需要先手动切到 24h。
2. 模型、错误、用户排行始终占据主页面，首屏不能集中回答“哪些分组需要处理、成功率/首 Token/缓存率如何、最近是否恶化”。
3. 后端对 `request_count < minimum_sample` 已返回 `health.overall=unknown`、`score=null`，页面却主要以灰色未知点和数值呈现，零流量时还可能把 `error_rate=0` 格式化成 100% 成功率，容易被理解为已验证健康。

T19 已在官方 V2 聚合中修正缓存率有效样本：仅 `actual_cost > 0` 且具备文本 Token Prompt Cache 语义的流水进入缓存分母，图片、视频、按次和零成本失败占位被排除。T22 复用该结果，不新增 SQL、API 或第二事实源。

## 2. 目标与非目标

### 目标

1. 未指定合法 `range` 时默认 24h；继续支持 90m、7d、30d 和 URL 深链。
2. 首屏保留整体成功率、首 Token、缓存率、按分组状态和最近趋势。
3. 模型明细、错误分类、用户排行进入默认收起的“详细分析”，首次展开后再加载当前 tab。
4. 零流量显示“已就绪·暂无流量”；有流量但不足 `minimum_sample` 显示“待观察”。两者使用中性色，不进入健康评分，也不显示为 100% 成功或真实健康。
5. 样本充足时继续直接展示后端 `healthy/warning/critical`，真实错误、低成功率和高延迟保持黄/红语义。
6. 桌面和 390px 移动端不产生整页横向溢出；密集矩阵和表格仅在自己的容器内滚动。

### 非目标

- 不改后端聚合、健康阈值、错误分类、数据库、迁移或历史数据。
- 不改 T19 有效样本谓词，不重新判断本地拒绝、禁用模型或参数校验失败。
- 不新增页面、API、事实源、外部控制面或 GitHub Actions。
- 不改变管理员吞吐可见性或普通用户隐私规则。
- 不改变 `channel_monitor_mode=v1|v2` 配置契约。

## 3. 方案比较与选择

### 方案 A：现有官方 V2 页面内重排（采用）

保留现有 API、响应类型、筛选、矩阵和图表组件，只调整默认 range、首屏顺序、低样本展示和详细分析展开逻辑。改动集中、回滚简单，也完整保留 T18/T19 的单一事实源。

### 方案 B：只用 CSS 隐藏明细

代码较少，但隐藏内容缺少明确可访问的展开控制，仍会在首屏加载明细 API，URL tab 状态也容易与视觉状态脱节。

### 方案 C：新增运营专用页面

可独立设计，但会复制官方 V2 的筛选、请求编排和状态逻辑，形成平行展示路径，后续更容易与官方事实源漂移。

## 4. 信息架构与组件边界

页面保持 `ChannelStatusV2View.vue` 单一数据编排入口：

1. 顶部工具栏：标题、更新时间、刷新、24h 默认时间窗和已有筛选。
2. 整体摘要：成功率、首 Token P50、缓存率；吞吐指标继续按既有管理员/隐私开关显示，但不是 T22 强调项。
3. 分组状态与最近趋势：默认 `group_by=platform_group` 的 `RelayPulseMatrix` 继续作为分组状态主视图；矩阵与折线切换保留。
4. 详细分析：一个原生可访问的展开区，内部保留模型、错误、用户排行 tabs 和既有内部滚动容器。

`RelayPulseMatrix.vue` 负责每个分组行的状态标签与指标占位；一个纯展示 helper 统一计算：

```ts
type MonitorReadiness = 'no_traffic' | 'observing' | 'scored'

function monitorReadiness(metrics: MonitorMetric, health: MonitorHealth): MonitorReadiness
```

- `request_count <= 0`：`no_traffic`。
- `health.score == null` 或 `request_count < health.minimum_sample`：`observing`。
- 其他：`scored`。

该 helper 只解释现有响应，不产生新健康分，也不把未知映射为健康。

## 5. 数据与控制流

1. 路由 range 经 `parseRange` 解析；合法值原样保留，缺失或非法值回退 24h。
2. 首次加载并行请求 dimensions、snapshot、matrix；matrix 默认按 `platform_group`。
3. 页面根据 snapshot 渲染整体摘要，根据 matrix rows 渲染分组状态和趋势。
4. 详细分析默认关闭，不发出 models/errors/users 请求。
5. 用户首次展开详细分析后请求当前 tab；切换 tab 时请求对应官方 endpoint。筛选变化时，只有详细分析已展开才刷新当前 tab。
6. URL 保留 range、platform、group、model、group_by、health_mode、trend_view；`tab` 仍表示详细分析内部 tab。

## 6. 展示与状态契约

### 整体摘要

| 状态 | 成功率 | 首 Token | 缓存率 | 颜色 |
|---|---|---|---|---|
| `request_count=0` | 已就绪·暂无流量 | `-` | `-` | 中性 |
| `0<request_count<minimum_sample` 或 `score=null` | 待观察 | 可展示已有值 | 可展示有效样本值或 `-` | 中性 |
| 样本充足 | 真实百分比 | 真实 P50 | T19 有效样本率 | 后端健康色 |

### 分组行与趋势区间

- 零流量行显示“已就绪·暂无流量”，状态点保持中性。
- 低样本行显示“待观察”，状态点保持中性。
- 样本充足行显示真实成功率、首 Token、缓存率和后端健康色。
- 无 bucket 的时间格显示“无流量”；有 bucket 但分数为空显示“待观察”，不绘制绿色健康色。
- 真实 `warning/critical` 不被中性状态覆盖。

## 7. 失败、安全与兼容语义

- 官方 API 加载失败继续使用现有全局错误提示，保留上一次成功 snapshot，避免把失败误画为无流量。
- 详细分析加载失败只报告明细错误，不改变首屏整体状态。
- 普通用户继续遵循吞吐隐藏设置；管理员错误样本详情权限不变。
- 旧链接 `?range=90m`、`?tab=errors` 等继续可解析；未指定参数的新访问默认 24h。
- `channel_monitor_mode=v1` 仍在 `MonitorV2RouteView` 直接渲染旧自建页面，是首选功能回滚。

## 8. 响应式与可访问性

- 390px 下顶部工具栏可在自身横向滚动，不扩大页面宽度。
- KPI 使用两列紧凑网格；文案允许换行且不截断关键状态。
- 矩阵和明细表只在现有 `.matrix-scroll` / `.table-container` 内滚动。
- “详细分析”使用 button + `aria-expanded` + `aria-controls`，支持键盘触发；折叠时内容从可访问树移除。
- 状态不仅依赖颜色，同时展示明确中文/英文文本。

## 9. 场景化验收矩阵

| 场景 | 预期 |
|---|---|
| 无 range 参数 | 首次请求和选中项为 24h |
| `range=90m/7d/30d` | 深链值保留并请求对应窗口 |
| 整体零请求 | 显示“已就绪·暂无流量”，不显示 100% 或健康色 |
| 整体 1..minimum_sample-1 | 显示“待观察”，不产生健康评分 |
| 分组零请求 | 分组行显示“已就绪·暂无流量”并使用中性色 |
| 分组低样本 | 分组行显示“待观察”并使用中性色 |
| 样本充足且低成功率 | 后端 warning/critical 黄红状态保持 |
| 样本充足且高 TTFT | 后端 warning/critical 黄红状态保持 |
| 页面初载 | 不请求 models/errors/users |
| 展开详细分析 | 请求当前 tab；模型/错误/用户可切换 |
| 390px | 页面 `scrollWidth <= clientWidth`；矩阵/表格内部滚动 |
| v1 回滚 | `channel_monitor_mode=v1` 路径行为不变 |

## 10. 测试策略

- Vitest：纯 helper 的零流量、低样本、已评分状态。
- Vitest：`ChannelStatusV2View` 默认 24h、合法深链、首屏不加载明细、展开/切 tab 请求、整体中性文案和真实异常色。
- Vitest：`RelayPulseMatrix` 分组状态文案、中性色与真实异常色。
- 现有 `MonitorV2RouteView` mode tests 确认 v1/v2 回滚路径未回归。
- 必要 `pnpm typecheck`、`pnpm build`、`git diff --check`。
- 使用 Playwright 在桌面和 390px 本地页面检查整页无溢出、详细分析展开和资源请求；不访问生产。

## 11. 发布、线上验证与回滚

- 无迁移、无配置 schema 变化、无生产数据写入；预期 `downtime_required=false`，最终以根 `main` 发布预检为准。
- 根发布后以登录态验证默认 24h、四窗口、首屏层级、低样本真实样本、详细分析和 390px 无溢出。
- 若预检为 `downtime_required=true`，根总控停在任何停服、迁移、重启或切换之前。
- 功能优先回滚为 `channel_monitor_mode=v1`；代码级回滚使用上一活动槽/不可变镜像。

## 12. 待决事项与批准记录

没有未决产品问题。2026-08-18 T22 委派已明确批准默认 24h、首屏字段、详细分析边界、低样本语义、T18/T19 复用、v1 回滚、无迁移和 390px 验收；项目约束 2.3 允许唯一发布总控代审既定队列规格。本规格未扩大批准范围。
