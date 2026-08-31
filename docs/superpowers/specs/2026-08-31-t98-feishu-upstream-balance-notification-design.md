# T98 飞书上游余额通知重构设计

> 状态：用户已于 2026-08-31 批准进入实现。该批准不构成主站部署、生产清库、生产配置修改或真实飞书发送授权。

## 1. Loop Brief

### Goal

移除旧飞书通知的内容、判定规则、历史记录和业务发送链，仅保留飞书 App 传输、群与接收人凭据、卡片结构和等级主题。首个新事件只消费 Sub2API 已有账号余额、账号元数据和原生分组排名，在 BaseURL 维度发送上游余额不足通知。

### Context

- 新通知服务全团队，发送到现有飞书群。
- 一个规范化 BaseURL 对应一个上游钱包余额，同一 BaseURL 下可以有多个现行账号读取该余额。
- 用户明确授权“上游登录密码”以明文进入群卡片和飞书消息历史。
- 用户明确授权旧通知生产历史不备份、全部删除。
- `relay-ops` 继续承载账务、外置化、成本、候选等非通知能力，但不再作为新通知运行时主路径。

### Constraints

- 不新增余额探测、HTTP 请求、监控周期、监控事实源或业务数据写入。
- 只处理已有 `account_monitor_balance`、账号、分组和排名数据。
- 新增的数据仅限通知活动状态、发送租约和非敏感投递结果，不得成为新的余额事实源。
- 不改变账号余额刷新、调度、计费、用量、分组或真实业务请求行为。
- API Key、飞书密钥及其他敏感信息继续脱敏。只有专用余额卡片中的“上游登录账号”和“上游登录密码”允许按用户授权原样展示；前者是受控登录标识，后者是明文秘密。
- 真实上游登录账号和密码不得进入数据库、普通日志、错误、trace、测试夹具、发布证据或 API 响应；测试只能使用显然虚构的值。
- 新通知不得依赖 `relay-ops` 运行时、旧通知表或旧重试 payload。

### Plan

1. 把飞书 App 客户端、卡片模型、红橙主题、`@` 接收人和 30 KiB 校验移植为 Sub2API 原生通知适配。
2. 在既有余额刷新成功后，仅基于数据库中的当前快照组合 BaseURL 事件。
3. 扩展 Sub2API 原生 `ops_alert_events`，用 `rule_id + normalized_base_url` 唯一标识活动事件，并保存不含密码的投递状态。
4. 先停掉全部旧通知 writer/retry/scheduler，再删除旧历史、表、规则和业务 wiring，最后启用新规则。
5. 用本地 fake transport 和验收站禁发送模式完成验证；只有满足主站明确发布授权后才允许生产启用。

### Implement

实现范围见第 9 节。正式实施计划必须在本规格批准后、从届时最新干净 `main` 创建独立 T98 worktree 时编写。

### Validate

验证必须覆盖数据资格、BaseURL 聚合、状态转换、并发 claim、失败重试、卡片限制、敏感数据边界、旧路径消失和无新增余额请求。详见第 11 节。

### Review

实现完成后由根线程核对：代码只消费既有数据；旧通知 writer 已全部移除；事件账本不含卡片或密码；生产切换不会让新旧 sender 并行。

### Done When

- 旧飞书通知业务代码、策略、任务和获授权历史均已删除。
- 新规则按本规格运行，直接相关测试通过。
- 候选合入、推送、部署并在线验证；主站成功后同 commit 同步验收站。
- 生产事件仅以 BaseURL 聚合，P1/P2 节奏、卡片字段和明文密码边界全部符合本规格。

## 2. 事实源与资格

### 2.1 账号范围

只有同时满足以下条件的账号参与聚合：

- `deleted_at IS NULL`
- `status = active`
- `platform = openai`
- `type = api_key`
- BaseURL 可以通过现有 `validateUpstreamBaseURL` 规范化

不额外要求 `schedulable=true`，也不因临时限流排除账号。用户要求的边界是“活跃账号”，不能偷偷改成管理员页的可调度子集。

