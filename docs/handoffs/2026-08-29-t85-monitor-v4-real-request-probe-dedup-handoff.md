# T85 Monitor V4 真实请求成功率与探测兜底去重交接

日期：2026-08-29  
状态：`READY_FOR_ROOT_REVIEW`（仅本地候选；未合并、未推送、未部署）

## 目标与已实现口径

用户确认的 Monitor V4 口径已实现：

- 每个 5 分钟桶有真实请求时，全部使用真实请求，主动探测不混入。
- 仅当无真实请求且桶已关闭，或当前桶已进入最后一分钟时，主动探测才作为兜底。
- 一个分组/桶的探测只贡献一个逻辑请求：任一探测运行成功即为 `1/1`；全部失败才为 `0/1`。同轮或跨运行的账号尝试不会扩大分母。
- 成功率为选中成功请求数/选中总请求数；空桶不计入分母；TTFT P95 与总耗时 P95 分别只用成功样本的各自非空字段。
- 只要 `request_count > 0`，即使 `success_count = 0`，成功率也必须返回 `0%`；`null` 仅表示确实没有被选中的请求。
- 同一真实请求同时存在 `usage_logs` 与 `ops_error_logs` 时，错误记录优先且只计一次；错误会尽可能归一到 usage 的 `logical_request_id` 键。
- 自动主动探测复用原生 `IsSchedulableAt(now)`，只有带明确 401/认证证据的 API Key 临时隔离可走半开恢复探测；余额不足、配额、`403`/预扣款等临时隔离不再自动探测。

## 提交与基线

- 初始基线：`main@32334261da92721e1ea6251df1d9a951c9d184ab`
- 实现提交：`af0bfb850`（`feat: correct monitor v4 request success metrics`）
- 已同步的最新根主线：`main@9ac9d1f2f`；候选同步提交：`c51486fe0`
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

结果：仓储定向测试通过；服务定向测试通过；server build 通过；Monitor V4 Vitest 为 3 文件 11 用例全部通过；`pnpm typecheck` 通过；`pnpm build` 以 exit code 0 完成。`git diff --check` 通过。

## 范围与风险

- 无数据库迁移、配置、依赖、GitHub Actions 或生产数据写入。
- 未执行验收站/主站部署、登录态 UI 验收或生产数据查询；`downtime_required` 须由根线程在合并后的 `main` 发布预检确认。
- 仓储测试使用 sqlmock 验证 SQL 合同与扫描列；未运行需要本机 Docker/PostgreSQL 的集成测试。
- 当前历史 scope 仍沿用既有 Monitor V2 的“当前可调度账号”范围，未在本任务改变该原生可见性边界。

## 根线程后续动作

1. 确认发布单车道与目标 `main` 未再次漂移；若漂移，按约束刷新候选并重跑上方直接验证。
2. 审核此候选为当前唯一可合并任务后，再发送带目标 SHA 的 `AUTHORIZE_MERGE_TO_MAIN`。
3. 合并后的 `main` 完成直接相关回归、构建、迁移/发布预检；未经“测试站验收通过，部署主站”或“快速部署到主站”授权，不得推送或部署主站。

## 回滚

候选尚未上线，不需要运行时回滚。若未来合并后的行为需撤回，可在根 `main` 反转实现提交 `af0bfb850`，再走既有受控发布链；无数据库回滚步骤。
