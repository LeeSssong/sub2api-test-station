# T13 NewAPI 上游倍率自动登记设计

**状态：** 正式规格已完成并自审，等待用户书面审阅与明确批准  
**任务状态目标：** 本窗口最终只推进到 `READY_FOR_ROOT_REVIEW`  
**基线：** `main@2a055e912c2aeec9045385465929b798e5e67d07`  
**候选分支：** `codex/newapi-rate-multiplier-registration`  
**发布边界：** 官方 `v0.1.177` 更新完成前，本任务只允许设计与规格，不进入实现、合并或发布车道

## 1. 问题证据与当前行为

Sub2API 已有两条可以复用的原生能力。

第一条是原生上游倍率探测：

```text
UpstreamBillingProbeService
  -> GET /v1/sub2api/billing
  -> accounts.extra.upstream_billing_probe
  -> 可选写入 accounts.rate_multiplier
```

该链路能读取原生 Sub2API 上游声明的 `resolved_rate_multiplier` 或
`effective_rate_multiplier`，并通过原子 repository 写入倍率、快照和调度 outbox。
NewAPI 不提供该原生端点，因此探测通常只能得到 `unsupported`，无法在请求发生前知道
token 实际所属分组的有效倍率。

第二条是请求后的 NewAPI 精确日志查询：

```text
真实请求成功
  -> usage_logs 落库并保存 upstream_request_id
  -> UsageCostEvidenceRegistrar.RegisterOnce
  -> GET /api/log/token
  -> request_id / upstream_request_id 精确匹配
  -> GET /api/status 读取 quota_per_unit
```

现有 `newAPIUpstreamUsageRecord` 只解析 `type`、`quota`、`request_id` 和
`upstream_request_id`。NewAPI 日志还在 JSON 字符串字段 `other` 中记录：

- `model_ratio`；
- `group_ratio`；
- `completion_ratio`；
- `user_group_ratio`。

其中 `group_ratio` 是该次请求最终生效的分组倍率，已经包含 NewAPI 对特殊用户分组的
覆盖结果。用户已明确批准把精确匹配日志中的 `other.group_ratio` 作为本任务唯一权威值，
不得改用 `model_ratio`、`user_group_ratio`、额度成本反推值或本站当前倍率。

当前缺口是：NewAPI API-key 账号即使已经发生真实请求，Sub2API 仍不会把该次请求暴露的
有效 `group_ratio` 登记到原生 `accounts.rate_multiplier`，管理员也看不到“已登记”状态。

## 2. 目标与非目标

### 2.1 目标

1. 仅对 NewAPI API-key 且没有原生 Sub2API 倍率声明的账号启用自动登记。
2. 首次合格真实请求成功后，从精确匹配的 NewAPI 日志解析 `other.group_ratio`，原子写入
   `accounts.rate_multiplier`。
3. 在 `accounts.extra` 保存明确的登记快照，并在管理员账号倍率界面显示“已登记”。
4. 已登记账号每个北京时间自然日最多由一笔合格请求完成一次成功刷新。
5. 并发请求通过数据库 CAS 领取刷新权；失败或无效响应不得覆盖现有倍率和最后成功快照。
6. 复用现有 usage 落库后精确日志查询、账号 repository、调度 outbox 和账号倍率 UI，
   不建设第二套倍率事实源或独立定时探测器。
7. 原生 Sub2API `/v1/sub2api/billing` 声明始终优先；账号后来获得原生声明时停止
   NewAPI 自动登记并清除 NewAPI “已登记”标记。

### 2.2 非目标

