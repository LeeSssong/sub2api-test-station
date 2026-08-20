# T43 调度 Top-K 放宽交接

- 任务：T43 调度 Top-K 放宽（快速迭代-10 后续）
- 基线：`main@b01f0d105`（本地根 main 在 `3ac10d847` 之后登记了 T41/T42 文档）
- 工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t43-scheduler-topk`
- 分支：`codex/t43-scheduler-topk`
- 当前状态：`READY_FOR_ROOT_REVIEW`

## 变更

- 将 `gateway.openai_scheduler.adaptive_top_k_enabled` 原生默认值从 `true` 调整为 `false`。
- 示例配置补齐自适应 Top-K 三项配置，并说明浅账号池默认采用宽候选池；显式设置为 `true` 才按质量分数差收窄。
- 增加默认关闭时的调度回归：3 个健康候选在 `lb_top_k=7` 下保持 `CandidateCount=3`、`EligibleCount=3`、`EffectiveTopK=3`、`TopK=3`，仍使用固定 load-balance 层。
- 显式开启自适应 Top-K 的既有测试全部保留，证明旧质量门槛与 sticky escape 行为仍可通过开关恢复。

## 变更文件

- `upstream/sub2api/backend/internal/config/config.go`
- `upstream/sub2api/backend/internal/config/config_test.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go`
- `upstream/sub2api/deploy/config.example.yaml`
- `docs/superpowers/specs/2026-08-20-t43-scheduler-topk-relaxation-design.md`
- `docs/superpowers/plans/2026-08-20-t43-scheduler-topk-relaxation.md`

## 验证

- RED：旧默认值下 `TestLoadDefaultOpenAIWSConfig` 失败（实际 `true`，期望 `false`）；新增调度场景确认旧自适应路径会收窄候选。
- GREEN：
  - `go test ./internal/config ./internal/service -run 'TestLoadDefaultOpenAIWSConfig|TestApplyOpenAIAdaptiveTopK|TestOpenAIAccountSchedulerAdaptiveTopK|TestOpenAIAccountSchedulerDefaultKeepsHealthyPoolWhenAdaptiveTopKDisabled' -count=1`
- `go test ./internal/service -run 'TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceTopKExcludesQuotaPaused|TestSelectTopKOpenAICandidates' -count=1`
- `git diff --check`
- 结果：全部通过。
- 额外全量包测试 `go test ./internal/service -count=1` 在候选与干净基线 `main@3ac10d847` 均复现两项既有失败：`TestOpenAIGatewayService_SelectAccountWithScheduler_Enabled_EmbeddingsSkipsChatOnlyStickyBindings` 与 `TestOpenAIGatewayService_SelectAccountWithScheduler_ClearsStickyAccountOutsideGroup`；失败断言分别为账号选择/会话清理，未触及本次 Top-K 代码。该基线差异已确认，不纳入本任务修复范围。

## 发布边界

- 无数据库迁移、无配置 schema 迁移、无 API 变更、无生产数据写入。
- 预期 `downtime_required=false`，最终以根发布预检为准。
- 功能回滚：设置 `gateway.openai_scheduler.adaptive_top_k_enabled=true`；二进制回滚沿用上一已验证蓝绿镜像。
- 根总控仍需完成合并后的直接回归、推送、宿主蓝绿发布和线上专项验证；本 worktree 未执行 merge/push/deploy。
