# XM API PLUS/PRO 上游发现报告

> **能力边界：** 这是一份 XM 渠道实例的证据报告，不是 XM 专用评测能力。公共能力是 vendor-neutral 的 `discover -> compatibility -> gateway -> billing -> capacity -> proposal`；`xm-plus`、`xm-pro` 仅作为 registry 中的 `channel_id` 传入，与 Wawazz、Aliu 或未来任何兼容上游使用同一实现。

> **Live follow-up (2026-07-22):** The shared `discover` command has now executed one authenticated `/models` request for each current registry instance, with zero generation and temporary-Key cleanup. This report preserves the pre-live discovery baseline; the superseding evidence is `2026-07-22-upstream-live-directory-discovery.md`.

**日期：** 2026-07-21（Asia/Shanghai）  
**结果：** `DISCOVERED_NOT_QUALIFIED`  
**范围：** 只读检查用户已登录的 XM API 与 Wawazz 供应商页面；没有复制或保存 Key，没有提交表单、发送模型请求、创建候选或修改生产。

## XM 已确认的非秘密事实

| 项目 | XM API Plus | XM API Pro20x |
|---|---|---|
| 供应商展示分组 | `GPT Plus` | `GPT Pro20x` |
| 页面标称倍率 | `0.045x` | `0.07x` |
| 页面展示 Base URL | `https://api3.xmhbao.cn` | `https://api3.xmhbao.cn` |
| 客户端示例 | `wire_api = responses`，默认 `model = gpt-5.5` | `wire_api = responses`，默认 `model = gpt-5.5` |
| 页面既有请求证据 | 无 | 1 次同步 `/v1/responses`；1.52K Token；总耗时 1.68s；实际 `$0.000121`、标准 `$0.0017` |

XM 页面同时展示了美国直连中并发、国内优化低并发和美国直连高并发入口；其中高并发入口写有“支持 1w 并发”的供应商声明。该数值没有经过项目测试，只能作为待验证声明，不能配置为 Sub2API 并发或写入 SLA。页面“模型广场”当前没有可展示目录，因此完整模型列表、模型分类和每模型价格仍为 `unknown`。

Pro 的单条历史记录与 `0.07x` 标称倍率方向一致，但页面金额仅显示有限小数位，且没有 API usage、Sub2API 扣费和上游精确账单三方对账，不能据此标记为 billing verified。

18:49 只读复核时，XM 账户余额为 `¥2.60`，GPT Plus 与 GPT Pro20x 两个工位均已由用户预先开机，累计调用 `290`。Plus 显示累计用量 `$0.117704`，Pro20x 显示 `$2.485646`；最近记录为 `gpt-5.6-sol` 的 `/v1/responses` 流式调用，可见总耗时约 `4.36s-23.45s`。这些是用户已有历史，不是本项目本轮发起的请求。两个分区仍明确显示“未启用监控”，模型广场显示 `0` 个可展示模型，所以完整模型目录、错误率和容量仍为 `unknown`。

22:20 对已登录 Key 管理页做了另一轮只读检查。页面可按分组创建 Key，并为 Key 配置美元额度上限、速率限制、有效期、IP 限制、启用/禁用状态；因此 live 评测可以为 Plus、Pro 各创建一把隔离低额临时 Key，而不复用页面现有 Key。页面现有两把 Key 均显示为活跃、永久有效；本轮没有读取/复制 Key 原文、打开复制动作、提交创建/编辑、改变分组或禁用任何 Key。

## 零费用网络预检

在本地工作站和首尔生产主机分别执行不带认证的请求，只记录 HTTP 状态、Content-Type 和耗时，不读取响应正文：