- 不支持 OAuth、非 API-key、非 NewAPI 或无法证明 NewAPI 身份的账号。
- 不使用 `/api/pricing` 和人工“本站账号 -> 上游分组”映射。
- 不使用 `model_ratio`、`completion_ratio`、`user_group_ratio`、`quota` 或费用公式反推倍率。
- 不改变用户扣费倍率、分组倍率、模型价格、利润公式、账号调度算法或请求路由。
- 不回填历史请求，不扫描历史 `usage_logs`，不主动制造请求。
- 不新增数据库表、列、迁移、环境变量、运行配置、依赖或 GitHub Actions。
- 不增加独立 cron、后台日任务或上游预请求探测。
- 不把一次失败查询、无效倍率或并发未获权写成“已登记”。
- 不提供批量登记、人工指定 NewAPI 分组、人工补录来源或历史倍率版本列表。

## 3. 影响对象与资格边界

### 3.1 合格账号

账号必须同时满足：

1. `account.type == "apikey"`；
2. 账号已由现有原生观察证明为 NewAPI，满足以下任一条件：
   - `accounts.extra` 中的账号余额观察来源为 `newapi`；
   - 原生 Sub2API billing probe 已明确返回 `unsupported`，且本次请求随后在
     `/api/log/token` 获得精确匹配；
3. 没有成功的原生 Sub2API billing 声明；
4. 本次本地 `usage_logs` 已成功持久化；
5. `upstream_request_id` 非空，并与 NewAPI 日志的 `request_id` 或
   `upstream_request_id` 精确相等；
6. 匹配行不是退款/冲正记录，并且 `other.group_ratio` 合法。

仅凭账号平台、API 地址文本、探测 `404`、账号名称或某个倍率数值，不足以把账号判为
NewAPI 或完成登记。

### 3.2 “没有原生 Sub2API 倍率声明”的定义

以下情况视为已有原生声明，不进入 T13：

- `upstream_billing_probe.status == "ok"`；且
- 快照包含通过现有原生校验的 `resolved_rate_multiplier` 或
  `effective_rate_multiplier`。

`accounts.rate_multiplier` 当前是否为 `nil`、`1` 或管理员手工值，不用于判断是否存在
原生声明。该列是写入目标，不是上游身份事实源。

### 3.3 原生声明后来出现

如果一个已登记账号后来成功取得原生 Sub2API billing 声明：

- 原生 probe 的现有倍率同步语义保持最高优先级；
- 在同一次原生快照持久化中删除 `accounts.extra.newapi_rate_registration`；
- T13 后续请求不再领取或刷新 NewAPI 倍率；
- 若原生 rate sync 开关未启用，当前 `accounts.rate_multiplier` 不被 T13 额外改写，
  由现有管理员/原生同步语义决定。

## 4. 方案比较与选择

### 4.1 方案 A：复用请求后精确 NewAPI 日志匹配（选择）

扩展现有 `newAPIUpstreamUsageRecord`，结构化解析 `other.group_ratio`。在 usage 成功落库后，
先用数据库 CAS 判断该账号是否需要首次登记或当日刷新；只有领取成功的请求才进行本次
倍率登记工作。现有逐笔成本证据如果已经查询同一日志，必须复用同一匹配结果，不能为
倍率再发一套重复请求。

优点：

- 直接拿到 token 对该次请求实际生效的倍率；
- 沿用已验证的 request ID 精确匹配；
- 不需要人工分组映射；
- 不增加定时器、迁移或平行事实源；
- 可以在首次真实流量后自动完成登记。

代价：首次登记和每日刷新依赖至少一笔真实成功请求；上游日志存在短暂延迟时，需要由
当天后续请求重试。

### 4.2 方案 B：读取 `/api/pricing` 并维护账号到上游分组映射（不选择）

`/api/pricing` 能返回全局 `group_ratio` 字典，但 Sub2API 不能从本地账号可靠得知 token
实际所属的上游分组。要使用该方案必须新增人工映射、编辑入口和漂移维护。

该方案增加第二套配置事实源，且映射错误会写入错误倍率，因此不选择。

### 4.3 方案 C：从 `quota`、模型倍率或费用差额反推（不选择）

通过 `quota / quota_per_unit`、本站模型价格或 `model_ratio * group_ratio` 尝试反推。
该结果混入模型、补全、缓存、特殊用户覆盖和 NewAPI 计费规则，无法保证等于有效
`group_ratio`，也违背用户已确认的权威字段，因此不选择。

