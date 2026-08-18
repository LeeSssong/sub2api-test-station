# T22 独立执行会话指令

## 任务

官方 Channel Monitor V2 简洁运营视图。执行会话须从最新干净根 `main` 创建分支 `codex/t22-channel-monitor-ops` 和独立 worktree `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t22-channel-monitor-ops`。不得修改根队列、项目总账、生产状态、发布证据，不得推送、合并或部署。

## 已批准范围

- 继续使用官方 V2 数据聚合和单一事实源，不切回自建 Monitor。
- 默认时间窗为 24h，保留 90m、7d、30d。
- 首屏保留分组状态、成功率、首 Token、缓存率和最近趋势；模型明细、错误分类、用户排行等移入“详细分析”。
- 无流量或样本不足分组显示“已就绪·暂无流量”或“待观察”，使用中性偏正向视觉，不计入整体异常和健康评分，也不伪造为真实健康。
- 真实错误、低成功率和高延迟继续黄/红显示。
- 本地拒绝、禁用模型、参数校验失败等未获得上游响应的请求不进入成功率和缓存率分母；复用 T19 已上线的有效样本口径，先确认没有重复实现。
- 保留 `channel_monitor_mode=v1` 回滚开关。
- 桌面和 390px 移动端无整页横向溢出。

## 流程与交付

1. 完整读取 `AGENTS.md`、两份强制事实源和本文件。
2. 扫描原生 Channel Monitor V2 当前代码、API、页面和测试，书面说明复用点。
3. 基于已批准产品规则完成正式规格、自审和实施计划；没有新的产品决策时可引用根总代审授权继续。
4. 按 TDD 实现，仅运行直接相关 Go/Vitest、必要 typecheck/build、gofmt、diff-check；不运行全仓、压力、mutation、soak 或无关浏览器矩阵。
5. 提交候选并停在 `READY_FOR_ROOT_REVIEW`，报告基线、commit/tree、变更文件、测试、迁移/配置、downtime 预期、回滚和未验证项。

## 当前依赖

T21 已 `DONE`；T23 在另一独立 worktree `IMPLEMENTING`。T22 可设计和实现，但不得与 T23 或其他任务共享写入者，不得进入根发布车道，直到根总控授权。
