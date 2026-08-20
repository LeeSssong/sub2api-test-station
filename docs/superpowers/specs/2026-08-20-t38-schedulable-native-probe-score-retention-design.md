# T38 可调度账号最近原生探测评分保留规格

## 1. 状态、基线与批准门禁

- 任务包：T38 可调度账号最近原生探测评分保留
- 工作区：`/Users/gongtengxinwen/.codex/worktrees/83d7/sub2api搭建`
- 分支：`codex/t38-retain-native-probe-score`
- 创建基线：`main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`
- 基线 tree：`213f223f8d79ce5fd5548fae4a578261ef884547`
- 当前阶段：`DESIGNING`
- 批准门禁：本规格完成自审后提交唯一发布总控代审；收到 `APPROVE_SPEC_T38` 前不进入实施计划或代码阶段。

## 2. 问题证据与可靠复现

### 2.1 原生能力盘点

现有实现已具备本任务所需的原生事实与计算能力，无需增加第二套监控或评分系统：

1. `account_monitor_results` 保存 Sub 原生主动连接探测的 `status`、`http_status`、`ttft_ms`、`latency_ms` 与 `checked_at`。
2. `AccountMonitorService.ListWindow` 按所选 `24h|7d|30d` 读取 `ListAggregates`、`ListLatest` 与 `ListTimelines`。
3. `Account.IsSchedulable()` 是当前账号调度资格的权威入口，综合 active 状态、手工调度开关、到期、过载、限流、临时冷却以及 API Key/Bedrock 额度。
4. `accountMonitorWindowScoreBreakdown` 已按现有成本、成功率、TTFT、总耗时权重计算评分；真实业务请求聚合只作为调用上下文，不进入质量评分。
5. 前端已经按 `quality_score`、`score_status`、`group_rank` 渲染评分与排名，本任务无需新增 UI 字段。

### 2.2 当前错误耦合

当前投影把“当前状态新鲜度/可用性”同时当成“所选窗口评分资格”：

1. `projectAccountMonitorProbe` 先用最新探测、新鲜阈值、连续失败和致命错误计算 `AvailabilityStatus`。
2. `accountMonitorScoreStatus` 仅让 `normal` 与 `abnormal` 进入评分；`unavailable` 与 `stale` 统一得到 `ineligible`。
3. `projectAccountMonitorProbe` 随后清空 `QualityScore` 与 `GroupRank`。
4. `accountMonitorWindowEvidence` 又把超过 `2 × interval_seconds` 的窗口聚合标记为 `Source="stale"`，评分函数对该 source 直接返回空值。

因此，所选窗口内即使已有大量成功的原生主动探测，只要最新探测失败导致当前状态为 `unavailable`，或最后一次探测超过当前状态新鲜阈值导致 `stale`，已有窗口评分也会被抹去。

### 2.3 可重复证据

基线上的直接测试明确锁定了现状：

```text
go test ./internal/service \
  -run 'TestAccountMonitorProbeProjectionUsesOnlyFreshProbeEvidence|TestAccountMonitorPausedProbeProjectionScoresRanksAndKeepsNoEvidencePending' \
  -count=1 -v
```

结果通过，其中现有子场景明确断言：

- `three consecutive failures is unavailable` → `score_status=ineligible`；
- `fatal authentication error is unavailable immediately` → `score_status=ineligible`；
- `stale probes cannot score` → `score_status=ineligible`。

这不是仓储缺数据或前端丢字段，而是 service 投影主动清空评分资格。

## 3. 目标

1. 当前调用时 `Account.IsSchedulable()` 为真的账号，只要所选评分窗口内存在至少一条成功的原生主动探测样本，就继续展示该窗口聚合形成的评分并参与排名。
2. 最新探测失败、连续失败、致命原生探测错误或探测证据超过当前状态新鲜阈值时，当前状态仍如实显示 `abnormal|unavailable|stale`。
3. 当前状态与评分资格解耦：状态回答“现在是否可用/新鲜”，评分回答“所选窗口内已有原生主动探测表现如何”。
4. 从未在所选窗口获得成功原生探测证据的账号保持暂无评分、未排名。
5. 继续只读 `account_monitor_results` 形成质量评分，不引入真实业务请求补分。

