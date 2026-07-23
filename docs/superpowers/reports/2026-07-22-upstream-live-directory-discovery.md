# 公共上游目录发现 Live 验证

**日期：** 2026-07-22（Asia/Shanghai）  
**结果：** `PASS / DISCOVERED_NOT_QUALIFIED`  
**能力边界：** vendor-neutral `discover`；当前仅以 registry 中的 `xm-plus`、`xm-pro` 两个实例执行。

## 执行边界

- 每个渠道使用独立临时 Key，只执行一次 authenticated `GET /models`。
- 没有发送 `/responses`、Chat Completions、同步、SSE 或其他生成请求。
- 没有创建 Sub2API 候选、账号、分组、用户或下游 Key。
- 没有改生产路由、倍率、价格、余额、并发、注册模式、probe 或 Feishu 模式。
- 临时 Key 在目录读取后已从供应商页面删除；最终 Key 页面显示 `0` 把。
- 报告、台账和命令输出不保存 Key、Authorization header、Cookie 或供应商响应正文。

## 目录结果

| Registry channel | Run ID | HTTP | 目录耗时 | 模型总数 | 文本可测 | 非文本 |
|---|---|---:|---:|---:|---:|---:|
| `xm-plus` | `fa32a9e3-c817-4a81-a43c-2c41ae1273da` | `200` | `2332 ms` | `20` | `15` | audio `1`、realtime `1`、image `3` |
| `xm-pro` | `94d3463f-7c07-4cd8-a0d7-3fec19a04be5` | `200` | `1477 ms` | `13` | `10` | image `3` |

两条追加式台账记录均为：

```text
status=partial
evidence_source=live_direct
qualification_status=discovered_not_qualified
request_count=1
generation_request_count=0
errors=[]
```

### `xm-plus`

文本可测模型：

```text
codex-auto-review
gpt-5.2
gpt-5.2-2025-12-11
gpt-5.2-chat-latest
gpt-5.2-pro
gpt-5.2-pro-2025-12-11
gpt-5.3-codex-spark
gpt-5.4
gpt-5.4-2026-03-05
gpt-5.4-mini
gpt-5.5
gpt-5.6
gpt-5.6-luna
gpt-5.6-sol
gpt-5.6-terra
```

非文本模型：`gpt-4o-audio-preview`（audio）、`gpt-4o-realtime-preview`（realtime）、`gpt-image-1`、`gpt-image-1.5`、`gpt-image-2`（image）。它们不进入本轮文本资格评测。

### `xm-pro`

文本可测模型：

```text
codex-auto-review
gpt-5.2
gpt-5.3-codex-spark
gpt-5.4
gpt-5.4-mini
gpt-5.5
gpt-5.6
gpt-5.6-luna
gpt-5.6-sol
gpt-5.6-terra
```

非文本模型：`gpt-image-1`、`gpt-image-1.5`、`gpt-image-2`（image）。它们不进入本轮文本资格评测。

## 下一阶段精确请求上界

公共 v3 profile hash：

```text
03cb79b0fc91b70f2dba01953db42f7b50245bf946dd15d002e7db5c86ba0390
```

每个渠道/角色的上界为：

```text
HTTP = 2M + 71 + K
generation = 2M + 70 + K
max output = 8 Token / generation request
```

其中 `K` 是必须单独批准的 topology verification 请求数，不能静默假设。

| Channel | M | K=0 HTTP / generation | K=4 HTTP / generation | K=4 最大输出 Token |
|---|---:|---:|---:|---:|
| `xm-plus` | 15 | `101 / 100` | `105 / 104` | `832` |
| `xm-pro` | 10 | `91 / 90` | `95 / 94` | `752` |
| 合计 | 25 | `192 / 190` | `200 / 198` | `1584` |

完整资格评测仍需新的明确批准，至少绑定：`K`、最大 HTTP 请求、最大 generation 请求、最大总 Token、最大货币成本、最大墙钟时间和停止阈值。在此批准前不得执行 compatibility、gateway、billing、capacity、shared-backup、observation 或 failover/failback。

### 建议的下一阶段授权（尚未批准）

为避免把目标拓扑验证和基础渠道资格混在一起，建议先批准以下最小阶段：

```text
K = 0
xm-plus: 101 HTTP / 100 generation
xm-pro:  91 HTTP / 90 generation
total: 192 HTTP / 190 generation
max total Token: 500,000（两渠道合计）
max currency: USD 0.20（每渠道 USD 0.10）
max wall clock: 45 分钟/渠道，90 分钟总计
```

停止条件固定为：首个 `401/403/429/5xx`、连接/TLS/超时、协议错误、SSE 未收到终止事件、累计 Token 或金额达到上限、墙钟达到上限、账单无法解释，或任何生产模式/路由快照不一致。该阶段不包含 Wawazz backup、shared pool、24–72 小时观察、failover/failback，也不改变生产配置。

## 当前判定

目录发现证明两个渠道都提供可测试的 Responses 文本模型集合，但不证明同步/SSE 兼容、计费正确、容量、稳定性、网络质量、转售授权或灾备角色。当前生产拓扑不变，`relay-ops` 继续 `read_only + dry_run`，D04 继续 `read_only` 且注册关闭。
