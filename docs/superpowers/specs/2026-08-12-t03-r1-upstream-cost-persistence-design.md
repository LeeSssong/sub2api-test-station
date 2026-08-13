# T03-R1 上游扣费异步持久化修复设计（已废弃）

> **状态：已由用户后续产品决策取代。** 本文件只保留历史追溯，不再是实施事实源。
> 当前有效规格为
> `docs/superpowers/specs/2026-08-12-t03-r1-account-financial-reconciliation-design.md`。
> 禁止依据本文件恢复“直接扩展 usage_logs”实现。

## 状态与批准门禁

- 顶层任务：T03-R1 上游扣费缺失与异步持久化修复
- 基线：`main@747c7fb14d1ded243794a77984778babece7c799`
- 分支：`codex/t03-r1-upstream-cost-persistence`
- 状态：方案和分段设计已获用户批准；书面规格等待用户最终审阅批准
- 未经用户明确批准本书面规格，不得调用 `writing-plans` 或开始实施

## 生产问题证据与当前行为

T03 已保留且不回滚的能力是：管理员打开
`GET /api/v1/admin/usage/:id/upstream-cost` 时，服务以精确请求 ID 即时查询
Sub `/api/v1/usage/records`，或查询 New API `/api/log/token` 并结合
`/api/status.quota_per_unit` 换算，随后临时返回上游实际扣费与利润。精确命中记录后，
原生扣费字段为 blank、`null` 或空字符串时，T03 会将其归一为 `0`。

该能力没有持久化：提交 `fe9d04c9ccd8e40faf1e58ff137004f57964392b`
未增加数据库字段、迁移、后台登记任务或 worker 消费链。管理员不打开详情时，
不会发生查询或登记。

2026-08-12 生产只读核验结果：

- 最近 500 条自然流水中 Sub 244 条、New 256 条，精确
  `upstream_request_id` 覆盖为 500/500。
- 同一批 500 条中，持久化上游实际扣费和利润为 0/500，缺失率 100%。
- 最近 24 小时 Sub 精确上游请求 ID 覆盖 3865/4016（96.24%），New 为
  1770/1770（100%）。七日历史覆盖较低，但本任务明确不回填历史。
- 生产 `usage_logs` 有 `actual_cost`、`upstream_request_id` 和 `account_cost`，
  但没有上游实际扣费、利润、登记状态、失败原因或登记时间字段。
- `account_cost` 是既有账号成本快照，不是 Sub/New 原生逐笔账单，禁止替代本任务事实。

## 目标

1. 对 T03-R1 部署后新产生的流水，在请求结束后无需管理员打开详情，复用现有
   usage 异步记录链执行一次原生账单精确读取并持久化结果。
2. 管理员列表、详情和兼容接口读取同一持久化事实。
3. 精确命中账单记录后，实际扣费为有效有限数值时保存原值；字段明确为 blank、
   `null` 或空字符串时保存 `0`。
4. 利润固定为 `usage_logs.actual_cost - upstream_actual_cost`。
5. 无精确账单记录或读取失败时，保存稳定的 `unavailable` 失败语义，不伪造数值。

## 非目标

- 不回填任何历史流水，不扫描部署前记录。
- 不建立延迟补查、定时扫描或上游重试窗口；每条新流水只执行一次业务账单读取。
- 不估算、不模糊匹配、不使用 `account_cost` 作为回退。
- 不引入 external-primary、relay-ops 主账务路径或新的管理页面。
- 不修改本站客户计费、账号调度、倍率、余额或上游凭据。
- 不启动或触碰 T05。

## 影响用户与边界

- 管理员：在既有用量列表、详情及兼容接口中看到持久化的上游扣费、利润或安全失败原因。
- 普通用户：接口、DTO 和页面不增加上游扣费、利润、登记状态或失败原因。
- 推理调用方：账单读取发生在响应后的 usage 异步任务中，不延长或改变已返回的推理响应。
- 历史记录：新增字段保持 `NULL`，显示“历史记录未登记”，不等同于零成本或登记失败。

## 方案比较与选择

### 方案 A：扩展现有 usage 异步记录链并原子落库（已选）

请求完成后，现有 usage 任务执行一次原生账单读取，将确定的成功或失败结果与流水
一次插入。它复用当前用量 worker、关键任务同步降级和唯一约束，不引入第二套任务生命周期。

优点是结果与流水原子一致、读取简单、没有补写空窗；代价是需要同步扩展较长的
`usage_logs` schema、插入、扫描、服务模型和 DTO 链。

### 方案 B：事务 Outbox 只投递已确定结果

usage 与携带已确定账单结果的 Outbox 事件同事务写入，worker 只负责数据库投递，
不得再次访问上游。它可跨进程恢复，但增加新表、领取、消费、清理和中间态；对于可随
usage 一次落库的结果复杂度过高。

### 方案 C：进程内更新队列

usage 落库后，以内存队列更新账单字段。实现较轻，但蓝绿切换、崩溃或队列溢出会丢失
结果，不能满足可靠持久化要求。

