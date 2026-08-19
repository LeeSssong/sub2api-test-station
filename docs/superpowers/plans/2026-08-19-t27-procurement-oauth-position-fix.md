# T27 双视图经营页实施计划

## 文件范围

- `upstream/sub2api/backend/internal/handler/admin/dashboard_handler.go`: 为 self-purchased endpoint 增加兼容 range 解析。
- `upstream/sub2api/backend/internal/handler/admin/dashboard_handler_account_profitability_test.go`: 固定时钟验证 today/24h/7d/31d、日期优先和非法值。
- `upstream/sub2api/backend/internal/service/account_procurement_profitability.go`: 保留 NULL 修复与 OAuth/结算过滤。
- `upstream/sub2api/backend/internal/service/account_profitability_test.go`: 保留并回归 NULL/OAuth/结算测试。
- `upstream/sub2api/frontend/src/api/admin/selfPurchasedProfitability.ts`: 接受并传递 `FinancialRange`。
- `upstream/sub2api/frontend/src/api/admin/__tests__/selfPurchasedProfitability.spec.ts`: 新增请求参数合同。
- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`: 一级 segmented view、独立状态/加载、USD/CNY 条件渲染和文案。
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`: 双视图、错误隔离、范围、刷新、摘要和移动端回归。
- `upstream/sub2api/frontend/src/i18n/locales/{zh,en}/admin/index.ts`: 将既有 USD `netProfit` 标签改为经营利润/Operating profit（仅既有键值）。

## TDD 步骤

1. 在 handler 测试先写 range RED：注入固定北京时间，断言 24h 精确滚动，today/7d/31d 自然日开始且结束为当前时刻；断言显式日期优先、未知 range 400。
2. 新增 self-purchased API RED，断言 `get({range:'7d'})` 原样传给 endpoint。
3. 重写页面聚焦测试为 RED：首次只请求 USD；切换 CNY 后请求同一 range；范围和刷新只加载当前视图；两视图 DOM/错误隔离；CNY 七项摘要与完整表；USD 排除说明和经营利润文案；390px 自购容器局部滚动。
4. 实现 handler 兼容解析，使用可注入 `now` 以获得确定测试；不改变显式日期和无参数旧行为。
5. 扩展 API 类型，重构页面为 `activeView` + USD/CNY 独立 load 状态和按需加载；保留现有 stale-response 保护、结算、排序和数据格式化。
6. 更新 zh/en 既有净利润标签，不新增平行翻译体系。
7. 运行直接相关 Go service/handler、前端 self-purchased API、AccountProfitabilityView/API 与必要 AccountMonitor 测试；运行 typecheck、production build、gofmt、git diff --check。
8. 自审变更只覆盖 T27，提交新候选并保持 worktree clean；报告 baseline、SHA/tree、文件、测试、未验证项、无迁移/配置、回滚与风险。

## 完成门槛

已批准 A 方案、时间窗兼容和既有 T27 修复全部通过直接测试；不修改根账本/队列，不合并、推送、部署或清理。
