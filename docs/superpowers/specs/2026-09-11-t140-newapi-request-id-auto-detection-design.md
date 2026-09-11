# T140 NewAPI 请求 ID 自动识别与原生倍率优先设计

## 1. 问题与当前行为

当前 `upstream_request_id.go` 只读取账号 `extra.upstream_request_id_header` 显式指定的响应头。未配置时，即使上游返回 `X-Oneapi-Request-Id`，也不会写入 `usage_logs.upstream_request_id`，后续 NewAPI `/api/log/token` 无法精确匹配用量记录。

现有倍率注册已经具备主要门禁：账号必须是 API Key、开启倍率同步、不是手工倍率模式、存在可信 NewAPI 身份、请求 ID 和匹配的 `other.group_ratio`，且 Sub 原生倍率探测不能为 `ok`。本次不另建倍率系统，只补齐请求 ID 自动识别并收紧原生优先语义。

## 2. 目标与非目标

目标：

- 显式响应头配置继续拥有最高优先级。
- OpenAI API Key 使用非官方自定义 BaseURL，或已有可信 NewAPI 身份时，自动读取 `X-Oneapi-Request-Id`。
- 自动倍率接管始终先走 Sub2API 原生逻辑；只有原生明确 `unsupported` 时，NewAPI 才能兜底。
- 不新增迁移、数据库表、配置批量写入或历史回填。

非目标：

- 不自动为所有账号开启倍率同步。
- 不批量修改现有账号倍率或 `extra`。
- 不把任意 OpenAI 兼容中转站直接认定为 NewAPI 账本。
- 不通过管理页面或浏览器实现、验证本功能。

## 3. 方案比较

### A. 保守自动识别（采用）

显式配置优先；未配置时，自定义 OpenAI BaseURL 或可信 NewAPI 身份只触发读取 `X-Oneapi-Request-Id`。BaseURL 本身不授予倍率写入权限，NewAPI 写倍率仍要求原生探测明确 `unsupported` 和既有身份、同步门禁。

优点是覆盖新账号并避免把 BaseURL 当成计费身份。非 NewAPI 兼容站即使返回同名头，也只会形成请求 ID 观测，不会接管倍率。

### B. 所有 OpenAI API Key 均读取

覆盖最广，但官方 OpenAI 和其他中转站也会进入观测范围，不符合保守要求。

### C. 仅已有 NewAPI 身份读取

最安全，但首次原生探测完成前无法记录请求 ID，可能延迟一个自然请求周期。

## 4. 端到端控制流

1. 上游响应返回后，`UpstreamRequestIDHeaderName` 计算有效响应头名。
2. 若账号有非空显式 `upstream_request_id_header`，直接使用显式值。
3. 否则，仅当账号是 OpenAI API Key，且满足以下任一条件时返回 `X-Oneapi-Request-Id`：
   - `account_monitor_balance.source == newapi`；
   - Sub 原生倍率探测状态为 `unsupported`；
   - `credentials.base_url` 是非官方 OpenAI 自定义地址。
4. 请求 ID 按既有规则裁剪并写入用量行；不将推断结果写回账号 `extra`。
5. 用量落库后的倍率链先检查 Sub 原生倍率结果：
   - `status=ok`：以原生倍率为准，禁止 NewAPI 覆盖；
   - `status=unsupported`：允许进入 NewAPI 兜底；
   - 失败、未知、尚未探测等其他状态：不猜测为 NewAPI，不写倍率。
6. NewAPI 兜底仍须满足同步开启、非手工模式、可信身份、精确日志匹配和合法 `group_ratio`，再通过现有 CAS 更新倍率。

## 5. 接口与字段契约

保持现有公开接口与字段不变：

- `UpstreamRequestIDHeaderName(account *Account) string`
- `UpstreamRequestIDFromHeaders(account *Account, h http.Header) string`
- `extra.upstream_request_id_header`
- `usage_logs.upstream_request_id`

新增内部、可测试的判定函数：

