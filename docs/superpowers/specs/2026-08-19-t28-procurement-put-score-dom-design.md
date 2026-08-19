# T28 采购 PUT 与评分 DOM 稳定性设计

## 问题证据与原生盘点

Sub 原生账号模型已经提供 `accounts.procurement_cost_cny`、`accounts.estimated_usable_quota_usd` 和 `account_procurement_cost_versions`，现有 `AccountProfitabilityService.UpdateProcurementConfig` 负责版本化、审计、幂等和事务写入；本任务复用这些字段和入口，不新增表、事实源或管理页面。

当前 `PUT /api/v1/admin/accounts/:id` 在采购字段存在时先调用通用 `UpdateAccount`，再调用采购台账服务。采购事务失败会把已完成的账号更新暴露成 Internal Server Error；即使调换顺序，采购-only 请求若在台账成功后仍调用通用更新，后一步失败仍会形成半成功。清空路径会写入 `cost_pending` 版本，读取 NULL 成本/额度时必须使用 nullable 扫描。前端 `AccountMonitorView` 已有保存后 reload，但原实现把幂等键保存在 API 模块 Map 中，关闭重开、修改 payload 或保存/清空切换仍可能复用陈旧键。页面已有按 `group_rank` 升序、未排名账号置后的排序和普通 CSS Grid 行优先布局，但缺少乱序输入与响应式最终 DOM 顺序保护测试。

## 目标与非目标

目标：

1. 采购新录入、修改、清空在一次原生采购事务内完成，避免通用账号更新先提交造成半成功。
2. 为采购 PUT 提供前端稳定 `Idempotency-Key`，服务端保持同键重放成功、跨账号/跨请求冲突可解释。
3. 正确处理 `cost_pending` 的 NULL 成本/额度、数据库事务回滚和错误映射，返回可执行的管理员错误而非笼统 internal error。
4. 保存成功后 reload 成功才关闭弹窗并提示成功，reload 失败保留错误状态；服务错误覆盖前端提示。
5. 保证账号卡片按 `group_rank`（与 QualityScore 强到弱结果一致）进入最终 DOM，桌面左到右、换行后上到下；390px 保持同序且无隐式 reverse/order 反转。

非目标：不改变评分算法、评分权重、盈利公式、采购成本字段语义、版本台账 schema、调度行为、生产配置或发布链；不使用 GitHub Actions。

## 方案比较与选择

方案 A（推荐）：在 `Update` handler 中先执行采购台账事务；采购-only 请求随后用 `GetAccount` 返回刷新投影，只有请求确有非采购字段时才调用既有 `UpdateAccount`，且始终传入 nil 采购更新。幂等键由成本弹窗会话按账号和 payload 管理。优点是采购-only 路径不再有第二次写入，且保持既有 PUT/响应契约；缺点是混合 PUT 仍是两个既有服务事务。

方案 B：扩展 `UpdateAccount` service 接口，让通用字段和采购台账共享一个数据库事务。原子性更强，但需要跨 repository 重构，风险和测试面明显扩大。

方案 C：保留当前顺序，仅在采购失败后补偿通用更新。补偿难以覆盖并发和外部副作用，错误窗口仍存在，不采用。

## 端到端数据与控制流

前端 `AccountMonitorView` 在一次成本弹窗会话中按 `account_id + payload` 生成 `account-procurement-<account_id>-<uuid>`；同会话同 payload 的网络/未知结果重试复用该键，关闭重开、成功、payload 改变或保存/清空切换均生成新键。API 只接收并透传显式键，不持有跨会话状态。服务端验证两个采购字段必须同时出现，数字满足有限值/非负成本/正额度；采购请求先调用 `UpdateProcurementConfig`，该服务在单个 PostgreSQL 事务中锁定账号和当前未结束版本，读取 `sql.NullFloat64`，安全处理 `cost_pending` NULL，结束旧版本，插入新版本或清空版本，更新 accounts 投影并写 audit log。采购-only 请求随后 `GetAccount` 并返回刷新值，不执行通用更新；混合请求才调用 `UpdateAccount`，采购字段保持 nil。页面对乱序账号输入应用 `group_rank` 升序，普通 Grid 不使用 reverse/order，DOM 顺序即桌面行优先和 390px 单列顺序。

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
| 评分 DOM | 乱序输入仍按 `group_rank` 强到弱进入最终 DOM；桌面行优先、390px 单列同序，无 reverse/order 隐式反转 |

## 测试策略

后端 service 既有 sqlmock 覆盖新录入、修改、清空 `cost_pending` NULL、重复键 replay 和事务错误；handler 新增采购-only 成功不调用通用更新且返回刷新采购值、台账失败不调用通用更新。前端 API/AccountMonitorView 测试覆盖显式键透传、同会话同 payload 重试、关闭重开、payload 改变、保存/清空切换、PUT+reload 成功、reload 失败和服务错误；页面级测试以乱序输入断言最终 DOM 排序与无 reverse/order 类。运行受影响 Go/Vitest focused tests、typecheck、build、gofmt 和 diff-check。

## 发布、回滚与未决项

无迁移、无配置变化、预期 `downtime_required=false`；仅在根总控合并后的 main 上发布。回滚为恢复上一已验证镜像/提交，不修改生产数据。未决项：通用字段与采购字段同 PUT 的跨服务事务仍保持既有边界，若未来要求全量原子化另立任务。

## 批准记录

本规格依据 T28 用户任务包、原生约束与 2026-08-19 代审授权形成；进入计划和实现前由发布总控代审确认范围未扩大。
