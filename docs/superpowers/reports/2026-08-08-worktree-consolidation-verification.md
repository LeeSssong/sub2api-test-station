# 非 main 工作区整合候选验证

验证日期：2026-08-08（Asia/Shanghai）

## 候选身份

- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/worktree-consolidation-20260808`
- 分支：`codex/worktree-consolidation-20260808`
- HEAD：`8cc6c8c22`
- 已同步根目录 `main@a0aae015a`，同步提交：`5fc9aeaba`
- Task 2 最终修复提交：`c4e22e19c`
- Task 3 流程约束提交：`eff539f95`

## 整合范围

- 已为所有可读取的非 `main` worktree 保存二进制 diff、暂存 diff、未跟踪文件和状态元数据；恢复包位于 `.superpowers/sdd/2026-08-08-worktree-consolidation/recovery/`。
- `codex/fix-relay-admin-auth` 的账号监控探测口径、管理员流水成本详情、上游请求 ID 和 relay-ops 对账能力已选择性整合。
- `codex/account` 的过时代码未整体覆盖候选；已保留恢复证据。
- `fix/sub2api-0171-release` 的旧 updater/迁移发布逻辑已判定被当前本地/主机链 supersede；未恢复 GitHub Actions。
- `codex/fix-monitor-reliability` 的旧 `Dockerfile.release-layer` 已明确归档为 0.1.168 stale generated evidence，不作为发布输入。

## 合并后验证

- 前端：账号卡片、管理员流水详情、API 契约共 4 个文件、62 项测试通过；`pnpm typecheck` 通过；`pnpm build` 成功。
- 后端：账号监控 service、admin handler、repository、DTO 使用 `go test ... -count=1` 通过。
- relay-ops：`go test ./... -count=1` 通过。
- 发布链：updater delegation 与 blue-green host safety/recovery/immutable release suites 全部通过。
- `git diff --check` 通过；`git ls-files -u` 无未解决冲突；候选工作树干净。

## 保护线程与生产边界

- `新建运营界面` 当前只交付设计文档，未产生可并入本候选的运行时代码；其设计 worktree 保留为保护例外。
- `优化账号卡片` 在暂停指令到达前已由其独立发布 worktree 切换生产到 blue（提交 `8562ca848...`）；该动作不属于本整合候选，本候选未再次执行生产切换。应用内悬浮层线上验收仍未完成，相关 worktree 和证据保留。
- 本候选未推送、未部署、未执行蓝绿切换。项目总账继续保持“进行中”；只有后续明确生产窗口完成“推送 + 部署 + 线上验证”后才能标记完成并清理已整合 worktree。
