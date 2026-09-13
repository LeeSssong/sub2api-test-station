# 账号监控主排名对齐当前调度投影

日期：2026-09-13（Asia/Shanghai）  
状态：用户已确认方案 A 与本轮范围，进入实施  
基线：`main@1713c2e334`  
候选：`.worktrees/monitor-scheduler-rank` / `feat/monitor-scheduler-rank`

## 1. 问题与证据

生产 green `1713c2e334` 只读核对：

- GPT-Pro 实际调度只选 `366 codex2api-pro`（`priority=1`，`selected_rank=1`，近 2 小时候选 `[366]`）。
- 分组 Tab 已消费调度器只读投影 `Project()`，OpenAI/Grok 组内顺序与真实调度一致。
- 全站 Tab 仍按 T114 质量分降序，并清空全站行的 `scheduler_rank`。不可调度的 `小鸡毛-pro`（质量分 93）排全站第一；真实调度第一的 366 落到第 18。
- 生图、grok heavy 等组同样已有组内调度名次；Claude Code / deepseek 无 openai/grok 投影。
- 卡片 R2 主表面不展示质量分（旧质量列在 `v-if="false"`）。

质量分与调度排名本来就可以不一致：资格过滤、自有号优先、API-key 优先级加成、5m/55m 短窗 vs 24h 回看。监控页不应再拿质量分冒充调度顺序。

## 2. 目标与非目标

### 目标

1. 全站列表、全部分组列表、卡片主排名数字只消费调度器投影名次。
2. 质量分保留为可见指标，不决定 DOM 顺序。
3. 打开或切换 24h/7d/30d 时读取**当时**的只读投影；24h 窗口只服务时间线与回看指标。
4. 覆盖全部现有分组，不写 Pro 特例。

### 非目标

- 不改调度器选择、重试、利润门、优先级权重。
- 不新增 10–15 秒名次轮询、WebSocket 或独立排名表。
- 不把「刷新全部」改成排名刷新；它继续只做主动探测。
- 不把 `schedulable=true` 但当前不合格的账号硬编进名次。
- 不改账务、迁移、生产数据。

## 3. 方案选择

已确认方案 A：分组用该组 `scheduler_rank`；全站对账号各组成品名次取最佳（最小名次，并列看更小 `rank_total` 再 `group_id`），带 `best_scheduler_group_name`。无投影账号未排名并排在后面。

不采用全站只列各组名次、也不把全站拆成重复行。

本轮刷新策略：打开即读当前 `Project()`。现有 5 秒并发轮询保留。不增加整页自动重拉。

## 4. 数据流

```text
打开监控页 / 切换 range
  -> GET /api/v1/admin/accounts/monitor?range=...
  -> ListWindow 组装账号、窗口证据、分组
  -> 每组 attachSchedulerProjection：调用现有 Project()（不占槽、不选号、不探测）
  -> 分组列表按该组 scheduler_rank
  -> 全站行投影最佳组内 scheduler_rank，保留 quality_score
  -> 前端全站与分组均按 scheduler_rank 升序，未排名在后
```

`Project()` 的 `SnapshotAt` 使用本次 `ObservedAt`。页面展示该快照，不另造事实源。

## 5. 契约

- 分组账号：`scheduler_rank` / `scheduler_rank_total` / `scheduler_explanation` 与当前组投影一致。
- 全站账号：`scheduler_rank` 为所属分组中的最佳名次；`best_scheduler_group_name` 标明来源组；`quality_score` 仍返回。
- 无投影（非 openai/grok，或不合格）：`scheduler_rank` 为空，不编假名次。
- `quality_rank` 可保留兼容，不参与主排序，不作为卡片主排名徽章。
- 卡片主表面增加「质量分」指标；不恢复「全站质量排名 / 组内质量排名」主排序语义。

## 6. 失败与安全

- 投影失败：分组标记 `scheduler_unavailable`，全站这些账号视为未排名，质量分与时间线仍返回。
- 不返回凭据。不因看排名而触发探测或占槽。

## 7. 验收

1. 全站顺序 = 最佳组内调度名次 ASC，然后账号 ID；高分不可调度账号不能排到有调度名次的账号前面。
2. 每个 OpenAI/Grok 分组顺序 = 该组投影名次。
3. `codex2api-pro` 在 GPT-Pro 为 1/1 时，全站也带 `scheduler_rank=1` 且 `best_scheduler_group_name=GPT-Pro`。
4. 卡片能看到质量分数字，列表不按该分数排序。
5. 打开页面只拉一次监控列表；切换 range 再拉一次；不出现整页定时重拉。
6. 「刷新全部」仍走 `RunAll` 探测，不改变本任务的排名口径。

## 8. 测试

- service：全站最佳组名次、跨组取 min、无投影按账号 ID、质量分仍在。
- handler：全站 JSON 暴露 `scheduler_rank` 与 `best_scheduler_group_name`，同时保留 `quality_score`。
- frontend view：全站与分组 DOM 按 `scheduler_rank`。
- card：展示质量分；不把质量排名当主排名。

## 9. 发布

无迁移、无配置项、无调度算法改写。完成后停在候选分支，不推送、不部署，等待明确授权。

## 10. 用户批准记录

- 方案 A：全站投影最佳组内调度名次。
- 无投影平台未排名并排在后面。
- GPT-特惠 306 等漏投候选不纳入本任务。
- 本轮只做「按调度排名 + 打开即读当前投影」，不做轻量名次轮询。
