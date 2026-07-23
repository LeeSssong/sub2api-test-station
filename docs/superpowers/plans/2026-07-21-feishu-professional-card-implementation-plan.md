# Feishu Professional Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 relay-ops 告警、恢复和命令回复改为统一的 Feishu Interactive Card，并完成本地回归验证。

**Architecture:** `notify` 负责强类型卡片模型和三类业务模板；`feishuapi` 负责通用 App Bot wire payload；Webhook 和 App Bot 从同一业务卡片生成各自的传输格式。命令 worker 只依赖 notify 的消息模型，现有审计、去重、重试和路由控制保持原样。

**Tech Stack:** Go 1.24, encoding/json, existing relay-ops internal packages, Go table-driven tests.

## Global Constraints

- 原实现阶段不部署生产；后续部署已在用户批准的独立任务中完成。任何阶段都不修改 `RELAY_OPS_MODE=read_only`、飞书 `dry_run`、路由、上游、数据库或密钥。
- Feishu 消息内容上限按 30 KB 执行。
- 卡片发送失败不得额外发送文本降级消息。
- 不记录或展示 Key、Token、Cookie、提示词、响应正文、完整 chat/open ID。
- 保留 `SendText` 兼容接口和现有持久化发送重试。

---

### Task 1: Lock the card contract with failing tests

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu_test.go`
- Modify: `relay-ops-service/internal/feishuapi/client_test.go`
- Modify: `relay-ops-service/internal/commands/worker_test.go`

- [x] Add tests for alert/recovery/command card headers, semantic sections, redaction, 30 KB rejection, App Bot interactive wire payload, Webhook card wire payload, and command sender using structured messages.
- [x] Run focused tests and observe RED because the structured card model and `SendMessage` API do not exist yet.

### Task 2: Implement the shared model and three templates

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/delivery.go`

- [x] Add typed card/header/element/action types and JSON validation.
- [x] Implement `RenderAlert`, `RenderRecovery`, `RenderCommand`; retain `RenderFeishu` as alert compatibility wrapper.
- [x] Add safe text projection and outbound conversion without leaking secrets.
- [x] Run notify tests green.

### Task 3: Extend App Bot and Webhook delivery

**Files:**
- Modify: `relay-ops-service/internal/feishuapi/client.go`
- Modify: `relay-ops-service/internal/notify/feishu.go`

- [x] Add `OutboundMessage` and `SendMessage`; route `SendText` through it.
- [x] Make App Bot send interactive cards with content as a JSON string.
- [x] Make Webhook send interactive cards with the same JSON object.
- [x] Preserve token refresh, URL validation, response limits, and redacted errors.
- [x] Run focused API and notify tests green.

### Task 4: Switch command replies to command cards

**Files:**
- Modify: `relay-ops-service/internal/commands/worker.go`
- Modify: `relay-ops-service/internal/commands/worker_test.go`
- Modify: `relay-ops-service/internal/app/app_test.go`

- [x] Replace worker text sender dependency with structured message sender.
- [x] Render success, failure, disabled, and unknown-command cards while retaining all existing statuses and audit hashes.
- [x] Verify failed delivery still records one reply attempt per retry and never sends a second text fallback.

### Task 5: Integrate and verify all producers

**Files:**
- Modify: `relay-ops-service/internal/acceptance/service.go` only if template-specific recovery fields are needed.
- Modify: `relay-ops-service/internal/nativealerts/service.go` only if template-specific recovery fields are needed.
- Modify: `relay-ops-service/internal/collection/pricing.go` only if template-specific alert fields are needed.

- [x] Confirm all existing producers use alert/recovery card templates through notify.
- [x] Run package tests, full race tests, `go vet`, and `git diff --check`.
- [x] Review diff for production/config changes and document that deployment remains pending explicit approval.

## Verification Evidence

- Focused packages: `notify`, `feishuapi`, `commands`, `app`, `acceptance`, `nativealerts`, `collection` passed.
- Full: `go test ./... -race -count=1` passed.
- Static: `go vet ./...` and `git diff --check` passed.
- Original implementation session: deployment intentionally not performed.
- Production follow-up: code is present in `sub2api-relay-ops:feishu-proactive-alert-20260721-v3`; production remains `read_only` + `dry_run`.
- Real Feishu visual acceptance: daily report card and one read-only command reply card passed. Alert/recovery card templates share the verified App Bot transport but await the next natural event for visual acceptance; existing synthetic messages predate the card image and remain plain text.
- Production evidence: `docs/superpowers/reports/2026-07-21-feishu-professional-card-production-verification.md`.
