# T85 Monitor V4 真实请求成功率与探测兜底去重交接

日期：2026-08-29
状态：`READY_FOR_ROOT_REVIEW`（仅本地候选；未合并、未推送、未部署）

## 目标与已实现口径

用户确认的 Monitor V4 口径已实现：

- 每个 5 分钟桶有真实请求时，全部使用真实请求，主动探测不混入。
- 仅当无真实请求且桶已关闭，或当前桶已进入最后一分钟时，主动探测才作为兜底。
- 一个分组/桶的探测只贡献一个逻辑请求：任一探测运行成功即为 `1/1`；全部失败才为 `0/1`。同轮或跨运行的账号尝试不会扩大分母。
- 成功率为选中成功请求数/选中总请求数；已结算桶必须由真实请求或一个探测逻辑请求占位；TTFT P95 与总耗时 P95 分别只用成功样本的各自非空字段。
- 只要 `request_count > 0`，即使 `success_count = 0`，成功率也必须返回 `0%`；正常情况下每个已结算桶都会贡献真实请求或探测逻辑请求，`null` 仅允许表示窗口/事实源完整性故障。
- 用户新增强约束：每个已结算的无真实请求 5 分钟桶都必须有一个主动探测成功/失败终态；失败、超时、无可用账号、执行/读取/持久化异常均按 `0/1` 计入分母，不允许正常无样本。
- 同一真实请求同时存在 `usage_logs` 与 `ops_error_logs` 时，错误记录优先且只计一次；错误会尽可能归一到 usage 的 `logical_request_id` 键。
- 自动主动探测复用原生 `IsSchedulableAt(now)`，只有带明确 401/认证证据的 API Key 临时隔离可走半开恢复探测；余额不足、配额、`403`/预扣款等临时隔离不再自动探测。
- 真实事实 scope 与探测准入已解耦：V4 真实请求按当前已知账号/分组成员读取历史事实，探测调用仍按当前调度门禁筛选账号。
- 新增 `account_monitor_bucket_terminals` 最小观察终态表，以 `(group_id, bucket_start)` 唯一键幂等结算上一完整桶和当前桶最后一分钟；已失败终态可被后续同桶成功探测升级为成功。
- 运行器在 usage reader 缺失或读取异常时不再静默跳过账号探测；读取侧对历史缺失终态构造失败 `0/1`，并输出内部 `missing_probe_terminal_count` 完整性告警计数。

## 提交与基线

- 初始基线：`main@32334261da92721e1ea6251df1d9a951c9d184ab`
- 实现提交：`af0bfb850`（`feat: correct monitor v4 request success metrics`）
- 全失败成功率回归：`6c1f9475e`（`test: cover failed monitor v4 success rate`）
- 已同步的最新根主线：`main@84dc3c40a`；候选同步提交：`9dfcfe64d`
- 规格/计划：
  - `docs/superpowers/specs/2026-08-29-t85-monitor-v4-real-request-probe-dedup-design.md`
  - `docs/superpowers/plans/2026-08-29-t85-monitor-v4-real-request-probe-dedup.md`

## 主要变更

- 后端：`account_monitor_repo.go` 将 V4 聚合改为统一真实候选集、错误优先逻辑请求去重、真实桶优先、探测 run/bucket 两级去重与独立 P95 聚合。
- 运行层：`account_monitor_service.go` 增加严格的 401/认证恢复探测判定，避免余额和权限类阻塞账号反复探测。
- API：V4 合同从 `1` 升至 `2`；删除 availability、桶数、窗口外历史回退和 fallback 标志，改为成功率、成功/总请求数、来源计数及可空 P95。
- 前端：只显示“成功率”主环和“成功 N/M 次请求”；P95 无成功样本显示 `--`，前端合同校验拒绝来源计数不一致或旧版响应。

## 已验证

在同步最新 `main` 后重新执行：