同一规范化 BaseURL 下的全部合格账号都进入卡片，即使只有其中一个账号拥有最新有效余额快照。

### 2.2 有效余额快照

一个 `account_monitor_balance` 只有同时满足以下条件才有效：

- JSON 解码成功
- `version = 1`
- `status = ok`
- `source` 为 `sub2api` 或 `newapi`
- `value_usd` 非空、有限且 `>= 0`
- `observed_at` 非空且可解析

失败刷新会保留旧 `value_usd` 和 `observed_at`，但把 `status` 改为 `failed`；因此禁止只看数值和时间后继续使用失败快照。

Sub2API 原生 `/v1/usage` 直接提供 USD 余额；NewAPI 使用 `total_available / quota_per_unit` 归一为 USD。新通知不做人民币换算，卡片统一显示 USD。

### 2.3 BaseURL 唯一当前余额

对同一规范化 BaseURL 下全部合格账号的有效快照按 `observed_at DESC` 排序，最新快照的 `value_usd` 是唯一当前余额。

- 若没有有效快照：不发送，不改变活动事件状态。
- 若最新时间完全相同且数值冲突：视为当前值不唯一，不发送，不改变状态，并只记录非敏感错误码。
- 若 BaseURL 下没有活跃账号：跳过评估，不改变已有通知状态。
- BaseURL 为空或规范化失败的账号：不参与事件。

## 3. 排名语义

“当前账号在分组内的排名”固定解释为管理员账号监控页当前使用的 `scheduler_rank`，即原生当前调度排名，不使用 `quality_rank` / `group_rank`，也不自行重算。

- 一个账号属于多个分组时，逐项显示“分组名称：第 N 名”。
- 原生投影不可用或该分组无 `scheduler_rank` 时显示“未排名”。
- 账号排序先按最佳有效分组排名升序，再按账号 ID 升序。
- 排名不可用不阻断通知。

## 4. BaseURL 登录登记簿

### 4.1 已生成工作簿

受保护工作簿：

`outputs/feishu-balance-account-map-20260830/sub2api-account-login-map-20260830.xlsx`

验证结果：

- 表头：账号名称、账号ID、baseURL、账号、密码
- 主站现行账号 72 行，账号 ID 72 个且唯一
- 71 行有 BaseURL 和登录凭据，1 行 BaseURL 为空且账号/密码留空
- 35 个唯一非空 BaseURL
- 相同 BaseURL 的登录账号/密码冲突数为 0
- 35 个非空 BaseURL 均有完整登录账号和密码
- 目录权限 `0700`，工作簿权限 `0600`

### 4.2 运行时 JSON

运行时不得解析 Excel。实施时把工作簿转换为规范化 BaseURL 键控的 JSON：

```json
{
  "version": 1,
  "entries": {
    "https://example.invalid": {
      "login_account": "example",
      "login_password": "example"
    }
  }
}
```

上例仅描述结构，不能作为真实配置或测试凭据提交。

安全契约：

- host 文件为常规文件、非 symlink、精确 `0600`，父目录 `0700`
- 只读挂载到 Sub2API worker，建议容器路径 `/run/secrets/upstream-login-registry.json`
- 最大 1 MiB，JSON 使用 `DisallowUnknownFields` 并检查 EOF
- 重新规范化每个键；空键或同键冲突使整份文件无效
- 原始字节解析后清空；值只保留在进程内存
- 不提交 Git，不写数据库、API、普通日志或事件 dimensions

整份登记簿缺失、权限不安全、解析失败或存在冲突时，通知子系统 fail-closed，但 Sub2API 核心业务继续运行。单个 BaseURL 没有登记项，或登记项字段为空时，不阻断余额通知，卡片对应字段显示“未登记”。

## 5. 通知规则

### 5.1 状态

