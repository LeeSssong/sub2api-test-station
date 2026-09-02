# T118 Responses 流水请求元数据恢复规格

## 1. 任务与问题证据

**任务包：** T118

**问题：** 管理后台“用量明细”中，2026-08-30 之后新产生的 OpenAI Responses 流式成功流水大量显示 `入站: -`、`IP: -`；对照站点可显示 `/v1/responses` 和客户端 IP。前端组件本身已支持这两个字段，数据库 `usage_logs` 也已有 `inbound_endpoint` 与 `ip_address` 列。

**已核实证据：**

- 生产只读统计显示，近 90 分钟 `gpt-5.6-sol` 的 297 条 Responses 成功流水均为 `inbound_endpoint=NULL`、`ip_address=NULL`；同时段 Chat Completions 流水字段完整。
- 流式成功流水的缺失切点为北京时间 2026-08-30 16:27:54；此前 8 月 25–29 日流式成功流水两字段完整。
- 当前 Responses 成功路径在 `openai_gateway_handler.go` 中调用 `buildSuccessfulOpenAIUsageRecordInput` 后直接异步提交，未将请求元数据写入 `recordInput`。
- 当前 Messages 成功路径仍在异步提交前读取并赋值相同元数据；历史 Sub 原生 Responses 实现也采用该模式。
- 服务层 `OpenAIGatewayService.RecordUsage` 和仓储层已支持这些字段，前端仅对空值显示 `-`，因此根因在 Responses handler 的输入构造，不在查询或渲染。

## 2. 目标与非目标

### 目标

1. 恢复当前定制树中 Sub 原生 Responses 成功记账的请求元数据快照语义。
2. 对修复发布后新写入的 Responses 成功流水持久化入站端点、上游端点、客户端 IP、User-Agent、Session ID 和请求体哈希（其中可选值仍可为空）。
3. 保留当前 T113/T114 及其他定制字段和行为，包括 logical request、attempt、quota platform、请求级定价、安全重放和渠道归因。
4. 用直接回归测试防止 Responses 成功路径再次丢失请求元数据。

### 非目标

- 不回填或修改历史 `NULL` 流水；历史 IP 已无可靠来源。
- 不修改前端列、API 响应结构、数据库 schema、迁移、计费、余额、调度、重试、账号状态或上游协议。
- 不改变失败流水、failover 语义、WebSocket、Images、Embeddings、Messages 或其他入口的行为。
- 不新增 tracing 平台、第二事实源、生产数据修复脚本或真实上游流量。

## 3. 原生能力盘点与方案

Sub 原生能力已存在：`InboundEndpointMiddleware`/`GetInboundEndpoint` 标准化客户端路径，`ip.GetClientIP` 按现有可信代理规则取得客户端 IP，`usage_logs` 与 `RecordUsage` 已有对应字段。官方历史实现和当前 Messages 路径均在异步任务创建前完成快照。

方案比较：

1. **恢复原生采集与赋值模式（采用）**：在现有 Responses 成功路径同步提取并赋值元数据，同时保留定制字段。改动最小、语义与官方及 Messages 对齐。
2. **扩展公共构造器参数**：让成功输入构造器接收全部元数据。可减少遗漏，但会扩大签名和调用点，热修收益不足。
3. **服务层从 context 推导**：异步 worker 使用脱离 Gin 生命周期的 context，无法可靠取得原始请求信息，不能采用。

## 4. 端到端数据流与字段契约

请求处理链固定为：

`Gin 请求 -> 同步快照 -> OpenAIRecordUsageInput -> OpenAI RecordUsage -> UsageLog -> usage_logs -> 管理后台 UsageTable`

Responses 成功路径在 `submitOpenAIUsageRecordTask` 之前捕获：

