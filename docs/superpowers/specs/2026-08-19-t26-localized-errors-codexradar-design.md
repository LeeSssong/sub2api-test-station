# T26 用户错误中文投影与 CodexRadar 站长推荐设计

## 1. 问题证据与当前行为

基线为根 `main@418aa4303687d905b6b891b8263be87cdc4ab20c`。现有 Sub 已有可复用的原生能力：

- `internal/service/native_error_diagnostics.go` 已按请求阶段、归属、是否选中账号、状态码和原始证据形成管理员诊断，并在读取边界脱敏凭据；它是错误分类与管理员证据的既有事实源。
- `GatewayHandler` 与 `OpenAIGatewayHandler` 已集中处理 Anthropic、Chat Completions、Responses 的 JSON 与流内错误；`writeResponsesFailedSSE`、`buildChatStreamErrorSSE`、`buildAnthropicStreamErrorSSE` 已提供协议正确的终结帧。
- 当前这些写入链仍会把 `Invalid API key`、`Insufficient balance`、`Upstream ...`、透传的英文响应体、URL、Cloudflare Ray/request id 等内容直接交给普通用户。
- `MonitorV2View.vue` 已在“分组状态”区域渲染原生绿色监控卡；页面尚无外部站长推荐区。
- 2026-08-19 实测 `https://codexradar.com/api/radar-insights` 返回 schema 1、四个 `recommendations` 分类，以及 `model`、`effort`、`iq`、`average_duration_minutes`、`average_cost_usd`、分类/条目 `rule`、`generated_at` 与来源更新时间；响应未返回浏览器 CORS 许可。

因此采用“扩展原生错误投影 + Monitor V2 固定只读代理”的方式，不建设平行错误事实源、监控事实源或本站推荐算法。

## 2. 目标与非目标

### 目标

1. 所有用户侧网关错误先经过统一中文投影，再写入 Anthropic、Chat Completions、Responses 的 JSON/SSE 协议。
2. 本站余额不足精确显示：`余额不足，请充值后重试。`
3. 本站额度/订阅、认证、频率/并发、模型/分组权限、请求格式、资源耗尽与内部服务异常分别给出稳定、可操作的中文语义。
4. 内部服务账号余额、外部服务异常、原始 URL、内部账号/服务商、Cloudflare Ray、request id 与原始英文响应体不进入用户响应；管理员诊断继续保留脱敏后的阶段、归属、状态与原始证据。
5. 在 Monitor V2 分组卡下方显示 CodexRadar 的四类站长推荐，并严格使用其公开 DTO 与原始规则。

### 非目标

- 不改变管理员诊断字段、错误日志采集、计费、重试、调度或错误透传规则匹配事实。
- 不把本站监控、模型、成本、评分或推荐逻辑混入 CodexRadar 数据。
- 不持久化第三方数据，不向第三方写入，不实现通用代理。
- 不新增迁移、生产数据写入、GitHub Actions 或发布操作。

## 3. 方案比较

1. **逐调用点替换英文**：改动分散，容易遗漏流式终结与透传路径，且形成多个文案事实源。
2. **扩展原生诊断分类并在共用写入边界投影（采用）**：分类复用 `native_error_diagnostics` 的标记与语义；协议 writer 只负责把统一投影写成各自 envelope，覆盖面完整且改动最小。
3. **前端或客户端翻译**：API/SSE 使用者仍能看到原始内容，无法满足网关级隐藏要求。

推荐代理方面采用固定服务：后端构造唯一固定 URL，GET-only，校验响应后返回精简 DTO；短缓存与最近成功内存快照共用同一服务。相比浏览器直连可解决 CORS；相比数据库落地没有迁移、陈旧数据和第二事实源。

## 4. 统一错误投影合同

新增原生用户投影函数，输入至少包含 HTTP 状态、错误类型/code、原始 message、阶段/归属/是否选中内部账号（可从既有上下文取得）；输出仅包含协议兼容的 `type/code/message`。分类优先级：

