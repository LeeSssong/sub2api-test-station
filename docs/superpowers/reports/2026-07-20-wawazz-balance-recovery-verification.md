# Wawazz 余额恢复与 GPT-Plus 兼容性复验（2026-07-20）

## 结论

Wawazz 的余额阻塞已经解除，生产账号、原生监控、Python/Node 客户端和 `0.05x` 计费链路均恢复。根因是 Wawazz 会根据转发到上游的 `User-Agent` 返回不同结果；生产账号已增加账号级 `User-Agent: node` 覆写。

- Sub2API 原生 `GPT-Plus / Wawazz` 监控已从 `INSUFFICIENT_BALANCE` 恢复为 `operational`；恢复后四个连续有效样本为 `1754 / 2293 / 1919 / 2011 ms`。
- 同一生产账号 `wawazz-production-primary` 的官方账号连接测试成功并完整结束，约 `3.05s`。
- 下游 `User-Agent: node` 时，一次同步和一次 SSE 均返回 HTTP 200；SSE 收到 `[DONE]`。
- 修复前，下游为 `Python-urllib` 或 `OpenAI/Python` 时，上游稳定返回 HTTP 403，Sub2API 对用户映射为 HTTP 502；失败请求没有生成用量和扣费。
- 修复后，`Python-urllib` 同步和 `OpenAI/Python` SSE 均返回 HTTP 200，SSE 收到 `[DONE]`。

本轮只修改 Wawazz 账号 `credentials.user_agent`；没有修改生产路由、分组倍率、账号倍率、并发、优先级、模型映射、账号绑定或飞书命令模式，也没有重启容器。

## 原生监控恢复

生产监控配置保持：

```text
name: GPT-Plus / Wawazz
model: gpt-5.4
interval: 3600s
jitter: 120s
enabled: true
```

余额恢复后的历史：

| 历史 ID | 状态 | 延迟 | 说明 |
|---:|---|---:|---|
| 34 | `operational` | `2011 ms` | UA 修复前手动复核 |
| 33 | `operational` | `1919 ms` | 后续定时样本 |
| 31 | `operational` | `2293 ms` | 后续定时样本 |
| 30 | `operational` | `1754 ms` | 本轮手动恢复复核 |
| 28 | `error` | `30015 ms` | 恢复前余额不足 |
| 26 | `degraded` | `7085 ms` | 早期退化样本 |

该监控使用独立低额 Key 和固定 Go 请求形态，只证明上游基础可用，不足以覆盖客户 SDK 的 `User-Agent` 差异。

## 修复前用户网关验收

最终有效请求固定为 `gpt-5.4`、低输出、生产 `GPT-Plus` 分组：

| 请求 | HTTP | 首字/首响应 | 总耗时 | Token | 完整性 |
|---|---:|---:|---:|---:|---|
| 同步 | 200 | — | `3297 ms` | `4390 / 5` | 正常 JSON 结束 |
| SSE | 200 | `5280 ms` | `5307 ms` | `4390 / 5` | 收到 `[DONE]` |

Sub2API 管理端记录 ID 为 `23` 和 `24`，两条均为：

- 入站：`/v1/chat/completions`
- 上游：`/v1/responses`
- 分组：`GPT-Plus`（ID `6`）
- 账号：Wawazz（ID `8`）
- 计费 Token：`550` 输入、`5` 输出、`3840` 缓存读取
- 用户扣费：每条 `$0.002410`

下游返回的 `prompt_tokens=4390` 包含缓存相关 Token；Sub2API 管理端将其拆分为普通输入与缓存读取后计费，两者口径可以解释。

## 账号级修复与回归

Sub2API `v0.1.161` 的 Chat Completions 直转代码先透传白名单中的客户端 `User-Agent`，随后才应用账号级 `user_agent`。生产账号此前没有该字段，因此 Python UA 原样到达 Wawazz 并被拒绝。

更新前后安全快照：

```text
before: 451b1a2d26f8fd25713c5fd19e4c1252ff6b338f53242a3b139632cc0d3d1b32
after:  0af3c6169ab11ef1e0e852b0e56418e36559c892f0b7da9a55d37457f3a7d958
change: credentials.user_agent "" -> "node"
```

名称、平台、类型、状态、调度状态、分组 `6`、并发 `1`、优先级 `99`、成本倍率 `0.05`、Base URL 和模型映射逐项匹配。敏感 API Key 未读取或回传；Sub2API 的敏感凭据合并逻辑保留原 Key。更新触发的官方 Responses 能力探测返回 HTTP 200。

最终 Python 回归：

| 请求 | HTTP | 首字 | 总耗时 | Token | 用户扣费 | 完整性 |
|---|---:|---:|---:|---:|---:|---|
| `Python-urllib` 同步 | 200 | — | `4869 ms` | `4390 / 5` | `$0.002410` | 正常 JSON 结束 |
| `OpenAI/Python` SSE | 200 | `1942 ms` | `1989 ms` | `4390 / 5` | `$0.002410` | 收到 `[DONE]` |

