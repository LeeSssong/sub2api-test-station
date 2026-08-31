# T98 飞书上游余额通知重构验证报告

日期：2026-08-31

## Scope

- 分支：`codex/t98-feishu-upstream-balance-notification`
- 基线：`main@06695141ff6459c08f316a97e6049ba2a42034bd`
- 刷新基线：已合入 `main@a928c671d3133fc33d59cd6f56c351674af0406e`，刷新合并提交 `11ee7f9f6`
- 实现提交：`1b1245117`、`80667fbe5`、`20f672d30`、`51c9ca2fd`、`57fa4ebb8`、`c28168886`、`f997da054`
- 新路径只消费 Sub2API 已有账号、余额快照和 `scheduler_rank`；通知按规范化 BaseURL 聚合。
- 旧 relay-ops 通知 writer、策略、scheduler、retry/escalation、业务包和迁移重放已移除；账务、外置化、候选、成本和对账路径保留。

## Focused Verification

在 Go 1.27.0/darwin arm64 执行：

```bash
cd upstream/sub2api/backend
go test -count=1 -overlay=/tmp/t98_service_test_overlay.json \
  -run 'Test(NormalizeNotificationBaseURL|EvaluateUpstreamBaseURLBalance|UpstreamBalanceNotificationService|BuildUpstreamBalanceEvaluations|ProvideUpstreamBalanceNotificationService)' \
  ./internal/service
go test -count=1 -run 'Test(LoadUpstreamBalanceSecrets|RenderUpstreamBalanceCard|FeishuSender)' ./internal/notify
go test -count=1 -vet=off -run 'TestUpstreamBalanceEventRepository' ./internal/repository
go test -count=1 -run 'TestUpstreamBalanceEventMigration' ./migrations
go test -count=1 ./tools
go build ./cmd/server
```

结果：在刷新合入最新 `main` 后重跑，service 15/15、notify 7/7、repository 8/8、migration 1/1、converter 4/4 通过；server build 通过。

```bash
cd relay-ops-service
go test -count=1 ./...
go build ./cmd/relay-ops ./cmd/provision-billing-source ./cmd/retire-legacy-notifications
```

结果：在刷新合入最新 `main` 后重跑，34 个 relay-ops 包完成，0 个失败；三个二进制构建通过。`RELAY_OPS_TEST_DATABASE_URL` 未设置，因此现有真实 PostgreSQL 集成测试按自身门禁跳过；SQL 字符串、依赖图、清理顺序和双门禁由不依赖外部数据库的合同测试覆盖。

```bash
tests/relay_ops/legacy_notification_retirement_contract_test.sh
SUB2API_MODEL_DETECTOR_TOKEN=fake-contract-token tests/relay_ops/validate_relay_ops_contract.sh
tests/infra/upstream_balance_notification_secret_contract_test.sh
git diff --check
```

结果：全部通过。relay-ops 常驻进程与 provision 依赖图不可达旧通知包或独立清理包；Compose 不再给 relay-ops 挂载飞书、通知策略或 agent 配置；新 secret 目录只挂载到 production worker，默认开关为 false。

最终一致性和敏感边界检查：

```bash
git diff --check 06695141ff6459c08f316a97e6049ba2a42034bd..HEAD
git diff --check
test -z "$(git diff --name-only --diff-filter=ACMR \
  06695141ff6459c08f316a97e6049ba2a42034bd..HEAD -- '*.go' | xargs gofmt -d)"

git diff --no-ext-diff --binary \
  06695141ff6459c08f316a97e6049ba2a42034bd..HEAD \
  -- . ':(exclude)outputs/**' \
  | rg -i --quiet -- \
    '-----BEGIN (RSA|OPENSSH|EC|DSA) PRIVATE KEY-----|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|Bearer[[:space:]]+[A-Za-z0-9._~+/-]{24,}'

rg 'login_(account|password)|card_json|message_payload' \
  upstream/sub2api/backend/migrations/231_upstream_baseurl_balance_notifications.sql \
  upstream/sub2api/backend/internal/service/ops_alert_models.go \
  upstream/sub2api/backend/internal/repository/upstream_balance_event_repo.go
rg '"net/http"' \
  upstream/sub2api/backend/internal/service/upstream_balance_notification*.go \
  upstream/sub2api/backend/internal/repository/upstream_balance_event_repo.go
```

