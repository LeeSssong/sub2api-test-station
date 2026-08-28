# T84 自适应模型检测交接

## 任务状态

- 状态：`READY_FOR_ROOT_REVIEW`
- 任务包：T84
- 基线：`main@5df6cf20d`
- 候选分支：`codex/t84-active-probe-adaptive`
- 未合并、未推送、未部署

## 已实现

- 账号与分组新增主动探测开关，缺省开启，显式关闭才停用。
- 自动账号连接探测、定时模型检测同时检查账号开关、分组开关和当前北京时间 5 分钟真实请求空桶。
- 模型检测按自然 6 小时窗口运行：默认 `low`；仅明确可疑证据在下一窗口升为 `medium`、再下一窗口升为 `high`；`high` 完成后回到 `low`。
- `failed`、`insufficient`、检测器不可用、超时和无明确冲突证据的 `abnormal` 不升档、不立即重试。
- 入队后、实际访问 detector 前再次检查空桶；期间出现真实请求则记录 insufficient/bucket-busy 结果，不访问上游。
- 自动路径使用 `slot_key` 去重，每个账号每个 6 小时窗口最多一个 run。
- 创建账号、OAuth/RT 批量导入、Codex session/PAT 导入均传递 `active_probe_enabled`。
- 新增向后兼容 migration：`229_active_probe_switches.sql`；无生产数据回填和配置变更。

## 直接相关验证

- 前端 `CreateAccountModal.spec.ts`、`EditAccountModal.spec.ts`：`64/64` 通过。
- 后端主动探测/模型检测聚焦测试：通过，包含空桶二次检查、6 小时升档策略、失败/insufficient 不升档和 slot 去重。
- `go test ./internal/service -run 'Test(CreateAccountPersistsActiveProbeSetting|UpdateAccountPersistsActiveProbeSetting|BulkUpdateAccountsPersistsActiveProbeSetting|RunDueSlots|ScheduledExecuteRechecksUsage|NextScheduledDetectionProfile|DueDetectionSlot|ActiveProbeSwitches|AccountModelDetection)' -count=1`：通过。
- `go test ./internal/handler/admin -run 'Test(OpenAIOAuthCodexPATBoundary|ImportCodex|.*ActiveProbe|.*LongContext)' -count=1`：通过。
- `go test ./internal/handler/dto -run 'Test.*' -count=1`、migration 定向测试：通过。
- `go build ./cmd/server`：通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `git diff --check`：通过。

## 迁移与发布

- migration：`229_active_probe_switches.sql`（向后兼容默认值；需按根发布链执行迁移保护）。
- 配置变化：无。
- `downtime_required`：候选尚未执行发布预检，最终以根合并后的预检输出为准。
- 回滚：按根发布链回滚到基线 `5df6cf20d`；迁移按既有向前兼容策略保留，不做破坏性回退。

## 剩余风险

- 尚未在主站或验收站部署验证。
- detector 上游请求量仍取决于账号数量与每个自然窗口的空桶准入；slot 去重和执行前门禁已覆盖瞬时重复/繁忙桶场景。
