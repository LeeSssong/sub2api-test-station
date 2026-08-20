# T42 Monitor V2 时间新鲜度与刷新可靠性设计

**任务包：** T42（快速迭代-11）  
**基线：** `main@3ac10d8473923a9b017c4826024680c4361e8323`  
**状态：** 已按根任务授权的推荐方案进入实现  

## 问题与现状

Monitor V2 已使用原生主动探测结果、合同版本 `7` 和固定时间桶（24h=24、7d=28、30d=30）。快照的 `generated_at` 仅代表本次 HTTP 读取完成时间，用户无法判断卡片上的状态来自哪一次探测；后端虽在 SQL 中读取每个分组最新探测的 `checked_at`，却只投影为二值 current status。前端定时 GET 失败时直接发出 `fatal`，没有重试，导致偶发网络错误会切换基础页并停止自动刷新。

## 目标

1. 保持 Monitor V2 合同版本 `7`、现有字段语义、24/28/30 固定桶与原生探测来源。
2. 在最小合同扩展中返回每个分组最新原生探测时间 `source_updated_at`（无可用探测时省略），明确区分数据源更新时间和快照读取时间。
3. 在分组卡片上清楚展示最新探测时间，保留每个桶的既有状态、延迟和数量，不改变桶计算。
4. 定时刷新或切换窗口的 GET 失败时保留上一份快照并安排短延迟重试；成功后恢复管理员配置的刷新间隔；组件卸载或请求取消时不再重试。

## 非目标

- 不升合同版本，不改主动探测执行器、调度资格、数据库表或生产配置。
- 不新增网络补查、读时探测、回填、重试探测或第二数据源。
- 不改变状态二值语义、统计公式、时间桶边界或账号/分组可见性。

## 方案与选择

1. **仅改文案（不选）**：把 `generated_at` 改名为“最新”，无法表达真实探测时间。
2. **增加快照级 source watermark（不选）**：全局一个时间会掩盖分组之间的探测延迟。
3. **推荐：分组级 `source_updated_at` + UI 当前桶标记 + 前端失败重试**：复用 SQL 已读取的 latest probe `checked_at`，仅增加一个可选 RFC3339 字段；前端可在旧响应缺少该字段时兼容显示；刷新策略只在 GET 读取层重试。

## 数据与接口契约

后端 `MonitorV2NativeGroupProjection` 增加 `SourceUpdatedAt *time.Time`。`current_by_group` 聚合每个分组最新探测时间（`MAX(l.checked_at)`），并继续以 `checked_at >= freshSince` 判定 current status。`MonitorV2Group` 与 handler DTO 增加可选 `source_updated_at`，零值不输出。合同版本仍为字符串 `"7"`。

前端 `MonitorV2Group.source_updated_at?: string | null`，校验器在字段存在时要求 RFC3339，缺失归一化为 `null`。卡片显示“探测于 <时间>”或“暂无最新探测”；时间线保持现有视觉和数据契约不变。

## 刷新失败语义

`MonitorV2View` 成功响应按 `refresh_interval_seconds` 调度下一次刷新。非取消 GET 错误不会触发 fatal/fallback，也不会覆盖旧快照；安排 5 秒后重试，重试仍失败则继续 5 秒退避。成功响应清除重试并恢复配置间隔。隐藏页面清除计时器；回到可见状态立即读取；卸载取消请求并清除计时器。

## 验收矩阵

- API 合同接受/拒绝 `source_updated_at` 的 RFC3339/非法值，保持 v7 与固定桶长度。
- 后端 service/repository/handler 测试证明最新 `checked_at` 被投影和序列化，缺失时字段省略。
- UI 显示分组探测时间；旧 fixture（无字段）仍通过。
- UI GET 失败后在 5 秒重试，成功后恢复正常间隔；失败不触发 fallback；卸载/取消不产生后续请求。
- 直接相关前端 Vitest、后端 Monitor V2 测试、typecheck/build、`git diff --check` 通过。

## 发布与回滚

仅前后端源码与测试/文档变化，无迁移、配置或停机需求，预计 `downtime_required=false`。回滚为恢复本任务提交；旧 v7 客户端可继续读取新增可选字段之外的原字段。

## 批准记录

根任务已授权按推荐方案实施；本任务不修改 `main`、全局队列或项目进度总账。
