# T34 Monitor V2 渠道状态原生探测重构设计

## 状态

- 阶段：`DESIGNING`
- 规格结论：待根总控审查
- 实施门禁：根总控批准本规格前，不编写实施计划，不修改业务代码

## 问题

Monitor V2 当前混合两条来源：分组当前状态与时间线来自 `ChannelMonitorService`，TTFT、TPS 和总延迟来自 `usage_logs`。这使同一张卡片表达的状态与性能不属于同一条证据链，也把真实网关请求统计带入了本应展示原生账号主动探测的页面。

现有 `AccountMonitorService` 已经通过 `account_monitor_results` 保存账号主动探测的 `status`、`ttft_ms`、`latency_ms` 和 `checked_at`，并具备账号关系、当前结果、时间线和聚合能力。T34 应在这条链路上增加最小的分组只读投影，Monitor V2 只负责可见分组选择、API 投影和前端渲染。

## 目标

1. Monitor V2 的可用性、首字速度、平均耗时、当前状态与时间线全部只来自 `account_monitor_results` 原生账号主动探测。
2. 分组时间桶采用固定二态：至少一个当前可调度账号在桶内原生探测成功即为 `operational`，其余情况均为 `unavailable`。
3. 首字速度使用原生成功样本的 TTFT P50；平均耗时使用原生成功样本的 `AVG(latency_ms)`。
4. 分组倍率紧邻分组名称并作为醒目标识；移除“旗舰”徽标和旗舰排序语义。
5. 使用直白文案展示 `可用性：99%`、`首字速度：10.99s`、`平均耗时：10s`，并明确展示当前状态。
6. 保持 T34 开始时 Monitor V2 的页面宽度和响应式框架，不再放大页面。

## 非目标

- 不修改账号主动探测的调度周期、执行器、重试、探测模型或写入格式。
- 不修改账号调度选择、评分、计费、采购、倍率计算或分组成员关系。
- 不修改旧 Channel Monitor、账号监控管理页、CodexRadar 或其他页面。
- 不读取 `usage_logs`，不使用真实网关请求统计，不增加第二个探测器。
- 不新增数据库迁移，不修改生产数据，不进入合并、推送、部署或线上验证。
- 不修改全局任务队列、项目进度总账、发布证据或生产状态记录。

## 现状证据

- `backend/internal/service/monitor_v2.go` 依赖 `MonitorV2ProbeReader.ListUserView()` 和 `MonitorV2Repository.GetPerformanceStats()`，分别取得 Channel Monitor 探测与性能统计。
- `backend/internal/repository/monitor_v2_repo.go` 从 `usage_logs` 计算 TTFT P50/P95、延迟 P50/P95 和 TPS。
- `backend/internal/repository/account_monitor_repo.go` 已从 `account_monitor_results` 提供账号级聚合、latest 和 timeline；原生聚合已有 TTFT P50、延迟 P50/P95，但没有显式 `AVG(latency_ms)`。
- `Account.IsSchedulable()` 已包含 active、手动 schedulable、到期、过载、限流、临时冷却和 API Key/Bedrock 额度判断，是当前调度资格的权威入口。
- `AccountMonitorHistoryDays = 7` 同时被账号监控聚合窗口和结果清理使用，而 Monitor V2 支持 `24h`、`7d`、`30d`。
- 前端 v6 合同仍包含 `is_flagship`、`ttft_p95`、`tps`、`latency_p95`，卡片从时间线在浏览器中重新计算可用性，并显示“旗舰”。
- `wire_gen.go` 当前先构造 Monitor V2，后构造 Account Monitor；T34 注入既有服务时需要最小调整构造顺序。

## 方案比较

### 方案 A：扩展 AccountMonitorService 的分组原生投影（采用）

在 `AccountMonitorService` 增加只读的 Monitor V2 分组投影方法。服务加载账号、使用 `Account.IsSchedulable()` 确定当前资格集合，再调用 `account_monitor_repo` 的批量分组原生聚合。Monitor V2 只传入可见分组和窗口并消费结果。