结果：diff check 通过，全部变更 Go 文件无 `gofmt` 差异，高置信凭据签名无命中；事件 schema/model/repository 的敏感字段扫描和 evaluator/ledger 的 HTTP 客户端扫描均无命中。受保护 XLSX 与生成 JSON 未被 Git 跟踪。上述 `rg` 命令的无命中退出码是预期通过条件。

## Behavior And Safety

- `0 < USD < 5` 为 P2/orange/30 分钟；`USD = 0` 为 P1/red/5 分钟并 `@` 接收人，不调用 `urgent_app`。
- 卡片包含一次余额、BaseURL、登录账号和明文密码，并稳定列出同 BaseURL 下全部活跃 API Key 账号及分组调度排名；超过 30 KiB 整体拒绝。
- 事件迁移、事件模型和仓储接口不含登录账号、密码、card JSON 或 message payload；只持久化 scope、状态、时间、generation、lease 和非敏感错误码。
- evaluator/service/repository 文件不导入 `net/http`，没有新增余额请求或探测。唯一 HTTP 客户端是飞书传输适配器，定向测试使用本地 fake transport。
- 变更文件的高置信私钥、云密钥、长 token 和 bearer 签名扫描无命中。`login_password` 仅存在于受保护登记簿解析、内存卡片输入和显然虚构的测试值中。
- 工作簿和转换后的 JSON 未加入 Git；Task 6 的受保护临时转换产物已删除。
- 受控旧表清理命令默认 count-only；执行必须同时提供 `--execute` 和 `RELAY_OPS_RETIRE_LEGACY_NOTIFICATIONS_CONFIRM=DELETE_WITHOUT_BACKUP`。本次未执行生产 DROP。

## Known Baseline Failures

- `tests/infra/validate-baseline.sh` 默认因仓库当前要求 `SUB2API_MODEL_DETECTOR_TOKEN`、而示例环境未提供该值而失败。
- 注入虚构 token 后，nginx、Caddy 和隔离演练合同通过，但既有蓝绿拓扑测试仍报 `blue must use shared healthy PostgreSQL and Redis`；该旧断言没有接受当前已存在的 model-detector 依赖，与 T98 差异无关。
- service 定向测试继续使用既有 `/tmp/t98_service_test_overlay.json`，只排除两个与 T98 无关的已知坏测试文件：`openai_admission_first_output_wiring_test.go` 与 `openai_sticky_reference_test.go`。
- repository 定向测试使用 `-vet=off`，规避既有 `usage_log_repo_stats.go:1004` 的 `fmt.Sprintf` vet 失败；完整 `cmd/server` 仍成功构建。

## Release Boundary

- 新增 Sub2API migration `231_upstream_baseurl_balance_notifications.sql`；relay-ops 的 `015_retire_legacy_notifications.sql` 不进入普通 `Migrate`，只能由独立受控命令执行。
- 配置新增 worker secret 目录和默认关闭开关；relay-ops 通知/agent 运行时配置删除。旧飞书宿主凭据路径占位保留，仅供人工复制到新目录。
- 未推送、未部署、未发真实飞书、未修改生产配置、未执行旧表删除。`downtime_required` 必须由根线程合并后的发布预检判定。
- 当前候选已包含 `main@a928c671d` 且刷新后专项验证通过；若根审时 `main` 再次前进，必须再次进入 `REFRESH_REQUIRED`，不得直接整合旧候选。
