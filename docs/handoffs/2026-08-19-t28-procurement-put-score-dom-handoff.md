# T28 Handoff

- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`main@eaf59c5d6cd8a7b3581a37d61d1694cf1558ca0b`
- 候选分支：`codex/t28-procurement-put-score-dom`
- 范围：采购 PUT 事务顺序/错误隔离、前端采购幂等键与 PUT+reload 反馈、评分 DOM 左到右评分/优先级/排名。
- 规格：`docs/superpowers/specs/2026-08-19-t28-procurement-put-score-dom-design.md`
- 计划：`docs/superpowers/plans/2026-08-19-t28-procurement-put-score-dom.md`

## 变更文件

- `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- `upstream/sub2api/backend/internal/handler/admin/account_handler_procurement_cost_test.go`
- `upstream/sub2api/frontend/src/api/admin/accounts.ts`
- `upstream/sub2api/frontend/src/api/__tests__/admin.accounts.upstreamBillingProbe.spec.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

## 实现摘要

采购字段存在且服务可用时，handler 先调用既有 `UpdateProcurementConfig`（单事务、版本台账、审计、幂等），失败立即映射并返回，不再先执行通用账号更新；随后通用更新传入 nil 采购字段，保留原有账号更新响应契约。前端采购编辑请求生成并复用 `Idempotency-Key`，成功后清除，失败重试继续复用；现有保存后 reload 成功/失败反馈保持不变。评分卡 DOM 顺序固定为评分、优先级、排名。

## 验证

- `go test ./internal/handler/admin ./internal/service -run 'Test(AccountHandler|ExistingAccountUpdateRoutesProcurement|UpdateAccountProcurementCost|Procurement)' -count=1`：通过。
- `pnpm vitest run src/api/__tests__/admin.accounts.upstreamBillingProbe.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`：100 tests 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `git diff --check`：通过。

## 发布边界

- 无迁移、无配置变化、未修改生产、未使用 GitHub Actions。
- `downtime_required`：预期 `false`，以根合并后发布预检为准。
- 未执行：合并、推送、部署、线上验收、worktree/分支清理。
- 回滚：根总控恢复上一已验证 main 提交/镜像；本候选保留供修复。
- 剩余风险：同一 PUT 同时修改通用账号字段与采购字段仍是两个既有服务事务；如需跨服务全量原子化另立任务。
