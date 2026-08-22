# T52 调度公平性与管理员实时参数页交接

状态：`READY_FOR_ROOT_REVIEW`（候选未合并、未推送、未部署）

## 候选

- 基线 `main`：`c6fc031c65e4c6902afa2a894eb18f4484a15983`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t52-scheduler-fairness`
- 分支：`codex/t52-scheduler-fairness`
- 候选提交：`6b4e68b9c3f0f5d5bdc6ddcec83a841c06b9194f`
- 候选 tree：`e48612e546912f84e1e979617bd07f05eb993734`
- 规格：`docs/superpowers/specs/2026-08-22-t52-scheduler-fairness-design.md`
- 计划：`docs/superpowers/plans/2026-08-22-t52-scheduler-fairness.md`

## 实现

- 复用原生 `settings` key/value、管理员设置 API 和 `SettingsView`。
- 新增候选池模式 `top_k` / `all_eligible` / `hybrid`，探索比例、饥饿阈值、公平权重和按分组 JSON 覆盖。
- `fairness_weight` 已纳入候选评分；`all_eligible` 不再被 adaptive Top-K 收窄；hybrid 在阈值/比例触发时最多增加一个最久未用候选，仍经过现有资格、并发、失败域和最终抢槽检查。
- sticky 请求不进入公平探索；计费、usage_logs、重试、S1/S2 和并发语义未改。
- 中英文管理员页面所有新参数及既有调度权重均有字段旁小字说明，明确作用、范围和调大/调小效果。
- 无数据库迁移、无生产数据写入、无 GitHub Actions 变更。

## 验证

- `go test ./internal/service -run 'SchedulerFairness|OpenAIAccountScheduler' -count=1`：通过
- `go test ./internal/handler/admin -run 'SettingHandler' -count=1`：通过
- `pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts`：37/37 通过
- `pnpm typecheck`：通过
- `go build ./cmd/server`：通过
- `pnpm build`：通过
- `gofmt` 与 `git diff --check`：通过
- 未运行全仓测试、生产发布链和线上专项验收；这些由根发布总控在合并后的 `main` 上执行。

## 发布属性与回滚

- 预期 `downtime_required=false`，最终以根发布预检为准。
- 回滚：恢复上一已验证的 `main`/蓝绿制品；运行时也可将公平参数切回 `top_k` 或恢复旧设置值。
- 生产验证重点：管理员 GET/PUT 设置实时回显；Pro/特惠组账号覆盖率、最大账号占比、最久未使用账号和错误率不恶化。

## 剩余风险

- 候选池很浅且所有账号都在阈值内时，hybrid 的 ratio 由 session hash 稳定分桶，短窗口内不是严格 round-robin。
- `LastUsedAt == nil` 的账号仍保留现有候选排序兼容语义；已有历史时间戳的长期闲置账号会按阈值优先探索。
- 生产行为尚未在合并树验证，不能标记 `DONE`。

下一步：根发布总控审查候选并发出 `AUTHORIZE_MERGE_TO_MAIN` 后，再合并、发布和线上验收。
