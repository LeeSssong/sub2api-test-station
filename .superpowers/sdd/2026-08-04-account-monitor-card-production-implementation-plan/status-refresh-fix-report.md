# 账号监控状态与单卡刷新修复报告

- 日期：2026-08-04
- 基线：`e08f542cbbed4be94e08e75256b670c63d251726`
- 分支：`codex/account-monitor-completion`
- 状态：本地实现完成，等待独立审查；未推送、未部署、未做本轮生产复验。

## 根因与修复

1. 真实请求窗口已经达到 3 次时，评分证据优先使用 `real_requests`，但服务状态仍读取最新 probe 的 `LatestStatus`；一次失败 probe 会把真实请求成功的账号投影为 unavailable，并取消排名。
   - 新增 `accountMonitorWindowServiceState`，统一供全站与分组窗口投影使用。
   - `real_requests` 成功样本大于 0 为 available，成功样本为 0 为 unavailable；latest probe 状态、时间线和错误事实不改写。
   - 低于 3 次真实请求继续使用当前 monitor-probe 门槛；stale/pause 语义保持不变。
2. 窗口投影把 `checked_at` 覆盖成真实请求 `evidence.observed_at`，单卡刷新后页脚看起来不变。
   - 新增 `accountMonitorWindowCheckedAt`：优先 latest probe `CheckedAt`，没有 latest 才回退 evidence observed time；分组 `evidence.observed_at` 保持真实请求观测时间。
3. 单卡刷新沿用 Axios 默认 30 秒，并忽略关键重载 `load()` 的布尔结果。
   - `runOne` 使用 `240000ms` 专用 timeout。
   - POST 成功后必须重载当前范围；重载成功才显示“账号探测与监控卡片已刷新”，重载失败保留旧快照并显示“探测已完成，但最新卡片加载失败，请重试”。批量刷新同步避免静默成功。

## TDD RED 证据

- 后端：
  ```text
  go test ./internal/service -run 'TestAccountMonitorWindowStateUsesRealRequestsAndKeepsLatestProbeTimeSeparate' -count=1
  FAIL ... global real-success account 118 ... ServiceState:"unavailable" ... Eligible:false ...
  ```
- 前端：
  ```text
  pnpm exec vitest run src/api/__tests__/admin.accountMonitor.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
  3 failed, 15 passed
  - API 单测观察到 runOne POST 未传 timeout。
  - 单卡重载失败没有明确“探测已完成但最新卡片加载失败”提示。
  - 单卡 POST+重载成功没有明确成功提示。
  ```

## GREEN 与完整验证

以下命令均退出 0：

```text
cd upstream/sub2api/backend
go test ./internal/service -count=1                                  # final fresh run 99.200s, PASS
go test ./internal/repository ./internal/handler/admin ./internal/server/routes -count=1  # PASS

cd ../frontend
pnpm exec vitest run \
  src/api/__tests__/admin.accountMonitor.spec.ts \
  src/views/admin/__tests__/AccountMonitorView.spec.ts \
  src/components/admin/account-monitor/AccountMonitorCard.spec.ts   # 3 files, 34 tests PASS
pnpm typecheck                                                       # PASS
pnpm build                                                           # PASS

cd ../../..
git diff --check                                                   # PASS
```

前端构建保留项目既有 Browserslist、动态导入和大 chunk 警告；无新增错误，构建退出 0。

## 变更文件

- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- `upstream/sub2api/frontend/src/api/__tests__/admin.accountMonitor.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- `docs/project/account-monitor-v3-acceptance-contract.md`
- `docs/project/project-progress.md`
- `.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/progress.md`

## 关注点

- 当前只完成最小状态/刷新修复，未改评分公式、成本、并发、分组排序、数据库 schema 或营收/账务边界。
- 总账已恢复为“进行中”，历史提交 `82095b80770236eac24adb0bdb1b80cd639675cb` 的推送/部署/线上验证事实保留。
- 本提交必须经过独立审查；随后才可推送、部署，并针对生产账号 #118/#119 验证真实成功请求优先、probe 红柱保留、`latest.checked_at` 与卡片检查时间更新及成功/失败提示。

## 提交

- 提交 SHA：本报告随本次提交提交，最终 SHA 见 `git log`。
