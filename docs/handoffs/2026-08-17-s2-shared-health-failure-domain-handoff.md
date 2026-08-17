# S2 共享健康、故障域与抗故障重试交接

## Start Here

- 任务包：S2
- 分支：`codex/s2-shared-health-failure-domain`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/s2-shared-health-failure-domain`
- 初始基线：`main@1bc052d8e`
- 最新刷新主线：`main@b35d3f100`（merge commit `fbc79624a`）
- 已验证运行时 tip：`e3d905412`
- 候选状态：`READY_FOR_ROOT_REVIEW`；未合并根 `main`，未运行发布预检，未部署。

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

## Verification

See `docs/superpowers/reports/2026-08-17-s2-shared-health-failure-domain-verification.md`.

Focused repository, service, handler, config, server compile and server build checks pass. `git diff --check` and affected-file `gofmt` checks pass.

## Changes / Operations

- Database migrations: none.
- New persistent tables or columns: none.
- GitHub Actions: none.
- Configuration contract: additive `gateway.openai_shared_health` with defaults enabled, Redis timeout 75ms, stale snapshot 30s, attempts 4, switches 3, domains 2, total retry budget 5000ms, backoff 120–2000ms, half-open lease 15s.
- Production configuration mutation: none.
- `downtime_required`: not evaluated because release preflight is frozen; design target remains `false`.
- Deployment: frozen; no preflight, root merge, push of S2 code, blue-green switch, or production write performed.

## Open Risks

- `go test -tags=integration` for the real Redis concurrency case remains blocked by a pre-existing unrelated package collision: `user_profile_identity_repo_contract_test.go:577 stringPtr` conflicts with `usage_log_repo_stats.go:203 stringPtr`.
- Mixed-version and production Redis behavior still require the existing root preflight and post-deployment online acceptance after the user explicitly authorizes deployment.
- If rollback is required before deployment, revert the S2 commits. After deployment, use the existing immutable-image blue-green rollback chain; disabling shared health alone does not remove request-local retry caps, so a full code rollback is authoritative.

## Next Loop Brief

Goal: 由唯一发布总控只读复核 S2 候选，确认后合入根 `main`；继续停在部署冻结门禁前。
Context: S2 已刷新 `main@b35d3f100`；运行时 tip `e3d905412` 的直接相关测试、server compile/build、gofmt 和 diff-check 已完成。
Constraints: 根总控独占 `main`/总账/队列；不使用 GitHub Actions；不触碰 T15/T16/历史冻结 worktree；不启动 S3；没有用户新的明确部署指令时不做发布预检或生产写入。
Plan: 核对候选范围、验证证据和未验证项，更新根队列/总账为 `READY_FOR_ROOT_REVIEW`；后续 merge 与部署等待相应门禁。
Validate: 若后续合并根 `main`，仅重跑本 report 的直接相关命令和根范围检查；部署仍等用户新指令。
Done when: 当前阶段以候选和根总账均标记 `READY_FOR_ROOT_REVIEW` 为止。
