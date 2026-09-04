# T129 账号与分组监控运行时故障修复规格

## 问题证据与当前行为

- 主站运行镜像 `07ef269d4345f800a47b1965f5dbe15646bfa2c1` 的 `GET /api/v1/admin/accounts/monitor` 返回 500；服务日志明确为 `list real request timelines: pq: column "bucket_start" does not exist`。
- `ListRealRequestTimelines` 的 `real_buckets` CTE 在同一层 `SELECT` 中定义 `bucket_start` 别名，又在 `GROUP BY account_id, bucket_start` 中引用它。生产 PostgreSQL 将其解析为不存在的输入列。
- Monitor V4 定时刷新持续报 `monitor v4 cache denominator invariant violated`。SQL 正确沿用 Sub 原生渠道状态监控的 `input_tokens + cache_creation_tokens + cache_read_tokens` 分母，但 T128 新增的服务校验错误地要求分母等于 `cache_creation_tokens + cache_read_tokens`。
- 前端行为符合现有失败处理：账号页将 500 显示为“账号监控服务暂时不可用”；分组性能页继续显示最后一份可用快照，因此截图中的数据不是最新刷新结果。

## 目标与非目标

### 目标

1. 账号监控 1h、24h、7d、30d 时间线查询均不再引用未投影的 `bucket_start`。
2. Monitor V4 缓存命中率严格使用 Sub 原生渠道状态监控口径 `cache_read / (input + cache_read + cache_creation)`，且仓储投影满足服务不变量。
3. 主站与独立验收站部署同一根 `main` commit/tree，两个站点健康且目标接口恢复。

### 非目标

- 不改变请求成功、探测兜底、P95、排名、计费或调度语义。
- 不放宽 Monitor V4 不变量，不用旧快照掩盖刷新失败。
- 不新增 migration、配置、依赖、数据回填或生产业务数据写入。
- 不复制主站数据库、凭据、缓存、上传文件或日志到验收站。

## 方案比较与选择

1. **推荐：修正时间线 SQL，并让 Monitor V4 内部投影携带 input token 后按原生公式精确校验。** 直接消除生产错误，并保持服务层 fail-closed 校验。
2. 仅把服务校验放宽为“分母不小于缓存 token”。无法证明分母精确等于原生公式，拒绝。
3. 前端在接口失败时改用估算或静态快照。只能隐藏后端故障，不能恢复账号监控接口，拒绝。

## 实现设计

- 在 `ListRealRequestTimelines` 的 `real_buckets` CTE 中为 `date_bin(...)` 显式命名，并按相同表达式分组，避免 PostgreSQL 在同层 `GROUP BY` 解析输出别名。
- 保留 Monitor V4 聚合中 `cache_hit_denominator = input_tokens + cache_creation_tokens + cache_read_tokens` 和相同分母的 `cache_hit_rate`。
- 在内部 `MonitorV4GroupProjection` 增加聚合后的 `InputTokens`，并将 `ValidateMonitorV4Projection` 改为精确要求分母等于三者之和；该内部字段不扩展用户 API。

## 测试与验收

- RED：新增 PostgreSQL SQL 生成边界测试，证明旧 `real_buckets` 查询会引用不可见别名；新增服务校验测试，证明原生含 input 的合法分母被旧校验错误拒绝。
- GREEN：仓储定向测试通过；Monitor V4 service 定向测试通过；server build 和 `git diff --check` 通过。
- 主站：发布预检必须报告 `downtime_required=false`；发布后健康端点均 200，登录态 `/api/v1/admin/accounts/monitor?range=24h` 返回 200，日志不再新增两类错误，Monitor V4 快照时间推进。
- 验收站：只从同一干净根 `main` 使用独立站发布控制器部署；根、`/health`、`/readyz` 通过，source commit/tree 与主站一致，登录态目标接口返回 200。

## 发布、回滚与批准

- 用户于 2026-09-04 明确要求“直接开始实施部署到主站、验收站”，授权本任务完成后发布两个环境。
- 若预检返回 `downtime_required=true`，在任何停服、迁移、重启或切换前暂停并请求额外授权。
- 回滚使用发布链上一已验证槽位；若需要人工重新发布旧版本，先在根 `main` 形成并推送明确 revert。无数据恢复动作。

## 自审结论

- 无 TBD/TODO、未决产品选择或接口歧义；范围可由一个紧急修复任务完成。
- 两个故障共享同一监控仓储文件和同一线上验收窗口，不拆成独立发布。
