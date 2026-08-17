# T17 用量详情有效账号成本口径热修交接

## Start Here

- 任务包：T17
- 状态：`READY_FOR_ROOT_REVIEW`
- 分支：`codex/t17-effective-account-cost-hotfix`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t17-effective-account-cost-hotfix`
- 候选实现 tip：`e6fde59ba`
- 候选工作区：干净
- 根总控下一步必须先刷新到最新 `main`；候选当前未包含根 docs-only 提交 `85454d883`。

## Delivered

- 新增并测试 `effectiveAccountCost` 纯函数，严格遵循 Sub 原生 COALESCE 口径并保留零值语义。
- 管理员用量详情的“上游扣费”与“利润”改为使用有效账号成本计算。
- strict upstream evidence 继续显示状态、原因、来源和 PascalCase/snake_case 兼容，但不再决定主金额或利润。
- evidence unavailable、confirmed 与请求失败三类场景均有直接相关回归测试。
- 后端管理员 DTO/API 未改动；focused contract tests、前端 typecheck/build 均通过。

## Root Integration

1. 发布总控将候选刷新到最新根 `main`，如队列/总账状态要求先记录 `REFRESH_REQUIRED`，完成刷新后重新跑 T17 直接相关门禁。
2. 在合并后的根 `main` 上执行 focused tests、typecheck/build、`git diff --check`、迁移与 `.github/workflows` 零差异检查。
3. 由发布总控合并、推送并运行既有本地/宿主发布预检。
4. 若预检返回 `downtime_required=false`，按全局约束直接蓝绿发布和线上验收；若返回 `true`，停在用户授权门禁。
5. 线上验收需确认：详情、使用记录列表、账号利润/经营页对同一流水使用相同成本数学值；evidence unavailable 时详情仍显示有效账号成本与利润；健康端点正常。

## Constraints / Rollback

- 不使用 GitHub Actions。
- 不修改 T15 受保护 worktree、T16 冻结 worktree、历史 worktree 或根目录未跟踪文件。
- 不做数据库迁移、历史回填、生产数据修改、账务重算或 evidence 表清理。
- 功能回滚使用上一版已验证的不可变蓝绿镜像；本候选没有迁移回滚需求。

## Verification

详见 `docs/superpowers/reports/2026-08-17-t17-effective-account-cost-hotfix-verification.md`。

候选直接相关验证：前端 3 文件 / 38 测试通过，typecheck/build 通过；后端管理员/DTO focused tests、server compile-only 和 `go build ./cmd/server` 通过；迁移与 GitHub Actions 改动均为零。