### 4.4 批准记录

用户已明确确认：T13 使用 NewAPI 精确匹配日志中的有效 `other.group_ratio`。正式书面规格
仍需用户审阅并明确批准后，才能调用 `writing-plans` 或开始实现。

## 5. 架构与组件职责

### 5.1 Post-usage 登记协调器

登记发生在现有 usage 成功持久化之后，不进入请求转发或计费提交的关键路径。协调器负责：

1. 读取刚写入的 `usage_logs` 及其账号；
2. 调用 repository 尝试领取首次/当日刷新权；
3. 未领取时立即返回，不做倍率专用上游查询或写入；
4. 领取后复用现有 NewAPI 精确日志查询；
5. 解析并校验 `other.group_ratio`；
6. 成功时原子完成倍率、登记快照和必要 outbox；
7. 失败时释放领取权，保留原倍率和最后成功快照，使当天后续请求仍可重试。

现有 Gateway 与 OpenAI Gateway 已统一通过
`writeUsageLogBestEffortWithRegistrar` 在 usage 插入成功后调用 registrar。T13 复用该挂点，
不得在两个 gateway 各复制一套登记逻辑。

### 5.2 NewAPI 日志解析器

`newAPIUpstreamUsageRecord` 增加 `other` 原始字段。解析器：

- 首选 NewAPI 规范的 JSON 字符串；
- 为兼容已存在的代理序列化，允许 `other` 直接为 JSON 对象；
- 只读取顶层 `group_ratio`；
- `group_ratio` 必须是有限 JSON 数字；
- 合法范围固定为 `0 <= group_ratio <= 100`，与现有自动倍率同步保护上限一致；
- `0` 是合法免费倍率；
- 缺失、`null`、字符串、布尔、NaN、Infinity、负数或超过 `100` 均判为无效；
- 不保存完整 `other`，不记录 token、凭据或上游原始响应。

### 5.3 Repository CAS 与最终写入

新增窄 repository 能力，不扩散到通用账号编辑接口。它至少提供三个原子动作：

```go
ClaimNewAPIRateRefresh(ctx, accountID, beijingDate, claimToken, claimUntil) (claimed bool, err error)
CompleteNewAPIRateRefresh(ctx, input NewAPIRateRefreshCompletion) error
ReleaseNewAPIRateRefresh(ctx, accountID, claimToken) error
```

具体命名可遵循代码风格，但合同不可改变。

领取动作在数据库当前行上重新校验账号类型、NewAPI 身份、原生声明缺失、最后成功刷新日期
和未过期 claim。领取成功后把 token、日期和过期时间写入同一个 `accounts.extra` 子对象。

claim 租约固定为 5 分钟。进程崩溃或请求超时后，下一笔请求可在租约过期后重新领取。

完成动作必须在一个事务中：

1. 锁定账号当前行；
2. 验证 claim token 仍匹配；
3. 再次验证账号资格和原生声明缺失；
4. 写入按数据库列精度归一化的 `accounts.rate_multiplier`；
5. 写入最后成功登记快照并清除 claim；
6. 倍率变化时追加现有 account update scheduler outbox；
7. 事务提交后输出脱敏结构化系统日志。

任何一步失败都回滚整个完成事务。claim、倍率、快照和 outbox 不允许出现部分成功。

释放动作只能清除相同 token 的未完成 claim；不得清除另一个请求的新 claim，不得修改最后
成功快照。

### 5.4 管理员展示

复用现有 `UpstreamBillingRateCell` 和账号 `extra` DTO，不新增管理 API。

当 `newapi_rate_registration.status == "registered"` 时：

- 倍率单元格显示现有倍率数值；
- 显示紧凑状态“已登记”；
- tooltip/详情显示“NewAPI 请求日志自动登记”、首次登记时间和最近成功更新时间；
- 不显示 claim token、usage log ID 或原始 `other`。