优点：账号资格、新鲜度和原生结果仍由 Account Monitor 领域拥有；没有第二套探测逻辑；Monitor V2 不接触账号隐私或 SQL；便于分别测试资格选择与聚合 SQL。缺点：需要最小扩展 Account Monitor 接口，并调整 wire 构造顺序。

### 方案 B：Monitor V2 直接注入原生 repository（不采用）

让 Monitor V2 自己加载账号并直接查询 `account_monitor_results`。

优点：调用链短。缺点：Monitor V2 会重复账号关系、调度资格和新鲜度规则；后续 `Account.IsSchedulable()` 演进时容易产生口径漂移，也扩大了 Monitor V2 对内部账号数据的知情范围。

### 方案 C：新增独立聚合适配器（不采用）

新建一个专供 Monitor V2 的原生探测聚合服务，独立读取账号与结果表。

优点：文件边界独立。缺点：形成第二个原生监控领域服务，重复 Account Monitor 已有职责，不符合“直接复用或最小扩展既有原生链路”的范围。

## 架构

### AccountMonitorService

新增窄接口供 Monitor V2 依赖，概念签名为：

```go
type MonitorV2NativeProbeReader interface {
    ProjectMonitorV2Groups(
        ctx context.Context,
        groupIDs []int64,
        start time.Time,
        end time.Time,
        bucketSize time.Duration,
    ) (map[int64]MonitorV2NativeGroupProjection, error)
}
```

`AccountMonitorService` 的职责：

1. 加载当前账号完整记录并去重。
2. 只保留调用时 `Account.IsSchedulable()` 为真的账号。
3. 展开账号的原生 `GroupIDs`/`Groups` 关系，只建立请求中可见分组的 `(group_id, account_id)` 唯一 scope。
4. 加载 Account Monitor 设置，使用 `2 * interval_seconds` 作为当前状态的新鲜阈值；设置缺失时沿用现有默认值和持久化行为。
5. 调用原生 repository 批量查询，并把 repository 结果投影为不含账号标识的分组结果。

历史结果没有记录账号当时是否可调度，因此 T34 明确采用“当前资格集合回看所选窗口”：账号在生成快照时可调度，其窗口内原生结果才参与当前快照；当前不可调度账号的历史成功不参与。此规则不推断历史资格，也不改变调度器。

### account_monitor_repo

通过可选窄接口增加一个批量分组读取方法，生产 `accountMonitorRepository` 实现该接口。使用可选接口可避免把 Monitor V2 专属读取强加给所有 Account Monitor 测试桩；生产实现缺失该能力时返回明确错误，不降级到其他数据源。

查询输入是去重后的 `(group_id, account_id)` scope、`[start, end)`、桶宽和新鲜阈值。查询只读取 `account_monitor_results`，一次批量返回：

- 每组最新原生状态判定所需结果；
- 每组成功原生样本的 TTFT P50；
- 每组成功原生样本的平均 `latency_ms`；
- 每组固定时间桶的二态和桶内成功样本平均延迟；
- TTFT 与 latency 各自的非空成功样本数。

不复用或迁移 `monitor_v2_repo.go` 中的 `usage_logs` SQL；旧 Monitor V2 repository 及其 provider 在 T34 中删除。

### MonitorV2Service

Monitor V2 保留以下职责：

- 从 `GroupRepository.ListActive()` 选择启用分组；普通用户排除 exclusive 分组，管理员保留现有可见范围。
- 保持 repository 返回的稳定分组顺序，不再按“旗舰”置顶。
- 把 `24h`、`7d`、`30d` 转换为窗口和桶宽。
- 调用 `MonitorV2NativeProbeReader`，合并分组名称、平台、倍率和峰值倍率元数据。
- 校验最多 100 个分组、文本长度和固定时间线长度。
- 从系统设置读取页面刷新间隔；刷新间隔不参与健康或性能计算。

Monitor V2 不再依赖 `ChannelMonitorService`、`MonitorV2Repository`、主模型选择或真实请求样本门槛。

## 数据语义

### 窗口与时间桶

所有时间先转为 UTC，范围统一为 `[start, end)`，其中 `end` 是快照的 `generated_at`：

