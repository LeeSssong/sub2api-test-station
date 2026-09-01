# P1 余额耗尽账号隔离与恢复：恢复官方原生状态机

**日期：** 2026-09-01  
**任务级别：** P1 独立顶层任务  
**状态：** 书面规格待用户批准；本文件不授权实现、合并、发布或生产操作  
**当前 worktree 基线：** `200a1457b96d321296582ed93d8e867a97cbd5fa`  
**官方基线：** Sub2API `v0.1.183`，commit `e8cb019fabf8b55199436229044cbf9aa7a82564`  
**官方记录：** `upstream/sub2api/XINGQIAO_UPSTREAM.md`

## 1. 范围与事实源

本任务只处理“余额耗尽账号隔离/恢复为什么异常”。不讨论、不实现 P2-P5，也不顺带修改通知、排名、采购、经营页、探测器、账务或其他调度策略。

事实源优先级如下：

1. 当前 worktree 的 Sub2API 原生代码、测试和只读生产证据。
2. 官方 Sub2API v0.1.183 源记录与对应源快照。
3. 本规格中记录的用户已确认生产证据。
4. 历史 S1-R2 规格、计划、验证报告和交接，仅用于解释定制行为来源，不作为目标行为授权。

本任务不得修改 `docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md` 或根 `main`；不得访问生产、SSH、部署、推送或实现代码。书面规格获批准前不得调用 `writing-plans`。

## 2. 问题证据与当前行为

### 2.1 已确认生产证据

用户已提供并确认以下事实：

- `baitumax-pro` 返回 `insufficient_user_quota`，没有命中定制分类器，继续走原生 OpenAI 403 路径。
- `apizh-0.15` 与 `NV-plus` 命中定制 `balance_exhausted`，写入约十年的临时隔离。
- `NV-plus` 随后被定制逻辑的“账单探针连续两次成功”提前解封，但真实 `/models` 仍返回 502；因此“账单可查询”被错误地当成“推理服务可调度”。
- 用户后来把账号 `schedulable=false` 的操作是手动修改，不能当作自动隔离证据，也不能用于证明自动恢复或自动再次隔离。

### 2.2 当前代码行为

当前树保留了 2026-08-17 S1-R2 定制链：

- `deterministic_failure_isolation.go` 将有限错误码/文本归类为 `balance_exhausted`、`credential_invalid` 或 `model_unsupported`。
- `RateLimitService.HandleUpstreamError` 在普通 402/403/404 处理前抢占，余额分类写入 `temp_unschedulable`，凭据分类可能写入 `error` 或临时隔离，模型分类写入模型级限制。
- 当前余额分类使用 `deterministicBalanceIsolationDuration` 返回约 3650 天，而不是官方常规临时隔离的生命周期。
- 当前账单探针 `recordSuccessfulProbeRecovery` 维护进程内连续成功次数，达到 2 次后直接调用 `ClearTempUnschedulable`。该清理没有证明真实推理端点、模型目录或原生账号健康已恢复。
- 当前 `Account.IsSchedulableAt` 的原生判断仍以 `active`、人工 `schedulable`、过期、过载、429、`temp_unschedulable` 和适用 quota 为准；账号监控行当前直接暴露持久化 `schedulable`，没有一个明确命名的 effective 投影字段。
- 当前 CN provider 专用路径仍有独立余额/额度检查与恢复语义；它不是本任务要删除的对象。

### 2.3 根因判断

问题不是“缺少更多余额探测”，而是两套不同责任被混在一起：

1. 定制分类器抢占了官方上游错误状态机，把“明确余额/额度错误”扩展成自定义长期账号隔离。
2. 账单探针把财务读数成功误认为推理资格恢复，并清除了任意原因的 `temp_unschedulable`。
3. 监控展示没有清晰区分人工开关 `schedulable` 与当前原生有效可调度性，导致运维判断容易把展示字段当作调度事实。

## 3. 官方原生行为基线

