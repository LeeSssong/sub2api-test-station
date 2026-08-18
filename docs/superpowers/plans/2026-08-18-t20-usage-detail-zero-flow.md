# T20 实施计划

## 1. RED：锁定现有合同与新缺陷

- 扩展 `usage_log_repo_stats_test.go` 的 sqlmock 顺序，加入 `account_groups` 查询并覆盖 membership 扫描/查询失败。
- 在 `account_financial_test.go` 增加有效分组绑定、无流水零值账号和共享账号多分组断言。
- 在 `UsageDetailDialog.spec.ts` 将过时提示断言改为“不出现”，覆盖 unavailable/unsupported/请求失败三类 evidence。

## 2. GREEN：最小实现

- 在 usage reader 快照中增加 membership 类型与 `accountFinancialUsageMembershipQuery`，只选活跃账号和分组。
- 在 `AccountFinancialService.GetReport` 创建活跃分组后按 membership 初始化账号节点，再处理流水与探测行；保留历史行兼容和现有金额派生。
- 删除 `UsageDetailDialog` 的 `upstreamCostUnavailableMessage` computed 和提示节点；保留 evidence 加载及证据字段展示。

## 3. 验证与收口

- 运行 `go test ./internal/service -run 'TestAccountFinancialReport' -count=1`、`go test ./internal/repository -run 'TestReadAccountFinancialUsage|TestAccountFinancialProbe' -count=1`。
- 运行前端 `UsageDetailDialog.spec.ts`、`UsageDetailDialog.compat.spec.ts`，随后 `pnpm typecheck`、`pnpm build`。
- 运行受影响 Go 包 compile-only、`gofmt`、`git diff --check`，检查无迁移/配置/工作流变化。
- 形成 handoff，候选停在 `READY_FOR_ROOT_REVIEW`，不自行合并、推送或部署。
