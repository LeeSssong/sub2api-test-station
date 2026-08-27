# T83 主动探测空桶准入与模型检测降载设计

## 批准记录

- 2026-08-27：用户确认三条链路的范围是账号监控连接探测、渠道/性能监测探测、模型检测器。
- 2026-08-27：用户确认使用 Asia/Shanghai 的 5 分钟自然桶。
- 2026-08-27：用户批准本设计方向，并要求完成规格、计划、实现后按“快速部署到主站”紧急路径发布，主站成功后同步验收站。

## 问题与证据

上游截图显示来自本站生产 IP 的短时间连续 `/v1/responses` 请求，随后出现 404、422 和 429。只读核对确认账号 #276 “云桥-特惠”为 `apikey`、倍率 `0.0600`、`schedulable=false`，同一时段没有该账号的用户 `usage_logs`。

现状有三条主动流量链路：

1. `AccountMonitorRunner -> AccountMonitorService.RunAll -> ProbeAccountConnection`：默认每 300 秒执行，池筛选仅按 `status=active`，题面固定为 `hi`。
2. `ChannelMonitorRunner -> ChannelMonitorService.RunCheck`：每个 enabled monitor 立即执行并按 interval 重跑，primary 和 extra models 并发检查；失败路径最多六次请求。
3. `AccountMonitorRunner.detectionLoop -> AccountModelDetectionService`：当前按多个日内 slot 排队 `medium`（49 请求）检测；证据不足或异常可升级 `high`（158 请求），且定时选取 API Key 时不排除 disabled/unschedulable 账号。

这三个链路都没有“本时间桶已有真实用户请求就不探测”的准入规则。普通账号监控不能解释全部 burst，但模型检测的 `medium + high` 自动升级可以。

## 目标

1. 所有自动主动探测只在其对应的 Asia/Shanghai 当前 5 分钟自然桶内没有真实用户请求时执行。
2. 自动账号监控、自动渠道/性能监测的单次上游探测失败后不重试。
3. 自动通用探测题面每次均携带唯一随机 nonce，避免重复固定题面。
4. 定时模型检测每 6 小时最多一次，使用低档单 worker，不因失败、证据不足或异常自动重试或升级。
5. 禁止不可调度、非 active 的账号进入自动账号监控或自动模型检测。

## 非目标

- 不改用户请求的路由、失败重试、计费、倍率、余额、账号状态、历史流水或模型映射。
- 不把主动探测写入 `usage_logs`，也不新增平行使用统计或账务事实源。
- 不修改手动账号测试、手动渠道检查或手动低档模型检测的行为。
- 不改受许可的 v4.1.1 detector 核心、可信基线、题库或 prompt/hash 合同。
- 不新增数据库迁移或配置项。

## 方案比较

### 方案 A：只调整定时频率与重试

实现最小，但无法在有真实用户请求的桶内抑制探测，也无法防止重复固定题面。拒绝。

### 方案 B：复用 `usage_logs` 做当前桶准入，并沿用现有 singleton/in-flight/queue 去重（采用）

新增小型只读 usage-window 接口，按 `account_id` 或 `group_id` 查询当前自然桶是否存在真实请求。已有 `shouldStartSingleton`、账号监控 `activeRun`、渠道监控 `inFlight`、模型检测 slot-key/Claim 共同防止同实例重复。无迁移、无平行事实源，符合原生优先边界。

### 方案 C：新增跨进程 Redis/数据库探测租约

可覆盖多 singleton 误配，但需要新运行时状态、故障清理和可能的迁移；当前生产的自动 runner 本来由 `shouldStartSingleton` 约束，过度设计。拒绝。

## 数据与控制流

### 共同时间与真实请求判定

- `probeBucket(now)` 将时间转换为 `Asia/Shanghai` 并向下取整到 5 分钟；窗口为 `[bucketStart, bucketStart+5m)`。
- `usage_logs` 是唯一真实请求事实源。查询必须以 `created_at >= bucketStart AND created_at < bucketEnd` 限定当前桶。
- 账号范围使用 `account_id`；渠道/性能范围使用 `group_id`。未绑定 `group_id` 的渠道监控没有可验证的用户流量归属，因此其**自动**运行跳过；手动检查保持可用。
- usage 查询失败时 fail closed：跳过自动探测、记录脱敏 warning，不因未知流量状态发送上游请求。

### 账号监控

1. `RunAll` 仅把 `status=active && schedulable=true` 的账号纳入自动池。
2. 批量读取当前桶每个账号的真实请求数；有任意请求即跳过，不写主动探测结果。
3. 空桶账号最多执行一次现有连接测试；既有全局 `activeRun` 保证重入等待而非重复发包。
4. 题面由固定 `hi` 改为短、无敏感数据且包含 UUID nonce 的文本。每次生成一个新 UUID，保证正常运行中不重复。
5. 探测失败仅保存该次失败结果；不追加重试。

### 渠道/性能监测

