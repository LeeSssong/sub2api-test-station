# S1-R2 确定性故障原生隔离编排 Handoff

## 状态

`READY_FOR_ROOT_REVIEW`。候选只停留在本地独立分支；未合并、未 push、未运行发布预检、未部署、未访问生产。

## 身份

- 原始实现基线 `main`：`a00fdb186b9598c0ab0ca747d9dff1a5cea04ae2`
- 根审刷新基线：`main@5909039f5516b632720b5ee8c2e9f9f2de72339f`
- 运行时/测试实现提交：`4d49d2a2c71a0e8378a7321425e6c45d072e1864`
- 最近一次刷新合并提交：`3b775e63393497cb154ed6e7f038ce47ffc1baa2`（包含根 `main` 刷新合并）；最终候选 SHA 以当前 worktree fresh 执行 `git rev-parse HEAD` 为准。
- 分支：`codex/s1-r2-native-deterministic-failure-isolation`
- 当前 worktree：`/Users/gongtengxinwen/.codex/worktrees/6195/sub2api搭建`

## 交付内容

- 正式规格：`docs/superpowers/specs/2026-08-17-s1-r2-native-deterministic-failure-isolation-design.md`
- 实施计划：`docs/superpowers/plans/2026-08-17-s1-r2-native-deterministic-failure-isolation.md`
- 验证报告：`docs/superpowers/reports/2026-08-17-s1-r2-native-deterministic-failure-isolation-verification.md`
- 实现与测试：确定性分类器/配置、原生模型 `probe_required` 语义、RateLimitService 原生投影、SSE incomplete 账号模型 transient 接线及直接相关回归。

## 变更文件

`upstream/sub2api/backend/internal/config/config.go`、`deterministic_failure_isolation.go` 及其测试、`model_rate_limit.go` 及测试、`ratelimit_service.go` 及确定性测试、`openai_account_model_transient.go` 及测试、`openai_gateway_response_handling.go`、`openai_gateway_passthrough.go`、更新后的 model-not-found 回归，加上上述 spec/plan/report 文档。

## 验证结果

- 直接相关 service 测试：通过。
- `-tags unit` 的确定性投影与既有 OAuth/model recovery 回归：通过。
- config 测试、受影响包 compile-only、`go build ./cmd/server`：通过。
- `gofmt`、`git diff --check`：通过。
- 迁移集合无变化；`.github/workflows` 无变化。

### 根审 follow-up

- 发现并修正原分类器将所有 HTTP 402 直接视为余额耗尽的问题。
- 新增 generic 402 回归用例；当前仅稳定机器码或明确余额/额度耗尽消息进入 `balance_exhausted`，明确余额 402 仍保留分类。
- 修复提交：`4d49d2a2c71a0e8378a7321425e6c45d072e1864`；定向 service/unit/config/compile/build/diff 验证均通过。
- 候选随后已合入当前根 `main@774d3ae4e84051462709764b9eef7812db6a333e` 完成基线刷新，刷新合并无代码冲突；刷新后的同一组直接相关验证再次通过。

## 迁移、配置与发布

- 迁移变化：无；未使用 225，也未创建/预留 226 文件。
- 配置变化：新增 `rate_limit.balance_exhausted_isolation_minutes`，默认 90；有效范围 60–120，越界安全回退 90。
- `downtime_required`：`unverified`（本任务按冻结要求未运行发布预检）。
- 发布/生产：未执行。

## 回滚方式

候选未进入发布车道。若根总控未来整合后需要回滚，使用现有本地/宿主蓝绿链切回上一已验证镜像；已写入的原生账号/模型状态通过管理员 `recover-state`、账号测试或模型恢复按现有原生入口清理，不批量删除未知状态。

## 剩余风险

- episode 元数据嵌在原生 reason/model-limit payload 中，未提供独立历史 episode 查询表。
- `probe_required` 清理由受控成功探测/管理员原生恢复负责；旧宽恢复入口仍可能清除同账号其他模型限制，这是既有原生行为，本候选未改其范围。
- 未在真实上游、生产或发布链验证余额/模型目录信号；SSE 计费 drain、proxy circuit、sticky 与幂等仅由本地直接相关测试覆盖。
- generic 402 的既有通用 402 处理路径未在生产重放；本次修复仅收窄 S1 确定性分类，不改变该既有路径。
