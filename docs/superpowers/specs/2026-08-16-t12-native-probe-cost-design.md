# T12 经营页本站探测花费与排序 / USD 字段修订规格

状态：`DESIGNING`，本轮仅交付书面规格、自审和 handoff；未调用 `writing-plans`，未写实现、迁移或测试。

## 1. 现状证据与问题边界

- T11-R1 的 `/api/v1/admin/operations/account-financial` 已从原生 `usage_logs` 聚合请求数、Token、账号计费 `cost`、用户扣费 `user_cost`，并派生利润与利润率；`usage_logs.account_cost` 优先、历史空值回退原生公式。
- 当前 `usage_logs` 的 `user_id`、`api_key_id`、`account_id` 均为 `NOT NULL` 外键；`request_id`、`model` 也有必填约束。现有 `usage_completeness` 已存在且默认为 `complete`，但没有 `usage_source`。
- 因而“无用户/API Key 身份的 probe 行直接写入 `usage_logs`”不可实施：不能写 `NULL`，也不能写虚构的 `0` ID；若放宽约束，会改变用户账务表的完整性和所有既有查询语义。
- 账号监控自动探测、定时账号测试和管理员手动测试是本站运营调用；它们需要本站成本可核算，但不应改变用户余额、用户扣费、账号计费、利润或利润率。
- 用户要求是“未消费金额保持 USD”。现有未消费金额字段、接口名称和 DTO 兼容性保持不变；T12 仅在经营页校正/维持美元符号与 USD 文案语义，不做字段重命名、弃用或余额模型重构。

## 2. 目标与非目标

目标：

1. 保留六项财务指标及排序：请求数、Token、账号计费、用户扣费、利润、利润率。默认排序沿用现有页面；利润率空值置底，升降序可切换。
2. 增加独立“本站探测花费”摘要卡片、分组/账号字段，并可显示探测请求数与 Token；探测字段不进入六项排序。
3. 自动健康探测、定时账号测试、管理员手动测试都写入同一套本站 probe 记录链路；完整计价时记录 USD 成本，缺失用量时显式不完整且不估算。
4. 经营页所有金额继续以 USD、两位小数展示；内部数据库、服务 DTO 和聚合保留原始精度，禁止逐笔截断。现有未消费金额字段/接口名称不变。
5. 启用后从新的时间水位开始记录；不回填、不改写历史业务数据。

非目标：用户消费/余额扣减、余额 DTO 或字段迁移、API 字段重命名/弃用/alias、其他余额页面改造、`usage_logs` 用户事实源、普通用户接口、调度/路由、外部余额差额、人工覆盖、采购成本、汇率换算、第二个管理员入口、历史回填、GitHub Actions。

## 3. 唯一承载方案与取舍

### 选择：独立 append-only `account_probe_cost_logs`

新增一张仅服务本站探测成本的 append-only 表（建议名 `account_probe_cost_logs`），以 `account_id` 为必填外键，`group_id` 为探测发生时的可空快照；不包含 `user_id`、`api_key_id`，也不引用用户凭据。记录 probe 类型、幂等运行标识、请求/Token、模型、原生账号成本、`usage_completeness`、探测结果、失败原因和 `created_at`。

这不是第二套用户账务源：它只回答“本站探测消耗了多少”，不替代、改写或混入 `usage_logs`；用户六项财务仍只读 `usage_logs`。探测成本表不参与用户余额扣减、用户统计、普通管理员统计或任何用户账单。

不选把 probe 塞入 `usage_logs`：现有三列身份均必填，放宽约束或写占位 ID 都会破坏原生事实源。也不选余额差额/独立人工账：依赖外部控制面、无法按账号/分组守恒，且会形成不可审计的第二计费模型。

## 4. 数据契约与写入链路

### 4.1 `account_probe_cost_logs` 最小字段