| 当前 USD 余额 | 事件状态 | 等级 | 标题 | 主题 | @ 接收人 | 重复节奏 |
| --- | --- | --- | --- | --- | --- | --- |
| 无有效值 | unknown | 无 | 不发送 | 无 | 否 | 不改变状态 |
| `value_usd >= 5` | healthy | 无 | 不发送 | 无 | 否 | 清除活动状态，不发恢复消息 |
| `0 < value_usd < 5` | low | P2 | 上游账号余额不足 | orange | 否 | 同一 BaseURL 每 30 分钟最多一次 |
| `value_usd = 0` | zero | P1 | 上游账号余额为 0 | red | 是 | 同一 BaseURL 每 5 分钟最多一次 |

P1 只在卡片顶部 `@` 现有接收人，不调用 P0 `urgent_app` 加急接口。

### 5.2 状态转换

- `healthy -> low`：立即发送 P2。
- `healthy -> zero`：立即发送 P1。
- `low -> zero`：不等待 30 分钟，立即发送 P1。
- `zero -> low`：不等待 5 分钟，立即发送 P2。
- `low|zero -> healthy`：活动事件标记 resolved，不发送恢复卡片。
- `low -> low`：距上次确认发送成功满 30 分钟后才可再次发送。
- `zero -> zero`：距上次确认发送成功满 5 分钟后才可再次发送。
- 无活跃账号、无有效快照或数据读取失败：不发送，也不改变已有状态。

节奏从飞书确认发送成功的时间计算，不从首次判定时间或失败尝试时间计算。

## 6. 卡片契约

卡片复用现有宽屏结构、P1 红色、P2 橙色、接收人逻辑和 JSON 后 30 KiB 上限。

顶部只显示一次：

1. 当前余额，前缀为 `USD`，通常保留至少 2 位、最多 6 位小数；若常规舍入会让显示值跨越 0 或 5 的分类边界，则增加必要精度以保持展示与判定一致
2. BaseURL
3. 上游登录账号
4. 上游登录密码

下方“关联活跃账号”逐项显示：

- 账号名称
- 账号 ID
- 每个所属分组的当前调度排名

卡片禁止附加 API Key、账号 credentials、余额响应原文、规则解释、监控说明或恢复按钮。当前合格账号必须全部列出，不允许静默截断。若未来单个 BaseURL 的完整卡片超过 30 KiB，则本次发送 fail-closed 并记录非敏感错误码，不能发送缺账号的半张卡片。

匿名样式对照：

`outputs/feishu-balance-notification-design-20260831/feishu-balance-card-comparison.png`

## 7. 原生事件账本与并发

### 7.1 复用边界

使用 Sub2API 主库的原生 `ops_alert_rules` / `ops_alert_events`，不得复用 relay-ops 的 `incidents`、`notification_deliveries` 或 payload 重试表。

新规则在 `ops_alert_rules` 中使用稳定系统标识 `upstream_baseurl_balance_usd_v1`，固定阈值和通知语义；通用 Ops evaluator 不把它当作现有邮件指标规则执行。

扩展 `ops_alert_events` 以支持 BaseURL 范围和投递状态。建议字段：

- `scope_type = base_url`
- `scope_key = normalized_base_url`
- `notification_state = low|zero`
- `last_observed_at`
- `last_delivered_at`
- `delivery_generation`
- `delivery_attempt_count`
- `next_attempt_at`
- `delivery_lease_token`
- `delivery_lease_until`
- `last_delivery_error_code`

数据库建立部分唯一索引：活动状态下 `(rule_id, scope_type, scope_key)` 唯一。历史已 resolved 事件不受此索引限制。上述字段只能保存非敏感状态；禁止保存卡片 JSON、登录账号、密码或 API Key。

### 7.2 Claim 流程