| 窗口 | start | 桶宽 | 固定桶数 |
| --- | --- | --- | --- |
| `24h` | `end - 24h` | 1 小时 | 24 |
| `7d` | `end - 7d` | 6 小时 | 28 |
| `30d` | `end - 30d` | 24 小时 | 30 |

桶从 `start` 起连续生成，最后一个桶结束于 `end`。每个可见分组始终返回完整固定桶数，按 `bucket_start` 升序排列，避免前端自行补桶或按账号结果数量改变布局。

### 时间线状态

对每个分组、每个桶：

- scope 内任一账号存在 `status = 'success'` 的原生结果：`operational`；
- 其他所有情况，包括只有失败结果、所有指标为空或没有原生结果：`unavailable`。

多个可调度账号同桶内一成一败仍为 `operational`。`timeline.latency_ms` 是该桶成功且 `latency_ms IS NOT NULL` 样本的平均值并四舍五入到整数毫秒；没有这类样本时为 `null`。

### 当前状态

每个 scope 账号只取 `checked_at <= end` 的最新一条原生结果；同时间以结果主键较大者为新。分组满足以下条件时为 `operational`：至少一个账号的最新结果是 `success`，且其 `checked_at >= end - 2 * interval_seconds`。其他情况均为 `unavailable`。

当前状态不使用所选窗口的最后一个时间桶代替，以免刚进入新桶但尚未到探测周期时产生瞬时假红；它仍完全来自最新原生账号探测。

### 可用性

```text
availability = round(operational_bucket_count * 100 / total_bucket_count)
```

结果是 `0..100` 的整数百分比。固定桶没有成功证据时按 `unavailable` 进入分母，因此无探测历史的分组显示 `0%`，不会把“无数据”伪装成高可用。

### 首字速度与平均耗时

- 首字速度：窗口内 scope 账号所有 `status = 'success' AND ttft_ms IS NOT NULL` 原生结果的 TTFT P50，四舍五入到整数毫秒。
- 平均耗时：窗口内 scope 账号所有 `status = 'success' AND latency_ms IS NOT NULL` 原生结果的 `AVG(latency_ms)`，四舍五入到整数毫秒。
- 两个指标使用各自真实非空成功样本数，不要求共同样本数，不套用 v6 的五样本门槛。
- 对应样本数为零时，指标状态为 `insufficient_data`、值为 `null`；前端显示 `—`。

失败结果只影响时间桶状态，不进入 TTFT 或平均耗时计算。任何统计都不读取 `usage_logs`。

## 30 天历史保留

T34 将“账号监控评分/管理页的 7 天聚合窗口”和“原生结果物理保留期”拆开：

- `AccountMonitorHistoryDays = 7` 继续用于现有账号监控聚合、评分和展示，不改变其行为。
- 新增独立的原生结果保留期 30 天，仅把 `DeleteBefore` 的清理边界从 7 天改为 30 天。

这不改变探测调度或评分，只保留已产生的原生结果以支持 Monitor V2 `30d`。发布后没有历史回填：在新保留策略逐日积累满 30 天前，尚未保存的旧桶按既定规则显示 `unavailable`。该冷启动行为真实反映证据缺失，不从其他来源补数。

## API 合同 v7

`contract_version` 从 `6` 升为 `7`，避免 v6 客户端把新语义误当真实请求性能。响应仍使用现有快照外壳：

```json
{
  "contract_version": "7",
  "window": "7d",
  "refresh_interval_seconds": 60,
  "generated_at": "2026-08-20T00:00:00Z",
  "groups": [
    {
      "id": 7,
      "name": "GPT Pro",
      "platform": "openai",
      "rate_multiplier": 0.2,
      "peak_rate_enabled": false,
      "peak_start": "",
      "peak_end": "",
      "peak_rate_multiplier": 1,
      "status": "operational",
      "availability": {"state": "available", "value": 99, "sample_count": 28},
      "ttft": {"state": "available", "value": 10990, "sample_count": 27},
      "average_latency": {"state": "available", "value": 10000, "sample_count": 26},
      "timeline": [
        {"bucket_start": "2026-08-13T00:00:00Z", "status": "operational", "latency_ms": 10000}
      ]
    }
  ]
}
```

v7 删除：

