# T38 可调度账号最近原生探测评分保留直接验证

## 身份与范围

- 任务：T38
- 刷新基线：`main@beaf7aebdc1b20c70346b27164cd8291f9bb5d1a`
- 刷新基线 tree：`25c7cff2212bc8c9ff97d5fe84ca23bc075cc62e`
- 实现与合同提交 tip：`a1e1d5045bef89118127ea688b4d040e9825b59c`
- 验证文档准备前 tip：`505edeedee01339798777461a1a598d02afe59b1`
- 验证文档准备前 tree：`bb09fd65ff9f3cfe55e368bc30b8d842df857c53`
- 最终任务 tip/tree：以本报告与 handoff 提交后的外部 `git rev-parse HEAD` / `HEAD^{tree}` 回报为准。
- 生产代码范围：账号监控 service 的共享窗口评分资格 helper。
- 合同范围：service、admin handler JSON、账号卡片组件直接测试。

## TDD RED 证据

### 纯 helper 缺失

命令：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestAccountMonitorWindowScoreProjectionSeparatesCurrentStateFromScoreEligibility$' -count=1 -v
```

RED 结果：exit 1，编译失败：

```text
internal/service/account_monitor_service_test.go:454:43: undefined: accountMonitorWindowScoreProjection
FAIL github.com/Wei-Shaw/sub2api/internal/service [build failed]
```

该失败证明测试直接依赖本任务新增的状态/评分分离边界。

### ListWindow 复现

在同一生产实现尚未增加 helper 时加入 global/group 场景矩阵，再运行两项 focused tests，仍因 helper 未定义 exit 1。矩阵明确覆盖：

- 当前可调度 + selected-window 有成功样本 + 最新连续失败；
- 当前可调度 + selected-window 有成功样本 + 当前 stale；
- selected-window 纯失败；
- selected-window 无样本。

基线既有 focused tests 同时锁定旧语义：`unavailable|stale -> score_status=ineligible`，因此根因不是前端字段丢失。

## GREEN 实现证据

共享 helper：

```go
accountMonitorWindowScoreProjection(
    account Account,
    currentScoreStatus string,
    evidence AccountMonitorQualityEvidence,
) (AccountMonitorQualityEvidence, string, bool)
```

行为：

1. `SampleCount <= 0` 或 `SuccessSampleCount <= 0` 立即阻断评分；
2. 既有 `eligible|capped` 保持 T32 暂停账号语义；
3. T38 新保留路径严格调用 `Account.IsSchedulable()`；
4. scoring evidence source 规范为 `monitor_probe`，不改当前 `availability_status/stale/latest`；
5. global/group 共用 helper；group 仍额外要求既有成本资格。

首次 GREEN 命令：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorWindowScoreProjectionSeparatesCurrentStateFromScoreEligibility|TestAccountMonitorListWindowRetainsSchedulableUnavailableAndStaleNativeScores' -count=1 -v
```

结果：exit 0，2 个顶层测试全部 PASS；pure helper 的 7 个子场景全部 PASS。

T32 与评分数学直接回归：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorProbeProjectionUsesOnlyFreshProbeEvidence|TestAccountMonitorPausedProbeProjectionScoresRanksAndKeepsNoEvidencePending|TestAccountMonitorWindowScoreBreakdownSumsToRoundedQualityScore|TestCalculateAccountMonitorQualityScore' -count=1 -v
```

结果：exit 0，相关顶层/子测试全部 PASS；原评分公式、舍入、异常封顶与 T32 行为保持。

## API/UI 合同

Handler 合同：

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin -run '^TestAccountMonitorHandlerReturnsUnavailableAndStaleRowsWithRetainedNativeScores$' -count=1 -v
```

结果：exit 0，PASS。未修改生产 handler/mapping；现有 JSON 已支持：

- `availability_status=unavailable` + 非空 `quality_score/group_rank`；
- `availability_status=stale` + `stale=true` + 非空 `quality_score/group_rank`。

前端合同：

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

结果：exit 0，`1 file / 52 tests` 全部 PASS。现有生产组件已独立读取状态与 `score_status`，因此没有修改 `AccountMonitorCard.vue`。

TypeScript：

```bash
cd upstream/sub2api/frontend
pnpm typecheck
```

结果：exit 0，`vue-tsc --noEmit` 无错误。

## 最终直接验证矩阵

Backend focused：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'AccountMonitor.*(WindowScore|ListWindow|ProbeProjection|PausedProbeProjection|QualityScore|ScoreBreakdown)' -count=1
go test ./internal/handler/admin -run 'AccountMonitor.*(CompleteWindow|UnavailableAndStaleRows|List)' -count=1
go test ./internal/service ./internal/handler/admin -run '^$'
```

结果：三条命令均 exit 0；service、handler focused 与两包 compile-only 全部 PASS。

Account Monitor 全量命名回归：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'AccountMonitor' -count=1
```

结果：exit 0，PASS。

成本门禁与 Monitor V2 v7 边界：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorProjectionSeparatesManagementServiceAndGroupEligibility|TestAccountMonitor.*ProjectMonitorV2|TestAccountMonitorProjectMonitorV2' -count=1 -v
```

结果：exit 0，3 个测试 PASS；分组 `cost_ineligible` 继续无评分/排名，Monitor V2 继续以原生 `IsSchedulable()` scopes 与原 v7 repository 合同工作。

## 格式与范围

- `gofmt` 已应用于三份受影响 Go 文件。
- `git diff --check beaf7aebdc1b20c70346b27164cd8291f9bb5d1a HEAD --` 在 EOF 修复后 exit 0、无输出。
- 变更不包含 migrations、`.github/workflows`、`docs/project/project-progress.md` 或 `docs/project/native-sub-task-package-queue.md`。
- 数据库迁移：无。
- 配置变化：无。
- 生产数据写入：无。
- GitHub Actions：无。

## 未验证项与发布属性

- 已刷新合入根最新 `main@beaf7aebdc1b20c70346b27164cd8291f9bb5d1a`；根总控仍需在整合车道执行最终合并后门禁、发布预检与线上验证。
- 未执行 root-main 合并、推送、发布资格证据、发布预检、蓝绿部署或生产登录态专项。
- 预计 `downtime_required=false`；根合并后的发布预检是最终事实源。
- 回滚：撤销 T38 功能与合同提交，并通过受审根发布链重新发布；无数据回滚。
- 剩余风险：stale 分数来自当前所选窗口较早证据，必须与醒目的当前状态共同理解；排名是监控评分排名，不代表 scheduler 当前选择顺序。
