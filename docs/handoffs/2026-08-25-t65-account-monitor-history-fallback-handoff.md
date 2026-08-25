# T65 账号监控历史最终结果回退交接

- 状态：`READY_FOR_ROOT_REVIEW`
- 原始基线：`main@7fb71683b`
- 候选提交：`0f6222f0e`
- 候选 worktree：`.worktrees/t65-account-monitor-history-fallback`
- 候选分支：`codex/t65-account-monitor-history-fallback`
- 未合并、未推送、未部署，等待根总控按串行发布车道处理
- `downtime_required`：待根发布预检确认；候选未执行停机或生产动作

## 范围与实现

- 模型检测当前证据不足或失败时，继续展示当前最新状态，同时从最近 20 条检测记录回退到最近一次证据充分且已完成的最终 `normal/abnormal` 结果。
- 账号监控当前窗口没有有效评分时，复用最近 7/30 天的原生聚合评分，不返回空评分。
- 页面明确展示当前状态、历史最终结果、回退来源和时间；不新增平行事实源，继续复用原生检测运行记录、监控结果和评分算法。

## 主要变更与验证

候选实现覆盖账号监控 service/repository 投影、handler/API 合同、监控页面及直接测试。已通过直接相关门禁：

```bash
go test ./internal/service -run 'TestAccountMonitor' -count=1
go test ./internal/repository -run 'TestAccountMonitor' -count=1
go build ./cmd/server
pnpm vitest run <账号监控相关定向测试>
pnpm typecheck
pnpm build
git diff --check
```

全量后端测试存在大量既有基线失败，未作为本任务回归结论；直接相关 Go/Vitest/typecheck/build 均已通过。候选 worktree 当前保持干净。

## 发布与回滚边界

- 无数据库迁移、无配置变更、无生产数据写入。
- 根总控需在候选进入合并车道前盘点非 `main` worktree，先合并并验证 T65，再按队列处理 T66；推送、蓝绿发布和线上账号监控专项验收只能从合并后的 `main` 执行。
- 线上真实账号检测证据不足、历史最终结果回退和无评分账号显示仍需根发布后的登录态专项验收。
- 若合并或发布失败，保留候选 worktree、分支和证据，在原任务包继续修复；回滚恢复到合并前 `main` 或候选提交 `0f6222f0e` 对应的稳定版本。
