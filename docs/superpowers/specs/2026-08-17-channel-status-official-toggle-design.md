# 渠道状态官方聚合/自建监控可切换设计规格

## 1. 问题证据与当前行为

- `/monitor` 当前固定加载 `features/monitor-v2/MonitorV2RouteView.vue`。
- 该包装层挂载后总是请求 `/api/v1/monitor-v2?window=7d`，成功时展示自建 Monitor V2 卡片，失败时才回退到官方 `views/user/ChannelStatusView.vue`。
- 官方页面已经存在：`ChannelStatusView.vue` 根据 `channel_monitor_mode` 选择 `ChannelStatusV1View` 或 `ChannelStatusV2View`。
- 后端已有且正在使用的配置：
  - `channel_monitor_enabled`：是否启用渠道状态入口/能力。
  - `channel_monitor_mode`：`v1` 表示主动探测与自建 Monitor V2；`v2` 表示停用主动探测并使用官方被动聚合。
- 前端 `featureFlags.ts` 已提供 `isChannelMonitorV1Mode()` 与 `isChannelMonitorV2Mode()`，因此原生参数和模式解析无需扩展。

## 2. 目标

1. 当 `channel_monitor_mode=v2` 且渠道监控已启用时，`/monitor` 直接展示官方原生渠道聚合页面，不请求自建 `/api/v1/monitor-v2`。
2. 当 `channel_monitor_mode=v1` 时，保持现有自建 Monitor V2 页面、加载态、错误脱敏和官方回退行为。
3. 继续只使用现有一个模式参数完成官方/自建切换；生产切换通过设置 `channel_monitor_enabled=true`、`channel_monitor_mode=v2` 完成，回滚只需将 mode 设回 `v1`。
4. 切换过程不新增数据库迁移、后端接口、配置键、定时任务或平行事实源。

## 3. 非目标

- 不修改官方 `ChannelStatusV1View`、`ChannelStatusV2View` 的统计口径或展示组件。
- 不删除自建 Monitor V2 代码；它保留为 `v1` 可回滚实现。
- 不调整缓存命中率、分组成功率、P95/P95 TTFT 的统计公式；这些属于独立指标口径任务。
- 不改变管理员设置页已有的 `v1`/`v2` 选项和后端主动探测退休语义。

## 4. 方案比较与选择

### 方案一：新增第三个页面开关（不选）

新增 `channel_monitor_display_mode` 区分页面，同时保留 `channel_monitor_mode` 控制探测。优点是语义分离；缺点是出现两个容易不一致的配置，新增后端 DTO、设置持久化和迁移，违反原生优先和最小变更。

### 方案二：复用现有 `channel_monitor_mode`，在 `/monitor` 包装层分流（推荐）

入口包装层读取已存在的 `isChannelMonitorV2Mode()`：V2 直接渲染官方 `ChannelStatusView`，V1 执行现有自建加载/回退逻辑。优点是零后端改动、与主动探测/被动聚合语义一致、`v1` 可立即回滚；缺点是页面选择与监控模式共享一个参数，但这正是当前原生契约的既定含义。

### 方案三：路由表定义两个异步组件并在路由守卫选择（不选）

在路由层读取 Pinia 设置后动态选择组件。优点是包装层更薄；缺点是设置异步加载时容易出现路由级闪烁，错误回退和请求取消更难复用，测试范围更大。

选择方案二。

## 5. 架构与数据/控制流

```text
管理员设置保存 channel_monitor_mode
        |
        v
PublicSettings -> featureFlags.ts
        |
        v
MonitorV2RouteView.vue
  ├─ isChannelMonitorV2Mode()=true  -> ChannelStatusView -> ChannelStatusV2View
  └─ false                          -> getMonitorV2Snapshot -> MonitorV2View
                                      └─ 请求失败/运行时 fatal -> ChannelStatusView
```

- 官方模式首屏不创建 `AbortController` 请求，不访问自建 Monitor V2 API。
- 自建模式保持现有 7 天快照、加载态、错误脱敏及 fatal 回退。
- `ChannelStatusView` 继续作为官方原生选择器，官方 V2 的配置/矩阵/快照接口和状态展示完全复用现有实现。
- 当 `channel_monitor_enabled=false` 时，沿用现有入口开关语义；本任务不改变隐藏/禁用行为。

## 6. 接口与字段契约

- 新增接口：无。
- 新增字段：无。
- 复用字段：`PublicSettings.channel_monitor_enabled`、`PublicSettings.channel_monitor_mode`。
- 允许值：`channel_monitor_mode` 仅 `v1` 或 `v2`；后端已有非法值归一化为 `v1` 的 fail-safe 规则。

## 7. 失败与兼容语义

- 官方模式由官方页面自行处理其配置/快照错误；包装层不吞错、不降级到自建页面。
- 自建模式的现有 API 错误仍显示脱敏回退提示并渲染官方页面。
- 组件卸载时仍取消自建请求；官方模式没有待取消请求。
- 旧书签、旧路由和 `v1` 配置保持兼容。

## 8. 场景化验收矩阵

| 场景 | 设置 | 预期 |
|---|---|---|
| 官方聚合 | enabled=true, mode=v2 | `/monitor` 显示官方 `ChannelStatusView`/V2；`getMonitorV2Snapshot` 调用次数为 0 |
| 自建正常 | enabled=true, mode=v1；快照 200 | 显示自建 Monitor V2 卡片 |
| 自建接口失败 | enabled=true, mode=v1；快照失败 | 显示脱敏回退提示和官方状态页 |
| 模式非法/缺失 | 后端归一化为 v1 | 走自建路径，不破坏旧行为 |
| 回滚 | 将 mode 从 v2 改为 v1 | 刷新 `/monitor` 后恢复自建路径 |

## 9. 测试策略

- 在现有 `MonitorV2RouteView.spec.ts` 增加官方模式测试：官方组件出现，快照 API 未调用。
- 保留并运行现有自建成功、失败回退两例。
- 运行该 spec、前端 typecheck、生产 build 与 `git diff --check`。
- 不运行全仓测试、压力/soak 或无关浏览器矩阵。

## 10. 发布、线上验证与回滚

- 变更仅前端单文件与直接相关测试/文档，预期 `downtime_required=false`，沿现有蓝绿链快速发布。
- 发布前在根总控合并后的 `main` 上复跑专项测试、typecheck、build、diff-check 和发布预检。
- 上线后验证：设置 `channel_monitor_enabled=true`、`channel_monitor_mode=v2`；登录态打开 `/monitor`，确认官方 V2 页面出现且浏览器不请求 `/api/v1/monitor-v2`；健康端点保持 200。
- 回滚：将 `channel_monitor_mode=v1`，无需再次发布即可恢复自建页面；若需代码回滚，回退本次前端提交。

## 11. 用户批准记录

- 2026-08-17：用户明确要求“切换为官方，并设置一个参数自由切换官方以及现在自建的”，并要求不打断两个正在运行的快速迭代任务、快速热部署。该指令批准本规格的方案二和发布边界。