| 分类 | 识别证据 | 用户消息 |
|---|---|---|
| 本站余额不足 | 本地 billing/insufficient balance，且不是已选中内部账号后的服务错误 | `余额不足，请充值后重试。` |
| 本站额度/订阅 | local quota/usage/subscription | `额度或订阅不可用，请检查当前套餐后重试。` |
| 认证 | 401、authentication、invalid API key/token | `认证失败，请检查 API Key 后重试。` |
| 频率/并发 | 429、rate/concurrency/pending/RPM | `请求过于频繁，请稍后重试或降低并发。` |
| 模型/分组权限 | whitelist/not allowed/restricted/unsupported/permission | `当前模型或分组不可用，请调整后重试。` |
| 参数/请求格式 | 400/413、invalid request、parse/body/context | `请求参数或格式不正确，请检查后重试。`；过大上下文可显示 `请求内容过大，请缩短内容后重试。` |
| 本站资源耗尽 | local capacity/no available account/routing platform | `服务暂时繁忙，请稍后重试。` |
| 内部账号余额或外部服务异常 | 已选中内部账号、upstream/network/account_auth、5xx/529 或包含外部余额/Cloudflare 证据 | `服务暂时异常，请稍后重试。` 或繁忙语义 |
| 未识别服务错误 | 其它 5xx/upstream/server/api error | `服务暂时异常，请稍后重试。` |

用户消息必须经过最后的敏感标记守卫；任何包含 `upstream`、`上游`、URL、Ray ID、request id、内部账号/服务商名或残余英文原文的结果都回退到固定中文服务异常文案。错误 `type/code` 保持协议兼容与机器可分类，不承载内部身份信息。

管理员路径继续调用 `AttachNativeErrorDiagnosis`，保留其经脱敏的 `Stage/Ownership/OriginalUpstreamStatus/OriginalUpstreamMessage/Detail`；用户投影不修改或覆盖这些证据。

## 5. 协议与控制流

1. 业务/透传/失败切换代码先记录原始 ops 证据。
2. `GatewayHandler`、`OpenAIGatewayHandler` 进入共用错误 writer 前调用用户投影。
3. Anthropic JSON 保持 `{type:"error",error:{type,message}}`；Anthropic SSE 保持 `event:error`。
4. Chat JSON 保持 `{error:{type,code?,message}}`；Chat SSE 保持 `data:{error:...}`。
5. Responses JSON 保持 `{error:{code/type?,message}}`；Responses SSE 保持 `event:response.failed`，但 `response.error.message` 使用中文投影。
6. 服务层已经生成的 Chat/Anthropic SSE 错误 helper 同样调用投影，避免原始 `response.failed` 英文在协议转换后泄漏。

## 6. CodexRadar 后端合同

- 用户只读路由：`GET /api/v1/monitor-v2/codexradar-insights`，沿用 Monitor V2 登录态与 panel heavy rate limit。
- 远端目标编译期固定为 scheme `https`、host `codexradar.com`、path `/api/radar-insights`；handler 不接受 URL、host、path、method 或请求体参数。
- HTTP 客户端总超时 3 秒；仅接受 2xx JSON；响应体上限 512 KiB。
- 严格接受 `schema=1`、恰好四个唯一 key：`daily_development`、`hard_problems`、`background_automation`、`lobster_tasks`。每类 1–2 项；字符串有长度上限；时间为 RFC3339；数字必须有限且非负；未知关键字段可忽略，但缺失/类型错误/额外分类/超限均 fail closed。
- 对前端返回精简 DTO：`generated_at`、`source_updated_at`、四类 `{key,title,rule,items:[model,effort,iq,average_duration_minutes,average_cost_usd,rule]}`，不返回趋势、样本或其它站点数据。
- 成功结果缓存 60 秒；过期后远端失败或非法响应时返回最近成功快照并标记 `stale=true`。进程启动后尚无成功快照时返回 503 固定中文不可用消息。
- 缓存仅在内存中，进程重启即清空；不提交第三方评分或发起任何写操作。

