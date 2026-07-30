# Sub2API 原生 P0/P1 飞书桥接设计

## 背景与目标

Sub2API 原生 `OpsAlertEvaluatorService` 会生成 `ops_alert_events`，但原生发送链路只支持邮件。relay-ops 当前的飞书告警只覆盖自身计算的“用户影响”事故，不消费原生告警事件，因此后台出现 P0/P1 时不一定有对应飞书消息。

本设计新增一条独立桥接链路：

1. relay-ops 每分钟从 Sub2API Admin API 增量读取原生告警事件。
2. 只处理 P0/P1，P2/P3 保持不发送飞书。
3. 同一规则和同一组维度合并为稳定告警实例。
4. 支持首次触发、恢复、人工关闭、静默和持续提醒。
5. 复用 relay-ops 现有飞书 App/Webhook 发送、去重、失败重试和投递审计能力。
6. 保留现有“分组用户影响”告警，两条链路使用不同 family、source kind 和去重键。

不修改或重新构建官方 Sub2API 镜像。

## 已批准的生命周期语义

### 实例身份

实例键为：

```text
native-ops-alert:<rule_id>:<canonical-dimensions-sha256>
```

维度 JSON 先递归排序键名并规范化数值，再计算 SHA-256。事件 ID 不参与实例键，因此同一规则、同一维度恢复后再次触发，会在同一个 incident 上形成新的 occurrence，而不会生成无限增长的新 incident。

每个 Sub2API `ops_alert_events.id` 仍单独写入同步账本，用于跟踪源事件状态和防止重复处理。

### 首次触发

- 新发现的 `status=firing` 且 `severity=P0|P1` 事件，如果当前未命中静默，立即发送一次触发卡片。
- 原生 evaluator 已完成持续时间和冷却判断，因此桥接层不增加第二次确认窗口。
- 同一分钟内重复读取同一源事件，只更新 `last_seen_at`，不重复发送。
- P1 升级为 P0 时立即发送一次升级通知。
- 单纯指标数值变化不提前发消息，避免每分钟刷屏；最新值会写回 incident，供下一次持续提醒展示。

### 持续提醒

- P0：首次投递后第 5 分钟、第 15 分钟各提醒一次，之后每 30 分钟提醒一次，直到结束或静默。
- P1：首次投递后第 15 分钟提醒一次，之后每 60 分钟提醒一次，直到结束或静默。
- 持续提醒复用现有 incident escalation worker、notification delivery ledger 和失败重试。
- 持续提醒卡片明确标注“仍在告警”、已持续时间、当前值、阈值和原生事件 ID。

### 自动恢复

- 源事件变为 `resolved` 时，如果这一 occurrence 曾成功投递过触发或持续提醒，则发送一次恢复卡片。
- 恢复会取消待执行的持续提醒和未发送的旧触发重试。
- 未曾成功投递触发的事件不单独发送恢复，避免出现只有恢复、没有上下文的消息。

### 人工关闭

- 源事件变为 `manual_resolved` 时，发送一次“人工关闭”卡片，前提是这一 occurrence 曾成功投递过告警。
- 人工关闭立即取消持续提醒和旧消息重试。
- “人工关闭”不伪装成指标恢复；同一规则以后重新生成 firing 事件时，作为新 occurrence 再次触发。

### 静默

- 原生静默期间 evaluator 不会创建新事件，因此桥接层不会凭空生成告警。
- 对已经 firing 后才创建的静默，桥接层每分钟用受限只读数据库连接查询 `ops_alert_silences`，精确匹配：
  - `rule_id`
  - `platform`
  - `group_id`（含 NULL 精确语义）
  - `region`（含 NULL 精确语义）
  - `until > now`
- 命中静默后立即取消该 occurrence 的持续提醒和待发送告警重试，不发送“进入静默”消息。
- 静默期间源事件真正变为 `resolved` 时，如果此前曾成功投递告警，仍发送一次恢复卡片。
- 静默到期后源事件仍为 `firing`，发送一次“静默已到期，告警仍在发生”卡片，并从该次成功投递时间重新开始对应级别的持续提醒周期。
- 静默查询失败时采用 fail-closed：本轮不发送触发或持续提醒，任务记失败并在下一分钟重试；自动恢复通知仍可发送，因为恢复不会造成静默期告警刷屏。

## 数据获取与增量同步

### Admin API

在 `internal/sub2api` 增加严格 DTO 和方法：

```go
type OpsAlertReader interface {
    ListOpsAlertEvents(context.Context, OpsAlertEventCursor) ([]OpsAlertEvent, error)
    GetOpsAlertEvent(context.Context, int64) (OpsAlertEvent, error)
}
```

