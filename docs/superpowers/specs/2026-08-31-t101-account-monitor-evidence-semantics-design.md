# T101 账号监控证据语义修复规格

日期：2026-08-31

## Problem

管理员账号监控卡片把仓储层返回的非空真实请求桶直接渲染为等宽柱。当所选窗口只有两个非空桶时，两根柱会各占一半图表宽度，看起来像两个连续的大面积异常区，而不是 24 个时段中的两个样本。

全站视图复用分组卡片字段，但全站账号对象没有唯一 `group_profitability`。前端把字段缺失映射为“待确认”，错误地暗示存在一个尚未确认的全站利润率。图表旁“刷新主动探测”也容易被理解为会刷新真实请求时间线，实际它只触发主动状态探测。

## Goal

- 每张账号卡的真实性能图固定展示 24 个按时间顺序排列的等宽桶，空桶保留为无数据状态。
- 全站范围的利润率明确显示“按分组查看”；只有选择具体分组后才显示该分组的已确认或估算利润率。
- 图表文案明确区分真实请求证据与主动状态探测。

## Non-goals

- 不改变利润率公式、成本完整性判断、估算规则或排序。
- 不改变真实请求成功/失败去重、TTFT P95 阈值或柱高公式。
- 不改变主动探测执行、调度、计费、账号/分组数据、数据库迁移或配置。
- 不新增页面、图表库、依赖或 GitHub Actions。

## Backend Contract

`AccountMonitorRealRequestTimelineRepository.ListRealRequestTimelines` 对每个请求的账号 ID 返回恰好 `bucketCount` 个点。正常账号监控调用的 `bucketCount` 为 `AccountMonitorTimelineLimit = 24`。

- 每个桶的 `start_at` 和 `end_at` 由 `[since, until)` 等分得到，按索引升序排列。
- 没有真实请求的桶返回 `request_count=0`、`success_count=0`、`failure_count=0`、`ttft_p95_ms=null`。
- 有聚合结果的桶覆盖对应索引，不压缩或改变其他桶的位置。
- 完全没有真实请求的账号仍返回 24 个空桶。
- 现有 SQL 对 `usage_logs` 与 `ops_error_logs` 的去重、成功判断和 P95 聚合保持不变。

## Frontend Behavior

`AccountMonitorView` 继续通过现有 `rankingScope` 向卡片传递当前范围：

- `rankingScope='global'`：利润率值固定为“按分组查看”，不读取缺失或偶然存在的单个 `group_profitability` 作为全站结果。
- `rankingScope='group'`：`confirmed` 和 `estimated` 显示百分比；`no_real_request` 显示 `--`；其他不完整状态显示“待确认”。

真实性能图继续使用现有 24 柱布局和状态色：空桶灰色、全失败红色、TTFT P95 大于 5000ms 的非全失败桶黄色、其余有请求桶绿色。图表标题显示“真实性能 · 真实请求”，动作按钮显示“刷新探测状态”，其 title/aria-label 明确这是主动探测且不生成真实请求样本。

## UI States

- Loading/error/empty：沿用页面现有状态，不增加新容器。
- No timeline samples：24 根等宽灰色短柱，每根可聚焦，提示“暂无真实请求”。
- Sparse samples：有数据柱位于其真实时间索引，其他位置为灰色空桶。
- Narrow viewport：卡片沿用现有单列响应式布局；24 柱保持稳定宽度，不引起整页横向滚动。
- Accessibility：图表保留 `role='img'` 和汇总 aria-label；每根柱保留键盘焦点与 title；按钮使用准确的可访问名称，不只依赖颜色表达状态。

## Test Strategy

- Repository sqlmock：两个账号、24 桶；账号 7 只有第 4 和第 23 桶有数据，账号 8 无数据；断言两者都返回 24 个连续桶且数据写入正确索引。
- Card Vitest：稀疏/空数据合同渲染 24 柱；全站范围显示“按分组查看”；具体分组显示利润率；探测按钮与图表标题使用新语义。
- View Vitest：断言全站和分组切换时向卡片传入正确 `rankingScope`。
- 验证：定向 Go/Vitest、Vue typecheck、production build、gofmt、`git diff --check`，以及桌面和 390px 浏览器截图检查。

## Acceptance Criteria

- [ ] 24h、7d、30d 任一窗口的每个账号都返回并渲染 24 个时间桶。
- [ ] 两个非空桶不再各自扩张为半幅色块，空桶的位置可见。
- [ ] 全站账号卡利润率显示“按分组查看”。
- [ ] 具体分组账号卡仍正确显示已确认或估算的利润率。
- [ ] 图表与主动探测动作的文案不会暗示探测会生成真实请求数据。
- [ ] 不改变利润、调度、计费、探测逻辑、迁移、配置或生产业务数据。

## Risks And Rollback

- 返回 24 个桶会小幅增加账号监控响应体，但上限固定，且现有前端已经按 24 柱设计。
- 时间桶使用浮点秒宽；实现必须复用同一宽度计算生成边界和映射索引，避免累计漂移。
- 回滚仅需反转 T101 代码提交并走既有发布链；没有数据库或配置回滚步骤。
