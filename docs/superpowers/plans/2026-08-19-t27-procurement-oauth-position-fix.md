# T27 实施计划

## 文件范围

- `upstream/sub2api/backend/internal/service/account_profitability.go`: 可空旧版本扫描、cost_pending 分支、OAuth SQL 过滤。
- `upstream/sub2api/backend/internal/service/account_profitability_test.go`: RED/GREEN 覆盖保存与报告集合。
- `upstream/sub2api/backend/internal/handler/admin/dashboard_handler_account_profitability_test.go` 或现有采购 handler 测试：确认 API 透传/错误边界（仅在现有覆盖缺口时改）。
- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`: 移动自购面板节点。
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`: 顺序与移动端回归。

## 步骤

1. 在 service sqlmock 测试中新增 cost_pending 旧 NULL 重新录入场景，预期当前代码 RED；新增报告 SQL 合同要求 `a.type = 'oauth'`，并用混合账号结果断言只有 OAuth（RED）。
2. 在前端测试中新增面板相对 `summary-grid` 与 scope nav 的 DOM 顺序断言，以及 390px 自购表容器滚动/根节点宽度断言（RED）。
3. 实现 `sql.NullFloat64`/`sql.NullTime` 扫描；仅当旧版本有效且非 `cost_pending` 时执行旧消耗折算；加入两处 OAuth SQL 过滤，保持参数和写序列不变。
4. 移动 Vue 模板中的既有面板节点，避免重写字段/状态逻辑；补必要 class 以保持自购表横向滚动。
5. 运行聚焦 Go service、handler/API、前端 AccountMonitor/AccountProfitability 测试；失败时只修复本任务范围。
6. 运行必要 `pnpm typecheck`/production build（若项目脚本存在）、`gofmt`、`git diff --check`，检查 `git status` 干净后提交并交接 `READY_FOR_ROOT_REVIEW`。

## 完成门槛

功能实现和直接相关测试通过；无迁移/配置变化；不合并、推送、部署或清理 worktree。交接报告包含 baseline、candidate SHA/tree、变更文件、测试、未验证项、回滚和风险。
