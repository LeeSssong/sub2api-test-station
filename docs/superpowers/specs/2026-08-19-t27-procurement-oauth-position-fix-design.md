# T27 自购账号口径、保存失败与双视图经营页

## 现状证据

- `UpdateProcurementConfig` 曾把 `cost_pending` 的 NULL 成本/额度扫描到 `float64`，重新录入 7.7 CNY / 60 USD 返回 internal error；候选已用严格可空扫描修复并保留原事务、幂等、版本、actor 审计和 accounts 投影。
- `GetSelfPurchasedReport` 曾仅依赖采购台账/legacy 投影，未限制原生账号类型；候选已在台账/fallback/scoped/settlement 路径增加 `accounts.type='oauth'`，历史非 OAuth 数据保留但不展示或结算。
- 当前页面把 CNY 面板嵌在 USD 摘要与分组之间，且挂载时并发请求 USD 与 CNY；`selfPurchased.get({})` 使 CNY 后端默认本月，而 USD 使用 today/24h/7d/31d，范围不一致。

## 目标与非目标

目标：页头保留统一范围控件；新增默认 USD、次级 CNY 的一级 segmented view；每个视图只渲染和刷新自己的数据/错误；CNY 使用与原生 `AccountFinancialService` 相同的北京时间范围语义；USD 报表公式和接口保持不变；CNY 继续只包含“OAuth 且已有采购台账或 legacy 采购投影”的账号。

非目标：不新增汇率或混币汇总，不改变用户扣费、渠道 USD 公式、调度、采购成本公式、版本/结算语义，不新增迁移、历史回填、生产数据写入、平行页面或 GitHub Actions。

## 方案比较与选择

1. **A：同页一级双视图（已批准）**。共用范围和页头，按币种隔离信息架构、请求和错误；改动集中在既有 handler/API/page，认知和实现边界最清晰。
2. USD 页面保留内嵌 CNY 面板。改动更少，但币种层级混杂、分组归属误导且难以隔离加载错误，拒绝。
3. 新增独立路由。隔离最强，但新增导航、权限和路由维护面，超出本任务，拒绝。

## 信息架构与交互

页头范围按钮下新增 segmented control：`经营结果 · USD`（默认）和 `自购专题 · CNY`。USD 视图包含现有五项全站摘要、角色说明、业务分组 Tab、选中分组摘要、排序和账号表，并在摘要附近显示：`USD 经营结果未含自购账号 CNY 采购成本；自购实际采购利润请查看自购专题。` 所有 `净利润` 展示标签收敛为 `经营利润`，后端字段仍为 `net_profit`。

CNY 视图只包含自购摘要和现有长表，不渲染 USD 分组、分组摘要、排序或 USD 账号表。摘要至少展示账号数、人民币营收、已确认采购成本、待摊成本、采购损失、人民币净利润、利润率；表保留采购成本、预计额度、标准消耗、利用率、确认成本、待摊、损失、营收、净利润、利润率和状态。

全局刷新按钮只刷新当前一级视图。切换视图时，仅在目标视图尚未加载时加载，已加载数据保留；范围变化刷新当前视图并携带同一 `activeRange`。定时刷新也只刷新当前视图。USD/CNY 的 loading、refreshing 和 error 独立，任一失败不清除或遮蔽另一视图。

## 时间范围与 API 契约

`GET /admin/operations/self-purchased-profitability` 新增兼容查询参数 `range=today|24h|7d|31d`：

- `24h`：以当前时刻为结束，精确回溯 24 小时。
- `today`：北京时间当日 00:00 至当前时刻。
- `7d`：北京时间今日及前 6 个自然日，至当前时刻。
- `31d`：北京时间今日及前 30 个自然日，至当前时刻。

显式 `start_date/end_date/timezone` 保留原半开区间兼容行为；出现任一显式日期参数时优先日期模式，未传日期而传 `range` 时使用北京时间 range；均未传时保持旧本月默认。未知 range 返回 400。响应字段不变，无显示需求时不新增 `range` 字段。

前端 `selfPurchasedProfitability.get` 类型接受 `range`，CNY 视图每次范围加载均发送 `{range: activeRange}`。

## 数据与控制流

USD：view/range/refresh -> `accountFinancial.getReport({range})` -> 原生 T16 service/report -> USD UI。

CNY：view/range/refresh -> `selfPurchasedProfitability.get({range})` -> handler 按北京时间解析半开区间 -> `GetSelfPurchasedReport(start,end)` -> OAuth 且具采购事实的 rows/summary -> CNY UI。

保存与结算继续走既有事务。`cost_pending` NULL 版本关闭后直接用新输入创建 active 版本，不读取旧 usage 消耗；有效旧版本继续原剩余成本/额度算法。结算查询继续要求 OAuth。

## 失败、安全与兼容

两视图分别保存最近成功数据和错误；当前视图刷新失败时保留旧数据。切换视图不触发另一视图请求。旧日期查询客户端不受 range 新参数影响；旧无参数请求仍返回本月。账号资格同时要求 OAuth 和采购事实，不把所有 OAuth 静默推断为采购资产。两币种不相加、不换算。

## 验收矩阵

| 场景 | 预期 |
| --- | --- |
| 首次进入 | 只请求并显示 USD；CNY 面板/表不渲染 |
| 切到 CNY | 请求 `{range:'today'}`；只显示 CNY 摘要/表 |
| CNY 范围切到 24h/7d/31d | 分别发送 range；后端返回精确 24h 或北京时间自然日窗口 |
| USD/CNY 任一失败 | 只显示该视图错误，另一视图已加载数据保留 |
| 当前视图刷新 | 只请求当前 API |
| USD 文案 | 显示 CNY 成本排除说明，净利润标签为经营利润 |
| CNY 摘要 | 展示 7 项批准指标，表字段保持完整 |
| 资格 | 仅 OAuth 且存在台账/legacy 投影进入 rows、summary、settlement |
| cost_pending 重新录入 | NULL 扫描成功，旧版本不参与折算，原写事务语义保持 |
| 390px | 页面无整页横溢出，自购长表仅自身容器滚动 |
| 兼容日期参数/无参数 | 日期模式与旧本月默认保持 |

## 测试、发布与回滚

TDD 增加 handler range 边界/优先级/非法值测试、self-purchased API range 传递测试、页面双视图渲染/按需加载/刷新/错误隔离/文案/390px 测试；保留现有 service NULL/OAuth/settlement 测试。只运行直接相关 Go handler/service、self-purchased/API、AccountProfitabilityView/API 测试及必要 typecheck、build、gofmt、diff-check。

无迁移、配置或生产写入，预期 `downtime_required=false`。回滚为回退 T27 候选提交；因无数据变更，无恢复或回填步骤。风险集中于客户端视图状态切换和时间边界，使用固定时钟 handler 单测与请求次数断言控制。

## 自审与批准记录

规格无 TODO/TBD、无混币或资格歧义，范围限于既有原生接口与页面。2026-08-19 用户在根总控明确批准 A 方案；根总控依据代审授权批准本次修订，可直接进入计划与 TDD。
