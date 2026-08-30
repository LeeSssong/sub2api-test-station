# Spec: Monitor V4 分组缓存 P95

## Problem

Monitor V4 分组卡片已有成功率、首字 P95 和总耗时 P95，但没有展示成功请求实际获得的缓存规模。Sub 原生 `usage_logs` 已持久化 `cache_read_tokens`，当前 V4 投影未读取该字段。

## Goal

在每个分组的 Monitor V4 响应和现有分组卡片中新增缓存 P95 字段：对所选时间窗内所有最终成功请求的 `cache_read_tokens` 计算 P95；成功但未命中缓存的请求以 0 参与。

## Non-goals

- 不改变成功率、真实请求优先/主动探测兜底、TTFT 或总耗时口径。
- 主动探测结果不参与缓存 P95；不新增数据库表、迁移、配置或缓存事实源。
- 不增加额外页面指标或新的监控版本。

## User/System Flow

`usage_logs` -> Monitor V4 分组投影 -> handler JSON -> 现有 `HybridPerformanceGroupCard`。仅 `successful = usage_completeness complete AND actual_cost > 0` 的真实请求进入缓存样本；探测事件和失败请求排除。没有成功样本时字段为 `null`，样本数为 0。

## Contract and Data Changes

- `MonitorV4Group.cache_read_tokens_p95`: nullable non-negative number.
- `MonitorV4Group.cache_read_tokens_sample_count`: non-negative integer; 与成功请求样本数一致（包含 0 值）。
- 后端投影通过 `PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY cache_read_tokens)` 计算。
- 保持 Monitor V4 contract version 2，新增字段为向后兼容的响应扩展；前端契约对字段作严格校验。

## UI States

- 有样本：显示缓存读取 token P95，沿用现有 token 格式化。
- 无成功样本：显示 `--`，不影响其他指标。

## Test Strategy

- Repository SQL contract test 锁定 `cache_read_tokens`、成功过滤和 P95 聚合。
- Service/handler 映射测试锁定字段透传。
- Frontend contract/card tests 锁定字段校验、格式化和空样本状态。

## Acceptance Criteria

- [ ] 分组响应返回缓存 P95 和样本数，成功请求 0 缓存不会被排除。
- [ ] 失败请求、主动探测不影响缓存 P95。
- [ ] 分组卡片显示缓存 P95；无样本显示 `--`。
- [ ] 现有 Monitor V4 成功率、TTFT/耗时、窗口切换行为保持不变。

## Risks

- 大窗口增加一次分位数聚合；通过沿用现有窗口过滤和索引，必要时观察查询耗时。
- `cache_read_tokens` 的单位为 token 数，页面文案必须明确为“缓存读取 P95”，避免误解为命中率或耗时。
