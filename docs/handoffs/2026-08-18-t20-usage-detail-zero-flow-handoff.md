# T20 Handoff

- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`main@d579e6f99f4f281227578676dff060df92e3f870`
- 候选分支：`codex/t20-usage-detail-zero-flow`
- 隔离 checkout：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t20-usage-detail-zero-flow`
- 范围：UsageDetailDialog 移除过时 evidence unavailable 提示；account-financial 分组按活跃 `account_groups` membership 预置零值账号后叠加现有 usage/probe 聚合。

## 变更

- `backend/internal/repository/usage_log_repo_stats.go`：在现有 repeatable-read 事务中读取活跃账号/分组 membership，加入内部快照字段。
- `backend/internal/service/account_financial.go`：分组创建后按 membership 建立零值账号节点，保留历史流水与探测行兼容。
- `frontend/src/components/usage/UsageDetailDialog.vue`：移除 unavailable reason 提示，保留 evidence 请求和辅助字段。
- 直接相关测试、规格、计划已随候选保留。

## 验证

- `GOCACHE=/private/tmp/t20-gocache go test ./internal/service -run 'TestAccountFinancialReport' -count=1`：通过。
- `GOCACHE=/private/tmp/t20-gocache go test ./internal/repository -run 'Test(ReadAccountFinancialUsage|AccountFinancialProbe)' -count=1`：通过。
- 前端 `UsageDetailDialog.spec.ts` + `UsageDetailDialog.compat.spec.ts`：25/25 通过。
- `vue-tsc --noEmit`：通过。
- `vite build`：通过；仅有既有 chunk/dynamic import 警告。
- `gofmt`、`git diff --check`：通过。

## 未验证与发布

- 未运行全仓测试、压力/mutation/soak 或无关浏览器矩阵。
- 未合并根 `main`、未推送、未执行发布预检/部署/线上验收；预期 `downtime_required=false`，最终以根合并后的预检为准。
- 无迁移、配置、依赖或生产数据变化；回滚为上一已验证根提交/活动槽。
- 当前沙箱禁止根 `.git` 的 index/object/ref 锁，无法在根目录完成提交、注册真实 git worktree 或授权合并；候选 checkout 保留完整改动，待根发布环境导入。