目标以官方 v0.1.183 原生状态机为准，恢复时不得保留 S1-R2 的定制抢占和定制恢复权。实现阶段必须以官方 commit `e8cb019...` 对照当前定制树逐项核对；本节定义行为合同：

### 3.1 账号级资格

`Account.IsSchedulableAt(now)` 是账号级调度资格的唯一组合判断。它至少组合：

- `status=active`；
- 管理员持久化开关 `schedulable=true`；
- 未过期（启用自动过期时）；
- 未处于原生 overload 窗口；
- 未处于原生 rate-limit 窗口；
- 未处于原生 `temp_unschedulable_until` 窗口；
- 对适用账号，未超过原生 quota。

任何外部投影、账单快照或监控结果都不能单独成为第二个调度 veto。

### 3.2 上游错误

- 原生 402、403、404、429、5xx 继续由官方对应处理路径接管。
- 403 不能仅因 HTTP 状态码被新分类器抢占；具体平台已有的原生 403 语义继续保留。
- 429 的原生窗口、冷却、半开和平台专用处理继续保留。
- 5xx/网络失败继续使用官方已有的短期错误、重试、故障转移或 transient 语义，不被余额逻辑扩大。
- 404/model-not-found 仅由官方既有模型可用性/模型限制语义处理，不由本任务新增长期模型状态机。

### 3.3 余额与 CN provider

- OpenAI/通用上游的余额或额度错误不再由 `deterministic_failure_isolation` 抢先改写为自定义长期隔离。
- CN provider 的专用余额检查、额度窗口、响应式 402/429、阈值停调和成功恢复继续保留，且仍由其专用服务负责；本任务不把 CN provider 的规则泛化到所有平台。
- 官方账单探针本身可以继续读取、保存和展示上游账单/余额快照，但它不是推理健康探针，也无权清理任意原生临时状态。

## 4. 目标与非目标

### 4.1 目标

1. 恢复官方 v0.1.183 的原生账号/模型/错误状态机，移除定制 `deterministic_failure_isolation` 对原生 402/403/404 的抢占。
2. 移除 upstream billing probe “连续两次成功清除任意 `temp_unschedulable`”的定制恢复行为。
3. 保留官方账单探针本身及其只读/账单快照职责。
4. 保留官方 402/403/429/5xx 语义及 CN provider 专用余额检查/恢复语义。
5. 账号监控仅增加或修正 `effective_schedulable` 展示投影，且该字段由同一原生 `Account.IsSchedulableAt(snapshot_time)` 计算，不写回账号、不改变原生状态。
6. 对既有约十年 `temp_unschedulable` 数据提供只读盘点、分批授权处置和可回滚运营方案，不擅自改生产数据。

### 4.2 非目标

- 不新增余额事实源、隔离表、恢复表或平行 scheduler veto。
- 不把账单探针升级为 `/models`、推理请求或模型检测的替代品。
- 不修改用户余额、上游账单、费率、采购成本、通知、质量评分或排名。
- 不自动批量恢复历史账号，不自动回填历史状态，不删除历史审计。
- 不重新设计 CN provider 的专用语义。
- 不处理 P2-P5，包括通知卡片、监控 V4、经营页、采购或其他独立任务。

## 5. 受影响账号与边界条件

### 5.1 直接受影响账号

- 已被 S1-R2 定制分类写入长期 `temp_unschedulable` 的账号，尤其是 `apizh-0.15`、`NV-plus`。
- 曾被账单探针成功恢复逻辑清除过临时隔离的账号，尤其是 `NV-plus`；其当前状态不得根据这次历史清除反推为健康。
- `baitumax-pro` 作为未命中定制分类、走原生 403 的对照账号，不能因本任务回滚而被额外恢复或额外隔离。
- 所有带 `temp_unschedulable_until` 的其他账号均为盘点对象，但只有可证明由本定制逻辑产生且获得授权的记录才可进入处置批次。

