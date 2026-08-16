# OAuth 图片编辑上传 MIME 兼容热修设计

## 状态与批准

- 状态：已批准进入实施计划。
- 基线：`main@44095897d1bba3302c877431ba9bb5b6e356ab46`。
- 批准依据：用户在顶层任务交接中已明确批准最小范围、行为方向、测试矩阵与禁止事项；2026-08-15 起唯一发布总控具备既定队列内规格/计划代审授权。本规格不扩大该范围。
- 候选终态只能是 `READY_FOR_ROOT_REVIEW`；本任务不得修改根 `main`、全局队列/总账、远端、生产数据或生产分组。

## Goal / Context / Constraints / Done when

- **Goal**：修复 OAuth 账号处理 `/v1/images/edits` 时，把 multipart 图片转换为 Data URL 后保留 `application/octet-stream`，导致上游返回 400 `unsupported MIME type` 的问题。
- **Context**：事故命中 `user_id=34`、API key id `50`、`group_id=19`；API-key 账号链路正常，事故不是图片尺寸问题。2026-08-16 10:02 的生产临时规避是把 OAuth 账号 `222/223` 移出 `group_id=19`，本任务不改变该生产状态。
- **Constraints**：只修改 OAuth Images Responses 构造逻辑及最小相关 service 单测、规格、计划和复审报告；不改错误码、错误文案/中文提示、`ErrorPassthroughRule`、客户端表现、依赖、配置、迁移、前端、GitHub Actions、发布脚本或生产数据。
- **Done when**：空 MIME 与规范化后的 `application/octet-stream` 能从文件字节识别出真实 `image/*` 并生成正确 Data URL；识别失败或非图片继续走现有 request-build 拒绝路径；显式 `image/*` 原样保留；最小验证与独立复审通过。

## 问题证据与当前行为

`openAIImageUploadToDataURL` 当前只在 `ContentType` 为空时调用 `http.DetectContentType`。multipart 客户端常把文件部件声明为 `application/octet-stream`，因此 OAuth Responses 请求最终包含：

```text
data:application/octet-stream;base64,...
```

上游 Responses 图片输入只接受图片 MIME，因而在请求到达上游后返回 400。API-key 链路直接转发 multipart，不经过这一 Data URL 构造路径，所以不受影响。

现有同名 helper 还被 Grok 图片编辑路径调用。直接修改共享 helper 会扩大到其他上传链路，不符合本热修边界。

## 方案比较

### 方案 A：OAuth Responses 私有 MIME 规范化 helper（采用）

在 `openai_images_responses.go` 增加仅供 `buildOpenAIImagesResponsesRequest` 使用的严格 helper。它保留现有共享 helper 给 Grok 使用，仅在 OAuth Responses 的图片和 mask Data URL 构造前执行兼容逻辑。

- 优点：影响面最小；API-key、Grok 和 multipart 解析语义不变；可直接覆盖事故路径。
- 代价：同文件内存在一个 OAuth Responses 专用薄封装，但职责清晰且没有通用重构。

### 方案 B：修改共享 `openAIImageUploadToDataURL`（不采用）

- 优点：代码最少。
- 缺点：会同时改变 Grok 图片编辑上传行为，违反“不扩大到其他上传链路”。

### 方案 C：在 multipart 解析阶段改写 `OpenAIImagesUpload.ContentType`（不采用）

- 优点：下游统一看到已修正 MIME。
- 缺点：会改变 API-key、内容审核和其他消费者观察到的上传元数据，范围明显过大。

## 详细设计

### MIME 规范化与选择

OAuth Responses 专用 helper 接收 `OpenAIImagesUpload`：

