# T12 本站探测花费与经营页实现报告

## 候选身份

- 任务：T12 经营页本站探测花费与排序 / USD 字段优化
- 最新刷新基线：`main@b16d45203ff4789aa7adf1f1dcb92f7a0801ca74`
- 最新主线刷新后运行时测试提交：`35baf14ae1b9f9ce522e27b13336872f237cec8a`
- 测试 tree：`f84461ee2358d2a4a0a1fc3a48932c40fe5d9ee9`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t12-native-probe-cost-cards`
- 分支：`codex/t12-native-probe-cost-cards`
- 状态：`READY_FOR_ROOT_REVIEW`；未合并、未推送、未部署、未访问生产

## 实现结果

- 新增 append-only `account_probe_cost_logs`，探测成本与 `usage_logs` 用户事实源隔离。
- `account_id` 使用 `ON DELETE RESTRICT`；`probe_run_id` 幂等，金额保留 `DECIMAL(20,10)` 精度。
- 只有 `monitor`、`scheduled`、`manual` 三类显式探测进入记录链路；recovery/legacy 调用不自动计量。
- 完整用量复用原生 `BillingService.CalculateCostUnified`；不完整或定价失败时成本为 `NULL`。
- 写入失败 fail-open，不改变原探测结果、用户余额、用户扣费、利润或利润率。
- account-financial 独立聚合 probe 数据；成功无记录为 `unavailable`，查询失败为报告级 `probe_data_error=true`、稳定错误码且所有 probe 字段为 `null`。
- 经营页保持全站、分组、账号三层；账号层一账号一卡，同卡显示用户流水账号计费和本站探测花费。
- 六项排序仅覆盖请求、Token、账号计费、用户扣费、利润、利润率；利润率空值始终置底，账号 ID 作为稳定同值次序。
- 页面金额统一 USD 两位小数、利润率两位；兼容字段 `user_unconsumed_balance_cny` 未重命名，但本页按 USD 展示。
- 账号卡片网格为移动端单列、`sm` 起两列且不增加第三列；页面主容器禁止横向溢出。

## 新鲜验证

以下命令均在 P0 收口后的最新刷新候选 tree 上执行，退出码为 0：

```text
go test ./migrations -run TestAccountProbeCostLogsMigration -count=1
go test ./internal/repository -run 'TestAccountProbeCost|TestReadAccountFinancialUsage|TestAccountFinancialProbe' -count=1
go test ./internal/service -run 'TestAccountProbe|TestAccountMonitorProbe|TestAccountFinancial.*Probe|TestAccountFinancialReport' -count=1
go test ./internal/handler/admin -run 'Test.*Account.*Test|TestAccountFinancial.*Probe|TestAccountFinancialReport' -count=1
pnpm vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts src/api/admin/__tests__/accountFinancial.spec.ts
pnpm typecheck
pnpm build
git diff --check
```

- 前端结果：2 个直接测试文件、19/19 用例通过。
- 构建结果：1044 modules transformed，构建成功。
- 构建保留既有警告：Browserslist 数据过期、动态/静态 import 混用、Node `DEP0190`；无新增构建错误。
- `gofmt -d` 对全部 T12 Go 差异无输出；`git diff --check` 通过。
- 最新 P0 的 auth、config、setting gate 与 session-binding 文件相对 `main` 零差异，T12 刷新未撤销或覆盖 P0 行为。

## 范围与安全扫描

- `main...HEAD` 没有任务侧 `docs/project/*` 差异。
- migration 未修改、插入、更新或删除 `usage_logs`。
- migration 不包含 `CASCADE`、`user_id` 或 `api_key_id` 占位字段。
- migration 明确包含 `accounts(id) ON DELETE RESTRICT`。
- 没有 `.github/workflows/*` 变化，没有 GitHub Actions。
- 没有任务侧 `docs/project/*` 变化；根队列与总账仍由唯一发布总控维护。
- 没有历史 insert/update、生产访问、生产账号修改或数据回填。

## 迁移、发布与回滚

- migration：`224_account_probe_cost_logs.sql`
- SHA-256：`6f737666ba9a4ddd98642f7d6fa21a6356d93f9b93f5444f65156f61011dfd4d`
- 语义：add-only 空表、索引、检查约束与 restrictive 外键；不回填。
- `downtime_required`：候选 worktree 不运行发布预检；必须在根 `main` 合并后的既有发布预检中判定。若为 `true`，停在任何停服/迁移/切换前等待用户确认。
- 回滚：停止 probe 新写入和页面展示，保留表与既有行；旧用户财务继续只读 `usage_logs`。禁止删表或改写历史作为回滚。

## 未验证项与剩余风险

- 尚未在生产执行 migration、蓝绿发布、健康检查或管理员登录态页面验收。
- 390px 和桌面布局由页级 DOM/class 合同覆盖；真实浏览器尺寸、无横向溢出与交互须在发布后做一次定向验收。
- 启用后才开始记录 probe；上线初期没有自然记录时应显示 `$0.00` 与“暂无探测记录”，不制造生产请求或回填数据。