编辑账号时仍允许管理员修改倍率，但必须显示明确提示：

> 该倍率由 NewAPI 请求日志自动登记；手动修改会在下一次每日成功刷新时被覆盖。

本任务不新增自动登记开关或“取消登记”按钮。账号上游身份发生实质变化时，现有账号更新
事务必须清除登记快照，使新身份在下一笔合格请求后重新登记。

## 6. 数据与接口合同

### 6.1 `accounts.extra` 快照

固定 key：

```text
newapi_rate_registration
```

最后成功状态示例：

```json
{
  "status": "registered",
  "source": "newapi_log_other_group_ratio",
  "group_ratio": 0.17,
  "registered_at": "2026-08-16T09:20:00+08:00",
  "last_observed_at": "2026-08-16T10:15:03+08:00",
  "last_refresh_date": "2026-08-16",
  "source_usage_log_id": 123456
}
```

领取中的内部状态可临时增加：

```json
{
  "claim_token": "uuid",
  "claim_date": "2026-08-16",
  "claim_expires_at": "2026-08-16T10:20:03+08:00"
}
```

完成或释放后必须删除 claim 字段。首次成功时 `registered_at` 与
`last_observed_at` 同时写入；每日刷新保留原 `registered_at`，只更新后三项。

`source_usage_log_id` 仅用于管理员/运维审计关联，不在 UI 显示。不得把 request body、
API key、完整 NewAPI 日志、上游 token、分组名或其他 `other` 字段写入 `extra`。

### 6.2 时间合同

- 每日边界固定使用 `Asia/Shanghai`，不得依赖宿主默认时区。
- “当天已刷新”只由最后成功快照的 `last_refresh_date` 判断。
- claim 失败、日志未出现、精确匹配失败或倍率无效均不写成功日期。
- 当天首次尝试失败后，后续合格请求仍可重试，直到有一次成功或当天结束。
- 跨日后的第一笔合格请求可以领取新日期，不等待固定时刻。

### 6.3 人工编辑与身份变化

- 已登记账号的手工倍率编辑保持允许。
- 手工编辑不清除 `newapi_rate_registration`，也不把手工值标为上游值。
- 下一次每日成功刷新可以覆盖该手工值；UI 必须提前说明这一语义。
- 修改账号 `type`、上游 base URL、API key/token 身份或其他会改变上游主体的凭据时，
  清除 `newapi_rate_registration` 和未完成 claim；保留当前倍率值，直到新身份首次成功登记。
- 普通名称、备注、并发、优先级、代理或分组绑定变化不清除登记。

## 7. 端到端控制流

```text
真实请求成功
    |
    v
usage_logs 成功插入，得到 usage_log_id + upstream_request_id
    |
    v
检查 API-key / NewAPI 候选 / 无原生 Sub 声明
    |
    v
DB CAS 领取 account_id + 北京日期，租约 5 分钟
    |
    +-- 未领取 --> 结束，不改倍率
    |
    v
复用 NewAPI /api/log/token 精确匹配
    |
    +-- 未找到/失败/退款/other 无效 --> 释放同 token claim，结束
    |
    v
解析 other.group_ratio
    |
    v
事务内再次校验资格 + claim
    |
    v
写 rate_multiplier + registered snapshot + 必要 scheduler outbox
    |
    v
管理员账号倍率单元格显示“已登记”
```

该流程是 usage 落库后的 best-effort 后处理。倍率登记失败不能把已经成功的用户请求改成
失败，也不能回滚既有扣费或 usage log。

## 8. 失败、安全与并发语义