用户已选择方案 A。

## 端到端数据与控制流

1. 网关完成一次 Sub/New 推理请求，得到本地 usage 数据和精确
   `upstream_request_id`。
2. Handler 通过既有 `submitUsageRecordTask` / `submitOpenAIUsageRecordTask`
   将工作提交给 usage worker；关键计费任务继续保留现有同步降级语义。
3. usage 任务在自身超时预算内执行一次原生账单读取：
   - Sub：调用 `/api/v1/usage/records`，沿用现有时间窗、有界分页和精确请求 ID
     匹配，读取命中记录的 `actual_cost`。
   - New API：调用 `/api/log/token` 精确匹配，读取 `quota`；调用 `/api/status`
     读取有效正数 `quota_per_unit`，计算 `quota / quota_per_unit`。
   - 现有 404-only Sub→New 原生端点识别可继续复用，不扩展为模糊发现。
4. 将读取结果归一化为内部登记结果。上游网络读取不得发生在数据库事务或行锁内。
5. 在创建 `usage_logs` 的同一次插入中原子保存本站用量、已经确定的上游扣费终态、
   利润和登记时间。
6. 管理员读取链直接投影持久化字段，不再因打开详情访问上游。

账单读取失败不得改变已经返回给调用方的推理响应。usage 记录自身的现有持久化失败
处理和同步降级保持有效；本任务不另建上游补查任务。

## 触发点与幂等键

- 触发点：仅为部署后新请求的既有响应后 usage 记录任务。
- 历史记录没有触发器、扫描器或回填命令。
- 流水幂等继续使用数据库唯一约束 `(request_id, api_key_id)`。
- 同一 usage 任务若因现有数据库投递机制重复执行，最多产生一条流水；不得用
  `unavailable` 覆盖已存在的 `confirmed` 事实。
- 数据库投递恢复可能造成一次任务重新执行，但不建立独立的业务延迟补查或定时重试。

## 持久化字段契约

在 `usage_logs` 增加向后兼容的可空字段：

- `upstream_actual_cost NUMERIC(20,10) NULL`：Sub/New 原生逐笔实际扣费。
- `upstream_cost_status VARCHAR(16) NULL`：新记录为 `confirmed` 或
  `unavailable`；历史 `NULL` 表示未登记。
- `upstream_cost_reason VARCHAR(64) NULL`：仅失败时保存稳定原因码。
- `profit NUMERIC(20,10) NULL`：仅 `confirmed` 时保存
  `actual_cost - upstream_actual_cost`。
- `upstream_cost_recorded_at TIMESTAMPTZ NULL`：本次登记终态写入时间。

约束：

- `confirmed` 必须同时具有 `upstream_actual_cost`、`profit` 和登记时间，原因为空。
- `unavailable` 必须具有原因和登记时间，成本及利润为 `NULL`。
- 历史未登记记录的全部新增字段为 `NULL`。
- 部署后经目标写入路径产生的新流水必须是 `confirmed` 或 `unavailable` 终态，
  不引入长期 `pending` 状态。

是否使用数据库 `CHECK` 约束由实施计划根据现有迁移风格决定，但应用层与测试必须执行
上述不变量。

## 失败与安全语义

### 成功

- 精确命中且扣费为有效有限数值：`confirmed`，保存原值。
- 精确命中且扣费字段明确为 blank、`null` 或空字符串：`confirmed`，保存 `0`。

### 不可用

- `record_not_found`：账单端点可用，但有界查找中没有精确记录。
- `endpoint_unsupported`：完成既有原生端点识别后仍不支持。
- `credentials_unavailable`：没有可用的原生账单凭据或地址。
- `authentication_rejected`：上游拒绝认证。
- `request_unavailable`：请求构建、网络或超时失败。
- `response_unavailable`：非成功响应、响应体超限、非法 JSON、非法非空扣费数值，
  或 New `quota_per_unit` 缺失、非法或非正数。
- `pagination_unavailable`：有界分页无法完成。

上述失败均立即持久化为 `unavailable`，不补查、不重试上游、不估算。

### 安全

- 不持久化或返回上游凭据、完整 API Key、原始响应、敏感请求体或上游错误原文。
- 失败原因只使用稳定枚举和固定安全文案。
- 普通用户不可见任何新增账务字段。
- `account_cost` 不参与本任务计算或回退。

## API 与管理员读取契约

- 管理员用量列表增加持久化账单投影，使列表无需逐行调用上游。
- 管理员 usage 详情读取相同字段。
- 保留 `GET /api/v1/admin/usage/:id/upstream-cost` 以兼容现有前端或调用方，
  但改为只读持久化结果，不访问 Sub/New。
- 历史 `NULL` 状态显示“历史记录未登记”。
- 新记录 `unavailable` 显示“未获取到扣费信息”及安全原因。
- 只有 `confirmed` 显示数值上游扣费和利润，包括合法的 `confirmed 0`。
- 普通用户列表、详情、统计接口和 DTO 不包含新增字段。

