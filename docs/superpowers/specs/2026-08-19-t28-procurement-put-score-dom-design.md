# T28 采购 PUT 与评分 DOM 稳定性设计

## 问题证据与原生盘点

Sub 原生账号模型已经提供 `accounts.procurement_cost_cny`、`accounts.estimated_usable_quota_usd` 和 `account_procurement_cost_versions`，现有 `AccountProfitabilityService.UpdateProcurementConfig` 负责版本化、审计、幂等和事务写入；本任务复用这些字段和入口，不新增表、事实源或管理页面。

当前 `PUT /api/v1/admin/accounts/:id` 在采购字段存在时先调用通用 `UpdateAccount`，再调用采购台账服务。采购事务失败会把已完成的账号更新暴露成 Internal Server Error；清空路径还会写入 `cost_pending` 版本，若查询/扫描/错误映射不完整会表现为 internal error。前端 `AccountMonitorView` 已有保存后 reload，但 `updateProcurementCost` 未生成并复用幂等键。`AccountMonitorCard` 的评分区域 DOM 顺序为评分、排名、优先级，目标为强到弱从左到右：评分、优先级、排名；三列在桌面和 390px 均保持稳定，不改变评分算法或排名口径。

## 目标与非目标

目标：

1. 采购新录入、修改、清空在一次原生采购事务内完成，避免通用账号更新先提交造成半成功。
2. 为采购 PUT 提供前端稳定 `Idempotency-Key`，服务端保持同键重放成功、跨账号/跨请求冲突可解释。
3. 正确处理 `cost_pending` 的 NULL 成本/额度、数据库事务回滚和错误映射，返回可执行的管理员错误而非笼统 internal error。
4. 保存成功后 reload 成功才关闭弹窗并提示成功，reload 失败保留错误状态；服务错误覆盖前端提示。
5. 调整最终评分 DOM 顺序并固定三列布局，桌面/390px 无横向溢出。

非目标：不改变评分算法、评分权重、盈利公式、采购成本字段语义、版本台账 schema、调度行为、生产配置或发布链；不使用 GitHub Actions。

## 方案比较与选择

方案 A（推荐）：在 `Update` handler 中先执行采购台账事务，再调用既有 `UpdateAccount` 处理通用字段，但始终传入 nil 采购更新；前端为每次编辑会话生成并复用幂等键。优点是采购失败不会留下通用更新前的半成功，且保持既有 PUT/响应契约；缺点是同一 PUT 同时修改通用字段和采购字段时仍是两个既有服务事务。

方案 B：扩展 `UpdateAccount` service 接口，让通用字段和采购台账共享一个数据库事务。原子性更强，但需要跨 repository 重构，风险和测试面明显扩大。

方案 C：保留当前顺序，仅在采购失败后补偿通用更新。补偿难以覆盖并发和外部副作用，错误窗口仍存在，不采用。

## 端到端数据与控制流

前端 `AccountMonitorView` 打开成本对话框时生成 `account-procurement:<account_id>:<uuid>`；保存/清空在同一会话复用该键调用现有 `/admin/accounts/:id` PUT，并在成功响应后按当前时间窗 reload。服务端验证两个采购字段必须同时出现，数字满足有限值/非负成本/正额度；采购请求有幂等键时先调用 `UpdateProcurementConfig`，该服务在单个 PostgreSQL 事务中锁定账号和当前未结束版本，读取 `sql.NullFloat64`，安全处理 `cost_pending` NULL，结束旧版本，插入新版本或清空版本，更新 accounts 投影并写 audit log；随后通用 `UpdateAccount` 始终传入 nil 采购更新，避免重复写投影。相同键和相同账号直接提交成功重放，不重复写入；同键不同账号返回冲突错误。采购服务错误通过现有 `response.ErrorFrom` 映射为明确的 4xx/5xx JSON，前端展示服务返回消息。

## 接口与字段契约

- `PUT /api/v1/admin/accounts/:id`
- JSON：`procurement_cost_cny` 与 `estimated_usable_quota_usd` 必须同时提供；录入/修改为 number；清空均为 `null`；省略表示不修改。
- Header：`Idempotency-Key`（前端生成并复用）；若未提供，保留现有 `X-Request-ID` fallback，仅在采购更新需要幂等时拒绝空值。
- 成功：现有账号响应，可能带 `X-Idempotency-Replayed: true`。
- 失败：沿用原生错误响应；校验错误 400；幂等冲突 409；数据库/事务错误 500，消息不泄露凭据。

## 失败、安全与兼容语义

事务失败必须 rollback，不更新 accounts 投影；`cost_pending` 版本允许 `cost_cny`/`estimated_usable_quota_usd` 为 NULL，扫描必须使用 nullable 类型。重复提交同一键不创建新版本；相同键负载不一致按现有 request_id 约束返回冲突。错误响应不包含 API key、credentials 或 SQL 细节。旧客户端未发送采购字段时行为不变；旧客户端使用 `X-Request-ID` 仍可工作。

## 验收矩阵

| 场景 | 期望 |
| --- | --- |
| 新录入成本/额度 | 200；投影和 active 版本一致；reload 后卡片显示新值 |
| 修改成本/额度 | 200；旧版本 ended，新版本 active，剩余额度规则不变 |
| 清空 | 200；投影 NULL，新增 `cost_pending` 版本，报表显示成本待录入 |
| 同键重复提交 | 200 replay；版本数量不增加 |
| 同键不同账号/负载 | 409；不写入 |
| 服务/事务错误 | 非 200，前端保留对话框并展示错误，不显示成功 |
| reload 失败 | 显示“保存成功但最新监控卡片加载失败”，不关闭对话框 |
| 评分 DOM | 最终 DOM 左到右评分、优先级、排名；桌面/390px 三列稳定、无整页横向溢出 |

## 测试策略

后端增加 service sqlmock 覆盖新录入、修改、清空 `cost_pending` NULL、重复键 replay、服务错误 rollback；handler 覆盖采购 PUT 不先写通用账号、错误映射和成功响应。前端 API/AccountMonitorView 测试覆盖幂等键复用、PUT+reload 成功、reload 失败、服务错误；AccountMonitorCard 测试断言 DOM 顺序和 390px 稳定类名。运行受影响 Go/Vitest focused tests、typecheck、build、gofmt 和 diff-check。

## 发布、回滚与未决项

无迁移、无配置变化、预期 `downtime_required=false`；仅在根总控合并后的 main 上发布。回滚为恢复上一已验证镜像/提交，不修改生产数据。未决项：通用字段与采购字段同 PUT 的跨服务事务仍保持既有边界，若未来要求全量原子化另立任务。

## 批准记录

本规格依据 T28 用户任务包、原生约束与 2026-08-19 代审授权形成；进入计划和实现前由发布总控代审确认范围未扩大。