- `is_flagship`
- `ttft_p95`
- `tps`
- `latency`
- `latency_p95`

v7 新增或重定义：

- `availability`：后端根据固定原生时间桶计算的整数百分比；`sample_count` 是固定桶数。
- `ttft`：原生成功样本 TTFT P50。
- `average_latency`：原生成功样本平均耗时。

指标状态只接受 `available | insufficient_data`。`availability` 在合法窗口中始终为 `available`；另外两个指标只有存在对应成功样本时为 `available`。前端运行时校验器要求 v7 精确必需字段，并拒绝 v6 的 `is_flagship`、P95、TPS 和旧 `latency` 字段。

API 不返回账号 ID、账号名、模型、供应商、凭据、错误详情或单账号结果。

## 前端设计

### 卡片信息层级

1. 第一行依次展示分组名称、紧邻名称的醒目倍率标识、当前状态二态标识。
2. 第二行使用直白中文标签展示三个值：
   - `可用性：99%`
   - `首字速度：10.99s`
   - `平均耗时：10s`
3. 时间线直接渲染后端固定桶；前端不再计算可用性、不补桶、不合并账号结果。
4. 删除“旗舰”徽标及相关文案、测试选择器和样式。

耗时统一把毫秒转换为秒，最多保留两位小数并去除末尾零；无样本显示 `—`。倍率最多保留四位小数并以 `×` 结尾。当前状态继续使用 `运行中` / `服务不可用` 二态文案。

### 布局约束

- 保持 T34 基线页面容器 `max-w-[1500px]`，不增加宽度。
- 保留现有单列卡片与桌面/窄屏响应式结构，只调整卡片内部信息顺序。
- 时间线固定尺寸不因文案或动态值改变布局；长分组名称可换行或截断，但不得覆盖倍率和状态。
- 不增加说明性页面文案，不修改 CodexRadar 区域。

## 错误语义

- 可见分组没有当前可调度账号：正常返回该分组，状态 `unavailable`、可用性 `0%`、性能指标无数据、时间线全为 `unavailable`。
- 当前可调度账号没有历史结果：同上，不视为 repository 错误。
- Account Monitor 设置、账号列表或原生聚合查询失败：Monitor V2 快照返回错误，由现有 handler 返回失败响应；不得回退到 Channel Monitor、`usage_logs`、旧缓存或伪造 `unavailable` 快照。
- 系统页面刷新间隔读取失败：保持现有默认刷新间隔；该设置不影响原生指标正确性。
- 请求上下文取消：立即返回取消错误，不继续组装部分快照。

## 构造与依赖

生产构造顺序最小调整为：先构造 `accountMonitorRepository`、`accountMonitorAccountRepository` 和 `accountMonitorService`，再把 `accountMonitorService` 作为 `MonitorV2NativeProbeReader` 注入 `MonitorV2Service`。

移除 Monitor V2 对 `channelMonitorService` 和 `monitorV2Repository` 的 provider 参数。其余 provider 和 handler 路由不变，不进行大范围 wire 重排。

## 测试策略

### Account Monitor service

- 只把 `Account.IsSchedulable()` 为真的账号展开到分组 scope。
- disabled、手动不可调度、过期、冷却、限流、过载和额度耗尽账号均不贡献结果。
- 多分组账号正确展开，重复关系只产生一个 scope；不相关分组被过滤。
- 设置的 `2 * interval_seconds` 正确传入当前状态新鲜边界。
- repository 能力缺失或查询失败时返回错误，不降级。

### account_monitor_repo sqlmock

- 查询只出现 `account_monitor_results`，不出现 `usage_logs`。
- 单次批量查询覆盖多个分组与账号 scope。
- latest 使用每账号最新一条，时间边界为 `[start, end)`，新鲜度边界正确。
- 同桶至少一次 `success` 即 operational；零结果桶仍返回 unavailable。
- TTFT 使用成功非空样本 P50，平均耗时显式使用 `AVG(latency_ms)`，样本数分别统计。
- 输入为空、重复 scope、非法窗口和非法桶宽得到稳定结果或明确错误。
- 清理使用 30 天物理保留期，现有 7 天聚合窗口保持不变。

