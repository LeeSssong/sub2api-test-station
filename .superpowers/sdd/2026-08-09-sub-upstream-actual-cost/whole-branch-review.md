# Whole-branch review

审查范围：`codex/usage-upstream-actual-cost` 当前候选工作区（Task 1 已提交，Task 2/3 仍在工作区未提交）。

## 结论

**未通过，暂不能合并。** 后端定向测试和前端构建通过，但前端定向 Vitest 有 1 个失败；另外上游记录匹配实现没有严格遵守计划中的优先级。

## 阻塞问题

1. `frontend/src/components/usage/usageDetail.ts` 的 `confirmedUpstreamActualCost()` 同时要求 `profit` 非空，导致后端返回 `status: confirmed`、`upstream_actual_cost` 有值但 `profit: null` 时，上游实际扣费被错误显示为 `-`。现有测试 `src/components/usage/__tests__/usageDetail.spec.ts` 已复现：期望 `0.0032`，实际 `null`。上游实际扣费和利润应分别校验，不能让利润字段缺失遮蔽已有的上游实际扣费。

## 重要观察

2. `backend/internal/service/sub_upstream_cost.go` 在遍历每条记录时按三种条件即时返回。这样会按“响应数组顺序”决定结果，而不是严格按计划的优先级（本站 upstream_request_id→上游 upstream_request_id，再本站 upstream_request_id→上游 request_id，再本站 request_id→上游 request_id）。同一页同时出现弱匹配和强匹配时，可能错误返回弱匹配；应分三轮匹配后再返回。

## 已验证

- `go test ./internal/service ./internal/handler/admin ./internal/server/routes`：通过。
- `npm run typecheck`：通过。
- `npm run build`：通过（仅既有 Vite chunk/browserslist 警告）。
- `npm test -- --run src/api/__tests__/admin.usage.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/usageDetail.spec.ts`：`33 passed, 1 failed`，失败即上述 `confirmedUpstreamActualCost` 测试。
- 管理员顶部已收敛为本站实际扣费、上游实际扣费、利润三项；管理员重复的本站 request ID 和 relay-ops/估算/待对账展示已删除；本站 API key 未进入响应或前端。

## Follow-up verification (2026-08-09)

已修复上述两项：上游实际扣费与利润分别校验；新增跨记录严格匹配优先级回归测试。复测结果：前端定向 Vitest 34/34 通过，`npm run typecheck` 与 `npm run build` 通过，后端 `TestSubUpstreamCost` 通过。
