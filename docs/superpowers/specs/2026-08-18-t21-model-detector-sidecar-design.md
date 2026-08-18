# T21 生产模型检测 Sidecar 接入与离线状态纠正设计

## 1. 问题与现状证据

- T15 已建立 Sub 原生账号模型登记、持久化异步任务、worker 调度、`GET /v1/catalog` 与 `POST /v1/detect` sidecar 合同。
- 生产 `SUB2API_MODEL_DETECTOR_URL` 与 `SUB2API_MODEL_DETECTOR_TOKEN` 均未配置，宿主也没有 detector 容器或已安装合同实现。
- `infra/compose.yaml` 当前没有把两个环境变量传入 API 和 worker，即使宿主 `.env` 配置也不会生效。
- catalog 配置缺失、连接失败、响应无效和“sidecar 正常但某模型未收录”都被折叠为空 catalog。前端因此把所有模型都标为“检测器暂不支持”，误报了故障类型。
- T15 规格明确保留外部 sidecar 和许可门禁；T21 不把 `tools/gpt56_api_detector-git` 核心、基线或报告代码复制进 Sub 镜像。

## 2. 目标

1. 后端显式区分 `ready`、`unconfigured` 和 `unavailable` 三种 detector 状态。
2. 只有 catalog 成功时，某个已登记模型不在 catalog 中才显示“检测器暂不支持”。
3. 未配置时显示“检测服务未接入”；不可达、非 2xx 或 catalog 无效时显示“检测服务暂不可用”。
4. 将既有 detector URL/token 配置通过 Compose 同时传给 API 和 worker，不把 token 返回前端或写入日志。
5. 接入符合 T15 合同和许可门禁的现有 sidecar 后，catalog 至少返回一个受支持模型，且一次立即检测进入终态。

## 3. 非目标

- 不改动原生连接测试、账号状态、质量评分、调度、计费、盈利或分组建议。
- 不改 migration 225，不新增迁移、历史回填或生产业务数据写入。
- 不把 detector 凭据、API Key、base URL、完整提示词或完整输出持久化。
- 不让前端直连 sidecar，不新建平行账号或模型事实源。
- 不使用 GitHub Actions。

## 4. 方案比较

### 方案 A：前端从空 catalog 推断离线

改动小，但空 catalog 既可能是故障，也可能是 sidecar 正常返回空基线，无法保持语义正确。不选。

### 方案 B：Sub 显式传递 catalog 可用性（选定）

sidecar 客户端使用稳定错误区分“未配置”与“不可用”，service 缓存模型和状态，admin API/projection 返回显式状态。前端仅渲染 Sub 语义，sidecar 仍是可替换的外部执行器。这是最小且可测试的端到端改动。

### 方案 C：把 detector 嵌入 Sub API 进程

部署表面更简单，但会扩大核心镜像的凭据和故障面，并破坏 T15 的外部 sidecar 与许可门禁。不选。

### 方案 D：同一不可变镜像分发独立 sidecar 二进制（生产接入选定）

在 Sub 构建上下文内独立实现最小合同服务，但以 `/app/model-detector` 独立二进制、独立 Compose 服务和独立 HTTP 进程运行。API/worker 仍只通过 T15 私网 HTTP 合同访问，不导入 detector 包或共享运行时状态。这样复用既有镜像资格、校验与传输链，同时不复制 PolyForm 工具核心、基线或报告，也不增加未审查的外部制品发布链。

## 5. 契约与数据流

### 5.1 状态契约

- `detector_state=ready`：`GET /v1/catalog` 成功、JSON 合法，可以判断单个模型是否受支持。
- `detector_state=unconfigured`：Sub 进程没有有效 `SUB2API_MODEL_DETECTOR_URL`。
- `detector_state=unavailable`：已配置，但 catalog 请求超时、网络失败、非 2xx、超限或 JSON 无效。
- API Key 账号在 `unconfigured/unavailable` 时的 projection 状态分别为 `service_unconfigured/service_unavailable`。
- OAuth 账号继续为 `unsupported`；`ready` 时单个模型不在 catalog 中继续为 `detector_unsupported`。

`AccountModelDetectionModelsResponse` 和 `AccountModelDetectionProjection` 新增 `detector_state`；现有字段不删除。模型 option 的 `reason` 只使用 `detector_unsupported`、`detector_unconfigured` 或 `detector_unavailable`。

### 5.2 加载与缓存

