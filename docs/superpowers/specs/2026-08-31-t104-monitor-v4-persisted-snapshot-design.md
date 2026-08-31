# Spec: T104 Monitor V4 持久化分组快照

日期：2026-08-31（Asia/Shanghai）

## 1. 问题与证据

当前 `/monitor-v4` 请求链为 `monitor_v4_handler -> MonitorV4Service.Snapshot -> ProjectMonitorV4GroupsForGroups`。每次打开页面、切换窗口或自动刷新都会重新扫描 `usage_logs`、`ops_error_logs`、`account_monitor_results` 和探测桶终态，长窗口查询因此占用数据库并让同一窗口在不同请求时间得到不同结果。

页面默认请求 `24h`；历史离线复算曾使用固定截点的 `7d` 或 `30d`。例如 T102 生产证据同时给出 24h Pro 约 56% 和 30d Pro 约 91.7%，把不同窗口或不同截点的数值直接比较会产生“页面约 60%、离线约 84%”的假象。当前响应的 `generated_at` 是本次 HTTP 请求时间，不是统计生成时间，无法证明两次结果使用了同一截点。

当前 SQL 已实现最终逻辑请求去重、真实请求优先、主动探测按桶聚合、成功样本截尾平均和成功真实请求缓存命中率；但历史缺失探测终态仅累加 `missing_probe_terminal_count`，没有生成失败逻辑请求。此前 T102 文档曾把该情况定义为“无样本”，而用户随后明确要求“失败就算为失败、按入分母”，并要求探测终态必须可保证。因此本任务以最新用户决定覆盖旧的“缺失不入分母”文字。

## 2. 目标

1. 由单例 worker 每 5 分钟计算并持久化 `24h`、`7d`、`30d` 三个窗口的 Monitor V4 分组投影。
2. 页面请求和窗口切换只读取最近一次成功持久化快照，不再触发全窗口统计 SQL。
3. 固定每次快照的 `generated_at`、`window_start`、`window_end`，使页面值与离线复算可按同一截点对账。
4. 保持用户已确认的最终结果口径：真实用户逻辑请求优先；桶内无真实请求时用同桶一次主动探测；最终用户可见失败计入分母；内部切号后最终成功的中间 attempt 不计失败；明确不支持模型和客户端责任错误不计本站服务失败。
5. 对已结算但缺少探测终态的桶 fail-closed 合成为一次 `0/1` 失败，并记录脱敏完整性告警；不能返回正常“无样本”来掩盖缺口。
6. 保持 TTFT/完整耗时字段名和页面 P95 文案不变，仍只从成功样本计算前后 5% 截尾平均；保持缓存命中率字段及 T102 口径。

## 3. 非目标

- 不改变请求路由、重试、计费、账号状态或 Sub 原生账号槽位语义。
- 不恢复或新增 admission、slow-session、账号级额外并发限制。
- 不修改 Monitor V2、账号评分、调度排序、主动探测题面或探测周期。
- 不建立第二套真实请求/计费事实源；快照表只是可重建的派生缓存，原始事实仍是 `usage_logs`、`ops_error_logs`、`account_monitor_results` 和 `account_monitor_bucket_terminals`。
- 不增加页面可见指标或 API 新字段；继续使用现有 `contract_version=2`、`generated_at`、成功率/P95/缓存命中率字段。
- 不做历史数据删除、回填或生产数据人工修正。

## 4. 方案比较与选择

| 方案 | 做法 | 取舍 | 结论 |
| --- | --- | --- | --- |
| A | 页面继续实时查询，仅增加索引/缓存 | 仍受窗口切换和多副本并发影响，无法固定截点 | 不采用 |
| B | Redis 保存整份窗口 JSON | 读快，但重启/故障恢复与多副本一致性依赖可变缓存，且不符合现有数据库派生投影模式 | 不采用 |
| C | PostgreSQL 派生快照表，worker 事务生成并原子替换，页面只读 | 具备重启可恢复、可审计的生成时间和事务原子性；增加一次迁移和 worker wiring | **采用** |

采用方案 C。快照按 `window + group_id` 保存最新成功生成，事务内统一替换三窗口；新生成失败时保留旧生成。生产多实例通过现有 `tryAcquireSingletonLeaderLock`（Redis 优先、PostgreSQL advisory fallback）保证只有一个刷新者。

## 5. 数据与控制流

### 5.1 刷新流程

