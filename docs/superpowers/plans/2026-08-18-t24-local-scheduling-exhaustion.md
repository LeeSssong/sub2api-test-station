# T24 本地调度耗尽错误契约实施计划

**Goal:** 让 Responses 与 Chat Completions 的本地账号池耗尽返回稳定 503、中文用户消息和准确管理员诊断，同时保持真实上游 503 原样。

**Architecture:** 扩展现有 `no_account_error` 分类结果作为唯一协议事实，复用 `handleStreamingAwareError` 输出 JSON/SSE，再扩展 T02 原生管理员诊断投影。所有变更只读现有请求上下文和错误日志，不改调度、重试或持久化结构。

**Tech Stack:** Go、Gin、OpenAI 兼容 JSON/SSE、Go unit tests。

**Spec:** `docs/superpowers/specs/2026-08-18-t24-local-scheduling-exhaustion-design.md`

## 全局约束

- 仅在 `codex/t24-local-scheduling-exhaustion` worktree 修改本任务规格、计划、实现、测试和交接。
- 不修改 `docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、根 `main`、生产、迁移、配置或 GitHub Actions。
- 不改变账号选择、冷却、并发、利润门、重试、计费、用量记录或真实上游错误透传。

### Task 1：锁定本地耗尽分类契约

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/no_account_error_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/no_account_error.go`

- [ ] 在 503 fallback、模型支持但临时不可用、空池用例中断言 `local_capacity_exhausted` 和中文消息。
- [ ] 运行 RED：`go test ./internal/handler -tags unit -run 'TestClassifyNoAccountError_(NilDiagnoser|HasModelSupport|ModelSupportedOnlyByRateLimitedAccount|NoAccountsInPool)' -count=1`。
- [ ] 最小修改 fallback 分类；保持 404 分支不变。
- [ ] 运行同命令 GREEN 并 `gofmt`。

### Task 2：统一 Responses 与 Chat Completions JSON/SSE

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/stream_error_event_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/gateway_handler_responses.go`
- Modify: `upstream/sub2api/backend/internal/handler/gateway_handler_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`

- [ ] 增加未开始流 JSON 和已开始流 Responses/Chat SSE 的 RED 断言。
- [ ] 删除通用 Gateway 首次选择失败对内部 `err.Error()` 的用户拼接，统一消费分类结果。
- [ ] 确认 OpenAI 原生 handler 的首次选择失败与空 selection 同样消费分类结果。
- [ ] 运行协议聚焦测试，确认 404 模型不支持不变。

### Task 3：补齐管理员诊断投影

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/native_error_diagnostics.go`
- Modify: `upstream/sub2api/backend/internal/service/native_error_diagnostics_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/ops_error_logger_test.go`

- [ ] 增加 routing/platform/未选账号、本地码、中文含义和无上游证据 RED 用例。
- [ ] 增加已选择账号真实 503 仍为上游类别的隔离用例。
- [ ] 最小扩展本地容量 class/explanation/默认阶段归属；保留现有 routing marker 和 SLA 口径。
- [ ] 运行 service/handler 聚焦 GREEN。

### Task 4：直接相关回归与候选交接

**Files:**
- Create: `docs/handoffs/2026-08-18-t24-local-scheduling-exhaustion-handoff.md`

- [ ] 运行直接相关 handler/service 测试及真实上游 503 透传既有测试。
- [ ] 运行受影响包 compile-only、`go build ./cmd/server`、`gofmt`、`git diff --check` 和范围检查。
- [ ] 记录基线、候选提交、文件、测试、未验证项、迁移/配置、`downtime_required`、回滚与风险。
- [ ] 提交候选并停止在 `READY_FOR_ROOT_REVIEW`；不合并、推送、部署或线上操作。

## 计划批准

2026-08-18：计划严格落实已批准 T24 范围，任务间没有产品决策或不可逆动作；按唯一发布总控代审授权批准执行。
