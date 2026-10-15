# 流式首字接近总耗时：诊断证据

本功能在现有 `request_timing_details.detail` JSON 中增加流式摘要。适用于 HTTP Responses 普通、透传，以及原生 Chat Completions 和 Responses/消息协议的 Chat 回退读循环。无需数据库迁移；现有计费、首字口径、重试策略及客户端响应协议保持原逻辑。WebSocket、非流式请求及其他未接入的读循环没有这些摘要。

## 如何取得证据

按消费记录 ID 调用现有管理员接口 `GET /api/v1/admin/usage/{id}/timing`，查看 `traces` 中的新增字段；沿用现有管理员认证和权限。现有耗时弹窗未增加摘要展示，可查看该接口的 JSON 响应。

数据库只读查询可按 `usage_log_id` 查 `request_timing_details.detail`，与 `usage_logs.first_token_ms`、`duration_ms` 联合分析。多次请求/重试必须分别查看每个 trace。`streams[].attempt` 对应 `attempts[].number`，优先关联物理 HTTP attempt；没有上游 HTTP trace 时为 0，不能据此推断账号或连接复用。

新增字段只在部署后产生，历史请求无法补采。

## 摘要字段

所有 `*_ms` 时间点均相对整笔本站 HTTP 请求开始；`*_wait_ms`、`*_gap_ms` 为持续时间。旧 `first_token_ms` 和 `duration_ms` 以转发开始计时，不能直接与新时间点相减。

`streams[]` 每个元素描述一次被接入的上游 SSE 流：

| 字段 | 含义 |
|---|---|
| `format` | `responses` 或 `chat_completions` |
| `content_type` / `content_encoding` | 扫描器入口看到的协议枚举；不保存原始头值。传输层可能已解压并移除编码头，`identity` 不能证明线路原本未压缩 |
| `read_calls` / `read_bytes` | 应用响应体 Read 次数、读到的总字节；不重复统计嵌套 HTTP 包装层 |
| `first_read_ms` / `last_read_ms` | 首次、最后一次读到正数字节的时间；未收到字节时无 `first_read_ms` |
| `read_wait_ms` / `max_read_wait_ms` | 调用响应体 Read 的累计耗时、最大单次耗时，含等待和 HTTP 解码工作；包含无字节的 EOF/错误 Read |
| `max_consumer_gap_ms` | 上次 Read 返回到下次 Read 开始的最大间隔；可能来自解析、调度、队列背压或下游写入，不能单凭此值判定模型慢 |
| `read_windows` | 按请求起点划分的 5 秒桶，正数字节的首末时间、Read 次数、字节数和 Read 耗时；Read 等待全部计入它返回时所在桶，不代表该桶内的网络吞吐 |
| `event_count` | 已解析到的上游 data 文档总数，含终态和 `[DONE]`；SSE 注释心跳不是 data 文档 |
| `events` | 按标准事件类型汇总次数、首末解析时间、载荷/内容/加密字段长度、推理及各识别口径的命中次数 |
| `samples` | 首 8 条及最近 8 条 data 文档摘要，包含解析时间、内容长度及识别布尔值 |

事件在 Responses 变换、纠正和脱敏之前记录。`semantic`、`visible`、`ttft` 表示该原始文档是否符合对应的现有识别条件；每次命中都会累加，**不是首字发生次数**。Chat 的 `visible` 沿用现有语义输出识别条件，可能包含推理或工具结构，不能与 Responses 的口径直接比较。原生 Chat 的 role-only chunk 可符合其现有 TTFT 规则，usage-only chunk 则不符合。

`content_bytes` 是被观察的文本/参数/音频/图片等字符串字段的 UTF-8 长度之和，`encrypted_bytes` 是 opaque 加密内容字段长度。它们不是 token 数或独立生成字节：delta、done、completed 可能重复携带同一内容。`reasoning_count` 表示推理类型或推理内容出现，不证明模型在其他时间段是否执行了内部推理。

`samples[].at_ms` 是消费者解析文档时刻，**不是网络帧到达时刻**。`last_read_ms` 是当时最近一次应用 Read 返回时间；异步泵可能提前读到后续数据，不能把它当成该事件所属 Read 的精确时间。结合 `read_windows`、首个事件以及各类型的首末时间判断读取与解析的先后。

请求级新增 `downstream_write_calls`、`downstream_flush_calls`、`downstream_max_write_ms`、`downstream_max_flush_ms`，分别观察本站 Write 和 Flush 操作。既有 `downstream_write_ms` 仍是两类操作的累计耗时；这些指标不表示远端客户端实际收包时间。

## 判读方法

| 证据组合 | 可以支持的判断 | 仍需核实 |
|---|---|---|
| 正文之前已有推理事件，`content_bytes` / `encrypted_bytes` 非零，正文时间晚 | 上游先传推理或加密内容；可见首字口径使等待包含该阶段 | 是否还有上游内部排队或缓冲 |
| `first_read_ms` 很晚，首末事件解析时间集中，正文后快速结束 | 数据很晚才在本站响应体可读，上游链路等待或集中返回 | 上游模型、代理、压缩等具体环节需对方日志 |
| 很早读到数据，data 文档解析很晚；中间读取仍有字节 | 文档分片、长行、解码/扫描或消费者处理可能造成延迟 | 首尾桶被截断时不可推断中间空白；需结合文档大小、线程和上游格式 |
| 早期文档 `content_bytes > 0`，`visible` / `ttft` 未命中，或出现 `unknown` 类型 | 特殊事件或现有口径未覆盖该字段，需要协议对照 | 内容是否应计入首字；不自动认定为 bug |
| `max_consumer_gap_ms` 与 Write/Flush 最大耗时都大 | 本站下游背压可能拖慢消费 | 客户端、本站代理和调度日志 |

