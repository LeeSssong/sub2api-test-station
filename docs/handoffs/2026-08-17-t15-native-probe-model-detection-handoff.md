# T15 账号监控原生探测模型与异步模型检测交接

- 状态：`READY_FOR_ROOT_REVIEW`（实现和直接相关验证通过；未合并 `main`、未推送、未发布、未触碰生产）
- 基线：`main@25310c2379ec10807f5dccd9dd5bf8997b491646`
- 分支：`codex/t15-native-probe-model-detection`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t15-native-probe-model-detection`
- 候选提交：以包含本交接文件的分支最终 `HEAD` 为准，由根总控按精确 SHA 授权后才能合并。

## 已实现合同

- 连接测试继续复用 `AccountTestService.ProbeAccountConnection`；按账号保存 `connection_probe_model`，优先 `gpt-5.6-sol`，失效时回退原生登记的首个文本模型。
- 新增独立 `account_model_detection_settings` 与 `account_model_detection_runs`；不写 `usage_logs`、`account_monitor_results`、账号状态、评分、调度权重或分组建议。
- 检测模型为原生账号登记模型与 sidecar catalog 交集；已保存模型失效时回退交集首项，无交集时为 `unsupported`。OAuth 保留原生连接测试，模型检测始终不支持。
- API 进程只 enqueue/reuse 持久化任务；仅 singleton worker 启动的 `AccountMonitorRunner` 轮询 queued run、原子 claim 并最多 4 路执行 sidecar，不在 API 进程发送凭据。
- 北京时间 `00:00/10:00/12:00/15:00/18:00/21:00` 固定时隹，30 分钟内补触发；`(account_id, slot_key)` 持久化去重，同账号 queued/running 复用。
- 执行前重读账号类型、原生模型和 fresh sidecar catalog；账号转 OAuth、凭据缺失或模型失效时不发送凭据。
- sidecar 请求不记录 API Key/上游地址；响应使用 body/枚举/字符串/JSON 大小限制，并递归移除 `api_key`、`authorization`、`base_url`、`prompt`、`output`、`request`、`response` 等敏感摘要键。
- 账号卡片增加默认收缩的检测状态行和详情弹窗，显示申报模型、Juice 摘要、指纹候选/相似度、版本、时间和错误；异常文案仅表述“检测器观察到异常”。新文案已补齐中英文 locale。

## 迁移与配置

- 新增 add-only migration：`upstream/sub2api/backend/migrations/225_account_model_detection.sql`。无历史回填；账号删除时 settings/runs 按批准规格 `ON DELETE CASCADE`。
- 可选宿主配置：`SUB2API_MODEL_DETECTOR_URL`、`SUB2API_MODEL_DETECTOR_TOKEN`。未配置时 catalog fail-closed 为空，页面显示不支持，不影响原生连接监控。
- `downtime_required`：候选阶段未运行发布预检，必须由根 `main` 合并后的既有发布链判定。若输出 `true`，必须在任何停服、迁移或切换前等待用户明确授权。

## 验证证据

- `go test ./migrations -run TestAccountModelDetectionMigration -count=1`
- `go test ./internal/service -run 'Test(AccountModelDetection|SelectConnectionProbeModel|DetectionModelOptions|Models|EnqueueImmediate|DueSlot|HTTPAccountModelDetectionSidecar|AccountMonitorRunner|Projection|CatalogFailure|RunDueSlots|Execute)' -count=1`
- `go test ./internal/repository -run TestAccountModelDetectionRepository -count=1`
- `go test ./internal/server/routes -run TestAccountMonitorRoutesRegisterWindowEndpointAndKeepLegacyEndpoint -count=1`
- `go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes ./cmd/server -run '^$'`
- `vitest run AccountMonitorCard.spec.ts AccountMonitorView.spec.ts`：2 files / 93 tests 通过。
- `npm run typecheck`、`npm run build`、`git diff --check` 通过。

## 许可证与未验证项

- `tools/gpt56_api_detector-git` 为 PolyForm Noncommercial 1.0.0。本候选没有复制其核心实现、基线或报告逻辑，只实现 Sub 协议、调度、存储和 UI。
- 生产 sidecar 接入必须先取得书面商业授权，或替换为合法独立实现。未满足该门禁时不得配置生产 detector URL/token。
- 未运行全仓测试、额外 reviewer、压力/mutation/soak、浏览器矩阵、live sidecar 或生产验收；这些均不在本轮直接验证范围。

## 根总控后续

1. 保留本 worktree/分支，等待用户下一条明确部署指令。
2. 未获指令前不得合并、push、发布预检或触碰生产。
3. 用户授权后，根总控先检查候选是否落后最新 `main`；如落后则进入 `REFRESH_REQUIRED`，刷新并重跑直接相关门禁。
4. 仅从干净、已推送的根 `main` 进入本地/宿主蓝绿链；不使用 GitHub Actions。

