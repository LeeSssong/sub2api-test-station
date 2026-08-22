# T54 分组调度策略与乐观体验卡交接

状态：`READY_FOR_ROOT_REVIEW`

## 候选身份

- 任务包：T54 分组调度策略与乐观体验卡
- 基线：`main@59a71ec345e00d162bf7ceb23997b588a5d2faf7`
- 候选分支：`codex/t54-group-scheduler-policy`
- 当前候选提交：见本文件提交后的 `git rev-parse HEAD`
- 当前根 `main`：`afa5acfd87dd44ebe49a5774c684ca0fe45826d1`
- 基线关系：根 `main` 已在 T54 基线之后继续推进 T53 发布修复；候选不得直接合并，根总控须先按约束刷新到最新 `main`，解决仅本任务范围冲突并重跑直接门禁。

## 交付内容

- 分组调度策略契约支持 `weighted_override` 与 `fair` 二选一。
- 公平预设支持特惠、均衡、Pro，并自动展开权值和公平参数。
- 旧分组 JSON 覆盖继续兼容读取；非法模式、字段、范围、非有限数值和空权值集合 fail-closed。
- 原生 OpenAI scheduler 在硬资格、S1/S2、并发、故障域、sticky、重试和半开边界之后应用分组策略；公平模式增加最长闲置探索与受限机会保护。
- SettingsView 使用分组优先的模式/预设控制，支持预设回填、只读解释值和清除继承。
- AccountMonitorCard 只显示乐观可用性、首 Token 延迟、完整响应耗时和成功时间线；隐藏探测样本数、失败数、失败原因、`evidence_source`、评分构成、推荐探测依据和失败时间线语义。失败时间线显示中性“暂无结果”。后台探测/API 字段和资格链保持不变。

## 验证证据

- 后端：
  - `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(OpenAIScheduler|NormalizeOpenAI|ResolveOpenAI|SettingParse)' -count=1`
  - 结果：PASS。
  - `cd upstream/sub2api/backend && go build ./cmd/server`
  - 结果：PASS。
- 前端：
  - `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`
  - 结果：3 files / 136 tests PASS。
  - `cd upstream/sub2api/frontend && pnpm typecheck`
  - 结果：PASS。
  - `cd upstream/sub2api/frontend && pnpm build`
  - 结果：PASS；仅有既有 router-link、Browserslist、Vite chunk 警告，无构建错误。
- 范围：`git diff --check` PASS；相对 T54 基线的实现文件仅在批准的 backend/frontend 文件集合内，另含任务 ledger 报告。

## 迁移、配置与发布边界

- 无数据库迁移、无配置 schema 变化、无生产数据写入。
- 预期 `downtime_required=false`，最终以根总控刷新后发布预检为准。
- 本候选未合并、未推送、未部署、未访问生产。
- 回滚方式：二进制沿用上一已验证蓝绿镜像；策略异常时清空分组覆盖或恢复旧全局/legacy JSON 覆盖。无迁移回滚动作。

## 已知风险与未验证项

- `LastUsedAt` 仍是账号级信号，不是持久化的每组配额；公平保护按当前分组候选池计算，不提供跨副本精确每组配额。
- Pro/Plus 指标偏差的事实根因未在 T54 修复：Monitor V2 仍按 schedulable 账号建立 scope，固定时间桶把无结果桶算作 unavailable，主动探测结果仍只有 `account_id` 而没有 `group_id`。因此不存在可查询的 Pro/Plus 专属事件样本；这需要另立只读诊断或 group-scoped probe 契约任务。
- 未验证生产分组配置、账号资格、探测 runner 实际运行、数据库样本分布和线上登录态页面；根总控必须在刷新候选后完成发布预检、蓝绿部署和线上专项验收。

## 根总控下一步

1. 在最新 `main` 上刷新候选，核对只包含 T54 批准范围。
2. 重跑本交接列出的直接后端/前端门禁和发布预检。
3. 以合并后的 `main` 执行唯一发布车道；预检 `downtime_required=false` 时按既有授权继续蓝绿发布。
4. 线上只读核对分组策略生效与乐观卡片内容，不制造 Pro/Plus 事件样本。
