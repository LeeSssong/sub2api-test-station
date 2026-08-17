# T16 经营页真实结果与视觉层级重设计交接

## 候选身份

- 任务：T16
- 分支：`codex/t16-profitability-visual-hierarchy`
- 基线：`main@483dde398`
- 实现候选提交：`a9068dbb683c583ad3cefe01943899638b7abe1e`
- 实现候选 tree：`bc9a257f41589f6d20a6f4770cd7171a8e5d7326`
- 当前分支 tip 另包含本 handoff 文档；根总控应以分支当前 `HEAD` 作为候选身份。
- worktree：`/Users/gongtengxinwen/.codex/worktrees/026c/sub2api搭建`
- 状态：`READY_FOR_ROOT_REVIEW`

## 变更范围

- `backend/internal/repository/usage_log_repo_stats.go`：在原 repeatable-read 原生用量快照内关联 `users.role`，按明确 `admin` 分类内部运营消耗，其余流水分类业务；保留原有效账号成本公式。
- `backend/internal/service/account_financial.go`：新增 `operational_cost`、`business_cost`、`business_revenue`、`total_cost`、`net_profit`、`external_margin`，并保持 `cost/user_cost/profit/margin` 数学一致。
- 后端直接相关 sqlmock、service、handler 回归测试。
- `frontend/src/api/admin/accountFinancial.ts`：新增字段归一化，兼容 snake_case、PascalCase 和旧响应回退。
- `frontend/src/views/admin/AccountProfitabilityView.vue`：五项经营摘要、当前角色历史风险说明、分组结果摘要、五列账号明细表、五项排序、负利润警示和受控表格横向滚动。
- 中英文 admin 本地化与页面/API 测试。
- 正式规格：`docs/superpowers/specs/2026-08-17-t16-profitability-visual-hierarchy-design.md`
- 实施计划：`docs/superpowers/plans/2026-08-17-t16-profitability-visual-hierarchy.md`

## 验证

后端（均通过）：

```text
go test ./internal/repository -run 'TestReadAccountFinancialUsage' -count=1
go test ./internal/service -run 'TestAccountFinancialReport' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancialReport' -count=1
go test ./internal/repository ./internal/service ./internal/handler/admin ./internal/server/routes -run '^$'
go build ./cmd/...
go test ./internal/repository ./internal/service ./internal/handler/admin -count=1
```

前端（均通过）：

```text
pnpm vitest run src/api/admin/__tests__/accountFinancial.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts
2 files / 19 tests passed
pnpm typecheck
pnpm build
```

范围与卫生：

- `git diff --check main...HEAD` 通过。
- 候选工作区干净。
- 相对基线无 migration、package/dependency、配置、infra、ops、workflow 或生产数据文件变化。

## 数据/产品边界

- 仅当前 `users.role='admin'` 的流水属于内部运营；缺失或其他角色归入业务侧。
- 历史角色变化可能重分类历史时间窗，页面已明确提示；没有不可变 actor 字段、迁移或回填。
- 管理员免费流水的真实上游成本保留在总消耗中，不以 `user_cost=0` 推断身份。
- 探测成本字段仍由后端兼容返回，但不再决定经营页主摘要或账号明细。

## 发布状态

- 当前用户指令为暂不发布，未合并根 `main`、未推送候选、未运行发布预检、未部署、未访问生产。
- `downtime_required=unverified`；后续只能由根总控在 T15 停机门禁处置、用户明确允许发布后再做合并和预检。
- 若根预检返回 `downtime_required=false`，按全局规则可继续蓝绿发布；若返回 `true`，停在人工门禁。
- 回滚：蓝绿切回上一活动槽/镜像，无数据回滚步骤。

## 剩余风险

- 当前角色 join 不是历史不可变 actor 事实；如产品要求历史精确分类，需另立最小数据契约任务。
- 未运行全仓、压力、mutation、soak、无关浏览器矩阵或线上验收，均不属于本候选直接功能门禁。