```bash
cd upstream/sub2api/backend
gofmt -w internal/repository/account_monitor_repo.go \
  internal/repository/account_monitor_repo_test.go \
  internal/service/account_monitor_service.go \
  internal/service/account_monitor_service_test.go \
  internal/service/account_monitor_types.go \
  internal/service/monitor_v4.go \
  internal/service/monitor_v4_test.go \
  internal/handler/monitor_v4_handler.go
go test ./internal/repository -run 'TestAccountMonitorRepository.*V4|TestAccountMonitorRepositoryProjectMonitorV4' -count=1
go test ./internal/service -run 'TestMonitorV4|TestAccountMonitor.*(Pool|Run|Probe)' -count=1
go build ./cmd/server

cd ../frontend
pnpm vitest run src/features/monitor-v4
pnpm typecheck
pnpm build
```

本轮新增验证结果：仓储 V4/终态/历史清理定向测试通过；服务 V4/探测池/RunAll 定向测试通过；迁移形状测试通过；`go build ./cmd/server` 通过；Monitor V4 Vitest 为 3 文件 11 用例全部通过；`pnpm typecheck` 通过；`pnpm build` 以 exit code 0 完成；`git diff --check` 通过。

全服务包 `go test ./internal/service -count=1` 仍有既有调度/韧性观测测试失败（如 sticky weighted scheduler、embedding sticky binding、resilience observability 等），未发现失败栈指向本次 T85 文件；未将其作为本任务通过依据。

## 范围与风险

- 新增数据库迁移 `230_account_monitor_bucket_terminals.sql`，仅建立探测终态观察表；无配置、依赖、账务写入、历史回填、GitHub Actions 或生产数据写入。
- 未执行验收站/主站部署、登录态 UI 验收或生产数据查询；`downtime_required` 须由根线程在合并后的 `main` 发布预检确认。
- 本次交接后的生产只读重算显示：8 月 27 日配置可见分组为 GPT-Pro、GPT-Plus、GPT-特惠；按“无真实请求桶缺失也 fail-closed 为 `0/1`”口径，三组成功率分别为 92.4242%、88.9780%、91.3601%。详细计算证据待根线程补充到独立只读报告。
- 只读报告：`docs/superpowers/reports/2026-08-29-t85-monitor-v4-2026-08-27-readonly-recalculation.md`。GPT-Pro 为 854/924（70 失败），GPT-Plus 为 444/499（55 失败），GPT-特惠为 6,482/7,095（613 失败）；达到 95% 分别还需修复 24、31、259 个失败。
- 仓储测试使用 sqlmock 验证 SQL 合同与扫描列；未运行需要本机 Docker/PostgreSQL 的集成测试。
- 运行层当前由每轮 `RunAll` 结算上一完整桶/当前最后一分钟桶；如果监控进程长期停止，仍需后续 watchdog/补偿任务补齐停机期间的终态。读取侧已 fail-closed，不会高估成功率。
- `account_monitor_bucket_terminals` 是观察终态 ledger，不替代 `usage_logs`、`ops_error_logs` 或账号级 `account_monitor_results`，也不记录请求体、凭据或完整错误正文。

## 根线程后续动作

1. 确认发布单车道与目标 `main` 未再次漂移；若漂移，按约束刷新候选并重跑上方直接验证。
2. 审核此候选为当前唯一可合并任务后，再发送带目标 SHA 的 `AUTHORIZE_MERGE_TO_MAIN`。本候选包含新增迁移，合并后需执行迁移/发布预检。
3. 合并后的 `main` 完成直接相关回归、构建、迁移/发布预检；未经“测试站验收通过，部署主站”或“快速部署到主站”授权，不得推送或部署主站。

## 回滚

候选尚未上线，不需要运行时回滚。若未来合并后的行为需撤回，可在根 `main` 反转实现提交 `af0bfb850`，再走既有受控发布链；无数据库回滚步骤。
