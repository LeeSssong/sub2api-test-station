# 飞书余额告警投递恢复设计

## 1. 问题证据与当前行为

生产 worker 当前运行通知开关为 `true`，飞书凭据文件和上游登录登记簿均存在且权限符合要求。数据库中存在 5 个 `upstream_baseurl_balance_usd_v1` 活动事件（3 个 low、2 个 zero），其中部分事件从未成功投递。

直接调用生产账号监控接口得到 HTTP 500，底层错误为：

```text
pq: column "bucket_start" does not exist
```

worker 同期日志记录 `upstream_balance_notification: evaluation failed`，错误阶段为 `read upstream balance projection`。余额通知的 `Evaluate`、`RunDue` 和 `deliver` 都读取 `AccountMonitorService.ListWindow` 投影，因此错误发生在飞书 HTTP 请求之前；飞书 sender 没有被调用。

当前生产版本为 `07ef269d4345f800a47b1965f5dbe15646bfa2c1`。根目录 `main` 已包含可分离的 `bucket_start` SQL 修复，但当前 `main` 还包含 T129 的 Monitor V4 缓存口径变更；本任务不得把该变更带入飞书修复。

## 2. 目标与非目标

### 目标

1. 修复 `ListRealRequestTimelines` 中 `real_buckets` CTE 对 `bucket_start` 的投影和分组，使 PostgreSQL 能正确执行账号监控投影查询。
2. 恢复飞书余额告警从余额评估、活动事件 claim、发送前 lease 校验到 Feishu sender 的完整路径。
3. 保持已有 5 个活动事件和其非敏感投递状态不变；修复发布后由既有 `RunDue` 自动重试，不做人工回填或手工伪造发送记录。
4. 增加直接相关的 SQL 列/表达式回归测试，以及通知链路在投影可读时可继续到 sender 的服务测试。

### 非目标

- 不修改 `input_tokens`、缓存命中率、缓存分母、Monitor V4 其他 SQL、指标展示或调度评分。
- 不修改余额阈值 `$5`、low/zero 状态、30/5 分钟重复节奏、BaseURL 去重、凭据新鲜度或排名语义。
- 不修改飞书 App、群、接收人、callback token、secret 挂载、卡片样式、重试退避或事件表结构。
- 不清理、删除、回填或重建现有 `ops_alert_events`；不直接写生产业务数据。
- 不部署验收站，不调用旧 `/admin/lab` 发布链，不使用 GitHub Actions。

## 3. 方案比较与选择

### 方案 A：只修复 `real_buckets` SQL（推荐）

将 CTE 改为显式投影 `date_bin(...) AS bucket_start`，并在 `GROUP BY` 中重复该表达式，而不是引用同层别名。保留生产当前所有通知和指标逻辑。优点是改动最小、直接对应已复现根因、无迁移和无数据变化；风险最低。

### 方案 B：在通知 reader 中绕过账号监控时间线

为余额通知新增独立余额查询，绕开 `ListWindow`。该方案会产生第二套投影/事实源，破坏 T98 已批准的原生复用边界，并可能使排名、快照时间和通知二次校验不一致。拒绝。

### 方案 C：放宽通知读取失败并使用旧事件金额发送

在投影失败时直接按事件表旧金额发送。该方案会绕过最新余额、凭据和 lease 指纹校验，可能发送过期或错误告警。拒绝。

选择方案 A。

## 4. 端到端数据与控制流

1. 账号监控 worker 按既有周期刷新/读取 `ListWindow`，`real_buckets` 正确生成每账号时间桶。
2. 余额通知 `Evaluate` 读取同一原生账号监控投影，按规范化 BaseURL 评估 low/zero/healthy。
3. low/zero 评估继续通过 `ops_alert_events` 的 claim/lease CAS 逻辑；healthy 继续 resolve 活动事件。
4. `RunDue` 对现有 firing BaseURL 逐范围刷新余额，再读取投影并执行发送前 lease、观察时间和金额匹配。
5. sender 使用现有受保护飞书凭据发送卡片；成功确认 delivery，失败沿用既有非敏感错误码和退避。
6. 发布后只通过日志、只读事件查询和实际飞书收件验证结果；不新增旁路发送脚本。

