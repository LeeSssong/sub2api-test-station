# T49 失败尝试从正常流水列表隔离

## 问题证据

重试审计改动会为每次上游失败写入 `usage_logs`，并将没有上游 usage 的行标记为 `usage_completeness = 'unknown'`。管理员正常用量列表和对应统计当前直接查询整张表，因此一次请求的多账号 failover 会显示成多条 0 token/0 元流水。

## 目标与非目标

- 正常 `ListWithFilters` 与 `GetStatsWithFilters` 默认排除 `unknown` 失败占位。
- 保留 `complete`、`partial` 和历史 `NULL`（按 `complete` 兼容）流水。
- 不删除历史数据、不改变扣费/重试语义、不改变失败审计链路、不新增迁移。

## 方案

1. 按 `actual_cost > 0` 过滤：会误伤合法零价或尚未产生金额的业务行，拒绝。
2. 在管理员前端过滤：分页总数和统计仍污染，拒绝。
3. 在原生 usage repository 的正常列表和统计查询统一按 `COALESCE(usage_completeness, 'complete') <> 'unknown'` 过滤，推荐。该条件只隔离已明确标记为未知的失败尝试，保留历史和 partial 语义。

## 数据与失败语义

过滤仅作用于读取查询；写入、扣费、重试和审计记录不变。数据库/查询错误继续原样失败，不静默清空。没有迁移、配置或生产数据修改。

## 验收矩阵

| 场景 | 预期 |
| --- | --- |
| unknown 失败 attempt | 正常列表和统计不可见 |
| complete 成功流水 | 可见/计入 |
| partial 流水 | 可见/计入 |
| usage_completeness NULL 历史流水 | 按 complete 兼容，可见/计入 |
| 失败审计/重试链 | 不受影响 |

## 测试与发布

新增 repository sqlmock 回归，补充管理员 handler 合同测试；运行受影响 Go focused tests、gofmt、compile/build 和 `git diff --check`。无迁移，目标 `downtime_required=false`；发布仍必须从根 `main` 走既有本地/宿主蓝绿链，失败保留候选与回滚镜像。

## 批准记录

用户在原“流水登记优化”窗口已确认该有界方案；本次续接沿用该批准，不扩大范围。
