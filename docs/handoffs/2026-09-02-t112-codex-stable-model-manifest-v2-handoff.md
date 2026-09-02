# T112 Codex 分组级稳定模型目录交接

## 交付范围

候选将 Codex `/models` 从单账号实时透传改为分组级实时 manifest 聚合：读取持久资格 OpenAI 账号，复用现有单账号 OAuth/API Key 认证与转换路径，并集成成功账号的模型并集。真实请求仍使用原生模型能力调度，未改变请求路由、配额、优先级或重试预算。

## 关键文件

- `upstream/sub2api/backend/internal/service/openai_codex_models_service.go`
- `upstream/sub2api/backend/internal/service/openai_codex_models_service_test.go`
- `upstream/sub2api/backend/internal/handler/openai_codex_models_handler.go`
- `upstream/sub2api/backend/internal/handler/openai_codex_models_handler_test.go`
- `docs/superpowers/specs/2026-09-02-t112-codex-stable-model-manifest-design.md`
- `docs/superpowers/plans/2026-09-02-t112-codex-stable-model-manifest.md`

## 行为与验证

- 账号分别返回 `gpt-5.5` 和 `gpt-5.6` 时，客户端收到稳定并集。
- 单个账号失败不阻断其他成功账号；全部失败且无缓存时 fail-closed。
- 分组缓存使用账号顺序无关、包含凭据/账号版本信息的哈希键；聚合 ETag 由聚合正文生成。
- stale 缓存异步刷新失败不覆盖旧聚合；OAuth 429 仍进入原生冷却记录路径。
- service/handler 定向测试及 race、server build、diff-check 已通过；全包既有失败详见验证报告。

## 当前门禁

候选状态为实现完成待根审查，保留当前 worktree 和分支。不得从该候选部署；后续如需发布，必须先按项目约束合入干净根 `main`、推送并获得明确部署授权。