## 5. 接口与数据契约

- `real_buckets` 输出列固定为 `(account_id, bucket_start)`。
- `bucket_start = date_bin('5 minutes', created_at, $2::timestamptz)`，并在投影和分组中使用同一表达式。
- 不新增 API 字段、数据库列、迁移、配置项或环境变量。
- `UpstreamBalanceNotificationService` 接口和 `UpstreamBalanceEventRepository` 事件状态契约保持不变。

## 6. 失败与安全语义

- SQL 仍失败时，账号监控接口返回原生内部错误，通知继续 fail-closed，不使用旧投影或旧金额发送。
- 单个 BaseURL 刷新失败不阻断其他范围；沿用现有 `RunDue` 失败计数和活动事件保留语义。
- 飞书发送失败继续写入非敏感错误码并按原有退避重试；不记录 token、密码、API Key、卡片明文凭据或完整响应。
- 修复不得改变通知 claim 的 at-most-one active lease 约束，也不得清除租约来“催发”。

## 7. 兼容性与迁移

- 仅修改后端原生账号监控查询及其测试；兼容 PostgreSQL 当前版本。
- 无数据库迁移、无配置变化、无凭据变化、无历史数据回填。
- 候选必须从当前生产基线派生，并在合并前明确排除 T129 缓存相关 diff。

## 8. 场景化验收矩阵

| 场景 | 预期结果 |
| --- | --- |
| `ListRealRequestTimelines` 执行正常 | 不再出现 `column "bucket_start" does not exist`，账号监控接口返回 200 |
| 活动 low 事件到期 | 读取最新范围投影，claim 成功后调用 sender 并确认 delivery |
| 活动 zero 事件到期 | 沿用 5 分钟节奏，发送 P1 卡片；不改变事件状态规则 |
| 某范围刷新失败 | 该范围保留 firing 并计入失败计数，其他范围继续处理 |
| 飞书 sender 返回错误 | 记录非敏感错误码，写入 next attempt，后续自动重试 |
| healthy 余额恢复 | resolve 活动事件，不发送恢复消息 |
| 缓存指标查询/校验 | 与本任务前相同；不得出现 `input_tokens` 或缓存公式 diff |
| 凭据/数据库审计 | 无 secret 输出、无迁移、无业务数据写入 |

## 9. 测试策略

- `go test ./internal/repository -run 'TestAccountMonitorRepository' -count=1`，覆盖 `bucket_start` 显式投影和表达式分组合同。
- `go test ./internal/service -run 'TestUpstreamBalanceNotification|Test.*Balance.*Notification' -count=1`，覆盖投影可读后 claim、sender、确认和失败重试边界。
- `go build ./cmd/server`。
- `git diff --check`，并执行范围检查，确认候选仅包含允许的 SQL 修复、直接相关测试、规格/交接文档。
- 不运行真实上游余额探测，不发送真实飞书消息，不在本阶段触碰生产数据。

## 10. 发布、线上验证与回滚条件

- 本规格不授权发布。候选完成后只能进入 `READY_FOR_ROOT_REVIEW`，等待根总控明确授权。
- 发布来源必须是根目录干净且与 `origin/main` 一致的 `main`；不得从候选 worktree 直接部署。
- 主站发布需用户明确使用“快速部署到主站”或“测试站验收通过，部署主站”路径；若预检返回 `downtime_required=true`，必须另行取得停机授权。
- 线上专项验证：账号监控接口 HTTP 200；worker 无新的投影读取错误；5 个活动事件可被重新评估；至少核对一条应发事件已更新 delivery 记录并确认飞书收件。
- 失败时保留候选、日志和发布证据，由蓝绿链恢复上一槽位；不得删除事件或凭据，不得把旧槽位视为问题已解决。

## 11. 仍待决事项与批准记录

- 待决：发布后是否由管理员在主站飞书群确认实际收件；该确认属于线上验收，不改变实现范围。
- 用户于 2026-09-04 明确确认本窗口只处理飞书推送问题，缓存及其他问题不纳入；并确认开始编写本规格书。
- 当前状态：规格已写入，等待用户审阅；未编写实施计划，未修改运行代码，未合并、推送或部署。