`ListOpsAlertEvents` 调用：

```text
GET /api/v1/admin/ops/alert-events
```

使用 `limit=500` 和 `before_fired_at + before_id` 进行倒序分页。每分钟从最新页向旧页遍历，直到遇到持久化高水位。获取到的页面反转为时间正序处理，保证触发与后续状态更新顺序稳定。

`GetOpsAlertEvent` 调用：

```text
GET /api/v1/admin/ops/alert-events/:id
```

每轮刷新所有同步账本中仍为 `firing` 的 P0/P1 事件。这样即使源事件很早以前触发，后续 `resolved` 或 `manual_resolved` 状态变化也不会因列表倒序分页而漏掉。

所有响应遵循现有 2 MiB 上限、超时、Admin Key 文件和 schema mismatch 处理方式。事件 ID、规则 ID、级别、状态和时间字段必须通过严格验证；异常维度不会直接写入飞书。

### 启动基线

首次启用桥接时：

- 导入当前仍为 `firing` 的 P0/P1 事件并立即按当前静默状态处理。
- 将最新源事件位置设为发现高水位。
- 不回放历史上已经 resolved/manual_resolved 的事件，避免部署时发送历史告警风暴。

后续重启使用持久化高水位和源事件账本继续，不依赖内存游标。

### 短生命周期事件

如果一个新事件在两次轮询之间已经从 firing 变为 resolved/manual_resolved，桥接层仍必须覆盖它，但不会连续补发“触发 + 恢复”两条消息：

- `resolved`：发送一张“已恢复｜轮询间短时告警”合并卡片，同时展示触发时间、恢复时间、峰值和阈值。
- `manual_resolved`：发送一张“人工关闭｜轮询间短时告警”合并卡片。
- 合并卡片使用源事件 ID 作为 dedup identity，只发送一次。

这样可以保证每个新产生的 P0/P1 原生事件都进入飞书，同时避免一分钟内已经结束的事件连发两张卡片。

## 只读静默数据库连接

新增独立 secret 配置：

```text
RELAY_OPS_SUB2API_ALERT_READ_DATABASE_URL_FILE
```

它必须是绝对路径、普通文件，权限为 `0600` 或 `0640`。当通知策略启用原生告警桥接时，该配置必填；未启用时可省略。

生产数据库账号只授予：

```sql
GRANT CONNECT ON DATABASE <sub2api_database> TO relay_ops_alert_reader;
GRANT USAGE ON SCHEMA public TO relay_ops_alert_reader;
GRANT SELECT ON TABLE public.ops_alert_silences TO relay_ops_alert_reader;
```

不授予其他表 SELECT，不授予 INSERT、UPDATE、DELETE、DDL 或 schema create 权限。部署验收必须验证允许查询静默表，同时拒绝查询 `ops_alert_events` 和用户、账号等业务表。

relay-ops 为此 secret 建立独立连接池，最大连接数为 2，设置短连接和查询超时。源事件仍只从 Admin API 获取。

## relay-ops 持久化模型

新增 migration：

```sql
CREATE TABLE relay_ops.native_ops_alert_sync_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    before_fired_at TIMESTAMPTZ,
    before_id BIGINT,
    initialized_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE relay_ops.native_ops_alert_events (
    source_event_id BIGINT PRIMARY KEY,
    rule_id BIGINT NOT NULL,
    incident_key TEXT NOT NULL,
    severity TEXT NOT NULL,
    source_status TEXT NOT NULL,
    fired_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    silenced BOOLEAN NOT NULL DEFAULT FALSE,
    dimensions_hash TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
```

约束和索引保证：

- `source_event_id` 幂等。
- 可快速列出 `source_status='firing'` 的事件。
- 高水位更新与事件 upsert 在同一事务完成，进程崩溃不会造成永久漏读。

原生告警生命周期仍写入现有 `relay_ops.incidents`、`notification_deliveries` 和 `notification_decisions`。新表只负责源同步，不复制飞书投递状态。

## 通知策略与审计

通知策略新增：

```json
{
  "feishu_notifications": {
    "native_ops_alerts_enabled": true
  }
}
```

对应 family 为 `native_ops_alert`，source kind 为 `sub2api_ops_alert_event`。

每次轮询至少记录以下决策之一：

- `delivered_or_reserved / new_firing_event`
- `evidence_stored / duplicate_firing_event`
- `suppressed / policy_disabled`
- `suppressed / active_native_silence`
- `suppressed / unsupported_severity`
- `delivered_or_reserved / terminal_event_summary`
- `recovered / source_resolved`
- `closed / source_manual_resolved`
- `failed / silence_lookup_unavailable`

