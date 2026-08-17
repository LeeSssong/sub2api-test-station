# T15 账号监控原生探测模型与异步模型检测设计规格

状态：已批准设计，进入实现。批准依据：2026-08-16 brainstorming 合同；用户已明确“规格书不用我确认了”，本任务不再重复请求正式规格确认。

## 1. 问题证据与当前行为

- 原生连接测试路径已经存在：`AccountTestService.ProbeAccountConnection` 最终调用 `TestAccountConnectionWithProbeKind(..., ProbeKindMonitor)`，但 `AccountMonitorService` 通过 `monitorModelForAccount` 临时推导模型，无法按账号保存连接测试模型。
- `/admin/accounts/:id/models` 和 `POST /admin/accounts/:id/models/sync-upstream` 已是原生账号模型来源。同步结果由原生编辑器并入账号模型白名单；T15 不建立第二份模型登记事实。
- `AccountMonitorRunner` 当前使用可调间隔并立即运行所有 active+schedulable 账号，不能表达北京时区固定时隙、持久化去重或 OAuth/API Key 分流。
- 现有 `account_monitor_results` 只保存连接探测指标。模型检测需要独立异步任务、独立结果和独立状态，不能污染连接探测历史、评分或调度。
- 现有卡片已经有“近期探测”区域；T15 在该区域增加连接测试模型入口，并增加默认收缩的检测状态行与详情弹窗，不重做卡片主样式。

## 2. 目标

1. 保留并扩展 Sub 原生连接测试，按账号保存 `connection_probe_model`；默认优先 `gpt-5.6-sol`，不支持时回退账号原生登记的首个文本模型，UI 不展示“自动选择”。
2. 增加独立 `model_detection_model`，候选为原生账号登记模型与 sidecar 运行时基线目录的交集；没有基线的已登记模型仍显示但置灰“检测器暂不支持”。
3. 由 Sub worker 作为唯一调度和持久化事实源，sidecar 仅执行一轮探针并返回脱敏摘要。
4. 支持北京时间 `00:00/10:00/12:00/15:00/18:00/21:00` 固定时隙、持久化去重、30 分钟迟到补触发，以及按账号立即异步检测和同账号排队/运行复用。
5. 在账号卡片提供八种检测状态、最近结果、申报模型、Juice 摘要、行为指纹候选/相似度、检测器版本、时间/错误、修改检测模型和立即检测。
6. 明确安全边界：不保存 API Key、完整提示词、完整输出、上游地址；凭据只在私网内存请求 sidecar。sidecar 故障只生成“检测失败”，不影响连接测试、账号状态、评分、调度权重或分组建议。

## 3. 非目标

- 不复制或打包 `tools/gpt56_api_detector-git` 的 PolyForm Noncommercial 核心实现、基线或报告逻辑进入商业生产镜像；在取得商业书面授权或替换为独立合法实现前，生产检测器接入保持配置门禁。
- 不改变 `usage_logs`、`account_monitor_results`、余额、扣费、利润、质量评分、调度权重、可调度状态或分组建议。
- 不检测 OAuth 账号；不因账号不可调度而跳过 API Key 检测（删除、类型变化或模型失效时执行前跳过/回退）。
- 不构建全局检测摘要、不引入平行账号模型登记页面、不记录凭据或完整请求/响应。

## 4. 方案比较与选择

### 方案 A：把检测逻辑直接嵌入 Sub worker

优点是部署简单；缺点是把高风险探针逻辑、凭据处理和许可证不明的检测器代码并入核心，扩大故障面并违背“sidecar 仅执行”。不选。

### 方案 B：Sub worker 调度/持久化 + 私网 sidecar 执行（选定）

Sub 负责模型交集、任务去重、时隙补偿、状态机和 UI；sidecar 只接收一次性内存凭据和模型，返回不含凭据的结构化结果。sidecar 不可用时 fail-closed 为“检测失败/不支持”，不影响原生主路径。该方案保留 Sub 原生事实源并隔离探针风险。

### 方案 C：前端直接调用 sidecar

会暴露凭据、绕过 worker 唯一事实源并无法可靠去重。不选。

## 5. 端到端控制流

1. 管理员打开账号监控页。Sub 读取原生账号模型白名单和 `model_detection_settings`，向 sidecar 请求带缓存的运行时基线目录；以交集生成连接/检测模型选项。
2. 页面保存模型时调用 Sub admin API。Sub 只保存模型 ID，不保存凭据。
3. 管理员点击立即检测：Sub 事务性地插入或复用该账号的 queued run，返回 `queued`；worker 异步领取唯一 queued run。
4. 固定时隙 worker 使用 `Asia/Shanghai` 计算 slot key，事务性去重并只对未删除、API Key、存在可检测模型的账号插入 scheduled run；错过时隙仅在 30 分钟内补触发。
5. worker 领取任务后从原生账号存储读取凭据，仅在私网 HTTP 请求生命周期内发送 sidecar；请求体只含一次性凭据、模型、申报模型和 run ID，不含完整提示词/输出持久化字段。
6. sidecar 返回状态、Juice 摘要、行为指纹候选/相似度和版本。Sub 对字段做长度/枚举校验、脱敏和状态映射后持久化；异常响应或 sidecar 超时保存 `failed`。
7. UI 轮询/刷新账号监控 projection，状态行默认收缩；详情弹窗按需读取最近结果。

## 6. 数据与接口契约

### 6.1 数据库

新增 migration `225_account_model_detection.sql`：