1. worker 在事务内读取/锁定 `rule_id + normalized_base_url` 活动行。
2. 首次低余额、级别切换、提醒到期或失败重试到期时，原子递增或确认 `delivery_generation`，写入随机 lease token 和过期时间。
3. 提交 claim 事务后重新读取当前合格账号、最新有效余额、排名和内存登记簿；先不发送。
4. 获取该 `rule_id + scope_key` 的数据库 scoped advisory lock。事件 evaluator 的状态转换也必须使用同一锁；余额刷新后的 hook 只能异步或 try-lock，锁繁忙时交给 due worker 复核，不能阻塞余额 CAS 或业务链路。
5. 在锁内做最终条件复核：`event_id + generation + lease_token` 仍有效、活动行仍 firing，并把刚读取的最新 `observed_at/value` 指纹与重新聚合结果绑定到本次发送。healthy、unknown、同时间冲突或无活跃账号时取消旧发送；low/zero 改级时取消旧 generation，并由当前状态重新 claim。
6. 只有最终复核成功才渲染并调用飞书。为关闭“复核后事件已切换但旧卡仍发送”的窗口，scoped advisory lock 持有到飞书调用和本地 outcome CAS 完成，并在所有退出路径释放；它只串行同一通知 scope，不锁账号、余额、调度、计费或业务请求表。
7. 成功后用 `event_id + generation + lease_token` CAS 写入 `last_delivered_at` 并清零失败状态。
8. 失败后用同一 CAS 增加失败计数和 `next_attempt_at`；状态已变化时旧 lease 不能覆盖新 generation。

多实例可使用行锁 / `SKIP LOCKED` claim，但数据库唯一索引是最终并发约束。任何进程内 mutex 都不能替代数据库约束。

### 7.3 发送失败与重试

- 飞书客户端保留现有 10 秒超时和 token/业务码失败后刷新 token再试一次。
- 应用级重试不持久化卡片；每次都从当前数据和受保护登记簿重新渲染。
- 连续失败延迟为 1、2、5、10 分钟。
- 第 5 次失败后不丢弃活动事件：zero 按 5 分钟、low 按 30 分钟继续尝试，直到成功、恢复或状态变化。
- 重试前若当前余额已健康，则 resolve 且不发送；若状态切换，则改发新等级；若数据不可用，则不发送、不推进成功节奏。

语义是 at-least-once。若飞书已接收但网络响应丢失，或飞书成功后本地 CAS 失败，最多可能产生重复消息；飞书接口没有可依赖的业务幂等键，规格不得承诺 exactly-once。

## 8. Fail-Closed 与业务隔离

以下情况不发送，并且不推进 `last_delivered_at`：

- 飞书 App ID/Secret/群 ID/接收人文件不可读或格式非法
- 整份登记簿不可读、不安全、格式非法或键冲突
- 余额、账号或事件账本读取失败
- 无有效余额快照
- 完整卡片超过 30 KiB

通知组件初始化失败不能阻止 API、网关、调度、余额刷新、计费或用户请求启动；只把通知子系统标记为不可用，并记录稳定、非敏感错误码和计数。不得把文件内容、BaseURL 登录信息、飞书响应正文或卡片正文写进错误。

单个 BaseURL 未登记或字段为空是可发送状态，字段显示“未登记”。排名缺失也是可发送状态，显示“未排名”。

## 9. 实现范围

### 9.1 Sub2API 原生新增/修改

- 飞书 App OpenAPI 客户端和专用余额卡片 renderer
- 飞书 App/群/接收人/登记簿安全 loader
- BaseURL 余额聚合与严格有效快照解析
- 原生活动事件 scope、claim、重试和状态转换
- 既有余额刷新成功后的评估 hook
- 只扫描通知活动事件的 due-delivery worker；它不得刷新或探测余额
- 配置、健康状态和非敏感指标
- 数据库迁移与直接相关测试

新 sender 的二进制运行在 Sub2API worker/后端进程中，不经过 relay-ops HTTP、数据库 bridge 或 scheduler。

### 9.2 relay-ops 删除

停止并移除所有旧通知 writer、retry、escalation 和 scheduler wiring，包括：

- 旧 notification policy 及 group impact、daily report、pricing event、ops monitor、native alert、alerting 开关
- incident lifecycle、delivery/retry/one-shot 业务路径
- group-impact 与旧日报/价格通知发送路径
- 旧 native ops alert bridge 配置和 reader/store wiring
- 旧 migration embed/拼接清单、旧通知 Store methods，以及启动时 `SupersedeLegacyNotificationIncidents` 等旧表读写副作用