一次中间同步请求在上游持续约 `69s`，测试客户端先在 45 秒超时并过早清理用户，导致 Sub2API 最终记录一次 `USER_NOT_FOUND` 用量写入错误。该错误来自测试清理时序，不是生产账号失败；后续脚本改为等待请求完成后再清理，最终同步/SSE 均通过。

## 三方计费核对

Wawazz 用量页在 `2026-07-20 22:34:57` 和 `22:35:02` 各记录一条 `gpt-5.4` 请求，实际费用均为 `$0.000121`。

```text
单条实测成本倍率 = 0.000121 / 0.002410 ≈ 0.0502x
两条用户扣费合计 = $0.004820
两条上游实际费用合计 = $0.000242
```

修复后的两条 Python 请求在 Wawazz 用量页分别显示实际 `$0.000121`，Sub2API 分别扣 `$0.002410`：

```text
两条用户扣费合计 = $0.004820
两条上游实际费用合计 = $0.000242
实测倍率 = 0.000242 / 0.004820 ≈ 0.0502x
```

结果与生产账号配置 `0.05x` 一致，差异来自上游页面显示精度。

## User-Agent 根因证据

Sub2API `v0.1.161` 的 OpenAI 兼容链路会把白名单中的下游 `User-Agent` 传给上游。保持模型、请求体、账号、分组和生产入口不变时：

| 下游 User-Agent | 同步 | SSE | 用量/扣费 |
|---|---|---|---|
| `Python-urllib` | 上游 403 / 下游 502 | 未形成有效验收 | 0 |
| `OpenAI/Python` | 上游 403 / 下游 502 | 上游 403 / 下游 502 | 0 |
| `node` | 200 | 200 + `[DONE]` | 正常 |
| Sub2API 官方账号测试（Go） | 200 | 完整结束 | 账号测试路径 |

移除 `max_tokens` 后 Python UA 仍返回同一 403，排除了低 `max_tokens` 转换导致的拒绝。最终仅把 UA 改为历史成功记录中的 `node` 即恢复，因此当前根因是 Wawazz 的上游访问策略对 UA 敏感，而不是余额、Key、模型映射或 Sub2API 安全检查。

## 清理与生产复核

- 所有临时下游 Key 和用户均通过官方 API 删除；按测试前缀检索剩余用户为 `0`。
- Wawazz 账号 `8` 仍为 `active + schedulable`，只绑定 `GPT-Plus`，并发 `1`、账号成本倍率 `0.05x`。
- Neko 主账号 `7` 仍只绑定 `GPT-Pro`；Aliu `2` 和 Neko 隔离复制账号 `9` 均不可调度、未绑定公开分组。relay-ops 路由配置仍把 Aliu 作为两个分组的共享灾备。
- 两个公开分组仍处于主路由；没有执行灾备切换。
- `relay-ops` 仍为 `read_only`，飞书命令仍为 `dry_run`，没有进入 `enabled`。
- 正式入口 `/health` 返回 200，基础容器未因本轮验收重建。

## 运行观察

- 账单页同时显示一个名为 `test` 的 Key 持续高频调用 `gpt-5.6-sol`；近 24 小时观察时为约 `2251` 请求、`370.53M` Token、实际消费约 `$16.89`，且仍在增长。本轮未修改或停用该 Key，所有权和用途尚未确认。

## 2026-07-21 High-Load Follow-up

At 02:26 Asia/Shanghai, a read-only browser check showed that the same load was still active:

- Near-24-hour aggregate: `3458` requests, `579.22M` Token, standard cost `$506.4435`, actual cost `$25.3222`, average duration `13.82s`.
- Growth from the earlier sample: requests `+53.6%`, Token `+56.3%`, actual cost `+49.9%`.
- Model concentration: `gpt-5.6-sol` accounted for `3362/3458` requests and `569.73M/579.22M` Token.
- Endpoint concentration: `/v1/responses` accounted for `3440/3458` requests.
- The latest visible rows all used the display name `test`; no Key value was read or recorded.
- Recent TTFT values were mostly `1.48s-2.84s`, while total duration included `63s` and `107s` tails.
- Visible account balance was `$15.67`; at the observed `$25.3222/day` burn rate, static runway was about `14.9` hours.

The page did not provide enough error aggregation to qualify this traffic as a stable capacity result. The Key was not disabled, edited or otherwise changed, and its business owner/purpose remains unconfirmed.
- 该并行流量与一次 `69s` 样本同时出现，可能造成队列和尾延迟；日报需要单独观察 GPT-Plus 的 TTFT/P95、并发占满和余额消耗速度。
- Wawazz 的余额、UA 兼容和 `0.05x` 门禁已完成；长期稳定性、大陆链路与商业授权仍未知。