- `isAutomaticOneAPIRequestIDEligible(account *Account) bool`
- `isCustomOpenAIBaseURL(account *Account) bool`

自定义 BaseURL 定义为：OpenAI API Key 账号存在有效 `credentials.base_url`，且解析后的主机不是 `api.openai.com`。空 BaseURL 按官方默认地址处理；OAuth、非 OpenAI 平台均不自动读取。

可信 NewAPI 身份继续复用现有 `newAPIRateRegistrationIdentity`，不新增身份字段。

## 6. 失败与安全语义

- 响应头缺失或为空时不影响请求和用量落库。
- 超长请求 ID 继续按列宽安全截断；WebSocket 无响应头时继续不记录。
- BaseURL 自动识别只选择要读取的头，不授权 NewAPI 倍率写入。
- 原生探测临时失败、鉴权失败或网络失败不等于 `unsupported`，不得切到 NewAPI 倍率。
- 原生 `ok` 永远阻止 NewAPI 覆盖，即使同时存在 NewAPI 请求 ID 和日志记录。
- 不记录 API Key、完整上游响应体或其他凭据。

## 7. 兼容性与迁移

无需数据库迁移、配置迁移或历史回填。已有显式头配置行为不变；官方 OpenAI 默认地址不自动采集；符合条件的账号从下一次自然请求开始记录请求 ID。

## 8. 验收矩阵

| 场景 | 自动读取 | NewAPI 可写倍率 | 预期 |
|---|---:|---:|---|
| 显式自定义头 | 显式头 | 按原门禁 | 兼容旧行为 |
| 自定义 BaseURL、无可信身份 | OneAPI 头 | 否 | 只留观测 |
| `source=newapi` | OneAPI 头 | 原生 unsupported 时是 | 正常兜底 |
| 原生 `unsupported` | OneAPI 头 | 是 | 查询 `/api/log/token` |
| 原生 `ok` 且有 NewAPI 记录 | 可读取 | 否 | 原生倍率保持优先 |
| 原生临时失败或未知 | 可按 BaseURL 读取 | 否 | 不猜测切换 |
| 官方 `api.openai.com` API Key | 否 | 否 | 不误采集 |
| OAuth 或非 OpenAI 平台 | 否 | 否 | 无行为变化 |
| 同步开关关闭 | 可读取 | 否 | 不写倍率 |

## 9. 测试策略

严格按 TDD 实施：先增加失败测试并确认失败，再写最小实现。

- 请求 ID 单测：显式配置优先、自定义 BaseURL、可信身份、官方地址、空地址、OAuth、非 OpenAI、非法 BaseURL。
- 倍率门禁单测：原生 `ok` 阻止覆盖；原生 `unsupported` 允许兜底；临时失败/未知不写入；同步关闭和手工模式不写入。
- 执行直接相关 Go 测试、`gofmt`、`go build ./internal/service`、`go build ./cmd/server`、`git diff --check`。
- 若完整 service 测试被主线既有缺失符号阻断，只记录阻断，不改无关代码绕过。

## 10. 发布、验证与回滚

候选必须从最新、干净且已推送的 `main` 创建独立 worktree。候选完成后先合入并推送根 `main`，任何部署制品只能从该 `main` 构建。主站发布仍需用户明确授权。

发布后只通过 SSH 读取服务器配置、流水、日志和数据库只读结果完成验证，核对 `source_commit/source_tree/image_digest`、服务健康、自然请求的 `upstream_request_id` 和倍率注册结果；不使用页面读取。

回滚使用上一已验证蓝绿槽或 Git 前向修复，不删除业务数据。已落库的请求 ID 和历史倍率证据保留。

## 11. 批准记录与待决事项

- 用户于 2026-09-11 选择方案 1，并明确“优先走 Sub 原生逻辑”。
- BaseURL 采用保守规则：只触发请求头读取，不单独授予 NewAPI 倍率写入。
- 本规格待用户书面确认后进入实施计划和代码阶段。
