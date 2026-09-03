# T123-T124 Semantic Failover and Balance Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Responses replay decisions semantic and sanitize every terminal error, while isolating Feishu balance notification refresh/delivery by normalized BaseURL scope.

**Architecture:** Extend existing stream observation/recovery metadata with explicit semantic-output and terminal state, and make handlers use those fields instead of writer byte counts. Add a narrow per-scope refresh result contract while preserving the plural compatibility method, then process each active scope independently through the existing event ledger and sender.

**Tech Stack:** Go, Gin SSE handlers, existing service/repository fakes, testify.

**Spec:** `docs/superpowers/specs/2026-09-03-t123-responses-semantic-failover-and-error-sanitization-design.md`, `docs/superpowers/specs/2026-09-03-t124-feishu-balance-scope-isolation-design.md`

## Global Constraints

- Preserve existing Responses, Chat/Messages, billing, event-ledger, lease/CAS, and Feishu card contracts.
- Never expose credentials, request IDs, URLs, or raw upstream error text to clients.
- No migration, external API, or real Feishu egress changes.
- Run only direct Go tests, `go build ./cmd/server`, `gofmt`, and `git diff --check`.

### Task 1: T123 semantic state and sanitized terminal responses

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_stream_read_error.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/stream_error_event.go`
- Test: existing handler/service stream and failover tests plus new focused regression tests.

- [ ] Write failing tests proving `response.created`/`response.in_progress` do not block failover, semantic deltas do block it, duplicate terminal events collapse, and request IDs/URLs/raw messages never reach client SSE.
- [ ] Run focused tests and confirm failure.
- [ ] Add explicit semantic/protocol/usage/side-effect/terminal metadata and classify Responses events with a whitelist; keep legacy `OutputStarted` as a derived compatibility field.
- [ ] Route every streamed failure through the fixed sanitization/template path before writing JSON or `response.failed`; preserve admin diagnostic fields.
- [ ] Run focused tests, `go build ./cmd/server`, `gofmt`, and `git diff --check`.

### Task 2: T124 per-scope refresh and health accounting

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

- [ ] Write failing tests proving scope A refresh failure does not block scope B evaluation/delivery, one account failure does not block sibling accounts, and all-failed scopes retain firing without claim/resolve.
- [ ] Run focused tests and confirm failure.
- [ ] Add per-scope refresh result contract and implement account-level error isolation; process each active normalized BaseURL independently with existing claim/lease/RecordFailure semantics.
- [ ] Add structured non-sensitive health counters/timestamps at the existing service observability boundary.
- [ ] Run focused tests, `go build ./cmd/server`, `gofmt`, and `git diff --check`.

### Task 3: Candidate handoff

- [ ] Review diff against both specs and confirm no migrations/config changes.
- [ ] Commit T123 and T124 changes on the candidate branch.
- [ ] Report tested tree, commit, files, tests, downtime and rollback details; wait for root integration authorization.

