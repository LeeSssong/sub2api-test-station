# T25 自建渠道监控最终视觉与主动探测重试交接

## 状态

`READY_FOR_ROOT_REVIEW`

## 基线与范围

- 基线：`main@7ed24d49137a66b5c3f9ac2626cc8cf1b59b96c3`
- 候选分支：`codex/t25-channel-monitor-final`
- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t25-channel-monitor-final`
- 范围：Monitor V2 自建卡片最终视觉收口、真实请求文案、倍率强化、趋势柱体乐观固定形态、主动探测首次失败后 5 次重试。

## 实现

- `MonitorV2GroupCard` 保留 TTFT/总延迟 P50/P95、TPS、缓存率及样本数；有效调用改为“基于 N 次真实请求。”；删除旧模型展开相关内容已保持不回归；倍率增加对比背景、边框和字重。
- `MonitorV2Timeline` 所有已有点统一青绿色和 75% 固定高度，结果/耗时只保留在 title/aria 文本。
- `MonitorV2View` 删除页面底部说明。
- `ChannelMonitorService` 每模型最多执行 6 次（首次 + 5 次重试），`operational/degraded` 立即停止；六次均失败只持久化最后一次结果；runner context 上限同步扩大。
- 无迁移、无配置 schema、无生产数据写入、无 GitHub Actions。

## 验证

- 前端 Monitor V2：4 个测试文件、27/27 通过。
- 前端 focused 视觉测试：11/11 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过，Vite 构建完成。
- 变更文件 ESLint：通过。
- 后端直接相关：`go test -tags=unit ./internal/service -run 'ChannelMonitor|RunCheckForModel|MonitorRunner' -count=1` 通过。
- `gofmt` 与 `git diff --check`：通过。
- 全量 `go test -tags=unit ./internal/service -count=1`：失败，既有 `TestGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID`、`...PersistsUpstreamRequestIDSeparately`、`...GeneratesLocalRequestIDWhenContextMissing` 等请求 ID 断言不符合当前基线实际 `billing:*` 行为；未修改这些无关代码。

## 视觉证据

- 桌面：`output/playwright/t25/channel-monitor-desktop.png`
- 移动端 390×844：`output/playwright/t25/channel-monitor-mobile.png`
- 移动端 DOM 检查：`scrollWidth=clientWidth=382`；柱体均为 `bg-teal-500`、`height: 75%`；倍率类包含 `text-base font-bold bg-primary-50`；底部说明不存在。

## 发布

- 迁移变化：无。
- 配置变化：代码默认行为无新配置；生产入口需保持/切换现有 `channel_monitor_mode=v1` 才会使用自建 Monitor V2，若仍为 `v2` 则继续官方页。
- `downtime_required`：候选无迁移，预期 `false`，最终以根合并后发布预检为准。
- 回滚：恢复上一生产槽/镜像；入口可用现有 `channel_monitor_mode=v2` 切回官方页。
- 剩余风险：重试最坏时长约 288 秒，短 interval 下由现有 in-flight 去重避免同一监控重叠运行；全量 service 基线存在上述既有失败。