1. API 或 worker 调用 `Catalog`。
2. URL 为空时客户端返回稳定 `detector_not_configured` 错误；其他请求失败映射为 `detector_unavailable`。
3. service 把 catalog 模型与 detector state 一起缓存 5 分钟；不使用过期模型假装 sidecar 仍可用。
4. 只有 `ready` 状态下才会选中检测模型和允许 enqueue。

### 5.3 Compose 与凭据

`infra/compose.yaml` 声明独立 `model-detector` 服务，使用与候选相同的已资格不可变镜像并以 `/app/model-detector` 命令启动，不发布宿主端口。共享 Sub 环境显式传递私网 URL 与 token，API 用于 catalog，worker 用于 catalog 和 detect。宿主执行器在全部迁移、容量和身份预检通过后才原子升级旧的已渲染 Compose；失败路径恢复旧 Compose 和密钥文件。token 在宿主生成并保存在受控 `.env`/0600 Compose 中，不进 Git、发布证据或 API 响应。

sidecar catalog 默认覆盖生产近 7 天已确认的 `gpt-5.6-terra`、`gpt-5.6-sol` 与 `gpt-5.4`，并允许运营环境通过 `MODEL_DETECTOR_MODELS` 显式替换。检测请求只在内存中使用账号 API Key/base URL，调用其 OpenAI 兼容 `/v1/models`：请求模型出现在上游目录时返回 `normal/match`，目录可用但缺失时返回 `abnormal/mismatch`，鉴权、网络或空目录返回 `insufficient`。

## 6. 前端交互

- 对话框在模型选择器上方显示一行状态：`unconfigured` 为“检测服务未接入”，`unavailable` 为“检测服务暂不可用”。
- 离线时检测模型选择器和“立即检测”禁用；连接测试模型仍可查看和保存。
- `ready` 时，只有不在 catalog 中的选项显示“检测器暂不支持”。
- 卡片状态行对应显示“检测服务未接入/暂不可用”，不改卡片其他探测、评分或操作。

## 7. 失败与安全语义

- catalog 失败只改 detector 状态，不修改账号行、不创建检测 run、不影响连接探测。
- 已排队 run 在 sidecar 变为不可用时继续按 T15 规则落为 `failed/detector_unavailable`。
- 错误返回稳定代码，不包含 token、API Key、base URL 或 sidecar 响应体。
- sidecar 不记录请求体、API Key 或 base URL；响应只含状态、候选模型、版本和稳定错误码。
- 生产实现为本任务独立代码，不复制 `tools/gpt56_api_detector-git`；未来替换为外部制品仍保持同一 T15 合同。

## 8. 验收矩阵

| 场景 | 预期 |
|---|---|
| URL 未配置 | API/projection 为 `unconfigured/service_unconfigured`，UI 显示“检测服务未接入” |
| URL 已配置但不可达 | API/projection 为 `unavailable/service_unavailable`，UI 显示“检测服务暂不可用” |
| catalog 正常且含 Terra/Sol/5.4 | 对应生产模型可选，立即检测可 enqueue 并进入终态 |
| catalog 正常但不含某登记模型 | 仅该模型显示“检测器暂不支持” |
| OAuth 账号 | 继续 `unsupported`，原生连接测试不变 |
| 离线状态下保存连接模型 | 连接模型可保存，检测模型不会被误选 |
| 窄屏 | 状态文案换行，对话框无横向溢出 |

## 9. 测试、发布与回滚

- 后端直接测试：sidecar 未配置/不可用/正常 catalog，service 状态缓存、projection、enqueue 门禁。
- 前端直接测试：三种 detector state、选项文案、按钮禁用与连接模型不受影响。
- sidecar 直接测试：鉴权、catalog、模型 URL 规范化、匹配/不匹配和上游失败终态。
- Compose/宿主合同测试：独立服务不发布端口；URL/token 同时进入 API 和 worker；旧生产 JSON Compose 只在所有只读预检通过后升级，失败恢复原文件。
- 只运行直接相关 Go/Vitest、受影响包 compile-only、前端 typecheck/build、gofmt 和 diff-check。
- 无数据库迁移，预期 `downtime_required=false`；仍以根 `main` 预检为准。回滚为上一已验证 Sub 镜像和删除宿主 detector 环境配置，不改数据。

## 10. 批准记录

用户于 2026-08-18 明确批准 T21 的目标、顺序与边界，并要求登记后直接开始。根发布总控根据全局代审授权确认本规格不扩大已批准范围，准予进入实施计划。