| 场景 | 结果 |
| --- | --- |
| NewAPI 日志有短暂延迟 | 本次释放 claim；当天后续请求可重试 |
| `/api/log/token` 鉴权失败或超时 | 不改倍率/成功快照；释放 claim |
| 精确 request ID 未匹配 | 不做模糊匹配；释放 claim |
| `other` 非法或缺少 `group_ratio` | 不改倍率/成功快照；释放 claim |
| 两笔请求同时到达 | 仅一个数据库 claim 成功，另一个立即结束 |
| 完成前进程崩溃 | 5 分钟租约到期后可重试；旧倍率保持 |
| claim 后账号身份被管理员修改 | 完成 CAS 失败；新身份不接受旧请求倍率 |
| claim 后出现原生 Sub 声明 | 完成 CAS 失败；原生声明优先 |
| 上游倍率与现值相同 | 更新最后成功观察日期；不制造无意义倍率变更 |
| 上游倍率变化 | 原子更新倍率和快照，并刷新调度账号状态 |
| 手工倍率刚被修改 | 当日成功自动刷新可覆盖；审计日志区分自动来源 |
| 日刷新失败 | 保留上次成功倍率和“已登记”状态，不伪造今天已刷新 |

日志只能包含账号 ID、usage log ID、旧/新倍率、结果码和时间，不记录凭据、完整 request ID、
原始 `other` 或响应 body。上游失败原因使用固定分类，不把原始敏感响应直接写入系统日志。

## 9. 兼容性与迁移

- 无数据库迁移；旧版本会忽略未知 `accounts.extra.newapi_rate_registration`。
- `accounts.rate_multiplier` 继续使用现有列和计费语义。
- 现有原生 Sub billing probe、rate sync 开关和管理员手工倍率 API 保持兼容。
- 新前端面对没有登记快照的旧后端时，继续按当前倍率单元格展示。
- 新后端面对旧前端时，倍率已经生效；额外状态只是不可见，不影响计费。
- 回滚代码不会自动还原已登记倍率。回滚后的旧版本继续使用最后写入的
  `accounts.rate_multiplier`，并忽略登记快照；这是可预期的向后兼容数据状态。
- 不对部署前历史请求补登记；每个账号必须等待部署后的下一笔合格请求。

## 10. 场景化验收矩阵

1. 已知 NewAPI API-key、无原生声明、无登记快照：首笔成功请求精确匹配
   `group_ratio=0.17` 后，倍率变为 `0.17`，状态显示“已登记”。
2. 同账号同日并发 20 笔合格请求：只有一笔取得 claim 并完成倍率刷新。
3. 同账号当天后续请求：不再执行倍率专用领取/写入，倍率和成功日期不变。
4. 次日北京时间 00:00 后首笔合格请求：完成一次新日期刷新。
5. 当天首笔请求查询失败、第二笔成功：第一笔不覆盖；第二笔仍可登记或刷新。
6. `other.group_ratio=0`：成功登记为 `0`。
7. `group_ratio` 缺失、字符串、负数、超 `100` 或非有限值：拒绝且保留旧值。
8. 日志只匹配本地 request ID、没有 upstream request ID：不得登记。
9. 日志是退款/冲正：不得登记。
10. API-key 但来源未知、OAuth、非 NewAPI：不查询、不登记。
11. 已有成功原生 Sub billing 声明：T13 不领取；原生 probe 清除旧 NewAPI 标记。
12. claim 后管理员更换上游 API key：旧请求完成 CAS 失败，不污染新身份。
13. 已登记后管理员手工改倍率：修改成功，UI 保留自动来源提示；下一次每日成功刷新覆盖。
14. 自动写入倍率变化：scheduler outbox 与倍率/快照同事务提交；outbox 失败时全部回滚。
15. 自动写入倍率未变化：成功日期更新，不产生无意义的倍率变更事件。
16. 前后端版本交错：无新字段的旧账号正常显示；旧前端继续使用新倍率。

## 11. 测试策略

T13 不适用官方更新免测例外，按小步发布政策保留定向门禁。

### 11.1 后端单元测试

- `other` JSON 字符串与对象解析；
- `group_ratio` 合法/非法边界；
- request ID 精确匹配；
- 退款排除；
- NewAPI/原生 Sub/未知来源资格判定；
- 北京自然日判断；
- 登记失败不影响 usage 落库成功结果；
- Gateway 与 OpenAI Gateway 共享同一 post-usage 挂点。

