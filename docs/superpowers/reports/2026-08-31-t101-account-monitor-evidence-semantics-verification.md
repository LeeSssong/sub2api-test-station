# T101 账号监控证据语义修复验证报告

日期：2026-08-31

## Scope

- 候选：`codex/t101-account-monitor-evidence-semantics`
- 初始基线：`main@3c5b9710a807904b8449708c71e3931b7f838490`
- 刷新基线：`main@879787096a7bc4b3ff4ab4820d4a5f3c3a63a29a`
- 实现提交：`ff0ff3670`、`00af1e05a`；刷新合并提交：`2e4884087`
- 范围仅为账号监控真实请求时间桶投影、全站/分组利润率展示语义和主动探测文案；不改变利润公式、调度、计费、探测执行或事实源。

## TDD Evidence

- Repository RED：新用例请求账号 7/8 各 24 桶，旧实现实际只返回 `2/0` 个非空桶。
- Repository GREEN：预建每个账号的 24 个时间桶，再按 SQL `bucket_index` 覆盖聚合结果；无请求账号仍保留 24 个空桶。
- Card RED：旧卡片在全站范围仍读取单个 `group_profitability`，图表和按钮仍使用未区分真实请求/主动探测的旧文案。
- Card GREEN：全站显示“按分组查看”，具体分组显示 `61.8%`，空时间线渲染 24 柱，按钮可访问名称明确“不生成真实请求样本”。
- 计划原拟新增 View stub 断言，但现有 View 已正确透传 `rankingScope`，相关 48 项测试直接通过；因此没有为了形式重复修改 View 或测试。

## Fresh Verification

刷新到最新 `main` 后执行：

```bash
go version
go test -vet=off -p 1 -run '^(TestAccountMonitorRepositoryRealRequestTimelineKeepsEmptyBuckets|TestAccountMonitorRepositoryProjectMonitorV4)' -count=1 ./internal/repository
go build -p 1 -o /tmp/sub2api-t101-server ./cmd/server
```

结果：Go `1.27.0 darwin/arm64`；repository 定向测试通过；完整 server 编译通过。

```bash
./node_modules/.bin/vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
./node_modules/.bin/vue-tsc --noEmit
./node_modules/.bin/vite build
```

结果：2 files、56/56 tests 通过；类型检查通过；Vite production build 通过，1098 modules transformed。构建只输出既有动态/静态 import 与 chunk 提示，没有构建错误。

## Browser Evidence

- 桌面全站：`output/playwright/t101/t101-desktop-global.png`，利润率显示“按分组查看”。
- 桌面具体分组：`output/playwright/t101/t101-desktop-group.png`，利润率显示 `61.8%`。
- 移动具体分组：`output/playwright/t101/t101-mobile-group.png`，390x844 下无文字、按钮或图表重叠。
- DOM 检查：`[data-test="real-request-bar"]` 为 24 个；移动端 24 根柱宽度均为 9px；两个真实数据桶没有扩张；`document/body scrollWidth = clientWidth = 390`。
- 每根柱保留 `tabindex="0"` 和完整 `title`；刷新按钮 title/aria-label 为“刷新主动探测状态，不生成真实请求样本”。

## Scope Checks

- `git diff --check`：通过。
- 相对刷新基线没有 `.github/workflows`、数据库 migration、`go.mod`、`go.sum` 或 `pnpm-lock.yaml` 变化。
- 无配置、账号/分组、凭据或生产业务数据写入。
- 临时预览入口已删除；Playwright/Vite 会话已关闭。截图只作为本地视觉证据，不包含登录态或凭据。

## Release Boundary

- 尚未在验收站或主站部署，未执行宿主发布预检。
- `downtime_required=unverified until root preflight`；按变更形状预期无需迁移停机，但只有受控发布链结果有效。
- T99 当前仍占用唯一整合/发布车道并等待独立主站授权；T101 在该车道释放前不能合并、推送或部署。
- 当前“推送部署/继续”不属于项目规定的两种主站明确授权语句，不能据此发布主站。

## Residual Risk

- 固定 24 桶会小幅增加账号监控响应体，但上限固定。
- 尚未使用验收站真实管理员登录态检查实际账号数据；该项应在根合并、发布验收站后完成。
