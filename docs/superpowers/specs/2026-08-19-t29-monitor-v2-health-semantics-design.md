# T29 Monitor V2 二态健康展示与统一指标口径设计

## 目标

Monitor V2 只表达两种用户可见服务状态：`运行中` 与 `服务不可用`。页面不展示任何百分比，也不展示真实请求成功率。TTFT、TPS、总延迟和倍率继续来自真实数据，并使用统一统计样本。

## 用户可见合同

1. 卡片、整体状态、时间线、悬浮提示和无障碍文本中不出现成功率、可用率、缓存命中率或其他百分比。
2. 分组状态只有 `operational` 和 `unavailable`：
   - 最新新鲜主动探测为 `operational` 或 `degraded` 时投影为 `operational`，文案为“运行中”。
   - 最新新鲜主动探测为 `unavailable`、`failed`、`error`，或没有新鲜有效探测时投影为 `unavailable`，文案为“服务不可用”。
3. 整体状态只使用相同二态：至少一个分组运行中时显示“运行中”，否则显示“服务不可用”。
4. 时间线点只携带探测时间、二态状态和可选延迟；运行中为绿色，服务不可用为红色。提示文本不出现“成功/失败请求”或计数。
5. 卡片只展示倍率、TTFT P50、TPS 和总延迟 P50，不展示样本数、缓存命中率、有效调用、成功率和可用率。性能指标结构中的 `sample_count` 只用于判断数值是否达到最小真实样本门槛，不进入卡片、悬浮提示或无障碍文本。
6. 名称含独立 `Pro` 层级标识或“旗舰”的分组固定置顶并显示“旗舰”徽标。Plus 及其他分组按原有稳定顺序展示。

## API 合同

Monitor V2 `contract_version` 从 `5` 升为 `6`。

- `group.status` 收敛为 `operational | unavailable`。
- 新增 `group.is_flagship: boolean`。
- 删除 `group.availability` 与 `group.cache_hit`。
- 保留 `ttft`、`ttft_p95`、`tps`、`latency`、`latency_p95`，便于现有服务层和管理诊断继续使用统一计算结果；页面仍只渲染 P50。
- `timeline[]` 删除 `state`、`value`、`success_count`、`eligible_count`，改为 `bucket_start`、`status`、`latency_ms`。
- 前端运行时校验器拒绝旧百分比字段，避免后续误接回页面。

## 统一统计口径

Monitor V2 不再拼接 Ops Overview、OpenAI Token Stats 与缓存 SQL。监控专用 repository 使用单个批量查询，对每个分组按其主动监控的主模型建立 scope，并从同一 `eligible` CTE 计算全部性能指标。

统一样本必须同时满足：

- `usage_logs.group_id` 等于当前分组；
- `created_at >= start AND created_at < end`；
- `usage_logs.model` 等于该分组最新监控配置的非空 `primary_model`；
- `actual_cost > 0`；
- `billing_mode='token'`，或历史空 billing mode 且图片/视频计数字段全部为零；
- `duration_ms > 0`；
- `first_token_ms > 0`；
- `output_tokens > 0`。

同一批行同时计算：TTFT P50/P95、总延迟 P50/P95、平均输出 TPS。三个指标的 `sample_count` 必须完全一致。repository 查询失败或不足 5 个样本时仅返回“样本不足”，不推测或填充数值。

主模型选择使用分组关联主动监控中 `PrimaryCheckedAt` 最新且 `PrimaryModel` 非空的记录；并列时使用稳定字符串顺序。不同分组配置不同主模型时如实按各自主模型统计，不人为覆盖结果。

## Pro 定位

Pro 的“最好”表达为明确的旗舰产品层级、固定第一顺序和真实同口径指标，不通过复制 Plus 数值、改变单位、选择性隐藏较差样本或人工改写统计结果实现。若统一口径后 Pro 的真实性能仍落后，页面仍展示真实值，后续应调整 Pro 上游账号池与调度，而不是修饰监控数据。

## 非目标

- 不改变主动探测的六次重试机制、探测调度或历史保留。
- 不改变分组成员、账号调度、价格、计费、用户错误投影或 CodexRadar。
- 不修改旧 Channel Monitor 用户详情接口；本次只收敛 Monitor V2 页面及其 API。
- 不新增迁移，不写生产业务数据，不使用 GitHub Actions。

## 验收

- 后端 service tests 锁定二态映射、时间线二态、Pro 置顶/旗舰及主模型 scope。
- repository sqlmock tests 锁定统一 CTE、资格谓词、同样本指标和输入边界。
- handler/API tests 锁定 v6 且百分比字段不存在。
- 前端 Vitest 锁定百分号、成功率、缓存率、波动文案不存在，二态配色、Pro 顺序和窄屏时间线正确。
- 运行直接相关 Go/Vitest、typecheck、frontend build、必要 Go build、gofmt 和 `git diff --check`，并在桌面与 390px 做视觉核对。
