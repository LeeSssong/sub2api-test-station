# T39 Responses 413 二次错误投影修复规格

## 状态与批准

- 任务：T39，状态从 `DESIGNING` 进入批准后的 `IMPLEMENTING`，候选最终只能到 `READY_FOR_ROOT_REVIEW`。
- 功能基线：detached `HEAD@5ded56aac949b6f1b8dced8a384b3761a54b39f5`。根总控后续仅将全局登记文档推进到 `main@e1a41d4d3`，业务源码相对本基线未变化。
- Brainstorming 分类：实现范围有界，但项目门禁要求完整正式规格流程，因此采用完整规格、自审与根总控代审记录。
- 澄清结论：队列已明确限定应用内入站/上游 413、JSON 和 Responses SSE、中文语义、机器分类、终止事件与脱敏；没有产品待决事项，无需扩大提问。
- 分段批准：问题与范围、控制流、协议与安全、验收与发布四段均由唯一发布总控依据 2026-08-15 代审授权批准。
- 规格自审：无 `TBD`/`TODO`，无相互矛盾条款，无 T40、Cloudflare 边缘接管、迁移或生产动作；单一实现计划可闭环。
- 批准结论：`APPROVED_BY_ROOT_RELEASE_CONTROLLER_PROXY`。证据、接口、失败语义、测试、回滚均闭环，不涉及范围扩大、不可逆数据操作、凭据风险、外部付费或停机决策。

## 问题证据与当前行为

Sub 原生能力已经存在，可做最小扩展：

- `service.ProjectNativeUserError` 已提供统一中文用户错误投影和敏感证据防泄漏。
- `OpenAIGatewayHandler.handleStreamingAwareErrorWithCode` 已在 JSON/SSE 共享错误链携带 HTTP status、错误 type/code/message。
- `writeResponsesFailedSSE` 已输出 Responses 规范的 `response.failed` 终止事件。
- `UpstreamFailoverError.IsOpenAIRequestBodyTooLarge` 已把上游非上下文窗口 413 识别为可切换账号的请求体限制，并在账号耗尽后固定返回 413。

当前缺陷由两个相邻行为叠加：

1. `ProjectNativeUserError` 先判断“已选择账号/上游阶段”，再判断 413，所以应用内上游 413 会被泛化为“服务暂时异常”。同一优先级也影响 JSON 终态。
2. Responses SSE 路径在 `handleStreamingAwareErrorWithCode` 投影一次后，`writeResponsesFailedSSE` 又以固定 502 投影第二次。第二次丢失原始 413 status/code，并可能覆盖第一次得到的正确语义。

既有 `openai_body_limit_failover_test.go` 仍断言英文 `Request payload is too large`，没有锁定中文投影、机器分类、单次投影数据流和脱敏的组合合同。

## 目标

- 应用读取入站请求体发现过大时，JSON 返回 HTTP 413、`error.type=invalid_request_error` 和中文“请求内容过大，请缩短内容后重试。”。
- 上游账号返回非上下文窗口 413、账号故障转移耗尽时，JSON 保持同样的 HTTP/type/中文语义，不因已选择账号而泛化。
- Responses SSE 已开始后，以唯一 `event: response.failed` 终止；其 `response.error.code=invalid_request`，message 为同一中文语义。
- SSE 写入保留调用链原始 status/type/code 到唯一投影点，不再以固定 502 二次投影。
- 上游原始响应体、URL、request ID、Cloudflare/Ray 信息、密钥和其他敏感证据不得进入用户响应。

## 非目标

- 不处理未进入应用的 Cloudflare HTML 413，也不声称应用能改写该响应。
- 不并入 T40 的 402、507、499、520–525 或正文/边缘错误映射扩展。
- 不改变上游 413 的账号切换资格、重试预算、上下文窗口错误判断、账务、调度或管理员诊断事实源。
- 不改 GitHub Actions、发布链、迁移、配置、生产数据、前端或其他协议路径。

## 方案比较

### 方案 A：413 确定语义前置 + Responses SSE 单次投影（采用）

将 413/明确 oversized marker 放在通用上游泛化之前；让 Responses SSE writer 接收原始 status/type/code/message，内部做唯一投影并序列化。JSON 与 SSE 共用同一投影函数，SSE 保留协议专用 error code 映射。

优点：从两个根因同时闭环；数据流清楚；不新增事实源；改动集中在现有投影和写入器。风险是写入器签名变化，需要更新少量直接调用点和测试。

### 方案 B：仅增加中文 oversized 关键词

让第二次投影识别第一次生成的中文 message。改动最少，但固定 502 和重复投影仍存在，机器分类继续依赖调用顺序，拒绝。

### 方案 C：413 专用 JSON/SSE 旁路

在 failover exhausted 分支直接手写中文 envelope/事件。能修当前场景，但会复制安全投影和协议逻辑，使入站与上游、JSON 与 SSE 逐渐分叉，拒绝。

## 端到端控制流

### 入站 JSON 413

