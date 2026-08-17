# S2 共享健康、故障域与抗故障重试交接

## Start Here

- 任务包：S2
- 分支：`codex/s2-shared-health-failure-domain`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/s2-shared-health-failure-domain`
- 初始基线：`main@1bc052d8e`
- 已刷新主线：`main@a533d84b0`
- 已验证实现 tip：`0e52328a9`
- 候选状态：`READY_FOR_ROOT_REVIEW`（根总控需在根 `main` 更新全局队列/总账）

## Delivered

- Redis v1 键空间与 Lua 幂等 event update，维护 account-model/domain failure streak、cooldown、error-rate/TTFT EWMA 和 TTL。
- 单调 fencing token 的 one-winner half-open lease；丢失或过期 owner 不能完成 lease。
- 本地 transient 先写、Redis best-effort 后写；读失败最多使用 30 秒可信快照，过期后返回 unknown。
- Scheduler 保持 S1 veto -> 当前请求排除 -> shared cooldown/half-open 顺序，不改变 eligible 候选的 Top-K/sticky 内部顺序。
- Responses、Messages、Chat Completions 共用请求本地预算：4 attempts / 3 switches / 2 domains / 5s。
- 429 尊重 `Retry-After` delta-seconds/HTTP-date；普通瞬时失败使用 120ms–2s 有界指数退避。
- 已保留输出后、tools、`function_call_output`、`tool_result` 和已知副作用请求的 no-replay 边界。

## Commits

- `04d4abe4a` shared health contracts/config.
- `6a2be320c` Redis repository, idempotency, fencing and generated Wire.
- `3bd59ddb4` scheduler/shared transient merge and half-open ownership.
- `9baff2649` request-local attempt/switch/domain/deadline budget.
- `5ca9cdebd` safe `Retry-After` and bounded backoff.
- `0e52328a9` refresh merge from `main@a533d84b0`.

## Verification

See `docs/superpowers/reports/2026-08-17-s2-shared-health-failure-domain-verification.md`.

Focused repository, service, handler, config, server compile and server build checks pass. `git diff --check` and affected-file gofmt checks pass.

## Changes / Operations

- Database migrations: none.
- New persistent tables or columns: none.
- GitHub Actions: none.
- Configuration contract: additive `gateway.openai_shared_health` with defaults enabled, Redis timeout 75ms, stale snapshot 30s, attempts 4, switches 3, domains 2, total retry budget 5000ms, backoff 120–2000ms, half-open lease 15s.
- Production configuration mutation: none.
- `downtime_required`: unverified until root release preflight; design target is `false`.
- Deployment: frozen; no preflight, push, blue-green switch, or production write performed.

## Open Risks

- Real Redis integration remains unverified because the unrelated integration-tag package does not compile due to the existing `stringPtr` collision.
- Mixed-version and production Redis behavior still require root preflight plus post-deployment online acceptance after the user explicitly reauthorizes deployment.
- If rollback is required before deployment, revert the S2 commits. After deployment, use the existing immutable-image blue-green rollback chain; disabling shared health alone does not remove request-local retry caps, so a full code rollback is the authoritative rollback.

## Next Loop Brief

Goal: 由唯一发布总控复核 S2 候选并将其合入根 `main`，但继续停在部署冻结门禁前。
Context: S2 分支已刷新 `main@a533d84b0`；实现、直接相关测试、server compile/build、gofmt 和 diff-check 已完成；验证与交接文档见本文及对应 report。
Constraints: 根总控独占 `main`/总账/队列；不使用 GitHub Actions；不触碰 T15/T16/历史冻结 worktree；不启动 S3；没有用户新的明确部署指令时不做预检、push 或生产写入。
Plan: 核对候选 SHA/范围和未验证项，更新根队列/总账为 `READY_FOR_ROOT_REVIEW`，再按根授权合并流程处理。
Implement: 仅根总控允许的 merge/文档状态更新；不扩大 S2 运行时范围。
Validate: 合并后重跑本 report 的直接相关命令和根范围检查；部署仍等用户新指令。
Review: S1 veto 优先级、no-replay、Redis fail-safe、账务幂等、Wire 生成一致性、无迁移/无 GitHub Actions。
Done when: 根 `main` 合并与直接相关验证完成，然后在用户明确部署指令前保持冻结。
