# T82 调度器健康隔离与故障切换优化交接

- 基线 SHA：`3b8c0e81d`
- 候选分支：`codex/t82-scheduler-health-failover`
- 当前状态：实现完成，直接相关测试与构建通过，等待根线程审查。

## 变更范围

- 默认质量门禁、sticky 逃逸后的请求级排除、候选/排除/最终账号结构化观测。
- 502/503 窗口化分级熔断：60 秒 2 次、5 分钟 3 次、15 分钟 5 次；同账号安全重试最多 1 次，跨账号最多 2 次。
- 本地与共享 Redis 半开状态机均要求连续两次成功后恢复；第一次成功保持 `half_open`。
- 余额不足使用 `temp_unschedulable` + `probe_required`，成功 billing probe 连续两次后清理隔离和运行时 blocker。
- API Key 401 保持账号 `active`，进入临时隔离并由 Account Monitor 探活；成功后清理持久化隔离与运行时 blocker，连续失败三次后探测间隔延长到 15 分钟。
- OAuth 401 保留 token cache 失效与 refresh 链路；不可恢复凭据仍按既有永久禁用语义处理。
- OpenAI handler 的跨账号切换上限统一限制为 2；502/503 的同账号池重试限制为 1。

## 修改文件

涉及 `upstream/sub2api/backend/internal/{handler,repository,service}` 下的调度、运行时隔离、探活、重试、可观测性及定向测试文件；另新增本交接文件。未新增数据库迁移、GitHub Actions、生产配置或凭据。

## 验证证据

在 `upstream/sub2api/backend` 执行：

```text
go test ./internal/repository -run 'TestOpenAISharedHealth(HalfOpenLeaseHasOneWinnerAndRejectsStaleFence|RecordAttemptIsIdempotentAndResetsOnSuccess)' -count=1
PASS

go test ./internal/handler -run 'TestOpenAIRetryBudget|TestOpenAI.*Failover|Test.*CompactLog' -count=1
PASS

go test ./internal/service -run 'Test(OpenAIScheduler.*QualityGate|OpenAIModelTransient|OpenAIHalfOpen|RateLimitService_Deterministic|UpstreamBillingProbe|AccountMonitorRunAllProbesAPIKey401RecoveryAndClearsTempState)' -count=1
PASS

go build ./cmd/server
exit 0

git diff --check
exit 0
```

另有一个与本任务无关的既有 Codex models handler 测试在宽泛手工筛选中仍返回通用错误文案；未纳入 T82 直接验证命令，也未改变该路径的显式上限测试行为。

## 未验证项与剩余风险

- 尚未连接真实验收站/生产 Redis 验证跨进程半开状态在真实网络抖动下的时序；仓储单测使用 miniredis 已覆盖第一次成功保持 `half_open`、第二次成功恢复 `healthy`。
- API Key 401 探活恢复依赖现有 Account Monitor 调度与上游探测配置，线上首次探活时间受五分钟周期影响。
- 未进行主站部署、线上流量回放或全仓无关回归；基线已有若干与 T82 无关的旧测试问题。

## 发布与回滚

- `downtime_required=false` 仅记录本候选发布预检结果，不构成主站发布授权。
- 当前未发布、未推送、未合并；主站仍未修改。
- 回滚方式：回滚候选分支上的 T82 commit；无数据库回滚步骤。
