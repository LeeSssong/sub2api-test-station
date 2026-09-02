# T112 Codex 分组级稳定模型目录验证报告

**日期：** 2026-09-02

**工作区：** `.worktrees/t112-codex-stable-model-manifest-v2`

**基线：** `main@b3cc8b5fe`，创建时与 `origin/main` 一致

## 通过项

- `go test ./internal/service -run 'Test(CodexModels|FetchCodexModelsManifest|IsRetryableCodexModelsManifest|BuildCodexModelsGroupManifestCacheKey)' -count=1`
- `go test ./internal/handler -run 'Test.*CodexModels' -count=1`
- 上述 service 定向测试 `-race`
- 上述 handler 定向测试 `-race`
- `go build ./cmd/server`
- `git diff --check`

覆盖：分组模型并集、slug 去重与稳定排序、Composite、单账号失败容错、全失败闭合、聚合 ETag/304、缓存键顺序无关、stale 聚合刷新失败保留旧值、OAuth 429 冷却副作用。

## 已知全包失败

`go test ./internal/service ./internal/handler -count=1` 仍包含与 T112 无关的既有失败，涉及 Codex 指纹 seed 生命周期、channel monitor 参数、CodexRadar、OpenAI 调度/流式响应、图片并发、服务 tier 校验和 usage record 等；没有新的 T112 Codex 聚合测试失败。全量失败不作为本候选的通过依据，也未修改这些无关路径。

## 发布状态

- 未提交、未推送、未合并到 `main`。
- 未构建部署制品、未部署验收站或主站。
- 未修改生产数据、配置、凭据或发布链。
