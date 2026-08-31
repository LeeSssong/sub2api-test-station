# T104 Monitor V4 持久化快照交接

日期：2026-08-31（Asia/Shanghai）
状态：`READY_FOR_ROOT_REVIEW`（候选本地完成；未合并根 main、未推送、未部署）

## 目标与最终口径

- Monitor V4 不再在页面打开、轮询或切换 `24h/7d/30d` 时执行全窗口统计 SQL；singleton worker 每 5 分钟以同一 `as_of` 计算三窗口并持久化，页面只读最近一次成功快照。
- 每个 5 分钟桶真实逻辑请求优先；无真实请求时使用一次主动探测逻辑请求。同桶多账号探测不放大分母，任一成功为 `1/1`，全部失败为 `0/1`。
- 已结算但缺少探测终态的桶按最新用户决定 fail-closed 为 `0/1`，进入失败分母，并保留内部 `missing_probe_terminal_count`。
- 成功率为最终成功逻辑请求数 / 有效逻辑请求总数；明确模型不支持、客户端责任错误和最终成功请求的中间切号失败不计本站失败，最终用户可见失败必须进入分母。
- `ttft_p95_ms` 与 `latency_p95_ms` 仍只使用成功请求，保持 P95 字段和页面文案，数值为各自前后 5% 截尾平均；`cache_hit_rate` 仍只使用成功真实请求的 Sub 原生 Token 口径。

## 候选指针

- Worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t104-monitor-v4-persisted-snapshot`
- 分支：`codex/t104-monitor-v4-persisted-snapshot`
- 初始基线：`main@5e6ccee143f07ee34017c25e75979b74b6bcfc77`，tree `42dda8e317725a710340b5624bbda887cd1f6a50`
- 刷新基线：`main@fde3ece1b6e20a9e0b6a7ff47bf1e0be03213178`，tree `3c7a8c6d85d18d9c3ecb1a40dd3efaeab95315ad`
- 刷新合并提交：`09942e3f6a43222b46833db1d5ac1a9caa364dd7`，tree `bb07534c9200d6d4bccefcb6a2ceaa674f74abe2`
- 刷新无冲突，包含根总控已完成的 T105 主线；T104 未修改 T105 文件。最终文档收口提交在本交接之后生成，根总控应以分支最终 HEAD 作为候选 SHA。

## 已交付

- 新增 expand-only `232_monitor_v4_snapshots.sql`，保存 `(window, group_id)` 最新派生快照；原始事实仍为 `usage_logs`、`ops_error_logs`、`account_monitor_results` 和 `account_monitor_bucket_terminals`。
- 新增快照 repository：三窗口/多分组在一个事务内 `DELETE + INSERT` 原子替换；任一步失败回滚并保留旧成功快照；读取校验 UUID、窗口、时间、版本、计数和跨行一致性。
- `MonitorV4Service.Snapshot` 保留当前用户 active/config/available/exclusive 分组裁剪和当前组元数据，但只读取快照；没有快照或损坏快照时 fail-closed，不回退实时全窗口 SQL。
- `RefreshMonitorV4Snapshots` 以同一 UTC 分钟截点计算 `24h/7d/30d`，全部投影合法后才以一个 UUID 发布。
- `AccountMonitorRunner` 增加可选 snapshot loop：启动立即刷新，之后每 5 分钟刷新；本地防重入、4 分钟超时、固定 leader key、10 分钟 TTL、Redis/PG advisory 协调，Stop 取消并等待 goroutine。
- API 字段和前端布局不变；`generated_at` 现在表示快照生成时间。新增直接合同测试覆盖三窗口、原快照字段和失败时保留上次成功窗口。
- T103 已废弃；本任务未新增或恢复 admission、slow-session 或账号级自定义并发控制，native-only guard 通过。

## 提交序列

- `23997a946`：持久化快照表和 repository
- `f2bf70309`：快照 repository/adapter 证据补强
- `3040f6901`：页面改读持久化快照，缺失探测终态进入 `0/1`
- `969d87fa3`：快照计数不变量
- `e134d50bf`：刷新前投影校验和失败边界
- `65d6fc97a`：五分钟 singleton worker 与 wiring
- `b547f0c56`：API/UI 直接合同测试
- `f01d09898`：刷新前恢复检查点
- `09942e3f6`：无冲突合入最新根 main

## 功能验证

刷新到 `main@fde3ece1b` 后执行：

```text
go test -vet=off -count=1 -run 'TestMonitorV4|TestAccountMonitorRepositoryProjectMonitorV4|TestMonitorV4Snapshots' ./internal/repository ./internal/service
PASS: repository 0.975s; service 2.098s

