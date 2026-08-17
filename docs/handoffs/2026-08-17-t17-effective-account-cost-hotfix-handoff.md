# T17 用量详情有效账号成本口径热修交接

## Start Here

- 任务包：T17
- 状态：`DONE`
- 分支：`codex/t17-effective-account-cost-hotfix`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t17-effective-account-cost-hotfix`
- 候选实现 tip：`e6fde59ba`
- 生产源：`main@892db8cefb37bcab14b0aded8082811ac3935f48`
- 生产活动槽：`blue`
- 恢复 bundle：`/Users/gongtengxinwen/Documents/sub2api-archives/t17-effective-account-cost-hotfix-9ffbdbc2.bundle`（SHA-256 `c8aa71b345f74486e97cafdd2a6078afe22b8fa6da62c7c35386646c767c3879`）

## Delivered

- 新增并测试 `effectiveAccountCost` 纯函数，严格遵循 Sub 原生 COALESCE 口径并保留零值语义。
- 管理员用量详情的“上游扣费”与“利润”改为使用有效账号成本计算。
- strict upstream evidence 继续显示状态、原因、来源和 PascalCase/snake_case 兼容，但不再决定主金额或利润。
- evidence unavailable、confirmed 与请求失败三类场景均有直接相关回归测试。
- 后端管理员 DTO/API 未改动；focused contract tests、前端 typecheck/build 均通过。

## Root Integration / Production

1. 候选刷新到最新根主线后以 `main@892db8cef` 推送并绑定 0600 测试证据。
2. 普通预加载蓝绿链完成 `blue` 提升，迁移哈希未变，等效 `downtime_required=false`；未使用维护授权。
3. 宿主记录 `/var/lib/sub2api/release-records/20260817T102828Z-production-2034943.json` 为 `succeeded/promoted`、`rolled_back=false`。
4. API/worker 使用同一不可变镜像并保持 healthy/restart 0，公网三项健康端点均 200。
5. 登录态页面确认 evidence unavailable 时详情仍显示有效账号成本和利润，且与列表成本数学值一致。
6. 控制器在宿主成功 final record 后因 SSH 关闭产生本地假阴性；已只读确认 release-state/final record/容器/标签一致且无 partial，不需要重试发布。
7. 候选功能 worktree/分支和临时 release worktree 已在归档后清理；既有用户可见 `codex/t17-usage-detail-effective-cost` worktree 保持原样。

## Constraints / Rollback

- 不使用 GitHub Actions。
- 不修改 T15 受保护 worktree、T16 冻结 worktree、历史 worktree 或根目录未跟踪文件。
- 不做数据库迁移、历史回填、生产数据修改、账务重算或 evidence 表清理。
- 功能回滚使用上一版已验证的不可变蓝绿镜像；本候选没有迁移回滚需求。

## Verification

详见 `docs/superpowers/reports/2026-08-17-t17-effective-account-cost-hotfix-verification.md`。

候选直接相关验证：前端 3 文件 / 38 测试通过，typecheck/build 通过；后端管理员/DTO focused tests、server compile-only 和 `go build ./cmd/server` 通过；迁移与 GitHub Actions 改动均为零。
