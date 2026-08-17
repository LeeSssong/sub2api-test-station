# S1-R2 确定性故障原生隔离编排 Handoff

## 状态

`READY_FOR_ROOT_REVIEW`。候选只停留在本地独立分支；未合并、未 push、未运行发布预检、未部署、未访问生产。

## 身份

- 基线 `main`：`a00fdb186b9598c0ab0ca747d9dff1a5cea04ae2`
- 运行时/测试实现提交：`436aa8b870d65b8285780e0e4254060e1cec8d6d`
- 最终候选 HEAD（根审目标）：包含本 handoff 的 docs-only 收口提交；精确 SHA 以提交后从仓库根目录 fresh 运行的 `git rev-parse HEAD` 与根审命令输出为准。
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