1. `AccountMonitorRunner` 仅在 `ProcessRoleAll/Worker` 的 singleton 进程启动第三个可停止的快照循环；API 进程不写快照。
2. 循环启动时执行一次刷新，之后按固定 `5m` ticker 运行。每轮将 `now` 截断到分钟，使用同一 `as_of` 计算全部窗口。
3. `MonitorV4Service.RefreshMonitorV4Snapshots` 读取当前 active group 与原生 channel-monitor `group_ids` 配置，构造全局分组集合；不读取某个用户的可见性。
4. 对 `24h/7d/30d` 分别调用现有原生 V4 projection。真实请求、错误过滤、桶仲裁、探测聚合、截尾平均和缓存命中率均由现有 repository SQL 完成；每个窗口都使用同一个 `as_of`。
5. 三个窗口和全部分组投影准备完毕后，repository 在一个 `READ COMMITTED` 写事务中生成 UUID `snapshot_id`，删除旧快照行并插入新行。事务提交前任何错误都回滚，旧快照继续可读。
6. 刷新失败只写脱敏日志（包含阶段、窗口和错误类别，不含凭据/请求体），不覆盖旧成功快照。

### 5.2 读取流程

1. `MonitorV4Service.Snapshot` 仍在请求时读取 active group、配置和用户可见分组，以复用原生权限/专属分组裁剪；名称、平台、倍率等元数据始终来自当前 `Group`，不从快照表取。
2. 按请求窗口从快照表读取最近一次成功生成的该窗口行，校验 `window`、`window_start/end`、`generated_at` 和 `snapshot_id` 一致性，再将投影合并到当前可见分组。
3. 快照存在但较旧时仍返回最近成功结果，`generated_at` 继续通过现有“更新时间”文案呈现，并记录一次限频 stale warning；不会在读请求中回退到实时 SQL。
4. 该窗口尚无成功快照时返回服务不可用错误，前端沿用现有可重试错误态；不伪造 0、成功或“无样本”。

## 6. 指标合同

每个已结算 Asia/Shanghai 5 分钟桶的选择顺序固定为：

- 有真实请求：只使用该桶的真实逻辑请求，探测完全排除。
- 无真实请求且已进入结算时点：使用同桶一个主动探测逻辑请求；同轮跨账号尝试先聚合，任一成功为 `1/1`，全部失败为 `0/1`。
- 已结算桶缺少探测终态：读取侧合成一个 `0/1` 失败并增加内部 `missing_probe_terminal_count`；该失败必须进入 `request_count` 和失败分母。
- 当前桶尚未进入最后一分钟时不提前结算；进入最后一分钟后必须在桶结束前落终态。

成功率为 `success_count / request_count * 100`。成功率为 0 的有请求组必须返回数值 `0`，只有无法构造任何窗口事实（例如尚无快照）才返回错误而非正常空值。

TTFT/完整耗时只筛选成功事件的非空值，分别排序并去掉前后 `floor(n*0.05)` 个样本后求算术平均；响应字段仍为 `ttft_p95_ms`、`latency_p95_ms`，页面继续显示 P95 文案。缓存命中率仅计算成功真实请求：

`SUM(cache_read_tokens) / SUM(input_tokens + cache_creation_tokens + cache_read_tokens)`。

明确模型不支持、客户端责任错误、`usage_completeness='unknown'` 和内部自动切号后最终成功的中间 attempt 不进入本站服务失败分母；最终用户看到的本站/上游失败仍进入分母。

## 7. 持久化契约

新增下一可用编号的迁移（当前候选暂定 `232_monitor_v4_snapshots.sql`；若根总控在整合前已占用该编号，只允许在根总控审核下整体改名，不改变语义）。表名为 `account_monitor_v4_snapshots`，每行包含：

- 主键维度：`window`、`group_id`；
- 生成标识和时间：`snapshot_id UUID`、`generated_at`、`window_start`、`window_end`；
- `contract_version`；
- 成功率、请求/成功/真实/探测计数和 `missing_probe_terminal_count`；
- nullable `ttft_p95_ms`、`latency_p95_ms`、`cache_hit_rate` 及其样本数；
- nullable `source_updated_at`；`current_operational` 为非空布尔值；
- 非负计数/窗口枚举/时间顺序检查约束，`(window, group_id)` 唯一约束和按 `window, generated_at DESC` 索引。

该表只保存最近一次成功生成；删除+插入发生在单事务中，MVCC 保证读者只能看到旧完整生成或新完整生成。原始事实表和探测终态表不受快照写入影响。

## 8. Worker 与并发安全

- 复用 `AccountMonitorRunner` 的 Start/Stop 生命周期，新增 `MonitorV4SnapshotRefresher` 可选依赖；没有依赖时不启动该循环。
- 循环每轮使用独立 context 超时，禁止 ticker 重入；本地 `snapshotMu` 防止同一进程重叠刷新。
- 通过现有 leader-lock helper 使用固定 key（`account-monitor-v4-snapshot`）和大于一轮预算的 TTL；锁失败只跳过本轮，不把失败写成成功。
- 不复用或修改任何 admission/slow-session 代码；账号探测仍由 Sub 原生调度和现有 `AccountMonitorRunner` 负责。
- runner Stop 必须取消 context、停止 ticker、等待 goroutine，避免测试和进程退出泄漏。

