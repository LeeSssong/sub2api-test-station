# T40 错误码/边缘错误中文映射补齐规格

## 状态与批准

- 任务：T40，范围限定为应用侧原生错误投影与管理员诊断扩展；候选最终只能到 `READY_FOR_ROOT_REVIEW`。
- 基线：最新干净 `main@c1ac170c2dcbed9887904a0c48fef61e726401ab`，包含已部署 T39 Responses 413 语义。
- Brainstorming 分类：architectural（跨 JSON/SSE 用户契约、管理员诊断和安全边界，但复用既有原生组件，不引入新子系统）。
- 现状探索：`ProjectNativeUserError` 是 JSON/通用 SSE 的共享用户投影；Responses SSE writer 已由 T39 保留原始 status/type/code/message 并单次投影；`ProjectNativeErrorDiagnosis`/`AttachNativeErrorDiagnosis` 是管理员错误列表与详情的共享诊断和脱敏边界。
- 根总控代审授权：依据项目约束 2.3 及当前顶层任务指令，唯一发布总控可代为审查批准既定队列任务的规格与计划。本规格按该授权记录为 `APPROVED_BY_ROOT_RELEASE_CONTROLLER_PROXY`；无产品待决项、无范围扩大、无不可逆数据操作或停机决策。
- 规格自审：已检查占位符、内部一致性、范围漂移和歧义；无 `TBD`/`TODO`，Cloudflare 边缘 HTML 413、T39 已部署语义和 T38 设计均保持边界。

## 问题与目标

应用侧已能统一投影 400/401/403/413/429/5xx，但 402、507 及 Cloudflare 常见 520–525 在正文/关键词命中时仍可能落入泛化错误；499 客户端断开也必须继续归入上传中断，而不是上游失败。管理员需要在既有错误详情中看到可审计的阶段、归属、原始状态和脱敏证据，普通用户只能获得稳定中文含义和建议。

目标：

1. 对已进入应用的 JSON 与 SSE 错误，按 HTTP 状态、错误类型/代码和正文关键词投影 402、507、520、521、522、523、524、525 的稳定中文消息；状态优先于已选账号的泛化。
2. 保留 499 的客户端断开/上传中断分类，且不把它误判为上游 5xx。
3. 管理员诊断在原有列表/详情边界中补充稳定机器分类和状态依据，继续脱敏 URL、Ray/request ID、密钥、完整上游响应；不改变管理员既有入口和数据源。
4. 明确未进入 Gin/应用的 Cloudflare HTML 413 不属于可投影输入，不作猜测、不回显、不泄露。

## 非目标与边界

- 不处理或改写边缘直接生成的 HTML 413；不会在应用中通过 HTML 文案猜测 Cloudflare 状态。
- 不新增数据库字段、迁移、配置、重试、调度、账务事实源、外部控制面或 GitHub Actions。
- 不改变 T39 的 413 单次投影、Responses `response.failed` 终止事件或其他任务的 499/413 语义；不实现前端页面。
- 只处理响应已进入应用的状态码。应用未收到响应时沿用现有网络/上游错误分类。

## 方案比较

### 方案 A（采用）：共享投影优先级 + 诊断状态证据

在 `ProjectNativeUserError` 中增加明确状态/关键词分支：402 余额/支付、507 资源不足、520–525 网关/连接/SSL 故障；分支位于已选账号泛化之前。同步在 `native_error_diagnostics.go` 中以原始上游状态和正文 marker 归入既有 `upstream_overloaded` 或 `upstream_failed`，499 在 request/upload 阶段保持 `upload_interrupted`。JSON、通用 SSE 和 T39 Responses SSE 继续共享投影。

优点是最小改动、契约单一、管理员与用户使用同一原生证据；风险仅为 marker 优先级需测试锁定。

### 方案 B：按状态码新增独立错误类和 API 字段

为每个状态新增诊断 class 和前端字段。可表达更细，但会扩大管理员 API 契约、前端消费面和兼容风险，超过任务边界，拒绝。

### 方案 C：在各 handler 旁路手写 JSON/SSE 文案

改动局部但会复制投影、安全过滤和协议终止逻辑，容易再次出现 T39 的二次投影问题，拒绝。

## 端到端控制流

1. Handler 或上游适配器得到 status/type/code/message；若请求尚未进入应用（例如边缘 HTML 413），本链不运行。
2. JSON 与通用 SSE 调用 `ProjectNativeUserError`；Responses SSE 由 T39 writer 在唯一投影点调用。
3. 投影按“确定性状态/marker -> 本地限制 -> 鉴权/权限 -> 频率 -> 已选账号上游泛化 -> 默认”顺序执行；用户消息只使用固定中文字典。
4. 管理员列表/详情调用 `AttachNativeErrorDiagnosis`。诊断读取原始上游状态与正文，仅输出既有 `class/stage/ownership` 和再次脱敏的证据；普通用户响应不包含诊断字段。

