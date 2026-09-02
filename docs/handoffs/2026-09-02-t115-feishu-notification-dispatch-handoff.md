# T115 飞书余额通知调度修复交接

## 状态

`READY_FOR_ROOT_REVIEW`

## 根因

生产存在多个 `firing` 的 BaseURL 余额告警，但过去 24 小时没有新的投递记录。通知服务的 `RunDue` 只刷新 `zero` 状态的活动 scope；`low` 状态的余额快照过期后，重新评估没有有效值，因此不会执行 claim/send。循环同时丢弃 `Evaluate` 和 `RunDue` 错误，导致该故障没有可见日志。

## 实现

- `RunDue` 对所有活动 BaseURL scope 刷新余额快照，覆盖 `low` 与 `zero` 告警。
- 通知循环记录 `Evaluate` 和 `RunDue` 失败，保留现有错误边界，不改变发送频率、去重键、卡片内容或恢复语义。
- 增加 `low` 活动 scope 刷新回归测试。

## Git

- 基线：`main@b15ccd267d6745166af4f36e75f31fbd2987ab13`
- Worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t115-feishu-notification-dispatch`
- 分支：`codex/t115-feishu-notification-dispatch`

## 验证

在 `upstream/sub2api/backend` 执行并通过：

```text
go test ./internal/service -run 'Test(UpstreamBalance|ProvideUpstreamBalance)' -count=1
go test ./internal/repository -run 'TestUpstreamBalanceEventRepository' -count=1
go build ./cmd/server
git diff --check
```

TDD RED 已确认：新增测试在旧实现下因 `low` scope 未刷新而失败；修复后 GREEN。

## 发布边界

- 未合并根 `main`。
- 未推送。
- 未部署验收站或主站。
- 未发送真实飞书消息。
- 无数据库迁移、生产数据写入或 secret 变更。

## 根线程下一步

刷新候选到届时最新干净 `main`，重跑上述直接相关测试，再按唯一发布车道审查、合并和部署。若发布预检返回 `downtime_required=true`，必须重新取得用户停机授权。
