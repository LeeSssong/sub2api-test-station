# T57 DOCX 经营分析总览交接报告

## 状态

`READY_FOR_ROOT_REVIEW`

基线：`main@e1a0039b0`

本报告只交接 T57 候选，不授权合并、推送、部署或线上验收。

## 交付内容

- 新增原生管理员接口 `GET /api/v1/admin/operations/business-overview`。
- 基于 `usage_logs` 聚合请求、分组和上游成本。
- 通过 T55 只读字段读取付费/赠送额度拆分、钱包快照和额度流水；T55 表未部署时返回 `pending`，不把成本归零。
- 无法通过稳定 reference 关联消费时返回 `pending_split`，收入、毛利和毛利率为 `null`。
- 新增 CNY/Q 经营总览页面，包含经营结果、充值与余额、Chart.js 充值/消耗趋势、站内分组毛利表。
- 复用原生管理员鉴权、operations 路由、布局和图表依赖；旧 USD account-financial/account-profitability 契约未改动。

## 验证证据

- 后端：`go test ./internal/service ./internal/handler/admin -run 'BusinessOverview' -count=1` 通过。
- 后端：`go build ./cmd/server` 通过。
- 前端：`pnpm vitest run src/api/__tests__/admin.businessOverview.spec.ts src/views/admin/__tests__/BusinessOverviewView.spec.ts --reporter=dot`，4/4 通过。
- 前端：`pnpm typecheck` 通过。
- 前端：`pnpm build` 通过，产物包含 `BusinessOverviewView` chunk。
- `git diff --check` 通过。

## 发布边界

- 无 migration、无账本/钱包/支付写入、无生产数据回填。
- `downtime_required` 尚未由根发布预检确认。
- T55 必须先合并、推送、部署并线上验收；T57 之后需要刷新到包含 T55 的最新 `main`，再重新运行直接相关验证。
- 当前未做生产部署和线上专项验收，不能标记 `DONE`。

## 剩余风险

- 当前 T57 候选基线不含真实已部署 T55，因此真实充值、钱包和拆分数据尚未在线验证。
- 历史期初/期末余额依赖当前钱包快照，边界不完整时保持 `pending`；未引入历史回填或估算。
- 预设倍率没有稳定财务输入时显示 `unavailable`。