### 5.2 不受本任务自动影响的账号

- 管理员明确手动设置 `schedulable=false` 的账号；除非另有书面授权，不恢复其人工开关。
- `status=error`、已过期、429、overload、quota 或模型级限制账号；它们继续由各自原生恢复入口处理。
- CN provider 专用余额/额度停调账号；本任务不覆盖其专用恢复判断。

### 5.3 时间与并发边界

- 所有 effective 投影使用一次捕获的 UTC `snapshot_time`，避免一张页面内按行跨越时间边界。
- 账单快照的 `observed_at`/`fresh_until` 只用于财务证据新鲜度，不等价于调度资格。
- 状态写入失败时不得升级为更宽的账号隔离；当前请求继续沿官方错误/故障转移合同处理。

## 6. 方案比较与选择

### 方案 A：纯原生方案（选择）

删除定制确定性抢占和账单探针定制恢复；保留官方原生错误处理、原生账号/模型状态、官方账单探针和 CN provider 专用路径。监控通过只读字段展示 `Account.IsSchedulableAt` 的 effective 结果。

优点：恢复单一事实源，行为与官方 v0.1.183 对齐，无迁移，无历史状态自动改写，最小化回滚面。缺点：明确余额耗尽不会再由本任务提供一个跨平台统一的长期隔离标签，运营需依据原生状态和专用 CN 证据判断。

### 方案 B：保留分类器但缩短为统一临时隔离

继续识别 `balance_exhausted`，但改成较短 TTL，并禁止账单探针恢复。

不选：仍会抢占官方 402/403/404 路径，继续维护第二套错误归类和跨平台行为；无法证明分类器不会误判 `insufficient_user_quota`、通用 403 或其他业务错误。

### 方案 C：新建余额隔离/恢复表并与原生状态双写

保留定制分类与账单恢复，但把 episode、探针证据和恢复条件持久化到新表，再投影到原生调度。

不选：引入第二套事实源、迁移、CAS、清理和一致性问题；与原生优先约束冲突，且不能解决账单成功不代表 `/models`/推理健康的基本误判。

**明确选择：方案 A，纯原生方案。**

## 7. 端到端状态与控制流

### 7.1 推理请求错误流

1. 网关按官方 v0.1.183 路径发送请求并接收上游响应。
2. `HandleUpstreamError` 不再调用或依赖 `deterministic_failure_isolation` 对 402/403/404 做前置分类。
3. 官方处理器按平台和错误类型决定：原生 error、temp unschedulable、429/5xx 冷却、模型限制、请求级 failover 或仅透传错误。
4. 账号选择下一候选时只调用原生 `IsSchedulable`/`IsSchedulableForModel` 及已有 transient/模型资格逻辑。
5. 任何原生状态写入都继续通过既有 repository、scheduler snapshot/outbox 和缓存失效链路。

### 7.2 账单探针流

1. 账单探针按既有启用开关、周期、身份校验、超时、快照持久化和速率同步合同运行。
2. 成功响应只更新其账单/余额快照及已有允许的同步字段。
3. 成功响应不得调用通用 `ClearTempUnschedulable`，不得清除 `error`、429、overload、人工暂停、quota 或模型限制。
4. 探针失败、unsupported 或快照过期只影响账单证据状态，不自动改变非 CN provider 的账号调度状态。
5. 如将来需要“探针成功后恢复某一类状态”，必须另立规格，定义状态所有权、CAS、真实推理/模型证据和授权门禁；本任务不保留该能力。

### 7.3 CN provider 专用流

CN provider 继续使用已有余额/额度服务：余额低于阈值时专用逻辑停调，余额恢复时只清理由该专用逻辑写入的暂停；coding plan 继续依赖滚动窗口快照和阈值评估。不得让通用账单探针跨越该所有权边界。

### 7.4 监控展示流

