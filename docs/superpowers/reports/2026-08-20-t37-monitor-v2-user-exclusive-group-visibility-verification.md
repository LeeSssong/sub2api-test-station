# T37 Monitor V2 当前用户专属分组可见性验证报告

## 结论

- 任务：T37
- 基线：`main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`
- 实现提交：`6784733402ba7852ec154324d40129f7e4ba6a0a`
- 实现 tree：`5c480e364e462c6983042b16f6c3481100a87b34`
- 验证日期：2026-08-20
- 候选状态：`READY_FOR_ROOT_REVIEW`
- 迁移：无
- 配置：无
- 前端：无
- 生产数据写入：无
- `downtime_required`：候选预期 `false`；根发布预检为最终事实

## 根因与实现

基线 handler 把管理员角色转换为 `MonitorV2ScopeAdmin`，service 因而保留全部 active 专属分组，并把这些 ID 送入 T34 的 `ProjectMonitorV2Groups`。修复后：

1. handler 只读取 `AuthSubject.UserID`，user/admin role 均传同一用户身份；
2. service 通过窄接口 `MonitorV2AvailableGroupReader.GetAvailableGroups(ctx, userID)` 复用 `APIKeyService` 原生授权；
3. 所有 active 非专属分组保持可见；
4. active 专属分组只保留原生可访问 ID 交集；
5. 过滤后的 ID 才进入 `ProjectMonitorV2Groups`；
6. 授权读取失败在原生投影前返回错误。

Monitor V2 合同版本仍为 `7`，T34 `account_monitor_results` 原生投影、稳定分组顺序、`no-store` 与 24/28/30 固定桶未变。

## TDD RED 证据

### RED 1：可见集合函数不存在

命令：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestMonitorV2VisibleGroupsKeepsPublicAndAuthorizedExclusiveInRepositoryOrder$' -count=1 -v
```

失败：

```text
internal/service/monitor_v2_test.go:41:18: undefined: monitorV2VisibleGroups
FAIL github.com/Wei-Shaw/sub2api/internal/service [build failed]
```

该失败证明测试先于纯过滤实现存在。

### RED 2：service 尚未接收用户与授权读取器

命令：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestMonitorV2SnapshotUsesCurrentUserAvailableGroupsBeforeNativeProjection|TestMonitorV2SnapshotStopsBeforeNativeProjectionWhenAuthorizationFails' -count=1 -v
```

失败包含：

```text
too many arguments in call to NewMonitorV2Service
cannot use 42 as MonitorV2Window value in argument to svc.Snapshot
```

该失败证明用户绑定的 constructor/Snapshot 合同尚未实现。

## GREEN 与最终新鲜验证

2026-08-20 在实现提交上新鲜运行：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestMonitorV2' -count=1 -v
go test ./internal/handler -run '^TestMonitorV2' -count=1 -v
go test ./internal/server/routes -run '^TestMonitorV2' -count=1 -v
go test ./cmd/server -run '^$' -count=1
go build ./cmd/server
```

结果：

- service Monitor V2 全部通过，包括：
  - public active 分组即使不在 `GetAvailableGroups` 中仍保留；
  - 仅获权 active 专属分组进入响应；
  - 未获权专属 ID 不进入 native reader；
  - 授权错误时 native reader 调用次数为 0；
  - v7 原生投影、微秒桶匹配、缺失结果补桶、24/28/30 桶、稳定顺序、native 错误传播继续通过。
- handler Monitor V2 全部通过，包括 user/admin 角色均只传 `UserID=42`、缺失主体返回 401、v7 白名单响应和 `no-store`。
- routes Monitor V2 两项通过：heavy query rate limit 与认证用户边界。
- `go test ./cmd/server -run '^$' -count=1` 通过。
- `go build ./cmd/server` 退出码 0。

完整最终测试输出临时记录于 `/tmp/t37-final-tests.log`，不作为仓库事实源。

## 范围与静态门禁

命令：

```bash
git diff --check b5ad0cdd624e3590bd0d19000c0f78cde200ef68...HEAD
git diff --name-only b5ad0cdd624e3590bd0d19000c0f78cde200ef68...HEAD -- \
  upstream/sub2api/backend/migrations upstream/sub2api/frontend .github/workflows
git diff --name-only b5ad0cdd624e3590bd0d19000c0f78cde200ef68...HEAD -- \
  docs/project/project-progress.md docs/project/native-sub-task-package-queue.md
rg -n 'MonitorV2Scope|GetUserRoleFromContext' \
  upstream/sub2api/backend/internal/handler/monitor_v2_handler.go \
  upstream/sub2api/backend/internal/service/monitor_v2.go
```

结果：

- `git diff --check` 通过。
- migration、frontend、GitHub Actions 差异为空。
- 项目总账与任务队列差异为空。
- Monitor V2 handler/service 中旧 role scope 符号为空。
- `MonitorV2ContractVersion = "7"` 保持；窗口仍为 24、28、30 桶。

## 未验证项

- 候选任务未访问生产，未执行登录态浏览器或线上 API 验证。
- 根总控合并最新 `main` 后仍需复跑直接门禁、发布预检、无停机蓝绿发布与 GET-only 线上专项验证。
- 未运行全仓测试或前端构建；本任务无前端变化，且项目最小验证政策要求仅运行直接相关门禁。

## 回滚

代码回滚可 revert T37 实现与文档提交。生产发布后由根总控保留上一蓝绿槽/不可变镜像作为即时回滚依据；无迁移、配置或数据写入需要回退。
