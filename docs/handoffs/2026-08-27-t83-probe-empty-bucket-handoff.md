# T83 交接：主动探测空桶准入与模型检测降载

## 候选

- 基线 main：`e6e36e426`
- 候选分支：`codex/t83-probe-empty-bucket`
- 候选 worktree：`.worktrees/t83-probe-empty-bucket`
- 目标：三个自动主动探测只在北京时间 5 分钟桶无真实请求时执行；自动失败不重试；模型检测每 6 小时一次、low、单 worker、无 high 升级。

## 已实现

- `usage_logs` 半开窗口 `created_at >= from AND created_at < until` 的账号/分组 `SELECT EXISTS` reader。
- 账号自动监控仅 active+schedulable；usage reader 缺失、查询失败或当前桶有请求时 fail-closed；prompt 使用 UUID nonce；手动 RunOne 保持 active 账号兼容。
- 渠道自动监控独立于手动 RunCheck；仅 group 关联且当前桶为空时执行；按 monitor+桶稳定选择一个模型；只执行一次模型检查；challenge 增加 UUID nonce；手动路径仍保留 retry。
- 模型检测 scheduled slots 改为 `00:00/06:00/12:00/18:00`（Asia/Shanghai），仅 active+schedulable 且空桶账号入队，profile=low；scheduled execute 不再 high escalation；v4.1.1 adapter worker 固定为 1。
- `cmd/server/wire_gen.go` 已将 `AccountUsageService` 注入账号模型检测与渠道 runner。

## 直接验证

以下命令在候选 worktree 通过：

```text
go test ./internal/service ./internal/repository -run 'ActiveProbe|RunScheduledCheck|GenerateChallengeIncludesUniqueNonce|RunDueSlots|DueDetectionSlot|ScheduledDetection' -count=1
go build ./cmd/server
python3 deploy/model-detector-v411-adapter_test.py
git diff --check
```

另外，完整 `go test ./internal/service ./internal/repository -run 'ActiveProbe|AccountMonitor|ChannelMonitor|AccountModelDetection' -count=1` 已运行；其中本任务直接相关用例通过，但仓库既有的高级 OpenAI scheduler 测试及若干旧 AccountMonitor 测试仍失败/超时，原因是它们假设自动路径没有空桶准入或依赖独立调度器行为，不属于本任务新增逻辑。该结果不得作为全包通过声明。

## 发布与回滚

- 无数据库迁移、无配置格式变化、无生产数据写入、无 GitHub Actions。
- 用户已明确授权紧急路径“快速部署到主站”。发布预检若返回 `downtime_required=true`，必须在停机/切换前再次取得明确停机授权。
- 主站只能从合并并推送后的 `main` 发布；主站成功后须用同一 commit/tree 立即同步或核对验收站。
- 回滚使用既有蓝绿发布链的上一已验证 source/tree；失败候选和证据须保留。

## 风险

- 自动探测 now 使用应用当前时间并映射到固定 Asia/Shanghai +08:00 桶；生产节点时钟需保持同步。
- usage reader 是自动路径的 fail-closed 门禁；若数据库/reader 不可用，主动探测会跳过而不产生上游流量。
- 完整 service 包仍存在与本任务无关的既有失败，合并后根线程只需重跑本任务直接相关门禁与必要构建。