1. 账号监控读取账号快照并捕获 `snapshot_time`。
2. 对每行调用与调度相同语义的 `Account.IsSchedulableAt(snapshot_time)`，得到 `effective_schedulable`。
3. `schedulable` 保留为管理员人工开关的原始字段；`effective_schedulable` 表示当前完整原生门禁结果。
4. 监控页可展示原因投影，但原因必须来自已存在的原生字段，且不可反向写状态。
5. 监控 API、页面刷新和详情弹窗不得以账单 `status=ok` 覆盖 `effective_schedulable=false`，也不得以历史探测成功覆盖当前原生状态。

## 8. 接口字段与 UI 契约

### 8.1 管理账号接口

账号管理接口继续返回原生字段：

- `status`：原生账号状态；
- `schedulable`：管理员持久化开关；
- `temp_unschedulable_until`、`temp_unschedulable_reason`：原生临时状态；
- `rate_limit_reset_at`、`overload_until`、quota 等已有原生字段。

本任务不改变这些字段的含义、写接口或恢复接口。

### 8.2 账号监控接口

在已有 `AccountMonitorAccount` 投影上增加或修正：

```json
{
  "schedulable": true,
  "effective_schedulable": false,
  "effective_schedulable_at": "2026-09-01T00:00:00Z",
  "effective_unschedulable_reason": "temp_unschedulable"
}
```

契约：

- `schedulable` 不得被替换为 effective 值。
- `effective_schedulable` 必须等于 `Account.IsSchedulableAt(effective_schedulable_at)`；若无法取得完整账号快照则 fail-closed 或返回明确的未知，不得默认为 true。
- `effective_schedulable_at` 是本次响应的快照时间，不是状态发生时间。
- `effective_unschedulable_reason` 只能是受限枚举或原生字段映射，例如 `inactive`、`manual_disabled`、`expired`、`overload`、`rate_limited`、`temp_unschedulable`、`quota_exceeded`、`model_unavailable`；它不是新的调度事实源。
- 账单余额快照继续使用既有 `balance` 字段和 `observed_at/status`，不与 effective 字段合并。

UI 要求：人工开关与有效状态分开显示；当 `schedulable=true` 但 `effective_schedulable=false` 时，显示当前原生阻断原因，不显示“余额探针已恢复”之类推断；当 `schedulable=false` 时，显示人工暂停，不能标为自动余额隔离。

## 9. 失败与恢复语义

### 9.1 通用失败原则

- 失败分类不确定时保持官方原生路径，不 fail-open 地把账号重新纳入调度。
- 账单探针读取失败不改变账号状态。
- 状态持久化失败不扩大隔离范围，不执行补偿性批量清理。
- 恢复必须由写入该状态的原生所有者或管理员原生恢复入口执行。
- 任何“成功”必须指明成功对象：账单读取成功、模型目录成功、推理请求成功、账号状态恢复成功，四者不可互换。

### 9.2 余额耗尽

本任务不新增通用 `balance_exhausted` 状态迁移。通用 OpenAI/上游错误按官方 402/403/429/5xx 语义处理；CN provider 保留专用余额语义。历史定制 reason 不再被新代码作为清理授权依据。

### 9.3 账号凭据与模型状态

凭据失效继续遵循官方 OAuth/API Key 恢复与 error 语义；明确模型不可用继续遵循官方模型级限制和探测/管理员恢复语义。账单探针成功不能清除任何一类状态。

## 10. 兼容与迁移

### 10.1 代码兼容

- 删除或停止使用 `deterministic_failure_isolation` 运行时路径及其专属配置读取；是否保留历史文件需由实施计划根据引用关系决定，但不得继续参与状态决策。
- 不新增数据库迁移，不改核心表结构，不新增平行状态表。
- 保留旧 JSON reason 的读取兼容：旧值可继续展示为历史/原生 reason，但新代码不得仅凭旧定制字段自动执行恢复。
- 保留既有管理员 `recover-state`、账号测试、模型恢复和 CN provider 专用恢复入口。
- 账单快照 schema 与既有 API 保持兼容；移除的是恢复副作用，不是账单观测能力。