## 用户消息与机器契约

| 输入 | 用户中文含义 | 建议 | 机器保留 |
| --- | --- | --- | --- |
| 402 / payment / balance / quota billing marker | 余额或额度不足，请充值或检查额度后重试。 | 检查余额/额度 | 原 type/code |
| 507 / insufficient storage / capacity exhausted（应用已收到） | 服务资源暂时不足，请稍后重试。 | 稍后重试 | 原 type/code |
| 520 / unknown web server error | 服务暂时异常，请稍后重试。 | 稍后重试 | 原 type/code |
| 521 / web server is down / connection refused | 上游服务暂时不可用，请稍后重试。 | 稍后重试 | 原 type/code |
| 522 / connection timed out | 连接上游服务超时，请稍后重试。 | 稍后重试 | 原 type/code |
| 523 / origin unreachable | 上游服务暂时不可达，请稍后重试。 | 稍后重试 | 原 type/code |
| 524 / a timeout occurred / origin timeout | 上游服务处理超时，请稍后重试。 | 稍后重试 | 原 type/code |
| 525 / SSL handshake failed | 上游安全连接失败，请稍后重试。 | 稍后重试 | 原 type/code |
| 499 / client closed / upload interrupted | 请求上传中断，请检查网络后重试。 | 检查网络 | `upload_interrupted` |

关键词匹配大小写不敏感，覆盖状态常见英文短语及既有中文 marker；正文只用于分类，不原样拼接到用户消息。若同一输入同时包含多个 marker，状态码优先，其次 499 上传中断，再按确定性 402/507/520–525 顺序，最后走既有分类。

## 管理员诊断与安全

- 402 归入既有本地额度/计费限制（未选账号）或已选账号上游失败（若证据明确来自上游），不凭状态单独暴露账号。
- 507 归入 `upstream_failed`，除非明确是未选账号的本地资源耗尽并命中现有 `local_capacity_exhausted` 规则。
- 520–525 归入 `upstream_failed`；522/524 可保留现有 overload/timeout marker 的 `upstream_overloaded` 结果，仅在已有证据满足条件时升级，不新增 class。
- 499 在 request 阶段或正文含 client-closed/upload-interrupted 时归入 `upload_interrupted`，stage=`request`、ownership=`client`；不因存在代理字段而改成 provider。
- 管理员可见的 `OriginalUpstreamStatus/Message/Detail` 沿用现有长度限制和 secret/URL/Ray/request ID 脱敏；用户 JSON/SSE 只含固定中文消息、机器 type/code，不含状态正文、URL、账号、密钥或 Cloudflare HTML。

## 兼容性、迁移与发布

仅扩展内部投影和诊断分支及其测试；无外部 API 字段新增、无迁移/配置/依赖/生产数据变化。预期 `downtime_required=false`，最终以根总控合并后发布预检为准。候选不得自行合并、推送、部署或访问生产；根总控从验证后的 `main` 使用既有本地/宿主蓝绿链发布。回滚为回退候选提交并沿发布链恢复上一已验证镜像，无数据回滚。

## 场景化验收矩阵

- JSON：402、507、520–525 每个状态均返回既有 envelope、固定中文消息且不含敏感正文。
- SSE：同一状态在通用 SSE 与 Responses `response.failed` 中保持中文语义；Responses 终止事件唯一，T39 413 不回归。
- 关键词/正文：无标准状态但命中每组 marker 时得到同一映射；大小写和中英文正文均覆盖。
- 499：客户端断开/上传中断保持 `upload_interrupted`，用户收到上传中断建议。
- 管理员：列表与详情 class/stage/ownership/status 一致，原始证据脱敏；普通用户不可见诊断字段。
- Cloudflare HTML 413：无应用 handler 输入、无新增测试伪装，明确不误判、不泄露。

## TDD 与验证策略

先在 service 投影测试和 native diagnostics 测试加入 RED：已选账号 402/507/520–525、关键词正文、499 分类和敏感正文不得泄露。再在 handler 直接测试 JSON/SSE envelope 与 Responses SSE 终止合同；确认失败原因是缺少映射后实现最小分支。GREEN 后运行直接相关 Go 测试、`go build ./cmd/server`、`gofmt`、`git diff --check`，不运行全仓/前端/浏览器/生产验收。

## 仍待决事项

无。根总控在合并后的 `main` 决定最终发布预检和线上专项验收；边缘 HTML 413 永远不作为应用验收样本。
