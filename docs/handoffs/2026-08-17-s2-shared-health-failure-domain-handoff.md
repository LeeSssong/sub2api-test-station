# S2 共享健康、故障域与抗故障重试交接

## Start Here

- 任务包：S2
- 分支：`codex/s2-shared-health-failure-domain`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/s2-shared-health-failure-domain`
- 初始基线：`main@1bc052d8e`
- 最新刷新主线：`main@566fc52ba`（merge commit `ce48bef0b`）
- 已验证运行时 tip：`e3d905412`
- 最终候选：`33d9fdb6a`
- 根合并提交：`d1f9bc06c`
- 最终发布源：`main@aab79007f`、tree `c33145f1fdac4bf4b28d4cdc516036d3d938f75e`
- 状态：`DONE`；已推送、已无停机蓝绿发布、已线上验证生效。

## Delivered

- Redis v1 键空间与 Lua 幂等 event update，维护 account-model/domain failure streak、cooldown、error-rate/TTFT EWMA 和 TTL。
- 单调 fencing token 的 one-winner half-open lease；丢失或过期 owner 不能完成 lease。
- 本地 transient 先写、Redis best-effort 后写；读失败最多使用 30 秒可信快照，过期后返回 unknown。
- 普通成功会以独立幂等 success event 回写 Redis，清零 shared failure streak/cooldown；Redis 写失败不阻塞本地成功清理。
- Scheduler 保持 S1 veto 优先，并在基础资格过滤后优先保留未命中已失败 `provider_channel` / `quota_pool` 的最佳 bucket；bucket 内原顺序及后续 Top-K/sticky 逻辑不变。
- Responses、Messages、Chat Completions 共用请求本地预算：最多 4 attempts / 3 switches / 2 domains / 5s。
- Redis 不可用且无 30 秒内可信快照时，请求预算收窄为最多再跨账号一次；Redis 正常返回空投影不误判为降级。
- 429 尊重 `Retry-After` delta-seconds/HTTP-date；普通瞬时失败使用 120ms–2s 有界指数退避和稳定 0–20% jitter。
- 预算提供稳定终止原因：`attempt_limit`、`account_switch_limit`、`failure_domain_limit`、`retry_deadline`、`unsafe_to_replay`。
- Chat Completions 普通失败现与 Responses/Messages 一致写入 shared failure event，并携带逻辑请求/attempt 元数据。
- 已保留输出后、tools、`function_call_output`、`tool_result` 和已知副作用请求的 no-replay 边界。

## Commits

- `04d4abe4a` shared health contracts/config.
- `6a2be320c` Redis repository, idempotency, fencing and generated Wire.
- `3bd59ddb4` scheduler/shared transient merge and half-open ownership.
- `9baff2649` request-local attempt/switch/domain/deadline budget.
- `5ca9cdebd` safe `Retry-After` and bounded backoff.
- `0e52328a9` refresh merge from `main@a533d84b0`.
- `fbc79624a` refresh merge from `main@b35d3f100` after root audit reopened S2.
- `e3d905412` close shared-success, degraded-budget, domain-preference, jitter/reason and Chat failure-event audit gaps.
- `ce48bef0b` refresh merge from `main@566fc52ba`, including the global no-downtime release authorization rule.

## Verification

See `docs/superpowers/reports/2026-08-17-s2-shared-health-failure-domain-verification.md`.

Focused repository, service, handler, config, server compile and server build checks pass. `git diff --check` and affected-file `gofmt` checks pass.

## Changes / Operations

- Database migrations: none.
- New persistent tables or columns: none.
- GitHub Actions: none.
- Configuration contract: additive `gateway.openai_shared_health` with defaults enabled, Redis timeout 75ms, stale snapshot 30s, attempts 4, switches 3, domains 2, total retry budget 5000ms, backoff 120–2000ms, half-open lease 15s.
- Production configuration mutation: none.
- `downtime_required`: `false`.
- Deployment: existing preloaded blue-green controller returned `succeeded`; active slot is `blue`, API and worker use the same immutable `release-aab79007f-52f7b7…` image, and the previous `green` slot remains available for rollback.
- Release record: `/var/lib/sub2api/release-records/20260817T072052Z-production-1893124.json` (`succeeded/promoted`, `rolled_back=false`).
- Test evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-aab79007f-s2-shared-health-v1.json` (`0600`).
- Online acceptance: `/healthz`, `/readyz`, `/health` all HTTP 200; API/worker healthy with restart count 0; PostgreSQL/Redis/Caddy IDs unchanged. Natural traffic produced 8 healthy account-model projections, 2 domain projections, and 13 idempotency markers; shared-health warning/panic/fatal counts were zero in both API and worker for the inspected 15-minute window.
- Recovery bundle: `/Users/gongtengxinwen/Documents/sub2api-archives/s2-shared-health-failure-domain-33d9fdb6a.bundle`, mode `0600`, SHA-256 `aea0bb53fa77a32976cb79219e36a94811175798b4299d58ecb4622ead2644f9`.
- Cleanup: after confirming the candidate was clean, an ancestor of `main`, and fully deployed/verified, the S2 feature worktree, local branch, and temporary release worktree were removed. T15, T16, and historical protected worktrees were unchanged.

## Open Risks

- `go test -tags=integration` for the real Redis concurrency case remains blocked by a pre-existing unrelated package collision: `user_profile_identity_repo_contract_test.go:577 stringPtr` conflicts with `usage_log_repo_stats.go:203 stringPtr`.
- Real Redis natural traffic proves the v1 projections are being written and read without shared-health warnings, but failure/cooldown/half-open production paths were not deliberately induced.
- If rollback is required, use the existing immutable-image blue-green chain to restore the preserved `green` image. Disabling shared health alone does not remove request-local retry caps, so a full code rollback is authoritative.

## Next Loop Brief

Goal: S2 已收口；下一独立循环可启动 S3 自适应选择、粘性逃逸与调度体验观测。
Context: 生产运行 `main@aab79007f`，S2 自然流量投影健康，S3 依赖门禁已解除。
Constraints: 从届时最新干净 `main` 创建独立任务/worktree；重新核对当前调度代码事实；不触碰 T15/T16/历史冻结 worktree；不改变 S1/S2 veto、价格、账务或外部控制面；不使用 GitHub Actions。
Plan: 登记 S3 为 `DESIGNING`，刷新历史拆分规格，完成正式计划后按 TDD 实施直接相关功能与测试。
Validate: 仅 S3 直接相关调度/handler/config 回归、必要构建与发布门禁；生产预检为 `false` 时直接无停机发布，为 `true` 时请求授权。
Done when: S3 独立完成推送、部署和线上验证。