`readLenientJSONRequestBodyWithPrealloc` 返回 `MaxBytesError` -> handler 调用现有 `errorResponse(413, invalid_request_error, ...)` -> `ProjectNativeUserError` 以 413 优先投影 -> HTTP 413 OpenAI JSON envelope。

### 上游 JSON 413

上游非上下文窗口 413 -> `UpstreamFailoverError` 保存脱敏客户端 status/message，原始 body 仅供内部诊断 -> 账号故障转移耗尽 -> `handleStreamingAwareError(..., streamStarted=false)` -> JSON writer 做一次 413 投影 -> HTTP 413 envelope。

### Responses SSE 413

同一 failover error 且流已开始 -> `handleStreamingAwareErrorWithCode` 记录 ops stream error，但不预先投影 -> `writeResponsesFailedSSE(status=413, type=invalid_request_error, code, message)` -> writer 做一次统一投影 -> `mapResponsesErrorCode` 生成 `invalid_request` -> 写出并 flush 唯一 `response.failed`。

非 Responses 流仍在通用 SSE envelope 写出前做一次现有投影，行为保持不变。

## 接口与字段契约

内部写入器契约调整为携带原始分类：

```go
writeResponsesFailedSSE(
    c *gin.Context,
    status int,
    errType string,
    code string,
    message string,
) bool
```

- `status` 只用于统一用户投影，不改变已经提交的 SSE HTTP 200。
- `errType` 投影后保持机器 type；`code` 若非空保持原值，否则由 Responses type 映射得到协议 code。
- wire event 固定为 `event: response.failed`，data 顶层 `type=response.failed`，`response.status=failed`，`response.output=[]`。
- JSON 413 保持 HTTP 413 和 `error.type=invalid_request_error`。
- 用户 message 固定为 `请求内容过大，请缩短内容后重试。`。

## 失败与安全语义

- 413 是请求尺寸这一确定性语义，优先于“已选择账号”的来源泛化；这不代表暴露具体账号、上游、代理或限制值。
- 其他已选择账号的余额、限流、5xx 等仍使用现有泛化语义，T39 不改变。
- `ProjectNativeUserError` 的安全检查继续作为唯一用户文案防线；测试使用 `must-not-leak` 等敏感上游 body 证明不泄漏。
- JSON marshal/SSE write 失败沿用现有 `c.Error` 与返回语义，不增加第二终止事件。
- Cloudflare 在边缘直接返回的 HTML 413 没有进入 Gin handler，无法也不应被本功能识别或改写。

## 兼容性与迁移

- 内部 Go 函数签名变化，无外部 API schema 新增或删除。
- HTTP JSON 和 Responses SSE 仅把错误 message 从英文/泛化中文纠正为既有中文投影；机器 type/code 和协议终止事件保持兼容。
- 无数据库迁移、历史回填、配置、依赖或生产数据变化。

## 场景化验收矩阵

| 场景 | HTTP/事件 | 机器分类 | 用户语义 | 脱敏 |
| --- | --- | --- | --- | --- |
| 入站 body 超限，未开始流 | HTTP 413 JSON | `invalid_request_error` | 请求内容过大 | 不含限制内部证据 |
| 上游 413，已选账号，failover 耗尽 | HTTP 413 JSON | `invalid_request_error` | 请求内容过大 | 不含上游 body |
| 上游 413，Responses 流已开始 | 唯一 `response.failed` | `invalid_request` | 请求内容过大 | 不含上游 body |
| 413 message 含敏感 URL/request ID | 同上 | 同上 | 固定中文 | URL/ID 不出现 |
| 已选账号普通 502 | 既有 JSON/SSE | 既有类型/code | 服务暂时异常 | 行为不变 |
| Cloudflare HTML 413 未进应用 | 不接管 | 不适用 | 不承诺改写 | 不进入测试伪装 |

## 测试策略

- TDD RED：先把现有 failover JSON/SSE 测试改为中文合同，并补充已选账号上下文；旧实现应分别得到泛化中文而失败。
- 在 service 投影测试增加“已选账号 413 仍为请求过大”和普通上游 502 不回归。
- 在 handler 测试解析 JSON 与 SSE data，精确断言 type/code/message、唯一终止事件和敏感字符串缺失。
- GREEN 后只运行直接相关 service/handler 测试、必要 `go build ./cmd/server`、`gofmt` 和 `git diff --check`。
- 不运行全仓测试、前端测试、浏览器、生产或 T40 测试。

## 发布、线上验证与回滚条件

- 候选只交给根总控，状态止于 `READY_FOR_ROOT_REVIEW`；本任务不合并 main、不推送、不部署、不访问生产。
- 无迁移/配置，预期 `downtime_required=false`，最终以根合并后的发布预检为准。
- 根发布后的最小线上专项验收应使用应用可控的入站/上游 413 路径，确认 JSON/SSE 合同与健康端点；不得把 Cloudflare HTML 413 当作本功能验收。
- 代码回滚为回退 T39 候选提交后重新走根发布链；运行回滚使用上一已验证蓝绿槽/镜像。无数据回滚。

## 待决事项

无。T40 与边缘 HTML 413 保持独立后续任务。