## 4. 非目标

- 保持评分公式、权重、阈值、舍入、成本分量和排序 tie-break 规则。
- 保持账号调度器、探测 runner、探测重试、账号配置与状态写入行为。
- 保持 T32 已上线的暂停账号探测与评分合同；T38 只为当前 `Account.IsSchedulable()` 为真的账号增加保留语义。
- 保持 Monitor V2 分组状态投影、经营页、账务、采购与用量事实源。
- 无 schema、迁移、历史回填、生产数据写入、配置项或 API 路由变化。

## 5. 关键术语与字段语义

### 5.1 “可调度”

本规格中的“可调度”严格指快照时调用原生 `Account.IsSchedulable()` 返回 `true`，不是仅查看持久化字段 `accounts.schedulable=true`。

这意味着以下任一条件都可使账号不进入 T38 新增的保留分支：非 active、手工关闭调度、已到期自动暂停、处于 overload/rate-limit/temp-unschedulable 窗口、API Key/Bedrock 额度耗尽。

### 5.2 “当前状态”

- `normal`：当前新鲜且最新探测成功。
- `abnormal`：当前新鲜但最新探测失败，尚未达到不可用判定。
- `unavailable`：当前新鲜但满足连续失败或致命探测错误等既有不可用判定。
- `stale`：没有当前新鲜结果；包括结果超过 `2 × interval_seconds`。
- `disabled`：保留既有兼容语义，本任务不扩展其使用范围。

这些状态继续由现有最新探测、新鲜度和失败规则决定，不由评分保留结果反向改写。

### 5.3 “可评分原生证据”的最低条件

对 T38 新增的“当前可调度账号保留评分”分支，最低条件为：

1. 证据来自所选 `24h|7d|30d` 窗口的 `account_monitor_results` 聚合；
2. `SampleCount > 0`；
3. `SuccessSampleCount > 0`；
4. 当前账号 `Account.IsSchedulable() == true`；
5. 评分所需成本资格继续满足既有全局/分组规则。

TTFT 或总耗时样本为空时，继续沿用现有公式：对应分量为零，其余已有分量仍可形成评分。纯失败样本没有成功质量证据，不进入保留评分；这避免仅靠成本分量生成误导性分数。

## 6. 方案比较

### 方案 A：前端缓存最后一个非空评分

做法：页面发现新响应的评分为空时，保留浏览器内旧值。

优点：前端改动小。

缺点：刷新或换设备即丢失；后端 API 仍错误；缓存可能跨范围、跨分组串值；无法提供一致排名。排除。

### 方案 B：让 `unavailable|stale` 全部继续评分

做法：只修改 `accountMonitorScoreStatus`，让所有有样本行进入评分。

优点：修改点少。

缺点：会让纯失败样本仅凭成本获得分数，也会覆盖当前不具备原生调度资格的账号；仍未显式分离状态证据和评分证据。排除。

### 方案 C：状态投影与评分资格分离（推荐）

做法：保留现有状态计算；新增窄的“窗口原生证据可评分”判定。当前 `Account.IsSchedulable()` 为真且窗口至少有一条成功探测时，评分使用同一窗口聚合，不受最新失败或当前新鲜阈值影响；状态字段保持原值。

优点：精确满足用户语义；继续复用原生事实源与公式；不新增字段、迁移或缓存；可同时覆盖 latest failure 与 stale 两条边界；对 T32 暂停账号规则影响最小。

代价：需要把 `ScoreStatus/Eligible` 的生成从单纯映射 `AvailabilityStatus` 调整为显式评分资格，并确保全局与分组投影共用同一判定。

选择方案 C。

## 7. 端到端数据与控制流