| 输入字段 | 来源 | 持久化字段 | 语义 |
| --- | --- | --- | --- |
| 入站端点 | `GetInboundEndpoint(c)` | `inbound_endpoint` | 标准化客户端 API 路径，如 `/v1/responses` |
| 上游端点 | `resolveOpenAIUpstreamEndpoint(c, account, result)` | `upstream_endpoint` | 当前账号/响应实际使用的标准化上游路径 |
| 客户端 IP | `ip.GetClientIP(c)` | `ip_address` | 复用既有可信代理与脱敏边界，不直接信任任意转发头 |
| User-Agent | `c.GetHeader("User-Agent")` | `user_agent` | 请求头快照 |
| Session ID | `service.ExtractClientSessionID(c)` | `session_id` | 显式客户端会话标识，缺失时为空 |
| 请求体哈希 | `service.HashUsageRequestPayload(body)` | `request_payload_hash` | 语义哈希，不保存原始请求体 |

这些值必须在请求 goroutine 中捕获，异步任务只使用值快照，不访问 `gin.Context`。空值或提取失败保持既有空值语义，不阻断响应、记账或扣费。

## 5. 实现边界

- 主要运行时文件：`upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`。
- 直接相关测试：扩展该 handler 的成功输入/请求元数据合同测试，至少断言端点、上游端点、IP、User-Agent、Session ID 和请求哈希被传入 `OpenAIRecordUsageInput`；覆盖 Responses 成功路径，避免只测试构造器的安全字段。
- 不修改公共 DTO、前端组件、仓储 SQL、Ent schema 或 migration。
- 现有 Messages 路径作为行为对照，不复制其无关逻辑。

## 6. 失败、安全与兼容性语义

- 元数据是观测字段，不是计费或调度前置条件；提取为空时请求仍按现有流程完成。
- 客户端 IP 继续使用既有 `ip.GetClientIP`，不新增或放宽代理信任。
- 请求哈希继续只保存哈希，不落原始请求体、API Key、Authorization、Base URL 或其他敏感内容。
- 历史记录保持原值；读取端不伪造 `/v1/responses` 或 IP。
- 无数据库迁移、无配置变化、无 API 合同变化；回滚为恢复上一个已验证镜像/蓝绿槽。

## 7. 场景化验收矩阵

| 场景 | 预期 |
| --- | --- |
| 新 Responses 非流式成功 | 入站端点、上游端点、IP 等字段按快照写入 |
| 新 Responses 流式成功 | 与非流式相同，端点/IP 不为空时后台显示实际值 |
| Responses 经 `/responses` 或 Codex alias | 入站端点由原生标准化逻辑得到 `/v1/responses` |
| IP 不可得或代理字段缺失 | 仅 IP 为空，不阻断请求；其他字段仍写入 |
| 历史缺失流水 | 继续显示 `-`，不回填 |
| Messages 成功/失败、Responses 失败或 failover | 行为和字段合同不改变 |
| 计费、调度、重试、账号状态 | 数值与状态不发生变化 |

## 8. 测试策略

- 先增加失败回归（RED），证明 Responses 成功输入缺少元数据时测试失败。
- 实现单一修复后运行 handler 直接测试及相关 service 测试。
- 运行必要的 `go build ./cmd/server`、`gofmt`、`git diff --check`；按当前项目政策不扩大到无关全量测试。
- 通过源码合同确认没有修改迁移、前端、计费、调度和部署授权链。

## 9. 发布、验证与回滚

- 本规格只授权本地实现和直接相关验证，不授权合并、推送、验收站、主站或生产数据操作。
- 候选完成后进入 `READY_FOR_ROOT_REVIEW`，由发布总控依次审查、合入根 `main`、执行直接门禁、推送并按用户明确授权选择发布路径。
- 发布后只读核对：新流水的 Responses 端点/IP 字段、Messages 不回归、三项健康端点和容器健康；不制造额外业务流量或历史回填。
- 若发布预检返回 `downtime_required=true`，在任何停服/迁移/重启/切换前暂停并请求明确授权；若为 `false`，仍需满足项目规定的主站授权路径。
- 失败回滚使用上一已验证蓝绿槽/镜像；不得修改历史流水以掩盖失败。

## 10. 待决事项与批准记录

- 历史空记录不回填：用户已明确确认。
- 采用 Sub 原生语义恢复而非整段回退旧源码：用户已明确确认。
- 2026-09-02：用户确认本修订设计，可进入书面规格审阅。
