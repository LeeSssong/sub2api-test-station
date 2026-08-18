# T21 模型检测 sidecar 生产验收

- 发布源：`main@aee203ac4983c56e961525cbb9f587d16b5aa74d`
- tree：`cf5a5f8d46951701cb235b37dd3041551288e1e5`
- 0600 证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-18-main-aee203ac4-t21-sidecar-ready-v2.json`
- 宿主记录：`/var/lib/sub2api/release-records/20260818T110720Z-production-3118657.json`
- 发布结果：`succeeded/promoted`，`rolled_back=false`，`downtime_required=false`，活动槽 `blue`

首次候选发布在生产变更前被镜像构建门禁拦截：model-detector 的 Go module cache mount 未指定 target，遮蔽 `/app/backend` 并产生 `go.mod file not found`。根总控增加 Dockerfile 合同测试，修复为显式 `/go/pkg/mod`，并用真实 `docker buildx --target backend-builder` 验证 `/app/model-detector` 构建成功后重新发布。

生产 API、worker、model-detector 使用同一不可变镜像且均为 healthy；公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。带私网 token 的 `/v1/catalog` 返回 `gpt-5.6-terra`、`gpt-5.6-sol`、`gpt-5.4` 等模型。管理员账号 `#23` 发起真实检测，运行 `9dc7b02a-f5f2-4ea1-a25f-2f2f6ab2c6dd` 从 queued 进入 `normal`，模型为 `gpt-5.6-terra`，detector 版本 `native-1`。

对 detector、活动 API 和 worker 最近日志执行精确敏感标记扫描，`api_key` 值、`base_url`、`Authorization`、`Bearer`、`SUB2API_MODEL_DETECTOR_TOKEN` 均为零匹配；API 响应只返回运行状态、模型与 detector 版本。T21 已完成生产闭环。
