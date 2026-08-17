# S1-R2 直接相关验证报告

**原始实现基线：** `main@a00fdb186b9598c0ab0ca747d9dff1a5cea04ae2`
**刷新验证基线：** `main@774d3ae4e84051462709764b9eef7812db6a333e`
**候选验证 HEAD：** `8d05a7fc5f39f385b14639bd91f60afc96db93e1`
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
8. 基线刷新：`git merge --no-edit main`
   - 结果：无代码冲突；刷新后的候选重新通过上述 service/unit/config/compile-only/build/gofmt/diff-check 门禁。

## 根审 follow-up

- 根审发现原分类器把所有 HTTP 402 都当作 `balance_exhausted`，与“稳定机器码/明确余额消息才可分类”的规格边界不一致。
- 先补充 RED 用例 `generic payment required is not balance evidence`，确认旧实现错误分类；随后以 `4d49d2a2c71a0e8378a7321425e6c45d072e1864` 收窄为仅允许机器码/消息 allowlist，保留明确余额 402 的分类。
- follow-up 直接相关 service、unit、config、compile-only、server build、gofmt 和 diff-check 均通过；generic 402 不再进入 S1 确定性余额分类，后续未命中仍由既有 402 处理合同接管。

## TDD 证据

- 分类器 RED：缺少 classifier/reason/config symbols；GREEN：定向 service tests 通过。
- `probe_required` RED：过去的 `rate_limit_reset_at` 被错误视为已到期；GREEN：原生 `Account` 调度测试通过。
- 原生投影 RED：402 仍调用 `SetError`、model-not-found 仍使用旧 reason；GREEN：定向 unit tests 与更新后的既有 model-not-found 回归通过。
- SSE RED：缺少 `recordOpenAIIncompleteStreamFailure`；GREEN：transient/replay boundary 测试通过。

## 未验证项

- 未运行全仓测试、压力/mutation/soak、重复浏览器矩阵或生产请求。
- 未执行发布预检、迁移预检、推送、部署或线上验收；`downtime_required=unverified`。
- 未验证真实上游余额/完整模型目录探测；本候选只接入现有错误与 transient 路径。generic 402 的旧版通用 402 处理路径未在生产重放，仍需发布后按既有线上验收边界观察。

## 风险与后续

- 当前 episode 元数据保存在原生 reason/model-limit payload 中，没有独立历史 episode 查询表；如需历史运营报表，必须另立任务。
- `probe_required` 模型限制依赖现有原生成功测试/管理员恢复清理；旧管理员“清除全部模型限制”仍是宽恢复入口，未扩大本包范围。
- OAuth 凭据最终失效仍由现有 token refresh 生命周期确认；本包不改变 refresh 安全边界。
