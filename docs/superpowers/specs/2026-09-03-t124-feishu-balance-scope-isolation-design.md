# T124 飞书余额通知按范围隔离与健康信号规格

**状态：** 修复方案已获用户确认，书面规格等待用户审阅批准  
**日期：** 2026-09-03  
**范围：** 上游余额刷新、评估、claim、飞书投递和运维健康信号

## 1. 问题证据与根因

生产已配置飞书开关、App、会话、接收人和阈值，历史也存在成功投递，因此不是凭据永久失效。worker 周期日志反复出现 `upstream_balance_notification: due delivery failed`，错误为 `refresh active upstream balance scopes`。`RunDue` 当前收集所有 firing BaseURL 后调用一次 `RefreshUpstreamBalanceScopes`；该函数汇总所有账号刷新错误并整体返回，导致任一账号/范围失败时，后续 evaluation、claim、send 全部跳过。两个 firing 范围因此可能同时漏通知。

## 2. 目标与非目标

目标：按规范化 BaseURL 隔离刷新、评估、claim 和投递；一个范围失败不阻断其他范围；同一范围单账号失败不阻断其他账号；保留 firing 事件直到有严格有效的新鲜证据；提供可判定的刷新、评估和投递健康信号。

非目标：不改变余额阈值、重复周期、事件账本 generation/lease/CAS、飞书卡片格式、账号余额事实源或调度算法；不手工修改生产事件、不制造余额、不用真实飞书 egress 做自动化测试。

## 3. 方案比较与推荐

1. **RunDue 失败后重试整批（不采用）**：继续出现头部阻塞，无法保证独立范围送达。
2. **按 BaseURL 分桶处理（采用）**：每桶独立刷新、读取、评估和投递，复用现有事件账本与退避，改动边界清楚。
3. **另起通知 worker/队列（不采用）**：引入重复 claim/lease 和新的事实源，风险大于收益。

## 4. 端到端控制流

```text
ListActive firing events
→ 按规范化 BaseURL 分桶
→ 每桶刷新账号（单账号错误隔离）
→ 校验快照新鲜度、凭据指纹和余额完整性
→ 仅对该桶 Evaluate
→ Claim/发送/Confirm；发送失败 RecordFailure 并退避
→ 汇总健康指标，继续处理下一桶
```

某桶全部刷新失败、快照过期、凭据指纹冲突或无可用余额证据时：只跳过该桶，保留 firing，不 resolve、不发送。某桶部分成功时，至少一个严格有效且新鲜快照可参与评估；失败账号不伪造余额，继续其他账号刷新。

## 5. 接口与字段契约

保留 `RefreshUpstreamBalanceScopes` 兼容入口，新增内部按 scope 返回结果的窄合同：`scope_key`、`refreshed_accounts`、`failed_accounts`、`fresh_valid_snapshots`、`stale_snapshots`、`credential_conflicts`、`observed_at` 和 `error_code`。`RunDue` 不再把所有 scope 的错误合并为单个提前返回。

健康信号至少包括：`last_evaluation_at`、`last_successful_scope_at`、`last_successful_delivery_at`、`consecutive_all_scope_failures`、`active_firing_scope_count`、`failed_refresh_scope_count`、`due_delivery_scope_count`、`oldest_due_age_seconds`。日志字段使用规范化 scope key 和非敏感错误码，不记录 API key、token 或密码。

## 6. 失败、安全与恢复语义

刷新失败与飞书发送失败必须分层：刷新失败不 claim；已 claim 后发送失败必须调用 `RecordFailure`，保留 generation/lease 校验并按现有退避。发送前继续执行余额、观察时间和凭据指纹复核，避免陈旧快照误报。健康状态恢复不自动清除 firing，必须由一次成功评估按现有 Resolve 语义处理。

修复后首次 due pass：当前余额仍低于阈值时按当前状态补发一次；余额已恢复则 resolve 且不补发历史；处于中间告警状态则按当前状态发送；无新鲜证据则保持 firing 等待下一轮。旧事件和卡片不回写。

## 7. 兼容性与发布

不新增数据库迁移和外部 API；复用 `ops_alert_events`、现有 lease/退避和飞书 sender。单桶失败不得影响其他桶的延迟预算。发布前仅运行直接相关 service/repository 测试、构建、格式和 diff 校验；本规格不授权合并、推送、部署或真实飞书发送。生产发布须从干净且已推送的 `main`，按验收站全局约束取得明确授权并保留回滚证据。

## 8. 验收矩阵

| 场景 | 预期 |
| --- | --- |
| BaseURL A 刷新失败、B 成功 | B 正常评估/发送，A 保留 firing |
| A 内一个账号失败、另一个成功 | 成功账号形成有效 scope 证据并继续处理 |
| 所有账号刷新失败 | 不 claim、不发送、不 resolve，健康信号递增 |
| 快照过期/凭据冲突 | 仅该 scope 跳过并告警 |
| claim 后飞书发送失败 | RecordFailure、退避，下一轮可重试 |
| 余额恢复 | Resolve，不补发历史低余额告警 |
| 两个 firing scope 并存 | 每个 scope 独立统计和投递，不互相阻塞 |

## 9. 测试策略

覆盖多 scope 隔离、单账号失败、全失败保留 firing、部分新鲜快照、过期/指纹冲突、claim/lease 并发、发送失败退避、恢复 resolve 和健康指标。使用 fake refresher、reader、repo、sender；禁止真实飞书 egress。完成后运行直接相关 Go 测试、`go build ./cmd/server`、`gofmt`、`git diff --check`。

## 10. 未决事项与批准记录

- scope 结果结构的具体 Go 类型名遵循现有 service 命名；语义字段不可删减。
- 健康指标暴露位置（现有 admin health endpoint 或结构化日志）在实施计划中确定，不新增平行监控系统。
- 用户已确认修复方向；当前仅等待书面规格批准，未实现、未测试、未提交、未推送、未部署。
