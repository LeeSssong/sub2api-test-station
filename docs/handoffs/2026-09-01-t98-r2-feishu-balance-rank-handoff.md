# T98-R2 飞书余额与排名修复交接

## 状态

`READY_FOR_ROOT_REVIEW`

候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-r2-feishu-balance-rank`
候选分支：`codex/t98-r2-feishu-balance-rank`
基线：`main@200a1457b`
候选提交：`a8f8f0bd822ea2eb7146009fb0520b9fc996b20d`（含余额字段增量；以实际提交为准）

## 变更

- 将 API Key 的余额/声明辅助刷新移到主动探测跳过判断之前；真实流量覆盖全部分组时仍刷新余额。
- 飞书余额通知改读 `ListWindow("24h")`，复用原生分组 `SchedulerRank` 投影。
- 飞书卡片顶部保留 BaseURL 聚合后的“当前余额”，关联账号明细新增各有效 API Key 的“余额：USD ...”字段；无效或缺失快照不伪造余额。
- 增加余额刷新、通知排名和卡片排名回归测试。

余额口径保持不变：同一规范化 BaseURL 下，以 `observed_at` 最新的有效 API Key 可用余额为准；不求和、不取最大值。

## 验证证据

- `go test -count=1 -run 'TestReadUpstreamBalanceEvaluationsUsesWindowSchedulerRanks' ./internal/service` 通过。
- service 定向测试：`Test(NormalizeNotificationBaseURL|EvaluateUpstreamBaseURLBalance|UpstreamBalanceNotificationService|BuildUpstreamBalanceEvaluations|ReadUpstreamBalanceEvaluations|ProvideUpstreamBalanceNotificationService)` 通过。
- notify 定向测试：`Test(LoadUpstreamBalanceSecrets|RenderUpstreamBalanceCard|FeishuSender)` 通过。
- 余额字段回归：`TestRenderUpstreamBalanceCardP2ShowsOneWalletAndAllAccounts` 通过。
- `go build ./cmd/server` 通过。
- 修改文件 `gofmt` 检查通过，`git diff --check` 通过。

## 发布边界与风险

- 未新增迁移、配置或余额事实源，未写入生产数据，未真实发送飞书。
- 当前仅完成候选提交，不得从该 worktree 合并、推送、构建部署制品或部署。
- 根 `main` 仍受既有推送阻塞影响；后续必须由发布总控按全局约束在干净且与 `origin/main` 一致的 `main` 上审查、合并、推送、部署和线上验证。
- 回滚方式：在 `main` 上形成明确 revert 或前向修复提交后，按发布总控流程重新发布；不得直接从候选分支发布。
