# 管理员监控页会话失效修复生产报告

日期：2026-08-03

## 结论

修复已推送并部署到生产蓝槽。`/admin/accounts/monitor` 首屏对账接口的 `401` 不再清除主站管理员登录态，也不会把用户跳转到 `/login`；普通主站 API 的 `401` 刷新/登出行为保持不变。

浏览器扩展在本次验收中能够列出已登录标签，但连续两次在接管账号页并导航到监控页时超时，因此“已登录浏览器视觉上停留在监控页”的最后一步尚未完成，不能把它写成已完成的浏览器验收。

## 根因

监控页首屏会调用 `/relay-ops/api/reconciliation/*`。该服务返回 `401` 后，被共享 `apiClient` 误判为 Sub2API 主站 JWT 失效，触发令牌刷新、清除本地登录态并跳转登录页。

## 修复范围

- 对账请求统一设置 `skipSessionRecovery: true`。
- 带该标记的 `401` 只返回调用方错误，不刷新令牌、不清除主站会话、不跳转登录页。
- 普通主站请求继续沿用原有会话恢复逻辑。
- 增加有/无 `refresh_token` 的回归覆盖，并验证对账请求携带隔离标记。

## 代码与发布证据

- 源码提交：`d4fb5e4a4b058f292a9df9f2c44d8bf01abfbe5f`
- 源码树：`80034d9d112cd3c2a41fc2d08aed470d2cf4b0d4`
- 生产镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-d4fb5e4a4b058f292a9df9f2c44d8bf01abfbe5f-170fac9f6f61acf7f53ac4074592338477c3fe705a571dddb050dab5cc05071e`
- 镜像摘要：`sha256:170fac9f6f61acf7f53ac4074592338477c3fe705a571dddb050dab5cc05071e`
- 发布记录：`20260803T152958Z-production-326982.json`
- 发布结果：`succeeded` / `promoted` / 未回滚；活动槽 `blue`
- 发布前状态校准备份：`/var/lib/sub2api/release-records/reconciliation-20260803T1785770963261Z.release-state.json`

## 验证

- 前端文件：217/217
- 前端用例：1516/1516
- 定向会话恢复回归、`pnpm typecheck`、`pnpm build`：通过
- 生产 API：`/api/v1/auth/me`、`/api/v1/admin/account-monitors`、`/api/v1/admin/ops/dashboard/snapshot-v2` 持续返回 200
- 生产容器：blue API、worker、PostgreSQL、Redis、Caddy、relay-ops 均正常；未重建 PostgreSQL/Redis/Caddy
- relay-ops 最近日志仅见已有的账单监控 schema mismatch，不再出现主站会话被清除的证据

## 剩余验收

浏览器扩展的标签枚举正常，但接管已登录账号页并导航到 `/admin/accounts/monitor` 连续超时。后续只需在浏览器扩展恢复可控后重新打开该地址，确认 URL 不跳转到 `/login` 且页面能完成首屏加载；不需要再次修改本次代码。
