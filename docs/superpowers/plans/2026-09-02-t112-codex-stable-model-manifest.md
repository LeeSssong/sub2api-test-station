# T112 Codex 分组级稳定模型目录实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Codex `/models` 基于分组内成功上游 manifest 的稳定并集返回模型目录，消除账号轮换造成的 5.5/5.6 漂移。

**Architecture:** 在现有 `OpenAIGatewayService` 中新增分组级聚合入口，复用单账号 `FetchCodexModelsManifest`、认证、转换和单账号缓存；聚合层负责候选账号快照、并发抓取、manifest 合并、分组缓存和聚合 ETag。handler 只负责调用分组入口和 HTTP 响应投影，真实请求调度完全不变。

**Tech Stack:** Go、Gin、标准库 JSON/HTTP、现有 Sub2API account repository 与 `singleflight`。

**Spec:** `docs/superpowers/specs/2026-09-02-t112-codex-stable-model-manifest-design.md`

## Global Constraints

- 只在当前候选 worktree 实现，禁止修改根 `main`、全局队列、项目进度总账、生产配置或数据。
- 只从现有原生接口扩展，不新建模型事实源，不改变真实请求账号调度。
- 保持 API Key、OAuth、Agent Identity、Composite 与自定义上游兼容。
- 本轮不推送、不部署、不使用 GitHub Actions。

### Task 1: Define Aggregation Contracts

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_codex_models_service.go`
- Test: `upstream/sub2api/backend/internal/service/openai_codex_models_service_test.go`

- [ ] **Step 1: Write failing unit tests** for merging two valid manifests, duplicate slug precedence, deterministic ordering, preserving top-level fields, and rejecting an empty successful set when no account succeeded.
- [ ] **Step 2: Run the focused service tests** with `go test ./internal/service -run 'TestCodexModelsManifestAggregation'` and confirm the new symbols/behavior fail for the intended reason.
- [ ] **Step 3: Implement minimal pure aggregation helpers** that decode only the top-level `models` array, preserve model object JSON, deduplicate by non-empty `slug`, sort deterministically, and compute the body ETag through the existing helper.
- [ ] **Step 4: Re-run the focused tests** and verify they pass without changing the existing single-account fetch behavior.

### Task 2: Add Group Snapshot and Cache

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_codex_models_service.go`
- Test: `upstream/sub2api/backend/internal/service/openai_codex_models_service_test.go`

- [ ] **Step 1: Write failing tests** for repository candidate enumeration, account-order-independent cache keys, one fetch per candidate, partial success, all-failure cache fallback, and concurrent singleflight refresh.
- [ ] **Step 2: Run those tests** and confirm they fail before production wiring exists.
- [ ] **Step 3: Add a narrow `FetchCodexModelsManifestForGroup` service method** that queries persistent OpenAI candidates, builds a stable snapshot key from group/client version/account credentials, fetches each candidate through the existing method, merges successful results, and applies fresh/stale/miss cache semantics.
- [ ] **Step 4: Keep failed refreshes from replacing a valid cached aggregate** and return a typed upstream error when no cache and no successful candidate remain.
- [ ] **Step 5: Run service tests** for cache and failure behavior.

### Task 3: Switch the Handler to Group Aggregation

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_codex_models_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_codex_models_handler_test.go`

- [ ] **Step 1: Write failing handler tests** proving OpenAI and Composite group requests return the union from multiple accounts, use the aggregate ETag, return 304 for the aggregate ETag, and fail closed when every account fails without cache.
- [ ] **Step 2: Run the focused handler tests** and confirm the old single-account selection path produces the expected failure.
- [ ] **Step 3: Replace the handler loop with the group service call** while preserving API key validation, platform guard, ops context, error mapping, and response headers.
- [ ] **Step 4: Run handler tests** and the related service package tests.

### Task 4: Compatibility Regression and Verification

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_codex_models_service_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_codex_models_handler_test.go`

- [ ] **Step 1: Run existing Codex OAuth, API Key, Agent Identity, custom `/models` conversion, ETag, failover, and Composite tests** together with the new tests.
- [ ] **Step 2: Fix only regressions caused by T112 and keep unrelated failures unchanged.**
- [ ] **Step 3: Run `gofmt` on touched Go files and `git diff --check`.**
- [ ] **Step 4: Run `go test ./internal/service ./internal/handler` or the narrow package subset supported by the local Go toolchain, then `go build ./cmd/server` if dependencies are available.
- [ ] **Step 5: Record implementation status in the candidate handoff/report only; do not edit root `project-progress.md` or queue files, do not commit unless explicitly requested, and do not push or deploy.