## 兼容与迁移

- 迁移仅增加可空字段及必要索引，不改写历史数据。
- 旧应用版本可忽略新增字段；新应用明确区分历史 `NULL` 与新流水
  `unavailable`。
- 蓝绿切换期间旧、新版本均可访问同一表；不得要求收缩迁移。
- 预期为 expand-only、无停机迁移，但发布预检必须根据真实迁移输出
  `downtime_required=true|false`；若为 `true`，立即停止等待用户确认。

## 场景验收矩阵

| 场景 | 持久化状态 | 上游扣费 | 利润 | 上游重试 |
|---|---|---:|---:|---|
| Sub 精确命中有效 `actual_cost` | confirmed | 原值 | 本站扣费减原值 | 无 |
| New 精确命中有效 `quota` 与单位 | confirmed | 换算值 | 本站扣费减换算值 | 无 |
| 精确命中，扣费 blank/null/empty | confirmed | 0 | 等于本站扣费 | 无 |
| 无精确记录 | unavailable / record_not_found | NULL | NULL | 无 |
| 端点不支持 | unavailable / endpoint_unsupported | NULL | NULL | 无 |
| 鉴权、网络或超时失败 | unavailable / 对应原因 | NULL | NULL | 无 |
| 非空非法扣费或 New 单位非法 | unavailable / response_unavailable | NULL | NULL | 无 |
| 部署前历史记录 | NULL / 历史未登记 | NULL | NULL | 无 |
| 重复 usage 任务 | 仍仅一条流水 | 不重复 | 不重复 | 无业务补查 |
| 普通用户读取 | 字段不存在于响应 | 不可见 | 不可见 | 不适用 |

## 测试策略

- 账单数值解析表驱动测试：数字、数值字符串、blank、`null`、空字符串、非法值、
  `NaN`/无穷或溢出边界。
- Sub/New 服务测试：精确匹配优先级、时间窗、有界分页、Sub→New 识别、New 单位换算
  与全部稳定失败码。
- usage 服务测试：账单结果进入 `OpenAIRecordUsageInput` / `UsageLog`，利润公式正确，
  `confirmed`/`unavailable` 不变量成立。
- Repository 测试：插入参数顺序、批量与 best-effort 路径、扫描、旧记录 `NULL`、
  唯一冲突和原子性。
- Handler/DTO/前端测试：管理员列表、详情和兼容接口一致；历史、confirmed、
  confirmed-zero、unavailable 展示；普通用户字段隔离。
- 迁移测试：字段类型、可空性、索引和既有迁移校验。
- 最终验证：相关 Go 聚焦测试、必要回归、`go vet`、服务构建、前端聚焦测试、
  typecheck、生产构建和 `git diff --check`。

## 发布、线上验证与回滚

### 发布

- 候选在独立任务中完成实现、任务复审和全分支终审后，只能汇报
  `READY_FOR_ROOT_REVIEW`。
- 未收到根任务带精确目标 main SHA 的 `AUTHORIZE_MERGE_TO_MAIN`，不得合并。
- 顶层任务不得自行推送或部署；发布不使用 GitHub Actions。
- 生产只能从合并后完成专项验证的 `main` 执行。

### 线上验证

- 仅观察部署后自然产生的新流水，不制造付费请求。
- 分别验证 Sub/New 新流水无需打开详情即可持久化终态。
- 验证管理员列表、详情和兼容接口读取相同事实，打开详情不再产生账单网络请求。
- 验证普通用户响应不含新增字段。
- 验证 API、worker、数据库及活动槽健康且无异常重启。
- 自然流量未覆盖的分支可用同一发布树合同测试补充，但不得声称线上出现该样本。

### 回滚

- 应用回滚到前一已验证镜像；新增可空字段保留，不执行收缩迁移。
- 不删除已经持久化的确认结果，不回填历史。
- 若迁移、部署或线上验证失败，保留候选、失败证据和修复内容，在同一任务继续修复。

## 待决事项

以下实现细节留给书面规格批准后的实施计划，不改变产品合同：

- 新增字段的最终 SQL 长度和是否增加 `CHECK` 约束。
- 为管理员列表筛选/排序所需的最小索引；不得默认增加无用途索引。
- usage 任务整体超时预算内如何为原生账单读取分配子超时，同时保持现有响应后行为。
- 各协议入口的最小接线清单，以测试证明覆盖而非扩大无关重构。

## 用户批准记录

- 用户确认历史范围：完全不回填。
- 用户选择方案 A：扩展现有 usage 异步记录链并原子落库。
- 用户批准第 1 段：数据流与持久化字段契约。
- 用户批准第 2 段：失败、安全、兼容迁移与管理员读取。
- 用户批准第 3 段：验收、测试、发布与回滚。
- 不可漂移语义：精确命中记录后 blank、`null`、空字符串为 `confirmed 0`；
  没有精确记录或端点、鉴权、网络、解析失败为 `unavailable`。
- 本书面规格仍需用户明确最终批准后，方可进入 `writing-plans`。
