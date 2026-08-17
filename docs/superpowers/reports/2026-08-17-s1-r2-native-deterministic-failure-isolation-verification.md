# S1-R2 直接相关验证报告

**基线：** `main@a00fdb186b9598c0ab0ca747d9dff1a5cea04ae2`  
**实现验证 HEAD：** `436aa8b870d65b8285780e0e4254060e1cec8d6d`  
**范围：** 原生确定性故障分类、账号/模型原生状态投影、SSE 未完成终态 transient。

## 验证命令与结果

1. `go test ./internal/service -run 'TestClassifyDeterministicUpstreamFailure|TestBuildDeterministicFailureReason|TestDeterministicBalanceIsolationDurationBounds|TestIsModelRateLimited_ProbeRequiredDoesNotExpire|TestRecordOpenAIIncompleteStreamFailureUsesExistingTransientAndReplayBoundary|TestOpenAIModelTransient_|Test.*Missing.*Terminal|Test.*missing.*terminal|TestRateLimitService_HandleUpstreamError_ModelNotFound' -count=1`
   - 结果：通过，`ok github.com/Wei-Shaw/sub2api/internal/service`。
2. `go test -tags unit ./internal/service -run 'TestRateLimitService_Deterministic|TestRateLimitService_HandleUpstreamError_(OAuth401|ModelNotFound)|TestRateLimitService_HandleUpstreamError_ModelNotFound|TestAdminService_ClearAccountError' -count=1`
   - 结果：通过，`ok github.com/Wei-Shaw/sub2api/internal/service`。
3. `go test ./internal/config -count=1`
   - 结果：通过。
4. `go test ./internal/service ./internal/config ./cmd/server -run '^$' -count=1`
   - 结果：三个受影响包 compile-only 通过。
5. `go build ./cmd/server`
   - 结果：通过。
6. `gofmt`、`git diff --check`
   - 结果：通过。
7. `git diff --name-only a00fdb186..HEAD -- upstream/sub2api/backend/migrations .github/workflows`
   - 结果：无输出；无迁移、无 GitHub Actions 变化。

## TDD 证据

- 分类器 RED：缺少 classifier/reason/config symbols；GREEN：定向 service tests 通过。
- `probe_required` RED：过去的 `rate_limit_reset_at` 被错误视为已到期；GREEN：原生 `Account` 调度测试通过。
- 原生投影 RED：402 仍调用 `SetError`、model-not-found 仍使用旧 reason；GREEN：定向 unit tests 与更新后的既有 model-not-found 回归通过。
- SSE RED：缺少 `recordOpenAIIncompleteStreamFailure`；GREEN：transient/replay boundary 测试通过。

## 未验证项

- 未运行全仓测试、压力/mutation/soak、重复浏览器矩阵或生产请求。
- 未执行发布预检、迁移预检、推送、部署或线上验收；`downtime_required=unverified`。
- 未验证真实上游余额/完整模型目录探测；本候选只接入现有错误与 transient 路径。

## 风险与后续

- 当前 episode 元数据保存在原生 reason/model-limit payload 中，没有独立历史 episode 查询表；如需历史运营报表，必须另立任务。
- `probe_required` 模型限制依赖现有原生成功测试/管理员恢复清理；旧管理员“清除全部模型限制”仍是宽恢复入口，未扩大本包范围。
- OAuth 凭据最终失效仍由现有 token refresh 生命周期确认；本包不改变 refresh 安全边界。
