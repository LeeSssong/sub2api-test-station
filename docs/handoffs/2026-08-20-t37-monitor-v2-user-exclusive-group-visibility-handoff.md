# T37 Monitor V2 当前用户专属分组可见性交接

## 状态

`READY_FOR_ROOT_REVIEW`

本任务没有合并 `main`、推送、部署、访问或修改生产。

## Git 身份

- 基线：`b5ad0cdd624e3590bd0d19000c0f78cde200ef68`
- 分支：`codex/t37-monitor-v2-user-exclusive-groups`
- 实现提交：`6784733402ba7852ec154324d40129f7e4ba6a0a`
- 实现 tree：`5c480e364e462c6983042b16f6c3481100a87b34`
- 规格提交：`99446215b`
- 计划提交：`587195999`
- 最终含交接提交与 tree：以本任务最终汇报中的 `git rev-parse HEAD` / `HEAD^{tree}` 为准

## 交付行为

- `/api/v1/monitor-v2` handler 使用 `AuthSubject.UserID`，不再按 user/admin role 选择可见范围。
- service 复用 `APIKeyService.GetAvailableGroups(userID)` 的窄接口。
- active 公开分组全部可见，包括没有出现在原生 available 结果中的公开订阅组。
- active 专属分组仅保留当前用户原生可访问交集。
- 未获权专属 ID 在 T34 `ProjectMonitorV2Groups` 调用前被裁剪。
- 授权读取失败直接返回错误，native reader 不执行。
- Monitor V2 v7、`Cache-Control: no-store`、稳定顺序、T34 `account_monitor_results` 投影和 24/28/30 桶保持。

## 变更文件

- `docs/superpowers/specs/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-design.md`
- `docs/superpowers/plans/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility.md`
- `docs/superpowers/reports/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-verification.md`
- `docs/handoffs/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-handoff.md`
- `upstream/sub2api/backend/internal/service/monitor_v2.go`
- `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`
- `upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go`
- `upstream/sub2api/backend/internal/service/wire.go`
- `upstream/sub2api/backend/cmd/server/wire_gen.go`

## 直接验证

新鲜通过：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestMonitorV2' -count=1 -v
go test ./internal/handler -run '^TestMonitorV2' -count=1 -v
go test ./internal/server/routes -run '^TestMonitorV2' -count=1 -v
go test ./cmd/server -run '^$' -count=1
go build ./cmd/server
```

范围门禁通过：`git diff --check`；无 migrations、frontend、`.github/workflows`、项目总账或任务队列差异；旧 `MonitorV2Scope`/role 分支已清除。

详细 RED/GREEN 与输出见 `docs/superpowers/reports/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-verification.md`。

## 迁移、配置与停机

- 迁移变化：无
- 配置变化：无
- 依赖变化：无
- 生产数据写入：无
- `downtime_required`：候选预期 `false`；根发布预检为最终事实

## 未验证项

- 未执行生产或登录态浏览器验证。
- 未在根 `main` 合并树上复跑测试、预检或发布。
- 未运行全仓/前端验证；本任务无前端差异，按直接相关最小门禁收口。

## 根总控建议验收

1. 确认候选相对目标 `main` 无漂移或先在候选刷新。
2. 合并后复跑 service/handler/routes Monitor V2 与 server build。
3. 预检确认 `downtime_required=false` 后走既有蓝绿链。
4. GET-only 登录态验证：
   - 无目标专属授权的管理员响应不含该分组；
   - 有原生授权用户响应包含该分组；
   - active 公开分组仍存在；
   - v7、24/28/30 桶与健康端点正常。

## 回滚与风险

- 回滚：revert T37 提交；生产侧保留上一蓝绿槽/镜像。
- 剩余风险：原生授权服务短暂失败会使 Monitor V2 请求 fail closed，而不是退回公开快照；这是批准规格的权限正确性选择。
