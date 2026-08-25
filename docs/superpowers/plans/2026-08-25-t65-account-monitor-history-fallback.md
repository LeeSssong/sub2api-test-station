# T65 账号监控历史最终结果回退实施计划

> For agentic workers: use superpowers:executing-plans to implement this plan task-by-task.

Goal: 让账号监控在当前证据不足时复用最近一次最终有效的模型检测与评分，并明确显示来源和时间。

Architecture: 在现有 AccountModelDetectionService 和 AccountMonitorService 读模型上增加历史最终结果选择与来源字段；评分继续调用原生评分算法，历史证据只作为展示回退，不新增表。前端 AccountMonitorCard 和模型检测对话框读取新的来源元数据并显示中文提示。

Tech Stack: Go、database/sql、Vue 3、TypeScript、Vitest。

Spec: docs/superpowers/specs/2026-08-25-t65-account-monitor-history-fallback-design.md

Global Constraints:
- 复用原生检测运行记录、account_monitor_results、窗口聚合和评分算法。
- 不新增数据库表、平行评分算法或调度事实源。
- 历史回退仅用于监控展示与既有排序，不改变调度资格。
- 所有实现先 RED 测试，再 GREEN 实现。
- 不修改 docs/project 全局队列和进度总账。

### Task 1: 扩展模型检测投影的历史来源契约

Files:
- Modify: upstream/sub2api/backend/internal/service/account_model_detection_types.go
- Modify: upstream/sub2api/backend/internal/service/account_model_detection.go
- Test: upstream/sub2api/backend/internal/service/account_model_detection_test.go

Steps:
- [ ] 写一个当前状态 insufficient、历史有 completed normal run 的失败测试。
- [ ] 运行 go test ./internal/service -run Historical，确认因没有回退元数据而失败。
- [ ] 增加最小 projection 字段与选择 helper，选择最新完成且有最终证据的 normal 或 abnormal run。
- [ ] 重跑聚焦测试确认通过。
- [ ] 提交 feat: expose historical model detection result source。

### Task 2: 增加账号监控评分回退

Files:
- Modify: upstream/sub2api/backend/internal/service/account_monitor_types.go
- Modify: upstream/sub2api/backend/internal/service/account_monitor_service.go
- Test: upstream/sub2api/backend/internal/service/account_monitor_service_test.go

Steps:
- [ ] 写当前窗口样本为零、历史有有效 aggregate 但评分为空的失败测试。
- [ ] 运行聚焦 service 测试确认当前实现返回不可评分。
- [ ] 使用现有 accountMonitorWindowScoreBreakdown 和历史 aggregate 实现有界回退；禁用或不适用账号仍不可评分。
- [ ] 验证实时证据行保持实时来源，回退行带 historical_final 来源。
- [ ] 提交 feat: retain last valid account monitor score。

### Task 3: 接入 API 类型与 UI 来源文案

Files:
- Modify: upstream/sub2api/frontend/src/api/admin/accountMonitor.ts
- Modify: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue
- Modify: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionDialog.vue
- Modify: upstream/sub2api/frontend/src/locales/zh-CN/admin.ts
- Modify: upstream/sub2api/frontend/src/locales/en-US/admin.ts
- Test: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts
- Test: upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts

Steps:
- [ ] 写 fallback DOM 文案和时间来源的失败断言。
- [ ] 运行聚焦 Vitest 确认断言失败。
- [ ] 增加可选来源字段并渲染非干扰提示；实时数据保持现有文案。
- [ ] 重跑聚焦 Vitest 确认通过。
- [ ] 提交 feat: show account monitor historical sources。

### Task 4: 集成验证

Steps:
- [ ] gofmt changed Go files。
- [ ] go test ./internal/service ./internal/repository ./internal/handler/...。
- [ ] pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts。
- [ ] go build ./cmd/server、pnpm typecheck、pnpm build、git diff --check。
