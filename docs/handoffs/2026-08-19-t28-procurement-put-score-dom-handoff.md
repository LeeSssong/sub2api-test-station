# T28 Handoff

- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`main@f27fc4d65ce41f9e32ea2ea5ddcb5b7b22bb676d`
- 候选分支：`codex/t28-procurement-put-score-dom`
- 范围：采购-only PUT 原子边界、成本弹窗会话级幂等键与 PUT+reload 反馈、账号卡片按评分排名进入最终 DOM。
- 规格：`docs/superpowers/specs/2026-08-19-t28-procurement-put-score-dom-design.md`
- 计划：`docs/superpowers/plans/2026-08-19-t28-procurement-put-score-dom.md`

## 变更文件

- `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- `upstream/sub2api/backend/internal/handler/admin/account_handler_procurement_cost_test.go`
- `upstream/sub2api/frontend/src/api/admin/accounts.ts`
- `upstream/sub2api/frontend/src/api/__tests__/admin.accounts.upstreamBillingProbe.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

## 实现摘要

采购字段存在且服务可用时，handler 先调用既有 `UpdateProcurementConfig`（单事务、版本台账、审计、幂等），失败立即映射并返回；采购-only 成功后直接 `GetAccount` 返回刷新采购值，不调用通用更新，混合 PUT 才调用通用更新且采购字段为 nil。前端由成本弹窗会话按账号和 payload 生成并显式传递 `Idempotency-Key`：同会话同 payload 的未知结果重试复用，关闭重开、成功、payload 改变、保存/清空切换均换键。页面保留既有 `group_rank` 升序与普通 Grid，新增乱序输入的最终 DOM 顺序和无 reverse/order 保护测试；与目标无关的单卡三格重排已撤回。

## 验证

- `go test ./internal/handler/admin ./internal/service -run 'Test(AccountHandler|ExistingAccountUpdateRoutesProcurement|UpdateAccountProcurementCost|Procurement)' -count=1`：通过。
- `pnpm vitest run src/api/__tests__/admin.accounts.upstreamBillingProbe.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`：101 tests 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `git diff --check`：通过。

## 发布边界

- 无迁移、无配置变化、未修改生产、未使用 GitHub Actions。
- `downtime_required`：预期 `false`，以根合并后发布预检为准。
- 未执行：合并、推送、部署、线上验收、worktree/分支清理。
- 回滚：根总控恢复上一已验证 main 提交/镜像；本候选保留供修复。
- 剩余风险：同一 PUT 同时修改通用账号字段与采购字段仍是两个既有服务事务；如需跨服务全量原子化另立任务。