同一请求的旧首字与总耗时接近只是一种筛选条件，不应单凭此条件把短答案或快速输出判为故障。比较多个账号时应同时看模型、推理设置、上游域名和正常对照样本。

## 采集边界

- 每笔请求最多保留 4 个流摘要。更多流通过 `streams_dropped` 明确计数，后续事件不会混入前一个流。
- 每个流最多保留 24 类事件汇总、16 条首尾事件样本及16个首尾读取桶。超过上限设流级 `truncated=true`；事件总数和读取总量仍累加，已保留类型的汇总继续更新。`truncated` 不等于响应被截断。
- 标准事件名使用固定白名单；任意上游自定义名称归为 `unknown`。格式错误文档归为 `invalid`。不保存正文、推理文本、加密内容、工具名称、任意头值、URL、凭据或错误消息，也不新增逐事件普通日志。
- 沿用计时详情的异步保存队列、128 KiB JSON 上限和 30 天保留策略。无计时 collector 的请求不包装响应体。
- 本站证据只能定位到本站可观测边界。确认上游内部缓冲、推理或调度原因，仍需用关联请求标识核对上游日志；不能只根据读到首字节的时间认定已收到模型正文。

## 本地候选验证

在 `upstream/sub2api/backend` 执行并通过：

```sh
go build ./...
go test -race ./internal/pkg/requesttiming
go test -tags unit ./internal/server/middleware -run '^TestRequestTiming' -count=1
go test -tags unit ./internal/repository -run '^TestTiming(WriteJoinsBillingIdentityAndKeepsDistinctTraces|BindingDoesNotWriteUntilRequestFinished)$' -count=1
go test -tags unit ./internal/service -run 'TestOpenAI(ResponsesStreamTimingEvidence|ChatFallbackStreamTimingEvidence|RawChatStreamTimingEvidence|StreamTimingDistinguishesLateBodyFromLateContent|StreamTimingRetainsUnsupportedContentEvidenceWithoutPayload|VisibleOutputClassification|ResponsesTTFT|NativeMetadataAndKeepalive|FirstOutput|Passthrough)' -count=1
go test -race -tags unit ./internal/service -run 'TestOpenAI(ResponsesStreamTimingEvidence|ChatFallbackStreamTimingEvidence|RawChatStreamTimingEvidence|StreamTimingDistinguishesLateBodyFromLateContent|StreamTimingRetainsUnsupportedContentEvidenceWithoutPayload)$' -count=1
```

另以最大 32 个上游 attempt、256 个 span、32 个长度上限诊断字段，以及填满每个已采集流的类型/样本/读取桶验证 JSON 保存大小：83,897 字节，小于 128 KiB。

服务测试使用仓库 `Makefile test-unit` 规定的 `unit` 标签。初次不带该标签的 service 测试因现有 `openai_apikey_cache_protocol_test.go` 引用只在 unit 文件定义的 `rawChatCompletionsTestAccount` 而编译失败；本次未修改这项既有测试组织问题。

验证环境为本地 Darwin/ARM64。未构建生产镜像、未部署、未进行真实上游或线上采集验证；部署后的请求才能用于实际问题定位。


## Chat → Responses 流式转换补充（2026-10-15，本地候选）

Chat Completions 转换为 Responses 再转换回 Chat 的流式路径接入相同读取与事件摘要，包括共用处理器的 Grok bridge。同步和异步读取都在扫描前包装响应体，绑定物理 HTTP attempt。不能补采历史请求。

该路径保留首个 data 文档计时的首字定义，记录为 ttft_mode=first_data，元数据也可命中 TTFT。first_visible 记录原始上游可见内容被解析时刻；first_output_flush 记录应用输出 Flush，均不保证远端已接收。

分别记录上游 terminal 和下游 outcome/client_disconnect，使上游完成但写回失败同时可见。events.client_write_error 和 events.upstream_read_error 标记首次观察到的错误时间；写错误时间不等于实际断开时间。HTTP 200、上游 completed、消费记录不能单独证明交付成功。用户取消、网络中断、代理超时仍需客户端/代理日志判因。

只保存类型白名单、长度、时间和结果，不保存正文、推理、工具参数或凭据。沿用现有大小与保留期限限制，不修改计费、首字、输出和重试行为，无数据库迁移。


## 大请求流式保活修复（2026-10-16，本地候选，未部署）

Chat → Responses → Chat 流式处理在收到上游响应头后，沿用现有 stream_keepalive_interval 定时发送 SSE 注释心跳。取消静默拒绝检测对心跳的抑制；检测仍可扣留元数据并识别空完成。心跳不设置 first_token_ms、不算语义输出、不改变 usage 或余额结算。新增 events.first_keepalive 标记第一次成功 Write/Flush 后的应用时间，不证明客户端实际接收。

兼容性取舍：心跳已提交 HTTP 200，此后不能再改 HTTP 状态或透明重放。现有 Chat handler 检查转发前后 Writer.Size，向客户端发送 SSE 错误并终止，而非跨账号重试。未发送心跳且未输出时仍保留既有 failover。keepalive=0仍表示禁用。本修复只覆盖已定位的 Responses→Chat 流处理器，不覆盖等待上游响应头阶段，不保证上游变快或消除所有网络超时。

本次不部署、不退款、不改计费策略。已新增回归覆盖大请求在正文前收到心跳、心跳不提前TTFT、空完成仍返回失败、心跳后handler返回SSE错误。诊断回归覆盖同步/异步、断连、迟到正文与迟到完整事件。