### Monitor V2 service/handler

- public/admin 分组可见性保持不变；没有可调度账号的可见分组仍返回。
- 三个窗口分别返回 24、28、30 个时间桶。
- v7 合同只含原生指标字段，移除旗舰、TPS、P95 和旧延迟字段。
- 原生 reader 错误向上返回；不再静默吞错。
- handler 保持 `Cache-Control: no-store`，不泄露账号级字段。

### 前端 Vitest

- 运行时校验器接受 v7、拒绝 v6 和被删除字段。
- 页面直接展示 `可用性：99%`、`首字速度：10.99s`、`平均耗时：10s`。
- 倍率紧邻分组名，页面不存在“旗舰”、TPS、P95 和浏览器端可用性计算。
- 二态当前状态、无指标占位、24/28/30 点时间线和窄屏布局正确。
- 页面容器仍为 `max-w-[1500px]`。

实施阶段只运行上述直接相关 Go/Vitest、必要的前端 typecheck/build、必要 Go build、格式化和 `git diff --check`；视觉变更在桌面与 390px 视口核对卡片内部，不扩大页面范围。

## 验收标准

- [ ] Monitor V2 业务路径中不存在 `usage_logs` 查询或 `ChannelMonitorService` 探测依赖。
- [ ] 可用性、首字速度、平均耗时、当前状态和时间线全部可追溯到 `account_monitor_results`。
- [ ] 时间桶严格执行“至少一个当前可调度账号成功即 operational，否则 unavailable”。
- [ ] v7 API 和前端只保留可用性、TTFT P50、平均耗时与固定二态时间线。
- [ ] 页面显示直白文案，倍率紧邻名称，“旗舰”完全移除。
- [ ] 页面宽度保持 T34 基线，账号探测调度、评分、计费、采购、CodexRadar 和其他页面行为不变。
- [ ] 30 天物理保留已与现有 7 天聚合窗口解耦，冷启动不使用其他数据源回填。
- [ ] 直接相关测试、必要构建/类型检查和 diff check 通过。

## 风险与控制

- **30 天冷启动偏低**：旧结果已清理，初期缺失桶按 unavailable。规格禁止从真实请求或旧监控补数，待原生历史自然积累。
- **结果存储量增加**：物理保留由 7 天增至 30 天，约为原来的 4.3 倍；不改变写入频率，依赖现有 `account_id/checked_at` 查询索引和批量 scope 查询，实施时用 SQL 测试锁定单查询路径。
- **当前资格回看历史**：历史没有资格快照。规格固定为当前资格集合，避免推测；未来若需要历史资格必须单独设计数据模型。
- **依赖重排影响启动**：只移动 Account Monitor 构造到 Monitor V2 之前并删除两项旧依赖，使用必要 Go build 验证 wire 生成结果。
- **公开合同破坏性变化**：v7 明确 fail closed，前后端同任务切换，`no-store` 防止旧快照缓存；不提供含混兼容字段。

## 自审结果

- 占位扫描：无 `TBD`、`TODO` 或待定口径。
- 一致性：数据源、时间桶、当前状态、指标样本、API 字段和前端文案使用同一原生定义。
- 范围：仅涉及 Account Monitor 原生只读投影、Monitor V2 投影/渲染、30 天结果保留和必要构造；没有延伸到调度、评分或其他页面。
- 歧义处理：明确了当前资格集合、无数据桶、边界时间、状态新鲜度、百分比舍入、耗时格式、30 天冷启动和错误不降级。

## 根总控审查请求

请根总控审查并明确批准或退回本规格。重点确认：

1. 采用 `AccountMonitorService` 最小扩展而不是 Monitor V2 直连 repository；
2. 历史桶使用“当前 `Account.IsSchedulable()` 资格集合”；
3. 物理保留期扩为 30 天，但既有评分/管理聚合仍保持 7 天；
4. v7 删除 TPS/P95/旗舰字段，不保留 `not_provided` 兼容壳；
5. 原生读取失败时请求失败，不降级为其他来源或伪造不可用。

批准前 T34 保持 `DESIGNING`，不进入 writing-plans 或实现。