go test -vet=off -count=1 -run 'TestAccountMonitorRunner|TestMonitorV4SnapshotRunner' ./internal/service
PASS: service 2.819s

go test -vet=off -count=1 -run '^TestMonitorV4SnapshotsMigration$' ./migrations
PASS: migrations 0.476s

git diff --check
PASS

bash ops/assert-native-openai-concurrency-only.sh --worktree "$PWD"
PASS: native_concurrency_guard status=passed mode=native_account_concurrency_only
```

## 未验证与环境阻断

- Handler 聚焦测试在执行目标测试前被仓库既有编译错误阻断：`handler_wiring_test.go` 的 `ProvideHandlers` 参数数量不匹配，以及 `openai_gateway_handler_test.go` 引用未定义 `openAIAccountScheduleModel`。T104 未修改这些文件。
- 前端候选没有 `node_modules`；离线 frozen install 被现有 lockfile/override mismatch 拒绝，因此新增 Vitest 未执行。测试代码已提交，但不能宣称通过。
- 按用户最新要求，本轮不运行全包回归、完整 server/frontend build、typecheck 或性能压测。
- 未在真实 PostgreSQL 执行迁移两次；幂等性由 `CREATE TABLE/INDEX IF NOT EXISTS` 静态合同覆盖。
- 未做生产数据固定截点复算或线上 API 验收；这些属于根合并/部署后的验证车道。

## 迁移、配置与停机

- 迁移：新增 `232_monitor_v4_snapshots.sql`，expand-only，无历史回填、删除或事实表改写。
- 配置：无新增配置项；快照周期固定 5 分钟，复用现有 singleton role 和 leader-lock 能力。
- 依赖：无新增依赖或锁文件变化。
- `downtime_required=unverified`：必须由根总控在合并后的干净 `main == origin/main` 上运行发布预检。任何 `true` 结果都必须停在停机授权门禁。

## 回滚与风险

- 候选尚未上线，当前回滚是根总控不合入该分支。
- 上线后应用回滚只能在根 `main` 形成明确 revert/前向修复并走受控发布链；迁移表可保留为空，不能从功能 worktree 执行人工 DROP。
- 首次 worker 生成完成前 API 会返回可重试不可用，不执行请求路径实时补算；刷新失败继续保留旧快照。
- 每 5 分钟执行三次全窗口投影，真实数据规模下的耗时和 DB 负载未在候选验证；根上线前/后需观察生成时长不得超过 4 分钟预算。
- 迁移编号 `232` 在刷新基线中未冲突；根整合时仍需重新核对最新 migration set。

## 根总控下一步

1. 确认本候选仍基于最新根 `main`；若根指针再次前进，在候选内安全刷新并只重跑上述功能测试。
2. 登记 T104 到全局队列/总账并审阅最终分支 diff、迁移编号与来源边界。
3. 若决定整合，发送带目标 main SHA 的 `AUTHORIZE_MERGE_TO_MAIN`；本线程不自行合并、推送或部署。
4. 合并后的根 `main` 只跑用户批准的最小功能门禁和必要迁移/来源预检。主站发布仍需验收站约束定义的明确授权，发布成功后同 commit 对账验收站。