### 11.2 Repository 测试

- 首次 claim；
- 同日并发只能成功一次；
- 过期 claim 可重领；
- token 不匹配不能完成或释放；
- 身份变化、原生声明出现时完成失败；
- 倍率、快照和 outbox 原子提交；
- outbox 失败完整回滚；
- 无倍率变化时只更新成功快照；
- 原生 probe 成功时清除登记标记。

优先使用现有 sqlmock 合同测试和项目已有 PostgreSQL integration fixture；不为本任务建立
新的测试基础设施。

### 11.3 前端测试

- 登记快照显示“已登记”、来源和时间；
- claim 内部字段不展示；
- 无快照保持现有展示；
- 编辑弹窗显示手工值将被每日刷新覆盖的提示；
- 长账号名、窄屏和深色主题不产生重叠或溢出。

### 11.4 最小验证命令类别

实施计划必须给出精确命令，至少覆盖：

- 相关 Go service/repository/handler 测试；
- 相关 Vue/Vitest 测试；
- 必要 Go compile/vet 或前端 typecheck/build；
- `gofmt`、`git diff --check`、范围和禁区检查；
- 发布预检与 `downtime_required` 输出。

不默认运行全仓测试、压力测试、mutation、长时间 soak 或无关浏览器回归。

## 12. 发布、线上验证与回滚

### 12.1 发布前条件

- 官方 `v0.1.177` 更新已经完成发布链切换；
- T13 从届时最新 `main` 刷新，完成冲突处理、专项验证和最终独立复审；
- 根总控明确发出 `AUTHORIZE_MERGE_TO_MAIN`；
- 合并后的 `main` 完成定向门禁和发布预检；
- `downtime_required=false` 才可按现有蓝绿链继续；若为 `true`，动作前等待用户确认。

### 12.2 线上专项验证

- 公网健康/就绪检查；
- 管理员登录态账号倍率单元格可见“已登记”；
- 使用一个符合资格的自然请求或明确授权的受控请求，确认倍率、快照和日期一致；
- 同日第二笔请求不再次改变成功日期或产生第二次倍率刷新；
- 不修改历史 usage、不人工篡改生产倍率来制造通过证据。

如果生产暂时没有自然合格请求，可以保留该子项为未验证，不能伪造登记成功；代码、发布和
其他健康项可分别报告，但任务不得在专项行为未闭环时标记 `DONE`。

### 12.3 回滚

- 应用回滚使用现有蓝绿旧槽位/上一已验证镜像；
- 无 schema 回滚；
- 已写入的 `rate_multiplier` 和 extra 快照保留，旧版本会安全忽略额外快照；
- 若自动倍率造成运行风险，管理员可在回滚后按现有账号编辑能力修正倍率；任何批量生产
  数据修复必须另行授权，不纳入自动回滚脚本。

## 13. 自审结论与待批准项

### 13.1 自审结论

- 无 `TBD`、`TODO` 或占位内容。
- 权威字段固定为精确匹配日志的 `other.group_ratio`。
- 首次登记、每日刷新、并发、失败、原生优先和人工编辑语义互不矛盾。
- 范围是一个可独立实现和发布的垂直功能，不包含历史回填、定时器或第二事实源。
- 无迁移、配置、依赖和生产数据预操作。

### 13.2 需要用户批准的书面决策

1. 已登记倍率由系统持续管理，但管理员仍可临时手工修改；下一次每日成功刷新会覆盖。
2. 不新增自动登记开关或取消登记按钮。
3. 日刷新首次失败时允许当天后续真实请求继续重试，而不是整天停止。
4. 原生 Sub billing 声明后来出现时，清除 NewAPI “已登记”标记并停止 T13 刷新。

用户批准本规格后，下一步才使用 `writing-plans` 生成逐文件、逐测试、逐提交实施计划。
