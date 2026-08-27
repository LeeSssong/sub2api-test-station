# T76 刷新与复审问题修复报告（2026-08-27）

## 刷新基线与保护

- 目标 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t76-scheduler-quality-ranking-consistency`
- 刷新基线：根 `main@031b58e4c`（来自当前本地根 main；未修改根 main）
- 原 T76 提交链从 `c4052296074290e8bb922bb9d9617e09a64a8168` 保留。
- 刷新前 dirty diff 已经以 stash 保存并恢复；与 `/private/tmp/t76-unfreeze-20260827/v1-worktree.diff` SHA-256 均为 `5e8ffd98f7e6ab69443036f7de104c6f7febcc16ec56544439b9881f52d18dbd`。
- v2 worktree 未读取、未修改、未清理；未执行 push、根 main merge 或任何部署。
- 刷新提交：`91f6ba2ef`（`fix: refresh t76 scheduler projection parity`）。

## Final whole-branch review round 2 的六项 Important

1. Grok quota projection 通过 `OpenAIGatewayService.Project` 使用 `getOrCreateOpenAIAccountScheduler` 返回的同一 live scheduler，因此复用 live `grokFreeQuotaGateCache`；补有 live-cache regression。
2. 只读 model transient eligibility 复用与 live `isBlocked` 相同的过期/半开探测语义：短期过期条目仍保持 blocked，超过 streak TTL 的陈旧条目才清理并放行；补有不获取 lease 的 regression。
3. 只读 runtime eligibility 在受限预算内调用 shared-health store 的 bounded read，使用与 live selection 相同的 context 和 veto 状态；补有跨实例 shared cooldown regression。
4. projection 读取 live runtime `subscriptionPriorityEnabled` 并对订阅池/普通池分区后分别按共享 comparator 排序；补有订阅优先 regression。
5. 策略原因只比较调度与质量排序的交集，资格被排除的账号不会制造顺序差异；quality-gate fallback 的资格原因优先于 strategy 原因，并保留排除账号 regression。
6. 自定义业务优先级 `1/1/1` 显示“体验均衡”；非均衡 latency 优先显示“生成体验优先”，避免误标“利润优先”；补有 label regression。

## 直接验证

已通过：

```text
go test ./internal/repository -run 'AccountMonitor' -count=1
go test ./internal/service -run 'AccountMonitor|OpenAIAccountSchedulerProjection' -count=1
go test ./internal/service -run '^TestOpenAIAccountSchedulerProjection' -count=1
go test ./internal/service ./internal/repository ./internal/handler/admin -run 'AccountMonitor|OpenAIAccountSchedulerProjection' -count=1
go build ./cmd/server
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm typecheck
pnpm build
```

结果：后端 service/repository/handler 聚焦测试通过；server build 通过；账号监控前端 2 文件 110 tests 通过；typecheck/build 通过；gofmt 已执行，`git diff --check` 通过。构建仅有既有 pnpm overrides、Browserslist、dynamic-import 与 Node localStorage/deprecation 警告，无失败项。

## 未验证项与边界

- 浏览器登录态桌面/移动端视觉验收仍未在本 worktree 执行，留给 root 发布后的线上专项验收。
- 本刷新不含数据库迁移、配置 schema 变化、生产业务数据写入、账务操作或发布动作。
- 调度器非 T76 的既有随机/粘性测试在本机存在时间敏感/环境相关失败，未扩大为本刷新阻断；T76 projection 与 monitor 直接测试单独通过。