### 10.2 历史十年临时隔离处置方案

本任务明确禁止自动回填、批量清除或直接修改生产数据。建议运营按以下授权门禁执行，具体命令和名单由后续实施计划/根总控另行批准：

1. 只读盘点：导出账号 ID、名称脱敏标识、状态、人工 `schedulable`、`temp_unschedulable_until`、reason 摘要、更新时间、最近原生错误/探测时间；不导出凭据或完整响应。
2. 所有权分组：区分人工暂停、CN provider 专用暂停、官方 401/429/overload/quota、定制余额 reason、未知 reason。
3. 先生成候选清单和每个账号的原生恢复前置条件；定制余额 reason 不能直接等价为健康，也不能仅凭账单 `ok` 放行。
4. 取得用户对具体批次、账号范围和恢复动作的书面授权后，使用官方管理员恢复入口逐账号或小批次操作；每批次前后保存脱敏状态快照和审计证据。
5. 恢复后必须以原生账号测试/模型目录/必要的真实健康证据确认，再决定是否保留人工 `schedulable` 开关；不得替用户打开人工开关。
6. 任一批次出现状态不一致、未知 reason、写入失败或线上异常，立即停止后续批次，保留候选与证据，按原生恢复入口或蓝绿回滚处理。

当前规格阶段只定义门禁和运营方案，不执行任何生产数据变更。

## 11. 场景验收矩阵

| 场景 | 预期原生行为 | 关键断言 |
|---|---|---|
| OpenAI 明确 402/余额错误 | 不经定制分类器抢占；走官方 402 处理 | 无 `deterministic_failure_isolation` 写入；状态符合官方路径 |
| OpenAI `insufficient_user_quota` 403 | 走官方 403 语义 | 不被余额分类器改写，不因文本单独清/封 |
| 通用 403 | 保留官方平台 403、计数、冷却或错误语义 | 不新增定制余额隔离 |
| OpenAI 429 | 保留官方 rate-limit、窗口和半开语义 | 账单探针不改变 429 状态 |
| OpenAI 5xx/网络失败 | 保留官方 transient/故障转移语义 | 不写通用余额状态 |
| 404/model unavailable | 保留官方模型可用性语义 | 不新增定制长期模型状态 |
| 账单探针一次成功 | 更新账单快照 | 不清除任何 `temp_unschedulable` |
| 账单探针连续两次成功 | 仍只更新账单快照 | 不调用通用恢复，不清除 NV-plus 类状态 |
| 账单探针失败/unsupported | 保留旧账单快照/失败状态 | 不改变调度资格 |
| CN provider 余额低/恢复 | 走 CN 专用停调/恢复 | 仅清理由 CN 专用逻辑拥有的状态 |
| 手动 `schedulable=false` | 保留人工暂停 | 监控标记人工暂停，不声称自动余额隔离 |
| `schedulable=true` 但有原生临时状态 | effective 为 false | UI 分开展示 raw/effective |
| 状态/过期/429/过载/quota 任一阻断 | effective 为 false | 与 `IsSchedulableAt` 结果完全一致 |
| 历史十年 reason | 只读展示/盘点 | 未获授权不改数据、不自动放行 |
| 状态写入失败 | 当前请求仍按官方路径结束 | 不扩大账号级隔离 |

## 12. 直接测试策略

书面规格批准后，只保留与本 P1 直接相关的最小验证：

- 删除/禁用定制分类器抢占的 service 回归，覆盖 402、403、404、429、5xx 和 `insufficient_user_quota` 对照。
- 账单探针生命周期测试：一次成功、连续两次成功、失败、unsupported、已有 temp 状态、已有 error/429/人工暂停，均断言不发生越权清理。
- CN provider 既有余额/额度测试回归，确认专用恢复未被破坏。
- `Account.IsSchedulableAt` 的快照时间边界测试。
- 管理账号接口 raw `schedulable` 与监控 `effective_schedulable` 的 API/组件测试。
- 直接相关包的 compile/build、必要 typecheck、格式检查和 `git diff --check`。
- 迁移集合、配置变化和 `.github/workflows` 范围门禁。

