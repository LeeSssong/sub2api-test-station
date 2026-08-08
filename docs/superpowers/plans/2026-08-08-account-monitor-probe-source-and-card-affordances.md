# 账号监控统一探测口径与卡片交互增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让账号监控卡片主指标统一来自主动探测，并增加成本来源警示、账号名称外链和评分构成悬浮展示。

**Architecture:** 保持现有 `/admin/accounts/monitor` 合同和蓝绿发布链。后端将卡片质量证据固定为主动探测聚合，并返回评分分项；前端卡片只负责展示探测口径、来源提示、外链和评分悬浮层。

**Tech Stack:** Go、PostgreSQL 聚合仓储、Vue 3 + TypeScript、Vitest、pnpm、现有 Docker blue-green release scripts。

## Global Constraints

- 账号卡片成功率、TTFT、总耗时、评分、排名资格和证据全部以主动探测为唯一来源。
- 真实调用统计仅保留在调用明细区域，不参与卡片主质量指标。
- 不新增数据库迁移，不改变调度、计费或探测任务。
- 生产发布必须使用现有本地/主机蓝绿链路，不使用 GitHub Actions，不停机。
- 只运行本功能相关测试、前端构建和部署健康/页面/API 验证。

---

### Task 1: 后端统一探测证据并返回评分分项

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Produces `AccountMonitorScoreBreakdown` JSON fields consumed by the card.
- `accountMonitorWindowEvidence` for card projections must select the latest valid monitor probe aggregate, preserving probe failure/expiry state.

- [ ] Write failing tests proving a window with 96 successful real requests and 9 failed probes uses probe success rate/evidence, and proving score breakdown sums to `quality_score` after rounding.
- [ ] Run the focused Go tests and confirm they fail for the missing probe-only behavior/fields.
- [ ] Implement probe-only evidence selection for global and group projections without changing the calls disclosure fields.
- [ ] Add cost/success/TTFT/latency score components and evidence source to the API projection, using the existing backend formula and one final rounding step.
- [ ] Run focused Go tests, `go test ./internal/service/...` and `go vet ./...` from `upstream/sub2api/backend`.
- [ ] Commit with `feat: use probe evidence for account monitor cards`.

### Task 2: 前端卡片来源、外链与评分悬浮交互

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

**Interfaces:**
- Consumes backend `score_breakdown`, `evidence_source`, `homepage_url`, and existing multiplier/procurement fields.
- Emits no new mutation events; external navigation uses an anchor with `target="_blank"` and `rel="noopener noreferrer"`.

- [ ] Add failing component tests for probe-specific success text, manual-cost warning tooltip, native-cost tooltip, homepage link attributes, and score breakdown hover content.
- [ ] Run the focused Vitest file and confirm the new assertions fail.
- [ ] Implement the card UI with accessible `title`/aria labels, keeping calls detail separate and showing probe sample counts in the main metrics.
- [ ] Run the focused Vitest file and related AccountMonitorView tests.
- [ ] Run frontend typecheck/lint/build commands from `upstream/sub2api/frontend`.
- [ ] Commit with `feat: clarify account monitor card evidence and affordances`.

### Task 3: 独立复核、发布证据与生产蓝绿验证

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-08-account-monitor-probe-source-and-card-affordances-production-verification.md`

**Interfaces:**
- Consumes commits from Tasks 1-2 and existing `ops/release-sub2api-blue-green.sh` evidence contract.
- Produces a production verification report with source/image identity, `downtime_required=false`, API response checks, and UI acceptance evidence.

- [ ] Review the complete diff for scope creep and verify no migration, scheduler, billing, or GitHub Actions changes.
- [ ] Run only relevant frontend/backend tests, build, and release evidence generation.
- [ ] Execute existing blue-green production release path and confirm health/readiness, monitor API probe-only fields, and browser card interactions.
- [ ] Record push, deploy, and online verification facts in the progress ledger; keep status “进行中” until all three are evidenced, then mark this item “已完成”.
- [ ] Commit the verification report and progress update.
