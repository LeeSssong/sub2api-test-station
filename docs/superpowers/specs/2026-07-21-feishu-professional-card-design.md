# Feishu Professional Card Design

## Goal

将 relay-ops 的告警、恢复和飞书命令回复统一为可读、可扫读、可审计的 Feishu Interactive Card，同时保持现有事件去重、重试、只读权限和 dry-run 约束不变。

## Scope

本阶段只改本地 relay-ops-service 代码和测试，不部署生产，不改变路由、上游、数据库状态、`RELAY_OPS_MODE` 或飞书命令模式。纯文本 `SendText` 继续保留给明确的非卡片用途。

## User-facing templates

### Alert card

- 红色或橙色状态头，标题包含事件名称和公开分组。
- 首屏展示状态、影响范围、当前值/基线和持续时间。
- 使用折叠前可读的 markdown 分段：已执行、观测结果、变化、需要关注。
- 只读分析和建议动作单独标识。
- 后台链接使用按钮，不把上游 Key、Cookie、提示词、响应正文、内部 URL 或完整身份 ID 放入卡片。

### Recovery card

- 绿色状态头，标题明确“已恢复”。
- 展示恢复状态、事件持续时间、恢复后的观察建议和告警重复抑制结果。
- 不重复告警卡的全部长内容，保持恢复消息紧凑。

### Command card

- 蓝色表示查询/拒绝，绿色表示成功，红色表示失败。
- 展示命令、执行者脱敏标识、公开分组、目标、结果、错误码和短审计 ID。
- 未知命令显示固定允许命令列表；不回显动态参数。
- dry-run 结果明确标注“仅预测，未写入路由”。

## Data model

`notify.FeishuMessage` 是业务消息模型，包含 `MsgType`、卡片结构和纯文本投影。卡片结构使用受控强类型字段，采用 Feishu 官方稳定示例的顶层 `elements`、`div + lark_md` 和 `action/button` 形态，序列化前校验标题、元素和总字节数不超过 Feishu 30 KB 消息限制。

`feishuapi.OutboundMessage` 是发送层模型，包含 `MsgType` 和序列化后的 `Content`。App Bot API 将卡片 JSON 放入字符串字段 `content`；Webhook 将同一 JSON 对象放入 `card` 字段。两者由同一个 `notify.FeishuMessage` 生成，避免文案漂移。

## Delivery contract

- `feishuapi.Client.SendMessage` 延续 tenant token 缓存和一次 401 刷新。
- `SendText` 保留并调用通用发送方法。
- `notify.AppClient` 使用 `SendMessage` 发送 interactive card。
- `notify.Client` webhook 发送 `msg_type=interactive` 和 `card` 对象。
- 卡片发送失败只返回错误，不额外降级发送文本，避免重复刷屏；现有持久化重试继续负责重试。
- 卡片发送前检查 chat ID、内容非空和 30 KB 上限；错误消息不泄露密钥、Token、Cookie、完整 chat/open ID 或响应正文。

## Compatibility and safety

- `RenderFeishu` 保留为告警卡模板入口，旧调用方无需改变业务语义。
- `SendIncident` 的 dedup key、message hash、Reserve/Finish 状态机不变。
- 命令权限、事件解析、路由锁和 dry-run 行为不变。
- 所有测试必须证明 card payload 合法、三类模板语义不同、App/Webhook wire format 正确、失败不发送第二条文本。

## Acceptance criteria

1. 告警、恢复、命令成功/失败/拒绝/未知命令均生成 interactive card。
2. App Bot 请求为 `msg_type=interactive`，`content` 是合法 JSON 字符串且小于 30 KB。
3. Webhook 请求为 `msg_type=interactive`，`card` 是与 App Bot 相同的 JSON 对象。
4. 卡片不包含上游凭据、提示词、响应正文或完整身份标识。
5. 现有 notify、commands、acceptance、nativealerts、pricing 和全量 Go 测试通过。
