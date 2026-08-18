# T24 特惠分组本地调度耗尽错误契约设计

## 1. 背景与证据

生产只读排查确认，请求在账号选择前耗尽本地候选池时，Responses 与 Chat Completions 会直接生成 HTTP 503。该响应没有上游响应体，因而不会进入 `ErrorPassthroughService`；当前用户会看到英文 `Service temporarily unavailable` 或包含内部筛选摘要的 `No available accounts: ...`。

现有原生能力已经覆盖大部分所需边界：

- `handler/no_account_error.go` 区分永久模型不支持（404）和临时账号池不可用（503）。
- `markOpsRoutingCapacityLimited` 已把本地容量耗尽标为 `routing / platform / gateway`，并与真正的上游错误上下文分开。
- `handleStreamingAwareError` 已分别支持未开始流的 JSON、Responses 的 `response.failed` 和 Chat Completions 的 SSE error 帧。
- T02 的 `ProjectNativeErrorDiagnosis` 已向管理员投影阶段、归属、账号是否已选和脱敏证据，但刻意未覆盖本地路由耗尽。
- 已选择账号后的真实上游 503 继续经过既有透传、失败切换和上游诊断路径。

因此本任务是对原生错误分类和输出的最小扩展，不建立新的错误系统或配置事实源。

## 2. 目标

1. 本地账号调度耗尽保持 HTTP 503，返回稳定机器码和中文泛化解释。
2. Responses、Chat Completions 的流式与非流式请求共享同一语义。
3. 管理员诊断明确显示 `routing / platform / 未选择上游账号`，并与真实上游 503 区分。
4. 永久模型不支持仍返回既有 404；真实上游 503 的状态、响应体透传、重试和诊断不变。

## 3. 非目标

- 不调整账号、分组、自购账号归属、优先级、冷却、并发、资格、利润门或调度算法。
- 不调整重试、账号切换、计费、用量记录或错误看板过滤口径。
- 不修改 `ErrorPassthroughRule`、外部控制面、数据库、迁移、生产数据或部署链。
- 不顺带处理 Messages、Gemini、图片、音视频、WebSocket 或其他入口。

## 4. 方案比较

### 方案 A：为本地 503 新增 ErrorPassthroughRule

优点是复用现有配置入口。缺点是本地耗尽没有上游响应体，现有规则匹配点不会执行；强行把本地错误伪装成上游错误还会破坏归属。否决。

### 方案 B：把本地耗尽改成 HTTP 429

优点是部分客户端会自动退避。缺点是 429 通常表示调用方或限额过载，而本场景是平台账号池暂时不可用；这会把管理员归属错误地指向客户端，并污染既有本地限流统计。否决。

### 方案 C：保留 HTTP 503，扩展原生分类器和诊断投影

在 `no_account_error` 的临时耗尽分支返回稳定错误码 `local_capacity_exhausted` 与中文消息；调用方继续使用现有 JSON/SSE 写入器；T02 诊断增加本地容量类别。该方案不触碰调度和上游透传，改动最小且归属准确。采用。

## 5. 对外契约

### 5.1 HTTP 与错误码

- 临时本地调度耗尽：HTTP `503`。
- Responses 非流式/流尚未开始：`error.code = "local_capacity_exhausted"`，`error.message = "当前服务资源暂时不可用，请稍后重试"`。
- Chat Completions 非流式/流尚未开始：`error.type = "local_capacity_exhausted"`，`error.message` 同上。
- 流已经开始：HTTP 已固定为 200。Responses 输出一个 `response.failed`，其中 `response.error.code = "local_capacity_exhausted"`；Chat Completions 输出既有 SSE error 帧，`error.type` 为同一码。消息保持一致。
- 永久模型不支持：继续 HTTP 404 和 `model_not_found`，不改中文化。

机器码使用小写 snake_case，以匹配当前 OpenAI 兼容响应的 `type/code` 约定。管理员诊断使用大写 `LOCAL_CAPACITY_EXHAUSTED`，与 T02 现有诊断码风格一致。

### 5.2 管理员诊断

对本地容量耗尽记录，`ProjectNativeErrorDiagnosis` 返回：