| 路径 | 本地 | 首尔 | 解释 |
|---|---|---|---|
| `GET /models` | `401 application/json` | `401 application/json` | 根 Base URL 下存在受鉴权保护的模型目录。 |
| `POST /responses` | `401 application/json` | `401 application/json` | 根 Base URL 下存在受鉴权保护的 Responses API。 |
| `POST /chat/completions` | `200 text/html` | `200 text/html` | 返回站点 HTML，不能视为 Chat Completions API。 |
| `/v1/models`、`/v1/responses`、`/v1/chat/completions` | `403 text/html` | `403 text/html` | 不应把 `/v1` 加入 benchmark Base URL。 |

因此 registry 使用 `https://api3.xmhbao.cn` 根地址是正确的，供应商页面的 `wire_api = responses` 也与网络边界一致。后续已完成 profile 驱动的 Responses 请求与 SSE 完成语义，以及公共 `discover` 的一次目录/零生成契约；XM Plus/Pro 的 live 目录发现已由 2026-07-22 新报告覆盖。任何生成、兼容性、网关、计费或容量 run 仍必须等待新的独立凭据、预算和清理授权。

## Wawazz 同期只读观察

Wawazz 18:49 页面显示：余额约 `$9.62`，当日 `3,472` 请求、`555.6M` Token、实际 `$32.1708`，累计 `5,996` 请求、`977.6M` Token、实际 `$51.3664`、标准 `$929.2584`，平均响应 `14.56s`。渠道状态页的 GPT-Plus 虽显示“正常”，但 7 天可用性仅 `94.70%`，近 60 次记录包含多次约 `30s` 错误和降级；GPT-Pro 页面显示 `100.00%`。这些供应商自报监控只作为风险证据，不替代本站对真实请求的错误率、TTFT P95 和总耗时 P95 统计。用户已确认高负载属于预期业务，不再追查 Key 归属。

## 仍缺的资格证据

- Responses 同步结果、SSE 生命周期与 `response.completed` 终止语义的确定性评测支持。
- Plus/Pro 每个文本模型的一次同步与一次完整 SSE。
- 有界并发 `1/2/3/5/8/10` 与 RPM `6/12/20/30` 实测。
- 每模型输入、输出、缓存读写价格和生效日期。
- API usage、Sub2API 用户扣费、上游标准/实际费用三方对账。
- 首尔生产机网络样本、持续观察、错误率和长尾分位数。
- 转售、退款、调价、容量和支持条款。
- 隔离对象、费用上限、停止阈值和清理授权。

## 下一门禁

两个 XM 渠道仍只进入 registry 的 `discovered_not_qualified` 状态。2026-07-22 live 目录发现得到 `M_plus=15`、`M_pro=10`，临时 Key 已清理；下一步必须为完整资格评测展示并批准精确请求、Token、货币和墙钟上限。只有完成资格评测并生成 secret-free proposal，且用户对 proposal ID/hash 明确回复“采纳”，才允许调整生产拓扑。

为使请求上限可验证，live 评测拆成两个授权阶段：

1. **通用模型目录发现（当前实例为 XM Plus/Pro）：** 每个渠道各 1 次带临时凭据的 `GET /models`，不发送生成请求。使用 Key 管理页已经确认存在的分组、额度、速率、有效期、IP 和禁用控制，为两个分组分别创建隔离临时 Key；Key 禁止进入聊天、仓库、命令参数或报告。
2. **通用完整资格评测（需另行批准）：** 目录发现得到每个渠道的文本模型数 `M` 后，再展示精确费用上限。当前 v3 每个渠道/角色的默认上界为 HTTP `2M+71+K`、生成 `2M+70+K`，其中 `K` 是另行批准的拓扑验证请求；逐模型最大输出 8 Token。每个 primary/backup 角色必须独立计算，不能把该公式误当成整套拓扑总预算。任何 `429`、`5xx`、超时、协议错误、SSE 未完成、计费不明或费用门槛触发即停止。

SSE 并发容量不属于当前 V2 结果，不能从同步容量阶梯推断。评测完成后仍只生成 secret-free proposal；生产路由必须等待用户明确采纳指定 proposal ID/hash。
