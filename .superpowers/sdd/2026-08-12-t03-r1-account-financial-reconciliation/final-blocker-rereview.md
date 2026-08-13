# T03-R1 Final Blocker Scoped Re-review

日期：2026-08-13（Asia/Shanghai）

## Scope

- Base: `5ee523bbbd33b5879a036b4c74b4d4e1c1387503`
- Head: `b4f38e7e74a504780e03d16749693a38c4578ae3`
- Review package: `review-5ee523bbb..b4f38e7e7.diff`
- 本次仅复核管理员 `UsageHandler.GetUpstreamCost` 的读时上游 fallback 阻断修复、定向回归与范围边界。

## Finding disposition

**Finding: ADDRESSED**

原 blocker 已消除：

- `UsageHandler` 不再保存 `SubUpstreamCostService`，`GetUpstreamCost` 中也不再存在 `GetByUsageID` 可执行 fallback。
- 正常注入路径只调用本地 `AccountFinancialService.GetUsageEvidence`；生产 wiring 仍由 `ProvideAdminUsageHandler` 显式设置该本地服务。
- 本地财务服务缺失时立即返回 HTTP 500，不再读 usage/account credentials，也不会访问上游。
- 回归测试向兼容构造器传入连接真实 `httptest.Server` 的 legacy service，结果仍为 HTTP 500 且 server 调用计数为 0，直接证明零 read-time upstream HTTP。
- 构造器保留原参数位作为命名的 ignored compatibility parameter，已有 handler 调用点无需批量改写；定向 handler/server/cmd 编译测试通过。

## Boundary review

- 实现变更仅限 `backend/internal/handler/admin/usage_handler.go` 及其定向测试；其余差异为 SDD/项目进度/终审与发布预检证据。
- 无 schema、migration、frontend、GitHub Actions、`main`、生产或部署路径变更。
- 管理员 handler 与前端范围静态扫描对 `GetByUsageID(` 零命中。

## Verification commands

All commands passed:

```text
go test ./internal/handler/admin -run 'TestAdminUsageGetUpstreamCost|TestAccountFinancial|TestAdminUsage.*(Evidence|UpstreamCost|Exception)' -count=1
go test ./internal/handler/admin -run 'TestAdminUsageGetByID|TestAdminUsageGetUpstreamCost|TestUsageCleanup|TestAdminUsageSearchUsers|TestAdminUsageRequestType' -count=1
go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage|TestUsage.*Detail' -count=1
go test ./internal/server/... -run 'Test.*AccountFinancial|Test.*AdminUsage' -count=1
go vet ./internal/handler/admin ./internal/handler
go test ./cmd/server -run '^$' -count=1
rg -n 'GetByUsageID\(' upstream/sub2api/backend/internal/handler/admin upstream/sub2api/frontend/src
git diff --check 5ee523bbbd33b5879a036b4c74b4d4e1c1387503..b4f38e7e74a504780e03d16749693a38c4578ae3
```

The `rg` command returned no matches. Worktree status was clean before this review evidence file was written.

## Verdict

- Finding: **ADDRESSED**
- Spec Compliance: **APPROVE**
- Code Quality: **APPROVE**
- Open findings: **0**

该 scoped blocker 修复可进入顶层任务的最终收口与 `READY_FOR_ROOT_REVIEW` 交接；本审查不授权合并 `main`、推送或部署。