- `class = local_capacity_exhausted`
- `code = LOCAL_CAPACITY_EXHAUSTED`
- `stage = routing`
- `ownership = platform`
- `upstream_account_selected = false`
- `original_upstream_status/message/detail` 为空
- 用户含义：`当前分组暂无可用服务资源`
- 建议：`请稍后重试；持续失败请联系管理员并提供请求 ID`

分组 ID/名称沿用日志现有字段；未选择账号时不生成账号 ID/名称。

### 5.3 上游真实 503

只要已选择账号或存在 `OpsUpstreamError` 证据，仍由现有上游路径处理：保留原始上游状态、透传规则结果、重试/切换行为和 `upstream_overloaded`/`upstream_failed` 管理员诊断。本任务的本地机器码不得出现在该路径。

## 6. 控制流

1. 请求完成认证、计费检查并进入账号选择。
2. 第一次选择没有任何已失败账号，选择器返回无可用账号或空 selection。
3. 现有持久配置诊断先判断是否为永久模型不支持；是则维持 404。
4. 其余临时耗尽标记 `markOpsRoutingCapacityLimited`，分类器返回本地容量 503 契约。
5. 未开始流时写 JSON；已经开始流时复用协议对应 SSE 终止事件。
6. `OpsErrorLoggerMiddleware` 持久化 routing/platform/gateway 证据；管理员读取时由 T02 投影本地容量诊断。
7. 若已经选择账号并收到真实上游 503，则不经过步骤 3–5 的本地首次选择分支。

## 7. 兼容性与安全

- HTTP 503 保持不变，已有按状态退避的客户端兼容。
- 只把泛型 `api_error/server_error` 收紧为稳定本地码；消息从英文和内部摘要改为中文泛化文本。
- 不向用户暴露筛选原因、账号名称、账号数量、冷却、余额、利润门或上游凭据。
- 管理员只看到既有日志已有的分组与脱敏证据；无新增敏感字段。
- 无 schema、配置、依赖或迁移变化，`downtime_required` 预期为 `false`，仍以根合并后发布预检为准。

## 8. 验收矩阵

| 场景 | 预期 |
|---|---|
| Responses 非流式，本地候选全部不可调度 | 503；稳定码；中文泛化消息 |
| Responses 流式，流尚未开始 | 503 JSON；同码同消息 |
| Responses 流式，流已开始 | `response.failed`；同码同消息 |
| Chat Completions 非流式，本地耗尽 | 503；`error.type` 稳定；中文消息 |
| Chat Completions 流式，流尚未开始 | 503 JSON；同码同消息 |
| Chat Completions 流式，流已开始 | SSE error；同码同消息 |
| 分组有账号但模型永久不支持 | 既有 404 `model_not_found` |
| 已选择账号后真实上游 503 | 既有透传/失败切换与上游诊断，不出现本地码 |
| 管理员查看本地耗尽详情 | routing/platform/未选择账号，无上游证据 |
| 用户响应与管理员日志 | 不含筛选摘要、账号或凭据 |

## 9. 测试策略

- 先扩展 `no_account_error_test.go`，证明旧实现仍返回英文泛型码。
- 为 JSON 与已开始流的 Responses/Chat Completions 增加协议级测试。
- 为 `native_error_diagnostics_test.go` 增加本地容量投影和真实上游 503 隔离测试。
- 运行 handler/service 直接相关测试、受影响包 compile-only、server build、`gofmt` 和 `git diff --check`。
- 不扩大到全仓、浏览器或生产测试。

## 10. 发布、回滚与待决事项

- 根总控合并后通过现有本地/宿主蓝绿链发布；预检 `downtime_required=false` 时按全局规则继续，`true` 时停在人工门禁。
- 回滚到上一已验证镜像/提交即可恢复旧错误文案；无数据回滚。
- 线上专项验证由根总控使用不改生产数据的既有自然失败样本或受控请求完成，并同时确认真实上游 503 未回归。
- 无待决产品事项。

## 11. 批准记录

- 2026-08-18：唯一发布总控已在派发 T24 时明确批准范围、协议覆盖、保留上游真实 503、TDD/直接相关验证及 `READY_FOR_ROOT_REVIEW` 停点。
- 2026-08-18：本规格按既有代审授权完成自审。占位符、矛盾、范围漂移和未决项扫描均为零；选择方案 C。
