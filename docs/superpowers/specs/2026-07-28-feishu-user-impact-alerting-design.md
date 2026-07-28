# 飞书用户影响告警设计

## 问题

生产飞书投递记录显示消息均被飞书接受，但现有系统只发送普通群卡片：

- 没有明确的负责人 `@`，也没有应用内加急；
- `200 delivered` 只能证明飞书接受消息，不能证明负责人看见或确认；
- 账号、分组、错误率会针对同一根因分别发卡，形成告警风暴；
- 同一分组在一小时内再次故障时，小时桶去重可能吞掉新的事件轮次；
- 已确认事件没有升级时钟，未处理事件不会再次提醒；
- 投递表没有保存飞书 `message_id`、加急结果或确认状态。

2026-07-28 的生产样本中，03:03 同时发送 9 张错误率卡片，03:18 又发送
9 张恢复卡片；`GPT特惠` 分组在数小时内多次故障/恢复。账号 36 的不可调度
与恢复卡片都返回 200，但没有强提醒或人工确认。

## 目标

将飞书告警改为按用户影响分级、可确认、可升级、可审计的单一告警链路：

- 用户完全不可用时立即发送 P0；
- 用户体验严重下降或容量失去冗余时发送 P1；
- 仅运营关注的变化发送 P2；
- P0/P1 明确 `@` 负责人，P0 调用飞书应用内加急；
- P0/P1 卡片提供“确认并接手”按钮；
- 未确认且未恢复的事件按固定时钟升级；
- 同一根因只保留必要的主告警；
- 每次故障轮次具有独立幂等身份，恢复和再次故障都不会被吞；
- 保存消息、加急和确认审计证据。

## 非目标

- 不自动暂停、启用、切换或充值任何 Sub2API 账号；
- 不因监控读取失败伪造业务故障；
- 不将 Base URL、凭据、上游原始响应或飞书 Open ID 展示在卡片中；
- 不改变真实请求调度、计费或用户权限；
- 不引入第二个告警群或外部值班平台。

## 严重度

### P0

满足任一条件：

- 公开分组 `Total > 0 && Available == 0`；
- 公开分组最近窗口存在请求且 `SuccessCount == 0`。

P0 使用一个窗口立即确认，卡片红色，标题以 `P0｜` 开头，`@` 所有配置的
负责人，并对这些负责人调用飞书应用内加急。

### P1

满足任一条件：

- 公开分组触发分层容量阈值，但仍至少有一个可用账号；
- active 账号变为不可调度；
- 明确探测到 `balance_exhausted`；
- 公开分组错误率连续两个窗口不低于 5%；
- 公开分组 TTFT P95 超过 3000ms，且不低于 24 小时基线的 1.3 倍，
  连续两个窗口成立。

P1 使用红色卡片，标题以 `P1｜` 开头并 `@` 配置的负责人。部分容量下降
继续使用两个 5 分钟窗口确认；明确的不可调度或余额耗尽使用一个窗口。

### P2

倍率变化、日报和不直接影响用户可用性的运营事件保持 P2。P2 不 `@`、
不加急、不升级。

恢复消息使用绿色卡片，不 `@`、不加急；只在对应事件轮次确实通知过时发送。

## 降噪与关联

- 站内运行的 availability、error rate 和 TTFT 只对公开分组发送，不再为每个
  账号复制相同的运行窗口告警。
- 单账号只保留不可调度、余额耗尽和倍率变化这类账号自身证据。
- 分组容量卡片列出不可用账号及原因，不再依赖额外账号错误率卡片表达影响。
- 分组可用性证据只由分组名和当前 `available/total` 构成，不包含小时桶。
  相同状态不重复发送，状态变化才成为新证据。
- P0 比 P1 优先；同一分组已经处于 P0 时，不再发送同一窗口的 P1 可用性卡。
- 每个 incident 记录维护 `occurrence_no`。健康后再次故障时递增轮次，
  投递幂等键由 `incident_key + occurrence_no + transition + evidence` 构成。

## 强提醒与升级

负责人 Open ID 从只读 secret JSON 文件加载，日志、数据库页面和消息正文
均不展示原始值。

| 严重度 | 首次通知 | 未确认升级 |
|---|---|---|
| P0 | `@负责人` + 应用内加急 | 5 分钟、15 分钟各重发一次并再次加急 |
| P1 | `@负责人` | 15 分钟重发一次 |
| P2 | 普通卡片 | 不升级 |

升级只在当前轮次仍为 confirmed/escalated、尚未确认且尚未恢复时发生。达到最后
一级后不无限重发。恢复后立即取消剩余升级。

飞书消息发送成功但加急失败时，消息本身记为 delivered，加急结果单独记为
failed。事件仍进入未确认升级流程，下一次 P0 升级会重新尝试加急，避免为重试
加急而重复发送首次卡片。

## 确认流程

P0/P1 卡片包含“确认并接手”URL 按钮，URL 指向现有 `/ops` 页面并携带稳定的
incident key 和 occurrence number，不包含密钥。

运维页使用现有 `localStorage.auth_token` 调用：

```text
POST /relay-ops/api/incidents/ack
Authorization: Bearer <admin session>
Origin: <production base origin>
Content-Type: application/json

{"incident_key":"group:GPT-PLUS:availability","occurrence_no":3}
```