删除获授权的旧通知历史和表，顺序必须满足外键：

1. `relay_ops.agent_analyses` 中旧 incident 分析
2. `relay_ops.notification_deliveries`
3. `relay_ops.incidents`
4. `relay_ops.notification_messages`
5. `relay_ops.notification_decisions`
6. `relay_ops.group_impact_signals`
7. `relay_ops.operational_baselines`
8. `relay_ops.native_ops_alert_events`
9. `relay_ops.native_ops_alert_sync_state`

删除不备份、不可恢复。应新增显式 retire migration 或受控清理脚本，不能只修改历史初始 migration。

relay-ops 当前每次应用启动和 `provision-billing-source` 执行都会重放拼接 migration。正式清理前必须先让兼容构建从 migration embed/拼接入口移除旧通知 `CREATE/ALTER`，并移除所有旧表查询、Store 方法和启动副作用；否则 DROP 后会在重启时重新建表。relay-ops 主进程和 provision 两条启动路径都必须在旧表不存在时通过测试，且不得创建或查询这些表。

### 9.3 必须保留

- 飞书 App ID/Secret、群 ID、接收人文件及 host 端受保护凭据
- 飞书 App token/send 客户端契约
- wide-screen 卡片模型、P1 红色、P2 橙色、`@` 接收人与 30 KiB 限制
- relay-ops 非通知类账务、外置化、成本、候选、质量报告等数据与能力
- Sub2API 原生余额、账号监控、用量、调度、计费和 Ops 邮件监控；后者不属于本次旧飞书清理范围
- 历史规格、计划和发布证据作为审计记录保留，但不参与运行时

## 10. 原子切换与回滚

### 10.1 切换顺序

1. 构建新代码，但新余额 sender 默认 disabled；完成凭据、登记簿、迁移和卡片只读预检，不发消息。
2. 先成功部署一个“旧通知永久 disabled、无旧 jobs/writers、无旧 migration 重放、且不依赖旧表”的 relay-ops 兼容镜像。此时旧表仍保留，因此该阶段可以按现有机制安全自动回滚。
3. 确认所有旧实例和 in-flight retry 已停止，并验证 relay-ops 主进程与 `provision-billing-source` 在旧通知表不存在的契约测试通过。
4. 把发布控制器的 `previous_image` / 自动 rollback 目标固定并验证为上述兼容禁发送镜像；执行静态检查或 dry-run，证明任何后续失败都不会启动依赖旧表或恢复旧 sender 的历史镜像。
5. 只有前四步全部通过，才在发布锁下执行获授权的旧历史删除与表清理；确认重启不会重建旧表。
6. 创建/验证原生活动事件 scope 与唯一索引。
7. 对当前生产已有数据做只读规则预览，只输出对象计数和状态计数，不输出 BaseURL 登录信息或密码。
8. 启用唯一的新 native sender；同一时刻不得存在旧、新两个 sender。
9. 启用后，当前已经 low/zero 的 BaseURL 可能立即发送第一条真实通知，这是预期行为。

### 10.2 发布门禁

- 规格批准只授权实现，不等于主站部署授权。
- 主站仍只接受“测试站验收通过，部署主站”或“快速部署到主站”两种明确授权。
- `downtime_required=false` 只表示无需停机，不构成发布授权。
- 验收站和本地测试使用 fake/disabled transport，禁止误发到全团队真实群。

### 10.3 回滚

- 新系统异常时立即关闭 native sender，核心业务保持运行。
- 可以回滚/修复新 sender 代码和配置，但不得恢复旧通知业务路径。
- 旧历史和表已按用户要求无备份删除，不能恢复。
- 因旧表不可恢复，生产回退必须使用“旧通知始终 disabled”的兼容构建，不能直接启动依赖旧表的历史 relay-ops 二进制。

## 11. 验证矩阵

