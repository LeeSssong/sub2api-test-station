# 账号监控成本、余额与评分权重执行进度

**上下文合同：** `docs/project/account-monitor-cost-balance-context.md`

**设计规格：** `docs/superpowers/specs/2026-08-06-account-monitor-cost-balance-and-score-weights-design.md`

**实施计划：** `docs/superpowers/plans/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan.md`

## 基线

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-monitor-cost-balance`
- Branch: `codex/account-monitor-cost-balance`
- Remote baseline: `origin/main@69caeaf816e3e01f9e0c6059c3c5262a4a12c2f6`
- Production source: `05985e62ec88b04d1e647a815eecdb1cf1155776`
- Shared `upstream/sub2api` tree: `fc455d6aecfdb07ab90587000d7c5e77902f5bb6`
- Design commit: `d2757ad99151920d5365d502ce6dfc099a527df9`

## 当前状态

`planning_awaiting_user_approval`

设计规格已由用户确认。任务专用上下文合同和实施计划已编写，尚未派发实施代理，业务代码未修改，生产未变更。

## 任务状态

| 任务 | 实施 | 独立审查 | 提交 |
|---|---|---|---|
| Task 1 预计额度字段与统一成本评分 | pending | pending | - |
| Task 2 余额快照与显式刷新策略 | pending | pending | - |
| Task 3 恢复分组评分权重入口 | pending | pending | - |
| Task 4 轻量成本弹窗与余额卡片 | pending | pending | - |
| Task 5 整体回归、视觉验收与生产门禁 | pending | pending | - |

## 生产门禁

用户授权零停机直接部署，不授权停止服务。协调者只使用已保存的 `sub2api-prod` SSH 别名；若候选迁移哈希或发布器返回 `downtime_required=true`，必须在生产变更前停止。