- `account_model_detection_settings(account_id PK REFERENCES accounts(id) ON DELETE CASCADE, connection_probe_model TEXT NOT NULL DEFAULT '', model_detection_model TEXT NOT NULL DEFAULT '', updated_by BIGINT NOT NULL DEFAULT 0, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`。
- `account_model_detection_runs(id UUID PK, account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, slot_key TEXT, trigger_kind TEXT CHECK (manual/scheduled), model_id TEXT NOT NULL, claimed_model TEXT NOT NULL, status TEXT CHECK (queued/running/normal/abnormal/insufficient/failed), juice_status TEXT, juice_summary JSONB, fingerprint_candidate TEXT, fingerprint_similarity JSONB, detector_version TEXT, error_code TEXT, error_message TEXT, queued_at, started_at, finished_at, created_at)`。
- 唯一约束：`(account_id, slot_key)`（slot 非空）；同账号 queued/running 部分唯一索引。
- 结果表只保存 bounded JSON 摘要；任何字符串字段由 repository 截断，禁止保存凭据、完整提示词、完整输出、上游地址。

### 6.2 sidecar HTTP

- `GET /v1/catalog` -> `{version:string, models:[{id:string, supported:bool}]}`；失败视为无可用基线。
- `POST /v1/detect`，私网 mTLS/共享令牌由宿主配置；请求：`{run_id, declared_model, request_model, api_key, base_url}`。`api_key/base_url` 仅内存传输，不进入 Sub 日志、数据库或响应。
- 响应：`{status:"normal"|"abnormal"|"insufficient", juice_status, juice_summary, fingerprint_candidate, fingerprint_similarity, detector_version, error_code}`。Sub 严格校验枚举和长度。

### 6.3 Sub admin API

- `GET /admin/account-monitors/:account_id/models`：返回连接测试模型和检测模型候选、选中值、`supported` 与 `reason`。
- `PUT /admin/account-monitors/:account_id/models`：仅允许保存来自原生登记模型的 ID；检测模型必须属于 sidecar 基线交集。
- `POST /admin/account-monitors/:account_id/detection`：异步 enqueue/reuse，返回 `{status, run_id}`。
- `GET /admin/account-monitors/:account_id/detection`：返回最近 25 条 bounded 结果。
- 现有 `GET /admin/accounts/monitor` projection 增加 `connection_probe_model`、`model_detection`（状态、选项、最近摘要）字段；兼容旧字段不删除。

## 7. 模型选择规则

- `connection_probe_model`：若原生登记文本模型包含 `gpt-5.6-sol`，默认选中它；否则选择原生登记排序后的首个文本模型。已保存值若仍登记且可用则保留，否则按上述规则回退。
- `model_detection_model`：已保存值若仍在“原生登记 ∩ sidecar catalog”中则保留；否则选择交集首项；无交集返回空并显示不支持。
- OAuth 账号的检测模型选项全部不可执行；连接测试仍走现有 OAuth 原生路径。

## 8. 失败、安全和兼容性

- 账号删除/转 OAuth/模型被移除：执行前事务性跳过或回退，不创建 sidecar 请求。
- 同账号 queued/running：立即检测复用现有 run，不创建第二个请求。
- sidecar 连接失败、超时、非 2xx、响应不合法：保存 `failed` 与稳定 `detector_unavailable`/`detector_invalid_response`，不改账号状态。
- 凭据只在 worker 内存和私网请求存在；日志只记录 account ID、run ID、状态、耗时和稳定错误码。
- 任何模型检测结果不参与原生评分、排序、调度、余额、利润、组建议。
- 配置缺失或未取得商业许可时，catalog 为空，UI 显示 `不支持`，不会阻塞连接测试和账号监控主页面。

## 9. 场景化验收矩阵

| 场景 | 预期 |
|---|---|
| API Key 账号含 gpt-5.6-sol | 连接测试默认选 Sol；检测交集可执行 |
| 无 Sol 但有文本模型 | 连接测试回退首个文本模型；页面不出现自动选择 |
| 原生登记模型无 sidecar 基线 | 模型可见但置灰不支持 |
| OAuth 账号 | 不创建检测 run；连接测试保持原生 |
| 重复立即检测 | 第二次复用同一 queued/running run |
| 固定时隙重复执行 | 同一 `(account, slot)` 只一条 run |
| 时隙延迟 30 分钟内 | 补触发一次；超过窗口不补 |
| sidecar 超时/坏响应 | run=failed；账号主状态/评分/调度不变 |
| 成功返回 normal/abnormal/insufficient | UI 显示对应中文状态和 bounded 摘要 |
| 删除或模型失效后执行 | 跳过或按规则回退，不发送凭据 |
| 390px 页面 | 无横向滚动，状态行收缩，弹窗可关闭 |

## 10. 测试与发布

- backend：model selection、repository CAS/去重、slot 计算、sidecar request redaction/response validation、handler 状态码和不影响主状态测试。
- migration：225 文件契约测试。
- frontend：`AccountMonitorCard.spec.ts`（状态行、弹窗、模型入口）与 `AccountMonitorView.spec.ts` API wiring；frontend typecheck/build。
- 只运行上述直接相关测试，不运行全仓、压力、mutation 或无关浏览器矩阵。
- 发布仍只能从合并并验证后的根 `main` 进入既有本地/宿主蓝绿链；本候选当前明确不部署，必须等待下一次用户部署指令。若未来预检输出 `downtime_required=true`，停在授权门禁。

## 11. 许可证门禁与待决事项

`tools/gpt56_api_detector-git` 当前声明 PolyForm Noncommercial License 1.0.0，禁止商业使用。T15 Sub 侧协议、调度、存储、UI 和 fake sidecar 合同可先完成；将其核心实现或基线复制到生产 sidecar 前必须取得书面商业许可，或由独立实现替换。该门禁不改变本候选的代码测试完成条件，但会使未配置合法 detector 的部署显示“不支持/检测失败”。
