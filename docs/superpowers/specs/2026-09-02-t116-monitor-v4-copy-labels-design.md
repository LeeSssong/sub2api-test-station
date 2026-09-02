# T116 Monitor V4 成功率文案收敛规格书

## 1. 问题证据与当前行为

Monitor V4 分组卡片已经使用 `success_rate`、`success_count` 和 `request_count` 形成成功率环。当前中文文案为“体验成功率”，卡片底部同时显示“综合成功 N/M 次请求”“真实请求成功 N/M”“探测补足 N 个空桶”。这些标签把内部样本来源和桶补足策略暴露给普通用户，造成同一指标出现多套名称，且与用户提供的原型截图不一致。

现有前端组件为 `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`，翻译位于 `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`，直接相关测试为 `HybridPerformanceGroupCard.spec.ts`。后端和 API 合同已稳定提供 `success_rate`、`success_count`、`request_count`；本任务不改变这些字段或其统计公式。

## 2. 目标与非目标

目标：

1. 圆环百分比下方的标签统一为“成功率”。
2. 圆环/卡片底部恢复简洁事实表达“基于 N 次调用”，其中 N 使用现有 `request_count`。
3. 用户可见区域不再出现“综合成功”“真实请求成功”“探测补足”“空桶”等内部实现术语。
4. 保留现有成功率数值、颜色阈值、P95、缓存命中率、倍率、持续监控状态和响应式布局。

非目标：

- 不改成功率、请求去重、真实请求优先或主动探测兜底逻辑。
- 不改 Monitor V4 API 合同、后端 DTO、数据库、迁移、调度、计费或运行状态。
- 不改英文翻译或其他 Monitor V2 页面文案，除非共享键导致类型/测试必须同步；默认仅调整中文 hybrid 文案。
- 不移除内部字段 `real_request_count`、`probe_fallback_bucket_count`，它们仍可由接口和内部诊断使用，但不在此用户卡片展示。

## 3. 方案比较与选择

**方案 A（推荐）：复用现有字段，前端只保留两条展示文案。** 将 `successRate` 翻译改为“成功率”，新增/恢复 `sampleCount: '基于 {count} 次调用'`，footer 只渲染该条并传入 `request_count`。删除三条内部来源文案和对应 DOM 测试选择器。优点是改动最小、与当前 V4 合同一致、不会造成第二套统计口径；缺点是内部来源不再直接显示，但这正是本次用户要求。

**方案 B：后端新增 display label 字段。** 由 API 返回本地化标签或展示计数。放大接口和多语言耦合，无法改善统计问题，不采用。

**方案 C：继续显示三条明细但改成更短中文。** 仍暴露实现细节，且不能恢复“基于 N 次调用”的单一表达，不采用。

## 4. 端到端数据与控制流

`GET /monitor-v4` 继续返回现有 `MonitorV4Snapshot`。前端合同校验继续验证 `request_count`、`success_count` 及其一致性。`HybridPerformanceGroupCard`：

1. 以 `success_rate` 计算环中心百分比和绿/黄/红色调。
2. 以 `request_count` 渲染 `基于 {request_count} 次调用`。
3. 不读取或渲染 `real_request_count`、`real_success_count`、`probe_fallback_bucket_count`、`probe_fallback_request_count` 的用户文案。

当成功率为空且请求数为 0 时，环中心仍显示 `--`，底部显示“基于 0 次调用”；这保留当前合同对无样本窗口的明确表达，不伪造成功率。

## 5. 接口与字段契约

API 路径、`contract_version=2`、字段名称和校验规则全部保持不变。现有字段继续满足：

- `success_rate = success_count / request_count * 100`（由后端计算，前端只格式化）；
- `request_count` 为非负整数；
- `success_count <= request_count`；
- `success_rate` 为空仅在 `request_count=0` 时允许。

新增的仅是 i18n 展示键（或将现有兼容键改值）：`successRate: '成功率'`、`sampleCount: '基于 {count} 次调用'`。不新增 API 字段。

## 6. 失败、安全与兼容性语义

API 加载失败、合同错误、空分组和刷新行为不变。计数异常仍由现有合同校验拒绝；本任务不在前端自行修正或补足计数。文案不包含账号、密钥、上游错误原文或其他敏感数据。英文 locale 不变，缺少英文 hybrid 键时继续沿用现有 fallback 行为。

## 7. 场景化验收矩阵

| 场景 | 期望 |
| --- | --- |
| 85% / 20 次调用 | 环下标签为“成功率”，底部为“基于 20 次调用” |
| 0% / 有请求 | 显示 `0%` 与对应调用次数，不显示内部来源词 |
| 无请求 / 空成功率 | 显示 `--` 与“基于 0 次调用” |
| 任意卡片 | 页面文本不包含“综合成功”“真实请求成功”“探测补足”“空桶” |
| 1h/24h/7d 切换 | 仍由既有 API 返回计数驱动，不改变请求参数或切换行为 |
| 窄屏卡片 | 文案不溢出、不遮挡指标，现有响应式布局保持 |

## 8. 测试策略

- 更新 `HybridPerformanceGroupCard.spec.ts` 的 i18n mock 和断言，覆盖“成功率”“基于 N 次调用”及内部术语不存在。
- 保留并运行成功率颜色阈值、空缓存、P95 秒格式、静态环中心和深色主题现有测试。
- 运行该组件专项 Vitest、必要的 Monitor V4 前端类型检查及 `git diff --check`。
- 不重复全仓测试，不添加后端测试，因为后端行为和 API 合同不变。

## 9. 发布、线上验证与回滚

本任务无迁移、配置变更、依赖变更或生产数据写入，预期 `downtime_required=false`；最终值仍以根 `main` 发布预检为准。候选只能先合入并推送干净的根 `main`，再按项目既有发布链部署。线上专项验收检查登录态 Monitor V4 卡片在桌面和窄屏显示“成功率”及“基于 N 次调用”，且不出现三种内部术语。若发布或验收失败，恢复上一已验证镜像/槽位，保留候选 worktree 和证据后修复。

## 10. 待决事项与批准记录

无产品待决事项。用户于 2026-09-02 明确确认：使用“成功率”，移除“综合成功”“真实请求成功”“探测补足”，恢复“基于 N 次调用”表达。规格书完成后等待用户书面审阅批准；批准前不调用 writing-plans、不修改运行代码、不派生实现代理、不合并、推送或部署。