1. `ListWindow` 按现有范围读取账号、窗口请求上下文、原生探测聚合、最新探测、时间线、分组与权重。
2. 对每个账号调用 `Account.IsSchedulable()`，得到快照时原生调度资格；该调用仅用于投影判断，不写账号状态，也不改变 scheduler。
3. `projectAccountMonitorProbe` 继续从最新探测、新鲜阈值、连续失败与致命错误计算 `AvailabilityStatus`、`Stale`、`ServiceState`。
4. 独立评分资格判定使用：当前原生可调度 + 窗口聚合至少一条成功样本 + 既有成本资格。
5. 评分证据使用所选窗口聚合的成功率、TTFT P50、延迟 P95 与样本数；即使状态为 `unavailable` 或 `stale`，该窗口证据仍以原生探测评分来源参与既有公式。
6. 全局与各分组分别使用原有权重计算；排序继续按评分降序、账号 ID tie-break；只对可评分行生成连续排名。
7. API 返回现有字段；前端无需本地缓存或默认分数，按已有 `quality_score/group_rank` 直接展示。

## 8. 接口与字段契约

不新增、删除或重命名 JSON 字段。

| 字段 | T38 后语义 |
| --- | --- |
| `availability_status` | 当前原生探测状态；可为 `unavailable` 或 `stale`，同时仍有评分 |
| `stale` | 当前状态证据是否超过新鲜阈值；不再等同于历史窗口评分缺失 |
| `score_status` | 当前窗口评分资格；有可评分原生证据时为 `eligible`，现有异常封顶场景继续为 `capped` |
| `quality_score` | 所选窗口原生探测聚合按既有公式得到的分数 |
| `group_rank` | 同一全局或分组评分集合中的连续排名 |
| `evidence_source` / `evidence.source` | 评分证据来源；评分使用窗口原生探测聚合时表达为 `monitor_probe`，不以状态 stale 覆盖评分来源 |
| `latest_status` / `latest` / `checked_at` | 最新原生探测事实，保持真实失败或旧时间 |
| `eligible` | 与 `score_status` 的评分资格一致，不代表当前服务可用，也不代表 scheduler 会选择该账号 |

兼容重点：调用方若要判断当前服务状态，继续读取 `availability_status/service_state`；若要判断是否有评分，读取 `score_status/quality_score/group_rank`。两者允许出现“当前不可用但有历史窗口评分”的组合。

## 9. 边界场景

### 9.1 最新失败

- 前置：`Account.IsSchedulable()==true`，所选窗口有成功探测，最新探测失败。
- 状态：按现有规则显示 `abnormal` 或 `unavailable`。
- 评分：使用所选窗口全部原生探测聚合，保留分数与排名。
- 说明：最新失败已经通过窗口成功率进入公式，不另造“上一次成功分数”缓存。

### 9.2 stale

- 前置：`Account.IsSchedulable()==true`，所选窗口有成功探测，但最后探测超过当前新鲜阈值。
- 状态：显示 `stale`/待确认。
- 评分：使用所选窗口原生探测聚合，保留分数与排名。
- 若所选 24h 窗口没有成功探测，而 7d 窗口有：24h 保持暂无评分；切到 7d 后显示 7d 聚合评分。这样不混用不同范围，也保持范围切换语义确定。

### 9.3 从未有有效成功探测

- `SampleCount==0`，或窗口内全部为失败且 `SuccessSampleCount==0`。
- 当前状态继续按已有事实显示 `stale|unavailable`。
- 评分、构成与排名为空。

### 9.4 当前已不具备原生调度资格

- T38 新增保留分支不适用。
- T32 已有暂停账号合同继续由既有路径决定；本任务不借机重写暂停、关闭调度或停止探测规则。

### 9.5 成本资格缺失

- 全局评分继续使用既有全局成本处理。
- 分组倍率待确认或成本超出分组门槛时，保持既有 `multiplier_pending|cost_ineligible` 与无组内排名语义。
- 当前可用性与原生探测样本不会绕过成本资格。

## 10. 失败与安全语义

- 仓储读取失败继续返回错误，不合成旧快照或默认分数。
- 评分只读取 `account_monitor_results`；`usage_logs`、`ops_error_logs` 与真实业务成功率不进入评分。
- 不触发额外主动探测，不写探测表，不延长探测历史保留期。
- 不因评分保留自动开启账号、清理冷却、恢复额度、改变优先级或进入 scheduler。
- 状态为 `unavailable|stale` 时仍保持醒目状态文案；评分存在不代表当前健康。