## 7. 前端视觉与交互合同

- `MonitorV2View` 在分组卡列表/空状态之后、页面结束之前插入 `CodexRadarRecommendations`。
- 深色容器与截图一致：标题“站长推荐”、说明、更新时间；四列卡分别使用绿色、琥珀、紫色、蓝色边框/标题色，原生监控卡继续使用 Sub 绿色体系。
- 每类卡显示原始 title/rule；每项显示模型短名与档位、IQ、整数分钟/一位小数分钟、美元金额；不改写模型或档位，不补本站数据。
- loading 使用四卡骨架；无快照且 503/合同错误时显示一行简洁“站长推荐暂时不可用”，不影响 Monitor 主体；stale 快照显示“最近成功数据”。
- 桌面 `xl:grid-cols-4`；390px 为单列，或卡片容器内受控横向滚动。页面根不得横向溢出，长规则可换行。
- 色板：日常开发 `#34d399`、难题攻坚 `#fbbf24`、后台自动化 `#a78bfa`、跑龙虾类任务 `#60a5fa`；背景与边框复用现有 dark tokens。签名元素是四类卡左侧高饱和色轨，与截图一致但不污染原生监控状态色。

## 8. 失败、安全与兼容性

- 远端超时、非 2xx、超大响应、JSON/字段变化均不进入缓存；有快照则回退，无快照则 503。
- 固定 URL、GET-only、无透传查询/头/请求体，避免开放代理与 SSRF。
- 前端推荐加载失败不触发 Monitor V2 fatal fallback，不切回官方监控页。
- 现有 Monitor V2 snapshot 合同版本不变；推荐使用独立 endpoint，便于关闭/回滚且不影响主监控。
- 无迁移、无配置变化；预期 `downtime_required=false`，以根发布预检为准。

## 9. 验收矩阵与测试策略

- 错误分类：本站余额、本站额度/订阅、认证、频率/并发、模型/分组、参数/格式、资源耗尽、内部服务余额/服务异常。
- 隐藏：英文原体、URL、Cloudflare Ray、request id、内部账号/服务商、“上游”均不出现在用户 JSON/SSE；管理员诊断仍有脱敏证据。
- 协议：Responses、Chat Completions、Anthropic 的 JSON 与 SSE/流式终结均覆盖。
- 代理：成功、超时、非 2xx、超大/非法字段、缓存命中、过期后最近成功回退、无快照 503、固定目标与 GET-only。
- 前端：真实 DTO 四分类、模型/档位、IQ、耗时、金额、更新时间、loading/error/stale；1440 与 390 无整页溢出。
- 只运行直接相关 Go/Vitest、必要 Go compile/build、前端 typecheck/build、gofmt、`git diff --check` 与两视口 Monitor Playwright。

## 10. 发布、回滚、待决事项与批准

- 本任务只提交候选与 handoff，停在 `READY_FOR_ROOT_REVIEW`；根发布总控负责合并、推送、预检、部署和线上验收。
- 回滚为撤销 T26 候选提交/上一蓝绿槽；推荐 endpoint 与组件可整体移除，不影响 Monitor 主体。
- 待决事项：无。第三方未来新增分类或改变字段时按 fail closed 处理，不在本任务内兼容推测字段。
- 批准记录：用户于 2026-08-19 明确批准本规格所述两项范围，并再次指示无需重复询问批准，完成规格自审后立即 writing-plans 与 TDD。

## 11. 规格自审

- 原生复用：已明确复用 `native_error_diagnostics`、既有错误分类、协议 writer、Monitor V2 页面和路由鉴权。
- 数据边界：CodexRadar DTO、缓存、失败回退与本站监控完全分离。
- 安全闭环：固定目标、GET-only、超时、大小/字段校验、无写操作、无开放代理。
- 协议闭环：三种协议的 JSON 与 SSE 均有写入点和验收项。
- 范围闭环：无迁移、无生产写入、无全仓扩验、无未决产品问题。