1. runner 的自动调用改为 `RunScheduledCheck`；管理员手动 `RunCheck` 保持现有“全部模型”语义。
2. `RunScheduledCheck` 读取 monitor 的 `group_id`；空值直接返回稳定 skip，不访问上游。
3. 若该 group 当前桶有真实请求，跳过；否则在 primary+extra 的有序模型列表中由 `(monitorID, bucketStart)` 稳定选择一个模型，仅发送一笔模型请求。
4. 自动路径直接调用 `runCheckForModel` 一次，不调用 `runChannelMonitorCheckWithRetry`；结果仍写入既有 history 和 `last_checked_at`。
5. `generateChallenge` 在原有可验证算术挑战中加入 UUID nonce。校验仍只验证算术答案，nonce 不出现在持久化错误、API 或 UI 中。

### 模型检测

1. `dueDetectionSlot` 只生成 Asia/Shanghai 每日 `00:00`、`06:00`、`12:00`、`18:00` 四个 6 小时 slot；每个 slot 仍由既有唯一 `slot_key` 去重。
2. 自动候选仅为 `status=active && schedulable=true && type=apikey`，且当前账号桶没有真实 `usage_logs`。
3. 定时 run 使用 `low` profile，并在 adapter 中把 licensed session worker 强制为 1；一个定时 run 的 detector 协议内仍可能有多个经许可校准请求，这是该检测器的最小有效样本，而不是本服务重试。
4. 完成定时 run 后不调用 `EnqueueEscalationHigh`。删除/禁用 scheduled medium-to-high 自动升级；手动 low 检测不变。
5. 不能向 v4.1.1 内部 prompt 注入随机 nonce：其基线使用精确 prompt/hash，改写会使 Juice/指纹结论失真。对该链路通过低档、单 worker、6 小时、无升级和无重试去除 burst；许可证核心不复制、不修改。

## 接口与实现边界

- 扩展 `UsageLogRepository`（及 repository 实现/测试 stub）提供有界 `HasAccountUsageInWindow` 与 `HasGroupUsageInWindow`，返回 bool，不返回用量金额或请求内容。
- 在 `AccountUsageService` 提供相应窄包装，供账号监控和模型检测复用。
- `ChannelMonitorService` 获得独立自动路径，防止修改手动接口合同；runner interface 从 `RunCheck` 增加 `RunScheduledCheck`。
- 新 skip 不写 `account_monitor_results` 或 `channel_monitor_histories`，避免把“用户流量占用桶”误报成主动探测失败。
- 新日志只含 scope kind、数字 ID、bucket 和 skip reason；不得含 API Key、URL、prompt、nonce 或完整上游响应。

## 失败与安全语义

| 情形 | 行为 |
| --- | --- |
| 当前桶有真实请求 | 跳过自动探测，不访问上游，不写失败结果 |
| usage 查询失败 | fail closed 跳过，记录脱敏 warning |
| 自动探测返回非成功 | 仅记录这一次结果，不重试 |
| 自动模型检测证据不足/失败/异常 | 完成当前 run，不入队 high 或第二次 run |
| 账号 disabled 或 unschedulable | 不自动探测、不自动检测 |
| 渠道监控无 group 绑定 | 自动路径跳过；手动保持原行为 |
| 手动管理员操作 | 保持现有立即执行范围；不被自动空桶策略改变 |

## 兼容性与迁移

没有数据库迁移、配置迁移或历史回填。已有主动探测与模型检测历史保持原样；新策略仅影响新发生的自动调度。API 响应字段保持不变。

## 验收矩阵

| 场景 | 可证明结果 |
| --- | --- |
| 账号当前桶存在使用流水 | `RunAll` 不调用连接 probe |
| 账号当前桶无流水 | 恰好一次 probe，题面含新 nonce |
| 渠道 group 当前桶存在流水 | runner 不发模型请求 |
| 渠道 group 空桶且有多个模型 | 只发一个稳定轮换模型、无 retry |
| 渠道无 group | 自动路径零上游调用 |
| 模型检测空桶 active/schedulable | 6h slot 仅 enqueue 一个 low run |
| 模型检测有真实请求/不可调度/disabled | 不 enqueue |
| 模型检测 low 结果为 insufficient/failed/abnormal | 无 high escalation |
| 同一 slot 重复 tick | repository 去重，不重复 run |
| usage 查询错误 | 三条自动路径均不向上游发请求 |

## 测试、发布与回滚

测试以 TDD 执行：先为 repository/service/runner 写 RED 用例，再最小实现至 GREEN。运行相关 Go unit/repository tests、adapter Python tests、`go build ./cmd/server`、`gofmt` 与 `git diff --check`；不扩大为全仓压力或无关 UI 测试。

候选合并后从干净 `main` 运行相同直接验证和发布预检。用户已明确授权紧急“快速部署到主站”；若预检 `downtime_required=false`，按既有本地/宿主蓝绿链发布、检查健康端点并做只读专项核验。若 `downtime_required=true`，在任何切换前停止等待额外停机授权。主站成功后立即从同一 commit 部署或只读核对验收站；不能复制主站数据或凭据。

回滚使用既有宿主上一蓝绿槽/不可变镜像。代码无迁移，回滚不需要数据修复。

## 自审结论

规格覆盖三条主动探测、5 分钟时区/身份、失败与重试、随机题面、模型检测许可边界、测试、主站紧急授权和验收站同步。无未决产品选择；可进入实施计划。