## 11. 兼容性、迁移与发布属性

- API 结构兼容：仅调整现有字段允许的组合。
- 数据库：无迁移、无回填、无生产数据写入。
- 配置：无新增或变化。
- 前端：预计无需功能修改；若直接合同测试发现前端把 `unavailable|stale` 强制转成无分数，只做最小兼容修正。
- 预计 `downtime_required=false`，最终由根总控在合并后发布预检确认。

## 12. 场景化验收矩阵

| 场景 | `IsSchedulable` | 窗口成功样本 | 当前状态 | 评分 | 排名 |
| --- | ---: | ---: | --- | --- | --- |
| 最新成功且新鲜 | true | >0 | normal | 有 | 有 |
| 单次最新失败 | true | >0 | abnormal | 有，沿用窗口公式/既有封顶语义 | 有 |
| 连续失败达到不可用 | true | >0 | unavailable | 有 | 有 |
| 致命 401/403/余额类探测错误 | true | >0 | unavailable | 有 | 有 |
| 最后探测超过新鲜阈值 | true | >0 | stale | 有 | 有 |
| 窗口内只有失败样本 | true | 0 | abnormal/unavailable | 空 | 空 |
| 窗口内无任何探测 | true | 0 | stale | 空 | 空 |
| 24h 无成功、7d 有成功，当前选择 24h | true | 0 | 按当前事实 | 空 | 空 |
| 24h 无成功、7d 有成功，切到 7d | true | >0 | 按当前事实 | 有 | 有 |
| 当前原生不可调度 | false | 任意 | 按既有 T32 语义 | 保持既有语义 | 保持既有语义 |
| 分组成本不合格 | true | >0 | 按当前事实 | 全局按既有规则；组内不合格 | 无组内排名 |

## 13. 测试策略

实施阶段严格 TDD RED→GREEN，仅覆盖直接相关功能：

1. service 纯投影测试：最新失败但窗口有成功样本时，状态不可用且评分/排名存在。
2. stale 边界测试：当前状态 stale，窗口有成功样本时评分/排名存在。
3. 最低证据测试：无样本、纯失败样本均保持空评分。
4. 原生可调度语义测试：覆盖 raw `schedulable=true` 但因 expiry/cooldown/quota 导致 `IsSchedulable()==false`，确保不误入 T38 保留分支。
5. 全局与分组投影测试：两处排名采用相同资格；分组成本资格仍生效。
6. handler JSON 合同：允许 `availability_status=unavailable|stale` 与非空 `quality_score/group_rank` 同时出现。
7. 前端直接组件合同：确认状态仍显示不可用/待确认，同时评分与排名正常渲染；仅在当前测试缺口需要时补充，不做页面重构。
8. 聚焦命令预计包括受影响 service/handler/frontend test、必要编译或 typecheck、`gofmt` 与 `git diff --check`；不扩大到全仓回归。

## 14. 回滚、未验证项与风险

### 回滚

回滚为撤销 T38 功能提交并重新走根总控发布链。无数据回滚、迁移回退或配置恢复。

### 留给根总控的验证

- 合并后最新 `main` 的直接相关回归与发布预检；
- 蓝绿发布、健康检查与线上登录态账号评分专项；
- 生产样本中验证“当前不可用/待确认 + 有评分/排名”组合。

### 剩余风险

1. stale 账号的评分可能来自所选窗口较早时段，管理员必须结合状态徽标理解；现有状态字段继续醒目展示以控制该风险。
2. 当前不可用账号仍进入评分排序，符合用户明确要求，但排名不代表当前 scheduler 选择顺序；字段语义在本规格中已明确分离。
3. `Account.IsSchedulable()` 使用当前时间；测试需覆盖时间边界，避免 raw 字段与原生方法口径漂移。

## 15. 待决事项

无产品待决事项。推荐方案、证据最低条件、latest failure、stale、范围切换、T32 兼容、发布和回滚边界均已闭环，提交根总控代审。