决策详情只包含事件 ID、规则 ID、级别、状态和允许展示的维度，不包含 Admin Key、数据库 URL、Webhook、App Secret 或原始响应体。

## 飞书卡片

新增专用渲染器，不与“分组用户影响”标题混用。

触发标题示例：

```text
P0｜Sub2API 原生告警｜错误率过高
```

恢复标题示例：

```text
已恢复｜Sub2API 原生告警｜错误率过高
```

人工关闭标题示例：

```text
人工关闭｜Sub2API 原生告警｜错误率过高
```

卡片最多展示：

- 原生规则/事件 ID
- 级别与状态
- 当前值、阈值
- 触发时间、恢复时间、持续时间
- 允许的维度：`platform`、`group_id`、`region`、`model`、`account_id`
- 运维后台链接

未知维度只参与哈希，不进入卡片。文本长度和卡片大小继续遵循现有 Feishu message 上限。

## 组件边界

### `internal/sub2api`

只负责安全、严格地读取原生告警 API，不包含生命周期逻辑。

### `internal/nativeopsalerts`

新包负责：

- 增量发现与活动事件刷新
- 级别过滤
- 维度规范化和实例键生成
- 静默查询
- 将源状态转换为 incident observation
- 选择触发、静默到期、恢复或人工关闭卡片
- 写决策审计

它不直接访问飞书 HTTP API。

### `internal/nativeopssilence`

只负责受限数据库连接和静默匹配查询。对外暴露窄接口：

```go
type Reader interface {
    IsSilenced(context.Context, Scope, time.Time) (bool, error)
}
```

### `internal/store`

负责同步游标、源事件账本和现有 incident/notification 的事务一致性。

### `internal/scheduler`

增加每分钟 `native-ops-alert-sync` 作业。`RELAY_OPS_MODE=closed` 时与其他作业一致，不执行。

## 失败处理

- Admin API 失败：不推进高水位，不修改 incident 状态，下一分钟重试。
- 事件详情刷新失败：该事件保持 firing，不发送恢复，下一分钟重试。
- 静默查询失败：触发和持续提醒 fail-closed；不推进该事件的通知生命周期。
- 飞书发送失败：沿用现有最多 5 次的持久化退避重试。
- 重启或多副本：scheduler claim、防重复源事件主键、incident occurrence 和 delivery dedup key 共同保证幂等。
- 卡片渲染失败：记录失败，不写成功投递，不推进提醒周期。

## 测试与验收

### 单元测试

- Admin API 列表分页、游标、单事件读取、非法 schema 和超大响应。
- canonical dimensions 在键顺序和 JSON number 表示不同的情况下产生相同哈希。
- P0/P1 进入，P2/P3 被抑制。
- 同一事件重复轮询只通知一次。
- 同一规则和维度恢复后再次触发产生新 occurrence。
- P1 升级 P0 立即通知。
- `resolved` 发送一次恢复，`manual_resolved` 发送一次人工关闭。
- 已投递事件进入静默后停止提醒；静默到期仍 firing 时恢复一次通知。
- 静默查询失败时触发和提醒 fail-closed。
- P0 为 5、15、之后每 30 分钟；P1 为 15、之后每 60 分钟。
- 未发送过告警的短生命周期事件只发送一张合并生命周期卡片，不补发两张卡片。
- 卡片不展示未知维度或任何 secret-like 字段。

### 集成测试

- migration 可重复执行。
- 高水位与事件 upsert 的事务回滚不会漏事件。
- 多 scheduler 实例只有一个获得每分钟 claim。
- 触发、重试、恢复会使用不同且稳定的 dedup key。
- 静默后取消旧重试和 escalation，恢复通知仍可投递。

### 生产验收

1. 用只读账号验证只能查询 `ops_alert_silences`。
2. 以 disabled 策略部署，确认只记录抑制决策，不发飞书。
3. 启用策略后创建受控 P1 规则，验证首次触发、15 分钟提醒和恢复。
4. 创建受控 P0 规则，验证 5 分钟、15 分钟提醒。
5. 对 firing 事件创建静默，确认提醒停止；静默到期仍异常时只恢复一条消息。
6. 手动解决事件，确认只发送一次人工关闭并停止提醒。
7. 核对 notification ledger、decision audit 和飞书 message ID。
8. 删除受控规则和静默，保留审计记录。

生产验收不得打印或回显任何 Admin Key、数据库 URL、飞书凭据或 secret 文件内容。

## 非目标

- 不把 P2/P3 转发到飞书。
- 不修改 Sub2API evaluator、规则定义或后台 UI。
- 不使用只读数据库连接读取 `ops_alert_events`。
- 不替换现有分组用户影响告警。
- 不把邮箱状态 `email_sent` 解释为飞书状态。