| 字段 | 约束/语义 |
| --- | --- |
| `id` | `BIGSERIAL` 主键，只追加 |
| `account_id` | `BIGINT NOT NULL`，引用 `accounts(id)`；账号删除按既有策略阻止或保留历史，不写孤儿行 |
| `group_id` | `BIGINT NULL`，探测时的真实分组快照；`NULL` 表示未归属，不按当前归属回推 |
| `probe_kind` | `monitor` / `scheduled` / `manual`，由调用上下文显式传入 |
| `probe_run_id` | `VARCHAR NOT NULL UNIQUE`，本站每次实际 probe attempt 的稳定幂等标识；重试同一落库动作不得重复计费，禁止伪造用户请求 ID |
| `model` | 实际探测模型，不能为空 |
| `input_tokens`、`output_tokens`、缓存 Token | `INT NOT NULL DEFAULT 0`，完整响应可填 |
| `account_cost` | `DECIMAL(20,10) NULL`，原生账号成本快照；完整才有值，禁止估算 |
| `usage_completeness` | 沿用现有语义 `complete` / `partial` / `unknown`；不得新增另一套含义相近的状态字典 |
| `probe_outcome` | `success` / `failure`；与用量完整性正交，失败响应若存在可核算用量仍按真实完整性记录 |
| `error_code` | 可空的稳定诊断码，不保存凭据或请求体 |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` |

无需 `usage_source` 列：表本身就是 probe 来源边界；`usage_completeness` 继续复用现有状态语义，但不扩展 `usage_logs`。

写入顺序：探测链路显式携带 `probe_kind`、`probe_run_id` 与账号/分组快照 → 复用原生模型计价和账号倍率快照计算最终 `account_cost` → 以 `probe_run_id` 幂等 append-only 写入 probe 表。落库失败只记录诊断并返回原有探测结果，不失败用户请求、扣费或监控主流程。缺 Token/价格/模型时写 `partial` 或 `unknown`、`account_cost=NULL`，绝不以余额差额或默认价格补齐。

### 4.2 add-only migration 与停机门禁

需要一条 add-only schema migration：新建表、索引（`created_at`、`account_id,created_at`、`group_id,created_at`）和必要的外键/检查约束；不改写 `usage_logs`，不回填历史。迁移只新增结构，但仍必须进入既有发布预检并输出 `downtime_required=true|false`；若预检判定需要停机，必须在停服或切换前暂停等待人工确认，不能绕过。

“不做历史迁移/回填”明确指：不为历史用户请求或历史探测生成新行，不更新/重算/改写既有 `usage_logs`、余额、成本和利润数据。迁移仅创建空表及约束。

## 5. account-financial API / 读模型

继续使用 `GET /api/v1/admin/operations/account-financial?range=today|24h|7d|31d`，在同一 repeatable-read 快照内分别读取：

- 用户财务：现有 `usage_logs` 聚合，保持 `requests`、`tokens`、`cost`、`user_cost`、`profit=user_cost-cost`、`margin=profit/user_cost`；probe 表永不参与这些字段。
- 探测财务：probe 表按 `(group_id, account_id)` 聚合，再折叠到分组和全站；字段为 `probe_requests`、`probe_tokens`、`probe_cost`、`probe_cost_status`。

`probe_cost_status`：`confirmed`（窗口内所有 probe 都有完整成本）、`incomplete`（存在 probe 但至少一笔成本缺失，金额对外显示 `—`）、`unavailable`（窗口无 probe 记录）。窗口有记录且总成本确为零时显示 `$0.00`；无记录同样显示 `$0.00` 并标注“暂无探测记录”，避免把零与未知混淆。

对外继续使用现有 account-financial 字段、接口名称与 DTO；不新增或弃用余额字段，也不增加兼容别名或迁移全局余额字段。经营页按响应的 `currency: "USD"` 将现有未消费金额显示为美元并保留两位小数，不做 CNY→USD 换算，不改其他余额页面。

账号、分组和未归属行都保留真实快照维度；不得按当前账号分组猜历史分组。探测字段只出现在管理员 account-financial DTO，普通用户 DTO/API 永不返回。

## 6. 经营页 UI 与排序

- 摘要新增独立“本站探测花费”卡片，显示金额、状态和探测请求数/Token。
- 账号表新增“本站探测花费”列；现有“账号计费”明确表示用户 `usage_logs` 成本。分组视图沿用同一字段。
- 六项排序只作用于请求、Token、账号计费、用户扣费、利润、利润率；探测花费不改变默认排序和不参与利润计算。刷新、窗口切换、分组切换后保留当前排序字段与方向。
- 金额统一 `$0.00` 两位小数；`probe_cost_status=incomplete` 显示 `—` 和短管理员提示；移动端 390×844 保持横向滚动、稳定列宽和可读性。

## 7. 失败、安全、兼容

- probe 表写入失败或读模型 probe 查询失败：用户六项财务仍可返回；探测卡片显示 `unavailable` 并提供管理员可诊断错误，不把整页伪装成用户账务失败。
- 用户 `usage_logs` 写入/扣费链路保持原样；probe 不进入 `usage_logs`，因此既有用户余额、用户用量、普通管理员统计无需增加来源过滤，也不会混入探测。
- 回滚只停止新 probe 写入/展示并保留空表及已写入记录；旧版本继续读取 `usage_logs` 用户口径。禁止删除表或改写历史数据作为回滚。

## 8. 场景验收与测试策略（仅规格，不在本轮执行）

| 场景 | 必须成立 |
| --- | --- |
| 仅用户请求 | 六项财务与 T11-R1 一致，probe 卡片为 `$0.00`/无记录 |
| 三类完整 probe | probe 表有对应 `probe_kind` 行；账号→分组→全站探测成本守恒；用户六项不变 |
| 缺 Token/价格 | `usage_completeness=partial/unknown`、成本 `NULL`、金额 `—`，不估算 |
| probe 写入失败 | 原有探测结果和用户链路成功；管理员日志可定位 |
| 历史窗口 | 启用前无新 probe 行，不回填既有业务数据 |
| 排序 | 六项可升降序，利润率空值置底；探测列不改变排序 |
| USD 显示 | 现有未消费金额字段/接口名不变；经营页显示 USD、两位小数；无汇率计算或其他页面改造 |
| 普通用户 | 看不到 probe 字段与 probe 记录 |
| 桌面/移动端 | 摘要、分组、账号列和不完整提示不重叠、不横溢 |

实施阶段至少覆盖 migration 静态门禁、三类来源传播、原生计价复用、probe/usage_logs 隔离 SQL、聚合守恒、失败降级、API 兼容和页面排序/格式化；本轮不编写这些测试。

## 9. 未决事项与批准请求

实现阶段只需在不改变上述语义的前提下确定具体 probe 调用点和 Ent/SQL 生成方式；不得重新选择 `usage_logs`、改造余额 DTO 或引入第三账务源。请根发布总控书面审阅并批准本修订规格；批准前不得调用 `writing-plans` 或开始实现。
