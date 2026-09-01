# T107 READY_FOR_ROOT_REVIEW Handoff

日期：2026-09-01
任务包：T107 OpenAI 明确余额错误统一账号隔离
状态：READY_FOR_ROOT_REVIEW

## 基线与候选

- 基线：`main@a4580f476fbb61f89ddbcba3a8f5a5429313ec2f`
- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t107-openai-balance-isolation`
- 候选分支：`codex/t107-openai-balance-isolation`
- 候选提交：以本线程 READY_FOR_ROOT_REVIEW 回报中的 `git rev-parse HEAD` 为准
- 候选 tree：以本线程 READY_FOR_ROOT_REVIEW 回报中的 `git rev-parse HEAD^{tree}` 为准

## 实现范围

- 将确定性余额/额度耗尽分类限制为 `PlatformOpenAI`。
- 增加结构化 `insufficient_quota`、`insufficient_user_quota`、`E44001`、error type/top-level code 及明确中英文余额耗尽文案识别。
- 仅接受 4xx 且排除 429；普通 402/403、权限/范围错误、模型错误、429、5xx 和其他平台不进入余额隔离。
- 在 Team 联动熔断之后、池模式和自定义错误码早退之前写入原生 `temp_unschedulable_until`，保持 `status=active`，通知运行时调度阻断，并返回 `shouldDisable=true`。
- 保留现有 `balance_exhausted` + `probe_required` 原因格式和余额探测恢复链路；未新增状态、字段、迁移、配置或账务事实源。

## 变更文件

- `upstream/sub2api/backend/internal/service/deterministic_failure_isolation.go`
- `upstream/sub2api/backend/internal/service/deterministic_failure_isolation_test.go`
- `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- `upstream/sub2api/backend/internal/service/ratelimit_service_deterministic_isolation_test.go`
- `docs/superpowers/plans/2026-09-01-t107-openai-balance-isolation.md`
- `docs/superpowers/handoffs/2026-09-01-t107-openai-balance-isolation-handoff.md`

## 验证

- 通过：确定性分类器新增 OpenAI 余额/额度代码、error type、顶层 code、中英文文案及排除项测试。
- 通过：池模式和自定义错误码早退优先级测试。
- 通过：运行时调度阻断通知断言。
- 通过：既有余额探测“两次成功后清除临时隔离”恢复测试。
- 通过：`go build ./cmd/server`。
- 通过：`gofmt` 与 `git diff --check`。
- 测试包运行时使用了本地临时补入再还原的既有缺失 `context` 导入；该无关文件未进入提交。该候选基线直接运行服务包测试仍会被既有 `gateway_forward_as_chat_completions_test.go` 缺失 `context` 导入阻断。

## 未验证项

- 未连接真实 PostgreSQL、验收站或主站。
- 未执行线上专项验收、真实余额错误流量或生产数据写入。
- 未执行全仓测试；既有服务包编译阻断与 T107 无关。

## 发布与回滚

- `downtime_required`：待根总控在干净且与 `origin/main` 一致的根 `main` 上预检。
- 本候选不授权合并、推送、部署或生产配置变更。
- 回滚：由根总控使用既有发布链回滚到上一已验证应用版本；不删除或回写任何余额/usage/error 历史记录。

## 剩余风险

- 余额错误的证据依赖上游返回的结构化字段或明确文案；没有明确余额证据的 402/403 仍按既有权限/计费策略处理。
- 线上恢复依赖现有余额探测成功语义；本任务未改变探测频率或恢复阈值。
- 根总控合并后需重新执行直接相关测试、构建、发布预检和授权门禁。

收到带目标 `main` SHA 的 `AUTHORIZE_MERGE_TO_MAIN` 前保持等待，不自行合并、推送、部署或清理 worktree/分支。
