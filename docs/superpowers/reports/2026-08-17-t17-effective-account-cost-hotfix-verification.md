# T17 用量详情有效账号成本口径热修验证报告

## 范围

- 任务包：T17
- 候选实现基线：`main@120562f95`（候选随后包含 `e887bfe44` 刷新提交）
- 实现 tip：`e6fde59ba`
- 目标：管理员用量详情的“上游扣费/利润”统一使用 Sub 原生有效账号成本；严格 upstream evidence 只作辅助核验状态。
- 分支：`codex/t17-effective-account-cost-hotfix`

## 已验证命令

前端（工作目录 `upstream/sub2api/frontend`）：

- `pnpm exec vitest run src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/UsageDetailDialog.compat.spec.ts`：通过，3 个文件 / 38 个测试。
- `pnpm typecheck`：通过。
- `pnpm build`：通过，Vite 完成 1046 个模块转换并生成生产资源。

后端（工作目录 `upstream/sub2api/backend`）：

- `go test ./internal/handler/admin -run 'TestAdminUsageGetByID|TestAdminUsageGetUpstreamCost' -count=1`：通过。
- `go test ./internal/handler/dto -run 'Test.*Usage.*' -count=1`：通过。
- `go test ./internal/server -run '^$' -count=1`：通过（编译门禁，无测试文件）。
- `go build ./cmd/server`：通过。
- 受影响 Go 测试文件执行 `gofmt -w` 后无工作区差异。

范围检查：

- `git diff --check`：通过。
- 数据库迁移改动：0。
- `.github/workflows` 改动：0。
- 后端生产代码改动：0；现有 DTO/API 字段契约已由 focused tests 覆盖。

## 行为矩阵

主金额公式固定为：

`COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`

- 显式 `account_cost`（包括 0）直接作为有效账号成本，不再次乘倍率。
- `account_cost` 为空时，使用 `account_stats_cost`，再乘 `account_rate_multiplier`。
- `account_stats_cost` 也为空时，使用 `total_cost`，再乘倍率。
- 所有成本源为空或非有限数时，主金额保持 `-`。
- 利润始终为 `actual_cost - effective_account_cost`。
- evidence `confirmed`、`unavailable` 或 evidence 请求失败均不替换、不隐藏主金额和利润。
- PascalCase/snake_case evidence 兼容层继续保留，strict evidence 的状态、原因和来源仍可辅助展示。

## 边界与未验证项

- 未修改价格、倍率、成本写入、经营页聚合、账务幂等、数据库、生产数据或历史 evidence 表。
- 未运行全仓测试、压力/soak/mutation、race 或无关浏览器矩阵；这些不属于 T17 直接相关门禁。
- `downtime_required` 尚未由根发布预检确认；候选在根合并前不得宣称可直接部署。
- 功能回滚方式：恢复上一版已验证的不可变蓝绿镜像；本改动无数据库迁移、无历史回填、无运行时开关。

## 结论

T17 候选实现和直接相关验证已完成，候选应停在 `READY_FOR_ROOT_REVIEW`。根总控仍需刷新到最新 `main`（当前根含 docs-only 提交 `85454d883`），再执行合并后的专项门禁、推送和发布预检。