1. `Data` 为空时保留现有空上传错误路径。
2. 对 `ContentType` 做首尾空白清理；用标准库 `mime.ParseMediaType` 获取用于比较的基础 media type。解析失败时不重写原值。
3. 当 MIME 为空，或基础 media type 大小写无关地等于 `application/octet-stream` 时，调用 `http.DetectContentType(upload.Data)`。
4. 只有最终用于校验的基础 MIME 为 `image/*` 才生成 Data URL。
5. 无法识别或识别为非 `image/*` 时返回错误，由 `buildOpenAIImagesResponsesRequest` 现有错误传播路径拒绝；不改 handler 的错误码、映射或中文提示。
6. 原请求已显式声明 `image/*` 时不嗅探、不重写，Data URL 中继续使用清理空白后的原始显式 MIME 字符串。

### 端到端数据流

```text
multipart upload
  -> ParseOpenAIImagesRequest（保持原样）
  -> OAuth account dispatch
  -> buildOpenAIImagesResponsesRequest
  -> OAuth Responses MIME helper
       explicit image/* ---------------> preserve
       empty/octet-stream -> sniff bytes -> accept only image/*
       unresolved/non-image ------------> existing build error path
  -> data:image/...;base64,...
  -> Responses upstream
```

图片输入和 mask 输入使用同一 OAuth Responses helper，避免两者行为分叉。

## 接口与错误契约

- 外部 HTTP 路径、请求字段和响应字段不变。
- API-key 账号链路不变。
- Grok 与其他上传链路不变。
- `ErrorPassthroughRule`、错误状态码、错误中文提示和 handler 映射不变。
- 新增的本地拒绝只覆盖此前会携带无效/非图片 Data URL 继续请求上游的输入；它复用既有 request-build error 返回链，不建立新错误类型。

## 兼容性、迁移与安全

- 无数据库迁移、配置、依赖、前端或发布脚本变化。
- 字节识别使用 Go 标准库，最多读取 `http.DetectContentType` 所需前缀，不增加网络或持久化操作。
- 显式 `image/*` 保持兼容，即使其字节不能被嗅探为图片，也不改变既有信任显式声明的行为。
- 非图片或无法识别的数据 fail closed，避免继续向上游发送错误 MIME。

## 验收矩阵

| 场景 | 输入 MIME | 文件字节 | 预期 |
|---|---|---|---|
| 空 MIME 图片 | 空 | PNG/JPEG 等图片 | 使用嗅探出的 `image/*` |
| 通用二进制图片 | `application/octet-stream` | PNG/JPEG 等图片 | 使用嗅探出的 `image/*` |
| 带参数的通用二进制图片 | `application/octet-stream; ...` | 图片 | 规范化后嗅探并使用 `image/*` |
| 空 MIME 非图片 | 空 | 文本/未知 | request-build 拒绝 |
| 通用二进制非图片 | `application/octet-stream` | 文本/未知 | request-build 拒绝 |
| 显式图片 | `image/png` 或其他 `image/*` | 任意非空字节 | 原 MIME 原样保留，不嗅探 |
| API-key 图片编辑 | multipart | 任意现有合法输入 | 行为不变 |

## 测试与最小验证

- 在现有 service 测试模块增加 table-driven 单测，覆盖空 MIME、`application/octet-stream`、带参数/大小写规范化、识别为图片、识别失败/非图片、显式 `image/*` 保持。
- 运行相关 `internal/service` 定向单测。
- 运行后端必要编译或类型检查。
- 运行 `gofmt`、`git diff --check`、文件范围检查和禁区检查。
- 不运行无关全仓测试、mutation、压力、长时间 soak、浏览器或其他模块验证。

## 发布、线上验证与回滚条件

- 本任务不发布。候选仅交给根发布总控。
- 预期无迁移/配置变化，`downtime_required=false`；最终值由根总控发布预检确认。
- 根合并后仅需 OAuth 与 API-key `/v1/images/edits` 定向验收及健康检查。
- 回滚方式是回退本热修代码提交；无数据回滚、分组回滚或迁移回滚。

## 待决事项

无。若实现需要修改共享 helper、multipart 解析、错误合同或其他上传链路，必须停止并报告根发布总控。

