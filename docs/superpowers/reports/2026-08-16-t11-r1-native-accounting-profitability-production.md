# T11-R1 Sub 原生计费聚合经营页生产验收

## 结论

- 状态：`DONE`。
- 最终发布提交：`7a7c9abd70fb108af6a06b93ef67eea3c4b34dab`。
- source/tested tree：`916aaf16ad6ac354b8981755b8072dedab4f6cf7`。
- 迁移集合：`d3fe99bba69b0cf0cca8a7f5ec45499921f3496f58dd74c3a671d90a653589b5`，与发布前生产一致，`downtime_required=false`。
- 0600 资格证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-16-main-7a7c9abd7-t11-r1-native-profitability-v2.json`。
- 宿主发布记录：`/var/lib/sub2api/release-records/20260816T025715Z-production-655862.json`，`result=succeeded`、`state=promoted`、`rolled_back=false`。
- 活动槽：`green`，活动上游：`sub2api-green:8080`。

## 最小发布门禁

- 后端 repository/service/admin-handler/server focused：通过。
- 前端 API/View/browser：19/19；lint、typecheck、production build：通过。
- `git diff --check`、范围/迁移/workflow 检查和发布脚本语法：通过。
- 未重复全仓测试、mutation、压力/重复浏览器、长时间 soak 或无关模块回归，符合 2026-08-16 小步快跑政策。

## 线上即时验收

- 登录态管理员经营页正常加载；今日/24 小时/7 天/31 天、刷新、全站、分组 Tab、分组摘要和账号行均可见。
- 页面字段为请求数、Token、账号成本、用户扣费、利润、利润率；金额为 USD。用户未消费余额作为独立 CNY 卡保留。
- 页面不存在“异常流水”摘要/账号入口、人工覆盖/今日覆盖控件，也不存在外部控制面或完整性 unknown 文案。
- 31 天浏览器可见加载约 `3378 ms`；宿主直连同一管理员 API 为 HTTP `200`、`0.152685 s`，返回 `range=31d`、`currency=USD`、150 个账号、12 个分组和有效摘要，因此无需追加 query plan。
- GPT-Pro 分组显示独立摘要和 68 个账号行，证明全站 → 分组 → 账号三层结构在线生效。
- 精确 CSS 视口 `390×844`：`innerWidth=390`、`innerHeight=844`；document `clientWidth=382`、`scrollWidth=382`，页面级横向溢出为 false；表格容器 `clientWidth=309`、`scrollWidth=1225`，仅表格按合同横向滚动。
- 七张摘要卡均 `overflow=false`、`outside=false`，`adjacentOverlap=false`；金额完整显示，可在卡内自然换行而不截断或互相覆盖。
- 移动端首屏证据：`output/playwright/t11-r1-native-profitability-production-mobile-exact-390x844.png`。包含账号标识的非必要全页临时截图已移动到系统废纸篓，未纳入报告或提交。
- 公网 `/healthz`、`/readyz`、`/health` 均 HTTP `200`。

## 回滚与收口

- 回滚方式：切回发布前 blue 应用/worker 镜像和 Caddy 上游；无数据库回滚或数据清理。
- 可恢复 bundle：`/Users/gongtengxinwen/Documents/sub2api-archives/t11-r1-native-accounting-profitability-6e38e2f9.bundle`，权限 0600，`git bundle verify` 通过，SHA-256 `568dad09a83e4dc043448733ba0d9195ef2721f4d64cf075200a7fc0f6824142`。
- 已删除干净且已合并的候选 worktree、本地候选分支和本次临时 clean release worktree；冻结 worktree、旧发布证据和根目录受保护内容均未修改。
