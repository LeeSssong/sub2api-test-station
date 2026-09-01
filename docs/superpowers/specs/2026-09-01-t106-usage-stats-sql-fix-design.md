# T106 用户用量汇总 SQL 修复规格

## Goal

恢复用户 `/api/v1/usage/stats` 汇总接口，使顶部请求数、Token、消费和 endpoint 统计继续基于同一组原生 `usage_logs` 过滤结果返回。

## Root Cause

`usageLogRepository.GetStatsWithFilters` 在 2026-08-30 的官方 0.1.183 合并重应用中被改为单次 scoped 聚合，但 SQL 模板残留两个 `%s`，`fmt.Sprintf` 只提供一个参数，生产 PostgreSQL 收到 `%!s(MISSING)` 后报语法错误。当前查询主体还保留 10 列普通汇总形状，而扫描代码要求 13 列的总计与 endpoint 分组结果，说明同一次重应用遗漏了 scoped `GROUPING SETS` 主体。

## Scope

- 仅修复 `GetStatsWithFilters` 的 SQL 形状。
- 继续复用现有 filter builder、参数顺序和 `usageLogNormalQueryFilter`。
- scoped CTE 只读取一次 `usage_logs`，总计、入口 endpoint、上游 endpoint 和 endpoint path 均聚合同一 scoped 数据集。
- 保留现有 Token、`total_cost`、`actual_cost`、有效账号成本和 endpoint 排序语义。
- 增加直接 repository 单元回归测试，覆盖 SQL 完整性、过滤参数和 13 列结果映射。

## Non-Goals

- 不修改 handler、service、前端、计费公式、明细接口或生产数据。
- 不新增第二套聚合或计费事实源。
- 不合并根 `main`、不推送、不部署、不读取凭据。

## Design

CTE 内按现有 conditions 构造唯一 scoped 数据集，并投影归一化 endpoint、Token、成本和耗时列。外层从 `scoped` 读取，通过 `GROUPING SETS ((), (inbound_endpoint), (upstream_endpoint), (inbound_endpoint, upstream_endpoint))` 一次返回总计和三类 endpoint 行；`GROUPING(...)` 两列继续驱动现有 Go switch，不改变响应结构。

账号成本只在 CTE 中使用现有表达式计算，外层统一 `SUM(account_cost)`，避免重新访问原表列或引入新口径。

## Acceptance Criteria

- 回归测试在旧实现上因 malformed/unscoped SQL 失败。
- 修复后查询不含 `%!`，只出现一次 `FROM usage_logs`，并从 `scoped` 聚合全部统计。
- 用户、起止时间和 `usageLogNormalQueryFilter` 保持在唯一 scoped WHERE 中。
- 汇总及 endpoint 结果映射测试通过，相关 Go 测试、gofmt 和 `git diff --check` 通过。
