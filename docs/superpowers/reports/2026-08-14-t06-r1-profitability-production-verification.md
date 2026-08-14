# T06-R1 利润页生产验收

日期：2026-08-14

## 发布身份

- 根 `main`：`459a020fd99b605c3da50ead2cbc10121e57cbcd`
- 源 tree：`48c97a63e5fffe9a7991bc7ce65eceb98a4d6b35`
- 候选分支：`codex/t06-r1-profitability-dark-theme-localization`
- 候选提交：`d50c47d744b405f54b8bf420de68a59ed70b9e0c`
- 迁移哈希：`6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`
- 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-14-main-459a020fd-t06-r1-profitability-theme-i18n-v1.json`

## 生产记录

- 宿主记录：`/var/lib/sub2api/release-records/20260814T181009Z-production-3436954.json`
- `result=succeeded`
- `state=promoted`
- `rolled_back=false`
- `downtime_required=false`
- 活动槽：`green`
- source commit/tree/tested tree 与本地 `main` 和证据一致

## 线上证据

- `/healthz`、`/readyz`、`/health`：均 HTTP 200。
- 管理员登录态利润页：`https://api.xingqiaolab.top/admin/operations/account-profitability`
- 深色主题下 `.card` 背景/边框/文字可读，表格 `.table-container`/`.table` 可读。
- 范围显示：`今日`、`24 小时`、`7 天`、`31 天`。
- 表头显示：`账号`、`收入`、`支出`、`盈利`、`利润率`、`异常`、`今日覆盖`。
- 初载、今日、24h、31d 和刷新请求均为原生 `/api/v1/admin/operations/account-financial`。
- 捕获的 `/api/v1/xingqiao/**` 请求数：0。
- 页面未显示“控制面”“完整性”或 `unknown` 文案。

## 回滚

无数据库或配置变更；如需回滚，按既有蓝绿链切回上一活动槽/上一绑定镜像，不做迁移回滚。
