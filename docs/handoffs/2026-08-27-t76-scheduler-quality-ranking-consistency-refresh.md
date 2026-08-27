# T76 调度与质量排名一致性刷新交接

## 状态

- 状态：`READY_FOR_ROOT_REVIEW`
- 刷新基线：`main@031b58e4c`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t76-scheduler-quality-ranking-consistency`
- 分支：`codex/t76-scheduler-quality-ranking-consistency`
- 刷新提交：`91f6ba2ef`（`fix: refresh t76 scheduler projection parity`）
- 本交接对应的刷新提交见本文件提交历史；未 push、未合并 root main、未部署。

## 交付内容

保留 T76 全部既有提交及解冻前 dirty diff，并修复 final whole-branch review round 2 的六项 Important：live Grok quota gate cache、过期 model cooldown/live `isBlocked` 一致性、shared-health veto、subscription-priority 分区、strategy 原因资格差异误归因，以及 `1/1/1` 自定义策略“体验均衡”标签。对应回归测试位于 `internal/service/openai_account_scheduler_projection_test.go`、repository/monitor 直接测试中。

## 测试与未验证项

- 已通过：T76 projection、account-monitor service/repository 聚焦 Go 测试。
- 已通过：管理员 handler/service/repository 聚焦测试、`go build ./cmd/server`、账号监控前端 110 tests、`pnpm typecheck`、`pnpm build`、gofmt 与 `git diff --check`。
- 浏览器登录态视觉检查、root 合并后的发布预检、验收站/主站部署和线上专项验收未在此 worktree 执行。
- 无迁移、无配置 schema 变化、无生产业务数据写入。

## 恢复证据

原 dirty diff 保全文件：`/private/tmp/t76-unfreeze-20260827/v1-worktree.diff`；SHA-256：`5e8ffd98f7e6ab69443036f7de104c6f7febcc16ec56544439b9881f52d18dbd`。v2 worktree 原样保留。