### 数据与规则

- `status=failed` 即使保留旧 value/observed_at 也不参与
- Sub2API/NewAPI 均得到 USD；无人民币换算
- 最新 `observed_at` 跨账号选出唯一余额
- inactive/error/deleted/非 OpenAI/非 API Key 账号排除
- 无活跃账号、无有效快照、读取失败均不改变状态
- `0`、`0 < x < 5`、`5`、`>5` 边界准确
- low/zero 双向切换立即发送，healthy 不发恢复

### 卡片

- P2 orange、不 `@`、标题准确、30 分钟节奏
- P1 red、`@` 现有接收人、不加急、5 分钟节奏
- 余额/BaseURL/登录账号/密码只显示一次
- 全部活跃账号及全部分组当前调度排名完整、排序稳定
- 未登记/未排名状态准确
- JSON 后不超过 30 KiB；超限不发送残缺卡片

### 安全

- DB schema/rows 不含 card payload、login_account、login_password 或 API Key
- 日志、错误、trace、测试 fixture、发布证据扫描无敏感值
- secret/registry 文件权限、symlink、大小、未知字段、尾随数据和重复键均 fail-closed
- P1/P2 卡片中的真实上游登录账号和密码只存在于渲染内存与用户已授权的飞书消息历史

### 并发与失败

- 两个以上 worker 同时评估同一 BaseURL 只取得一个有效 claim
- 不同 BaseURL 可以并行
- lease 过期可回收，旧 generation 不能覆盖新状态
- claim 后、真正发送前发生 low->healthy、low->zero、zero->low 或活跃账号全部消失时，旧 lease 绝不发送
- 发送失败不推进提醒时钟；成功后按 5/30 分钟节流
- ambiguous network/DB confirm failure 的重复风险有测试和明确日志码
- 重试重新读取当前数据，不发送陈旧卡片

### 旧体系清理

- relay-ops 不再构造旧 notifier/service/retry/escalator
- scheduler 不再注册旧通知 jobs
- relay-ops app/provision 启动均不再 embed、重放、查询或重建旧通知表
- 旧策略文件和配置开关不存在
- 获授权旧表按外键顺序清理且不会被重建
- relay-ops 非通知数据与服务保持可用
- 全仓和运行配置中不存在旧飞书业务发送入口

### 无业务影响

- 新实现没有新增余额 HTTP 请求或探测
- 余额辅助刷新失败路径不能被 notifier 当作成功；hook 必须重新读取并通过严格 `status=ok` 快照校验
- 通知失败不影响余额刷新 CAS、scheduler outbox、API、计费和网关
- 直接相关 Go 测试、迁移测试、配置测试、卡片测试和发布链预检通过

## 12. 方案比较与决策

### 已选：Sub2API 原生事件账本 + 原生 sender

优点：直接消费原生数据；不复制余额事实；BaseURL 聚合与刷新链一致；旧 relay-ops 通知可完整删除。代价：需要扩展原生事件 scope 和安全移植飞书传输。

### 未选：继续使用 relay-ops 作为通知运行时

拒绝原因：需要同步/复制 Sub2API 数据，继续保留用户要求移除的旧通知运行时和表，且旧 payload retry 会把明文密码写入数据库。

### 未选：无账本、每次刷新直接发送

拒绝原因：无法满足多实例去重、5/30 分钟节奏、级别切换、失败重试和恢复清理；并发刷新会重复发送。

## 13. 用户批准点

批准本规格即同时确认：

1. “分组内当前排名”使用 `scheduler_rank`，不是质量排名。
2. 失败重试只保存非敏感事件状态，不保存整张卡片；投递语义为 at-least-once。
3. 整份登记簿或飞书凭据无效时通知子系统关闭，但核心业务继续；单个 BaseURL 未登记仍发送并显示“未登记”。
4. 旧通知 writer 先完全停止，再无备份删除获授权历史和表，之后才启用新 sender。
5. 旧历史删除后不可恢复，回滚只关闭/修复新 sender，不恢复旧通知。
