# T38 READY_FOR_ROOT_REVIEW Handoff

## 状态

- 任务包：T38 可调度账号最近原生探测评分保留
- 状态：`READY_FOR_ROOT_REVIEW`
- 创建基线：`main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`
- 基线 tree：`213f223f8d79ce5fd5548fae4a578261ef884547`
- 分支：`codex/t38-retain-native-probe-score`
- 实现/合同 tip：`a1e1d5045bef89118127ea688b4d040e9825b59c`
- 验证文档准备前 tip/tree：`505edeedee01339798777461a1a598d02afe59b1` / `bb09fd65ff9f3cfe55e368bc30b8d842df857c53`
- 最终候选 tip/tree：见本任务提交报告中的 fresh `git rev-parse HEAD` 与 `HEAD^{tree}`；handoff 文档提交自身会推进 tip。

## 交付行为

1. 当前状态仍由最新原生探测、新鲜阈值、连续失败和致命错误决定，继续如实显示 `normal|abnormal|unavailable|stale`。
2. selected-window 评分资格独立判断：至少一个样本、至少一个成功样本，并满足 T38 新分支的快照时 `Account.IsSchedulable()`。
3. 最新不可用或当前 stale 的可调度账号仍用同一 `24h|7d|30d` 窗口 `account_monitor_results` 聚合计算原公式评分和排名。
4. 纯失败、无样本不评分；不跨窗口回退。
5. T32 已有 eligible/capped 暂停账号路径保持；global/group 共用 helper；group 成本资格保持附加门禁。
6. API/UI 无字段或生产组件变化，只增加直接合同测试。

## 提交

- `36386c3a0` `docs: specify t38 probe score retention`
- `55fe19f0e` `docs: fix t38 spec eof formatting`
- `64ebe17fd` `docs: plan t38 probe score retention`
- `f1188f325` `test: reproduce t38 probe score retention`
- `debb9c243` `fix: retain schedulable native probe scores`
- `a1e1d5045` `test: lock t38 score status compatibility`
- `505edeede` `docs: fix t38 plan eof formatting`
- 最终 verification/handoff 文档提交：以任务最终 HEAD 为准。

## 变更文件

- `docs/superpowers/specs/2026-08-20-t38-schedulable-native-probe-score-retention-design.md`
- `docs/superpowers/plans/2026-08-20-t38-schedulable-native-probe-score-retention.md`
- `docs/superpowers/reviews/2026-08-20-t38-spec-self-review.md`
- `docs/superpowers/reviews/2026-08-20-t38-plan-self-review.md`
- `docs/superpowers/reports/2026-08-20-t38-schedulable-native-probe-score-retention-verification.md`
- `docs/handoffs/2026-08-20-t38-schedulable-native-probe-score-retention-handoff.md`
- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

## 验证摘要

- TDD RED：helper 未定义，focused service build 如预期失败。
- TDD GREEN：状态/评分 helper 与 ListWindow global/group 矩阵 PASS。
- Backend：Account Monitor focused、全命名回归、handler contract、受影响包 compile-only PASS。
- 成本/v7：分组成本资格与 Monitor V2 原生 scope 合同测试 PASS。
- Frontend：账号卡片 `52/52` PASS；`pnpm typecheck` PASS。
- `gofmt` 与 `git diff --check` PASS。
- 详细命令与输出见 verification report。

## 未验证与根总控动作

- 当前根 `main@d2d1814e76abe0ff00ebb234240a8ea816157fcf` 已领先创建基线；整合前按根总控规则核对最新 main，必要时让候选进入刷新流程并重跑直接测试。
- 本任务未合并 main、未推送、未生成发布证据、未部署、未访问或修改生产。
- 根总控负责唯一整合、合并后直接门禁、推送、`downtime_required` 预检、蓝绿发布和线上专项。

## 迁移、配置、停机与回滚

- 迁移：无。
- 配置：无。
- 生产数据写入：无。
- `downtime_required`：预计 `false`，最终以根预检为准。
- 回滚：revert T38 提交并通过既有根发布链重新发布；无数据回滚。

## 剩余风险

- stale 状态下分数代表所选窗口历史原生探测表现，不代表当前健康。
- 评分排名不等于 scheduler 当前选择顺序。
- 根 main 漂移需在唯一整合车道处理，不得直接发布本 worktree。
