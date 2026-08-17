# T16 经营页真实结果与视觉层级重设计生产收口

## 发布身份

- 发布源：已推送根 `main@ad49f9004418d779dfb0d7967d3fc3681486fbbe`
- source/tested tree：`9ecfcfb5747bbf312900f615a20b16cf481dc370`
- 迁移哈希：`bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951`（无变化）
- 0600 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-ad49f9004-t16-profitability-v1.json`
- 宿主记录：`/var/lib/sub2api/release-records/20260817T184654Z-production-2406458.json`
- 结果：`succeeded/promoted`，`rolled_back=false`，`downtime_required=false`
- 活动槽：`blue`，活动上游 `sub2api-blue:8080`
- 不可变镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-ad49f9004418d779dfb0d7967d3fc3681486fbbe-8e203afb7e7314e1a4f3c508cfdfd12c444678f24210cf65525424c7e0a6f256`

## 线上验收

- 公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。
- 管理员登录态 `/admin/operations/account-profitability` 标题与页面正常加载。
- 页面摘要显示“业务营收”“总消耗”“净利润”“对外毛利率”“内部运营消耗”，并保留“已包含在总消耗中”和当前角色历史风险说明。
- 账号明细显示“账号 / 内部运营消耗 / 业务消耗 / 业务营收 / 总消耗 / 净利润”五列，排序入口可见，负利润以警示语义展示。
- 页面通过原生 `/api/v1/admin/operations/account-financial?range=today&timezone=Asia%2FShanghai` 返回 HTTP 200，未调用外部控制面。
- API/worker 使用同一不可变镜像；PostgreSQL、Redis、Caddy 容器身份保持不变。

## 未纳入本任务的回归噪声

- 刷新后额外执行的全量 `go test ./internal/service -count=1` 有两个与 T16 无关的 scheduler/sticky 断言失败；失败测试名、期望/实际值和日志路径已记录在 T16 handoff。T16 直接 repository/service/handler、compile-only/build、前端 19/19、typecheck/build 全部通过，发布链和线上功能验收均通过。

## 回滚与边界

- 无数据库迁移、历史回填、生产数据写入、账务重算或配置变化。
- 回滚依据为上一活动 green 槽、上一不可变镜像和 release-state/release-record；无数据回滚步骤。