## 9. API/UI 兼容

- `GET /api/v1/monitor-v4?window=24h|7d|30d` 路由、鉴权、`contract_version=2` 和现有字段保持不变。
- `generated_at` 改为快照生成时间，因此同一窗口在刷新间隔内稳定；`refresh_interval_seconds` 继续沿用现有设置读取，仅控制前端重新读取，不触发统计。
- 不新增缓存、探测或失败拆分字段，现有卡片布局、P95 文案和窗口控件保持不变。
- 用户可见分组每次仍按当前 active/config/available 权限实时裁剪，防止全局快照泄露专属分组数据。

## 10. 失败、完整性与安全

- 首次启动尚无快照：API 返回可重试的服务不可用；不执行读路径实时补算。
- 刷新 SQL、写事务或 leader lock 失败：保留旧成功快照并输出脱敏 warning；下一轮重试。
- 快照行缺列、跨窗口 `snapshot_id` 不一致、时间逆序或计数不变量违反：读取 fail-closed，记录完整性告警，不把损坏行转成正常指标。
- 缺失探测终态的合成失败只在派生快照中体现；不写 `usage_logs`、不改账号状态、不伪造探测成功。
- 迁移为 expand-only；不删除既有事实表或历史数据。所有日志和测试 fixture 使用虚构 ID/值，不包含密钥、API Key、请求体或生产账号凭据。

## 11. 测试与验收矩阵

### 后端

- 迁移合同：表、检查约束、唯一键、索引和幂等执行。
- repository：三窗口同一 `snapshot_id` 的事务替换；插入失败回滚并保留旧行；读取最新窗口行；跨窗口/分组隔离。
- SQL 回归：真实请求优先、错误优先去重、跨账号逻辑请求去重、探测一次化、缺失终态按 `0/1`、截尾平均边界（`n=1/19/20/21`）、独立 TTFT/耗时空值、缓存比例 0/NULL。
- service/handler：页面只调用 snapshot reader；`generated_at` 来自快照；当前可见性仍实时裁剪；无快照/损坏快照可重试且不触发 native projection。
- runner：启动立即刷新、5 分钟周期、刷新不重入、leader 未获取时跳过、Stop 释放循环。

### 前端

- 24h/7d/30d 控件仍发送对应窗口；窗口切换期间不改变字段/布局。
- 使用快照 `generated_at` 显示更新时间；API 响应契约和现有成功率/P95/缓存卡片测试继续通过。
- 首次服务不可用和刷新失败沿用现有错误/重试态，不渲染伪造指标。

### 直接验收

- 对固定 `as_of` 调用三窗口 API，与同一截点只读 SQL 复算逐组比对成功数、总数、成功率、TTFT、耗时和缓存命中率。
- 连续两次页面刷新/窗口切换只产生快照读取 SQL，不产生 `ProjectMonitorV4Groups` 全窗口统计调用。
- 刷新 worker 故障时页面继续显示最近成功生成和生成时间；恢复后下一轮原子替换。

## 12. 发布、回滚与依赖

- 本任务只在候选 worktree 实现和测试；根总控负责刷新、合并、推送、迁移预检、部署和线上验收。
- 新迁移预期会使 `downtime_required` 由根发布预检决定；未取得主站明确授权前不执行任何发布动作。
- 合并后必须从干净根 `main == origin/main` 构建；若迁移或健康预检失败，保留候选和旧快照，按发布链回滚到上一已验证镜像。数据库迁移为 expand-only，无回滚 SQL。
- 依赖现有 `AccountMonitorService` 原生 projection、`AccountMonitorRunner` singleton 生命周期、`LeaderLockCache/pg advisory` 和 PostgreSQL；不新增外部服务。

本轮按用户最新要求采用快速验证：只执行覆盖变更行为的 repository/service/runner/handler/frontend 功能测试与 touched-file 格式检查；不执行全包回归、完整 server/frontend 构建、长时间性能压测或额外审查。未运行的检查记录在交接和报告中，不得被表述为已通过。

## 13. 批准记录

- 用户已确认：真实请求优先、无真实请求桶主动探测兜底、最终失败入分母、缺失/失败不可伪装为无样本、成功请求 TTFT/耗时 P95（实现为前后 5% 截尾平均）、缓存命中率沿用 T102、保留 24h/7d/30d。
- 用户明确要求：统计改为约 5 分钟定时持久化，窗口切换读取最近一次结果；本轮只处理持久化与实际成功率差异，暂不处理账号资源优化。
- 根发布总控于 2026-08-31 明确解冻并要求继续 T104，T103 已废弃且 native-only 约束永久有效；该委托与此前用户“开始拆解/继续、只做持久化与成功率差异”的确认共同作为本规格的书面批准依据。本规格仅在候选内推进，未授权任何合并、推送或部署。
