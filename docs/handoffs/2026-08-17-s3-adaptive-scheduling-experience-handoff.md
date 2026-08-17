# S3 自适应选择、粘性逃逸与调度体验观测交接

## Start Here

- 任务包：S3
- 分支：`codex/s3-adaptive-scheduling-experience`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/s3-adaptive-scheduling-experience`
- 初始基线：`main@83c4554792d3424751a439f3fd1cc38a0542ed5e`
- 根主线发布源：`main@0720b8bf0b5e23486904e571f12b483e7329a9c0`
- 已验证运行时 tip：`0f4506bca758377be204b4acea446d72edd74221`
- 状态：`DONE`；已合并、推送、预检、无停机蓝绿发布并完成线上验收。

## Delivered

- S1/S2 硬门槛后的质量阈值与动态 Top-K，默认 `true/7/0.15`，非法范围启动时拒绝。
- 决策中输出候选数、有效数、effective Top-K、最低质量阈值、selection layer、sticky 保留/逃逸原因和 TTFT report-only 资格。
- TTFT report-only 默认开启但只记录资格，不发起、计费或取消第二个上游请求。
- Responses/Messages 逻辑请求记录 selection 与单一 terminal outcome，并沿用 S2 attempts/switches/domains/deadline/no-replay 预算。
- 复用现有有界运行时 resilience ledger 派生自动恢复、平均/P95 尝试、坏账号重复命中、预算耗尽、sticky、Top-K 与 TTFT 指标。
- 新增管理员端点 `GET /api/v1/admin/ops/openai-scheduler-experience`。
- 现有 Ops Dashboard 在 OpenAI Token Stats 下方增加独立卡片；继承 preset/custom time、platform、group 和 refresh，显示样本、新鲜度与当前运行窗口，低样本/no-data/error 不误导其他区块。

## Verification

详见 `docs/superpowers/reports/2026-08-17-s3-adaptive-scheduling-experience-verification.md`。

后端 config/service/handler/admin/routes focused tests、server compile/build、前端 3 files / 9 tests、typecheck/build、affected Go `gofmt`、candidate diff-check 和零迁移/零 GitHub Actions 范围检查均通过。

## Commits

- `0a33b7761` formal specification.
- `ec433a6c8` implementation plan.
- `ca4269833` adaptive scheduler configuration.
- `50594fd5f` dynamic Top-K.
- `4d574cd64` sticky explanation and TTFT report-only.
- `8b9a7d171` scheduler selection/outcome events.
- `f8f422605` Ops aggregation and admin API.
- `636c2d335` Ops Dashboard card.
- `21e4f076e` exact custom-time/runtime-window review fix.
- `0f4506bca` specification diff-check normalization.

## Root Integration

1. The candidate was refreshed from the then-current root and merged without conflicts.
2. S3 focused gates passed on the merged tree; no migration or workflow changes were introduced.
3. The root release controller pushed `main@0720b8bf0` and the host chain returned `succeeded/promoted` with `downtime_required=false`.
4. Online acceptance used health endpoints, immutable API/worker identity, the scheduler-experience Ops card and natural traffic only; no account mutation or manufactured upstream failure was performed.

## Operations / Rollback

- Database migration, backfill or production data mutation: none.
- Configuration defaults: `adaptive_top_k_enabled=true`, `adaptive_top_k_max=7`, `adaptive_top_k_score_gap=0.15`, `ttft_report_only_enabled=true`.
- `downtime_required=false`; no manual authorization was required under the global no-downtime rule.
- Release record: `/var/lib/sub2api/release-records/20260817T093040Z-production-1990545.json` (`succeeded/promoted`, `rolled_back=false`).
- Active slot: `green`; active upstream: `sub2api-green:8080`; API and worker use the same immutable image digest.
- Public health: `/healthz`, `/readyz`, `/health` all returned HTTP 200.
- The metrics ledger is process-local and reconstructible; restart may temporarily return `no_data`.
- Functional rollback: disable `adaptive_top_k_enabled` and `ttft_report_only_enabled`.
- Binary rollback: restore the previous immutable blue-green image; no database rollback or cleanup is required.

## Open Risks

- Freshly restarted API instances have an empty runtime ledger until natural traffic arrives.
- Low denominators remain `insufficient_data`; do not create synthetic production failures for acceptance.
- Existing frontend pnpm/localStorage/Browserslist/Vite warnings are unchanged and non-blocking.

## Next Loop Brief

Goal: 由唯一发布总控刷新、合并、推送并发布 S3；无停机时直接完成线上验收。
Context: S3 verified runtime tip `0f4506bca`; root `main@0720b8bf0` is promoted and online.
Constraints: preserve T15/T16/historical worktrees and root untracked files; no GitHub Actions; no full-suite/pressure/soak/mutation; no production account mutation or manufactured failure.
Plan: completed: refresh -> focused revalidation -> root merge -> push -> precheck -> no-downtime blue-green -> health/API/card/natural-event acceptance -> evidence/ledger/cleanup.
Validate: root merged-tree focused tests/build/diff/range checks plus release-controller and online evidence.
Done when: satisfied by pushed root `main`, successful production promotion, S3 online acceptance, evidence and ledgers closed.