只有 active admin session、同源请求、当前故障轮次匹配且事件尚未恢复时可以
确认。成功返回 204；过期轮次或已恢复事件返回 409；身份或来源不合法沿用现有
隐藏管理接口策略。确认写入管理员 user ID、确认时间和当前轮次，并停止升级。

运维页显示确认成功或失败结果，不在 URL 中放管理员身份或 bearer token。

## 数据模型

`relay_ops.incidents` 增加：

- `occurrence_no BIGINT NOT NULL DEFAULT 1`
- `acknowledged_occurrence BIGINT`
- `acknowledged_at TIMESTAMPTZ`
- `acknowledged_by BIGINT`
- `escalation_level INTEGER NOT NULL DEFAULT 0`
- `next_escalation_at TIMESTAMPTZ`

`relay_ops.notification_deliveries` 增加：

- `occurrence_no BIGINT NOT NULL DEFAULT 1`
- `transition TEXT NOT NULL DEFAULT 'confirmed'`
- `message_payload JSONB`
- `message_id TEXT`
- `urgent_status TEXT`
- `urgent_response_code INTEGER`

历史行默认属于轮次 1。迁移只新增 nullable/default 列，不删除或重写历史记录。

## 组件

- `incidents.Machine`：维护故障轮次并在 transition 中暴露 occurrence number。
- `notify.DeliverySender`：使用轮次幂等键，保存实际卡片、message ID 和加急结果。
- `notify.AppClient`：注入负责人 mention，并在 P0 消息后调用应用内加急。
- `alerting.Escalator`：查询到期且未确认的 P0/P1，按策略重发保存的安全卡片。
- `httpserver`：提供管理员确认接口，运维页执行确认并展示结果。
- `opsmonitor`：仅按公开分组发送运行窗口告警，保留账号自身状态告警。
- `group_availability`：动态选择 P0/P1、稳定证据并使用事件轮次投递。

## 配置

新增：

```text
RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE=/run/secrets/feishu-alert-recipients.json
```

文件格式：

```json
{"open_ids":["ou_example"]}
```

必须是 1 至 20 个非空、去重后的 Open ID。使用飞书 App 告警群时生产配置必须
提供该文件；Webhook 兼容路径允许没有负责人，但不能声称已应用强提醒。

## 错误处理

- 负责人 secret 不合法时，服务启动失败，避免静默降级为普通消息；
- 普通消息发送失败保持 failed 并允许既有幂等重试；
- 消息已发送但加急失败记录为 delivered + urgent failed；
- 确认接口不得确认旧轮次或 recovered 事件；
- 升级投递失败由 scheduler job 记录失败，下个调度周期重试同一级；
- 监控读取失败继续 fail-safe，不创建 P0/P1；
- 卡片和持久化 payload 继续执行现有脱敏与 30KiB 上限。

## 测试

- 状态机：首次故障轮次 1；恢复后再次故障轮次 2；同一轮次不递增；
- 投递：不同轮次的相同证据都能发送，同轮次重复仍被去重；
- 分组告警：0 个可用为 P0 并立即通知，部分容量不足为 P1 两窗口确认；
- 降噪：站内运行不再生成逐账号 error rate/TTFT/availability 卡；
- 飞书：P0/P1 mention，P0 urgent，P2/恢复不 mention、不 urgent；
- 部分失败：消息成功、加急失败时保存 message ID 和 urgent failed；
- 确认：管理员同源成功，旧轮次、恢复事件、非管理员和跨源失败；
- 升级：P0 在 5/15 分钟、P1 在 15 分钟触发；确认或恢复后不触发；
- 迁移：空库和现有 v2 schema 均可重复执行；
- 集成：真实 PostgreSQL 测试覆盖 reserve、finish、ack、claim escalation；
- 生产：发送一张明确标注“告警链路验收”的 P0 卡，验证 mention、应用内加急、
  确认按钮、数据库 message ID/urgent/ack 证据及不再升级。

## 验收标准

- [ ] P0/P1 能在卡片首屏说明受影响分组、剩余容量、原因和操作建议；
- [ ] P0/P1 会 `@` 配置负责人，P0 应用内加急成功；
- [ ] 管理员可从卡片确认当前事件轮次，确认后不再升级；
- [ ] 未确认 P0/P1 严格按策略升级，恢复后停止；
- [ ] 同一分组一小时内再次故障不会被旧 dedup key 吞掉；
- [ ] 一次站内错误窗口不会再产生逐账号告警风暴；
- [ ] 投递表能区分消息投递、加急和确认结果；
- [ ] 全量 Go 测试、Ruby 运维测试、构建和迁移验证通过；
- [ ] 生产只重建 relay-ops，Sub2API、PostgreSQL、Redis 和 Caddy 不重启；
- [ ] 生产验收后没有遗留测试 incident 或后续升级任务。

## 回滚

- 保留部署前 relay-ops 镜像、Compose、环境文件和数据库 schema-only 备份；
- 回滚只切回旧 relay-ops 镜像并移除新增 mount/env；
- 新增列向后兼容，旧镜像会忽略，不需要破坏性 schema 回滚；
- 测试事件在回滚前标记 recovered/acknowledged，避免旧任务继续发送。