不运行全仓压力、mutation、soak、无关浏览器矩阵或人为消耗生产余额的测试。真实生产验证必须由根总控按发布约束，从干净且已推送的 `main` 通过既有发布链执行。

## 13. 发布、线上验证与回滚

### 13.1 发布前置

- 实现只能在本任务独立 worktree 完成，不能从本 worktree直接部署。
- 合并前必须由根总控盘点全部非 `main` worktree，确认目标 `main` 未漂移，并按项目单车道合并。
- 发布只能从干净、已推送且 `HEAD/tree == origin/main` 的根 `main` 发起。
- 验收站/主站均遵守 `acceptance-station-global-constraints.md`；主站只有“测试站验收通过，部署主站”或“快速部署到主站”两种明确授权路径。
- 本规格不授权生产历史状态批量清理；任何清理都必须在独立书面授权后执行。

### 13.2 线上专项验证

在授权发布后，根总控验证：

- 服务健康端点与原生管理接口可用。
- 代表性账号的 raw `schedulable`、原生状态、effective 投影互相一致。
- `baitumax-pro` 类 403 不被显示为自动余额隔离。
- `apizh-0.15`/`NV-plus` 类历史记录只按授权处置；账单探针成功不会越权恢复。
- CN provider 的专用余额恢复仍保持原语义。
- 账号监控不把账单 `ok` 或历史成功替换为 effective 调度资格。

不通过人为耗尽余额或向真实上游制造失败来验收；优先使用已有只读证据、测试站隔离账号和发布树绑定的直接测试。

### 13.3 回滚

- 运行时回滚使用既有本地/宿主蓝绿链切回上一已验证镜像。
- 若已发生经授权的单账号原生恢复，回滚二进制不会自动反向写回；按保存的脱敏前后快照和管理员原生入口逐项处理。
- 禁止通过旧候选 worktree、临时 checkout 或直接数据库覆盖人工回滚。
- 发布或线上验证失败时保留候选 worktree、失败证据和未提交修复，禁止清理失败候选。

## 14. 未决事项与明确边界

以下不是本规格的产品方向分歧，而是实施/发布阶段必须由根总控或用户书面确认的门禁：

1. 是否批准本书面规格，批准后才能写实施计划。
2. 实施时应以官方 v0.1.183 源快照逐文件核对哪些 S1-R2 文件可删除、哪些引用需要最小兼容修正。
3. 历史十年 `temp_unschedulable` 候选清单、分批范围和任何恢复动作必须另获书面授权；本规格不预先批准。
4. 验收站和主站发布授权仍遵守项目全局约束；本规格不构成主站发布授权。
5. 若实施发现需要迁移、不可逆数据操作、凭据处理、外部付费或改变 CN provider 语义，必须停止并重新取得用户决策。

## 15. 用户批准记录

2026-09-01，用户已明确批准以下设计方向，作为本规格的输入：

- 恢复并保持官方 Sub2API v0.1.183（commit `e8cb019fabf8b55199436229044cbf9aa7a82564`）原生账号状态机。
- 移除 `deterministic_failure_isolation` 对原生 402/403/404 的抢占。
- 移除 upstream billing probe 连续两次成功即清除任意 `temp_unschedulable` 的定制恢复。
- 保留官方账单探针本身、官方 402/403/429/5xx、CN provider 专用余额检查/恢复语义。
- 账号监控只修正为显示 effective schedulability，不改写原生状态语义。
- 历史十年临时隔离只写迁移/运营方案和授权门禁，不擅自回填或修改生产数据。

**仍待用户书面批准：** 本正式规格书整体批准。批准前不进入 `writing-plans`、实现、测试、合并、部署或生产处置。

