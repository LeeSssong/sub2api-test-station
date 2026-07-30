# 原生运维重定向与飞书纯提醒验证报告

**日期：** 2026-07-30
**范围：** 当前分支的本地实现与结构验证；未部署、未调用生产写接口、未制造告警或飞书消息。

## 当前契约

- Caddy 对 `/ops`、`/ops/` 和 `/ops/*` 返回 `302`，`Location` 为 `/admin/ops`。
- relay-ops 不再挂载 `/ops`、`/relay-ops/api/ops-view`、`/relay-ops/api/incidents/ack` 或 `/relay-ops/api/feishu/events`；其 HTTP 服务只保留公开 `/pricing` 与样式资源。
- 飞书只出站发送告警、持续提醒、恢复和日报。所有卡片动作都只导航到 `/admin/ops`，不能确认接手或修改状态。
- App Bot 出站配置保留 App ID、App Secret、目标会话和通知策略；verification token、Encrypt Key、命令模式和命令路由文件不再属于 Compose 或环境示例契约。
- `infra/.env.example` 默认将 App ID、App Secret、目标会话、收件人、通知策略及对应宿主机文件变量全部留空，使 App Bot 保持一致的禁用态。运维契约对这十个 active assignment 逐行精确检查，因此相邻注释不能冒充有效的空默认值。注释另行给出启用时使用的收件人容器路径 `/run/secrets/feishu-alert-recipients.json` 和宿主机示例路径 `./secrets/feishu-alert-recipients.json`，并要求完整配置 App ID/App Secret/chat/recipients 集合；注释不证明真实秘密文件存在。
- 历史数据库迁移、历史 acknowledgement 字段和日期报告保持不变。

## 验证命令与结果

### Go 格式化、静态检查和全量测试

在 `relay-ops-service` 中运行：

```bash
gofmt -w $(find cmd internal -type f -name '*.go')
go vet ./...
go test ./... -count=1
```

结果：

- `gofmt`：退出码 `0`，无诊断。
- `go vet ./...`：退出码 `0`，无诊断。
- `go test ./... -count=1`：退出码 `0`；`cmd/relay-ops` 无测试文件，其余全部包为 `ok`。

全量命令未设置 `RELAY_OPS_TEST_DATABASE_URL`，因此数据库支持的测试仍按测试辅助函数的既有约定显示为 `ok` 但内部 `SKIP`。活动提醒与历史 acknowledgement 相互独立的中心存储集成测试已使用下述临时 PostgreSQL 单独执行，不以全量命令替代该证据。

### PostgreSQL 中心存储集成测试

使用本机已有的 `postgres:18-alpine` 镜像启动一次性容器；容器只监听随机分配的 `127.0.0.1` 端口，使用临时数据库和临时口令。就绪后在 `relay-ops-service` 中运行：

```bash
RELAY_OPS_TEST_DATABASE_URL='postgres://postgres:<temporary-password>@127.0.0.1:<temporary-port>/relay_ops_test?sslmode=disable' \
  go test ./internal/store \
  -run '^TestActiveReminderClaimIgnoresHistoricalAcknowledgement$' \
  -count=1 -v
```

结果：测试实际执行并通过，没有跳过：

```text
=== RUN   TestActiveReminderClaimIgnoresHistoricalAcknowledgement
--- PASS: TestActiveReminderClaimIgnoresHistoricalAcknowledgement (0.08s)
PASS
ok  	example.invalid/relay-ops-service/internal/store	2.012s
```

容器证据：

```text
SETUP: started codex-relay-ops-final-fix-019fb1a5-2 from postgres:18-alpine
SETUP: PostgreSQL ready after 2 check(s)
CLEANUP: stopped and removed codex-relay-ops-final-fix-019fb1a5-2 after 2 check(s)
```

清理后 `docker inspect codex-relay-ops-final-fix-019fb1a5-2` 不再找到容器；临时连接值未写入仓库。

### 路由契约

在仓库根目录运行：

```bash
bash tests/infra/validate-sub2api-update-routing.sh
bash tests/infra/validate-public-link-audit.sh
```

结果：两条命令退出码均为 `0`，输出：

```text
PASS: Sub2API update UI and routing contracts
PASS: public link audit static contracts
```

该契约确认 Caddy matcher 覆盖 `/ops` 和 `/ops/*`（包含 `/ops/` 与任意更深路径），重定向目标固定为 `/admin/ops`，且 Caddy 不再包含飞书 callback、事故确认 API、relay ops-view API 或 relay admin reverse proxy。

当前部署审计脚本 `tests/infra/audit-public-links.sh` 已逐一检查 `/ops`、`/ops/`、`/ops/incidents` 的 `302` 和 `Location: /admin/ops`，并以未认证请求检查以下三个端点为 `404`：

```text
GET  /relay-ops/api/ops-view
POST /relay-ops/api/incidents/ack
POST /relay-ops/api/feishu/events
```

POST 请求只发送空 JSON 对象，不带凭据；退役端点辅助函数以 `curl --disable` 忽略用户级 curl 配置，使用 `--max-redirs 0` 且不使用 `--location`。静态契约固定检查精确方法/路径表、循环到辅助函数的方法/路径透传、动态 `curl --request` 和不跟随重定向的约束。

本任务没有部署，因此没有对生产域名运行该部署后审计。relay HTTP server 的本地 Go 测试 `TestRetiredOpsAndAcknowledgementRoutesAreNotMounted` 已确认这些退役路由按各自原始方法返回 `404`。

删除孤立的 `internal/http/templates/ops-bootstrap.html` 后，`internal/http` 的 embed 通配范围中只剩仍由 `/pricing` 使用的 `templates/pricing.html` 与 `static/app.css`；扫描没有发现 `ops-bootstrap` 引用。

### Compose 结构

计划指定的命令是：

```bash
docker compose --env-file infra/.env -f infra/compose.yaml config >/dev/null
```

该命令未执行、在本地仍未验证，因为隔离工作树中没有被忽略的部署环境文件 `infra/.env`。本任务没有复制、伪造或创建该文件，也没有读取生产秘密。

根据任务控制器明确允许的替代方案，使用仓库支持的无秘密环境示例与 release 环境执行结构解析：

```bash
docker compose \
  --env-file infra/.env.example \
  --env-file config/releases/sub2api.env \
  -f infra/compose.yaml \
  config >/dev/null
```

替代命令结果：退出码 `0`。该结果只确认 Compose 文件能够用仓库提供的非秘密示例完成结构解析；不声称真实部署环境、秘密文件挂载或生产上线准备已经验证。

同时运行：

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
```

结果：退出码 `0`，输出：

```text
PASS: relay-ops outbound-only and native-ops contracts
```

### 退役行为扫描

按计划运行：

```bash
rg -n -S \
  '确认并接手|尚未有人确认接手|接手状态|ack_incident|ack_occurrence|/relay-ops/api/feishu/events|/relay-ops/api/incidents/ack|/relay-ops/api/ops-view' \
  relay-ops-service infra \
  --glob '!internal/store/migrations/**'
```

结果只命中 `*_test.go` 中的否定断言、退役路由 `404` 测试和旧队列 payload 兼容测试；生产 Go 文件与活动基础设施无匹配。历史迁移按要求未扫描、未修改。

补充生产/基础设施过滤：

```bash
rg -n -S \
  '确认并接手|尚未有人确认接手|接手状态|ack_incident|ack_occurrence|/relay-ops/api/feishu/events|/relay-ops/api/incidents/ack|/relay-ops/api/ops-view' \
  relay-ops-service infra \
  --glob '!internal/store/migrations/**' \
  --glob '!*_test.go'
```

结果：退出码 `1`，无匹配。

另运行：

```bash
rg -n -S 'URL: "/ops"|URL:"/ops"|其余对象请在 /ops 查看' \
  relay-ops-service/internal/notify
```

结果：退出码 `1`，无匹配。

## 出站通知覆盖

全量 Go 测试继续覆盖：

- 告警、简短持续提醒和恢复完整生命周期：`TestConsolidatedGroupIncidentLifecycleWithConciseReminder`。
- 恢复需要连续健康证据：`TestServiceRequiresTwoHealthyObservationsForRecovery`。
- 日报 one-shot 幂等：`TestServiceSendsIdempotentDailyDigestWithoutIncidentIdentity`。
- 事故证据去重、失败重试和新 occurrence：`TestDeliverySenderDeduplicatesSuccessfulEvidenceAndRetriesFailure`。
- 旧队列卡片在发送前归一化为 reminder-only：`TestDeliveryRetryServiceNormalizesLegacyAcknowledgementCardBeforeSending`。
- 活动提醒不受历史 acknowledgement 列影响：`TestActiveReminderClaimIgnoresHistoricalAcknowledgement` 已在临时 PostgreSQL 18 上实际执行并通过。
- 所有当前卡片只有 `/admin/ops` 导航动作且不含 acknowledgement 查询参数：`assertReminderOnlyCard` 被告警、提醒、恢复、日报和 retry 测试共用。

## 受保护文件与历史证据

运行：

```bash
git status --short -- \
  '监控日报-2026-07-28.md' \
  '**/监控日报-2026-07-28.md'
```

结果：退出码 `0`，无输出；该文件未修改、未暂存，也不属于本任务提交范围，因此保持在本任务所有提交之外。

运行：

```bash
git diff --name-only -- \
  relay-ops-service/internal/store/migrations \
  'docs/superpowers/reports/*'
```

在创建本报告前结果为空；没有历史迁移或既有日期报告被修改。本报告是唯一新增的日期验证报告。

## 结论

本地当前契约已统一到 Sub2API 原生 `/admin/ops`，relay HTTP 控制面和飞书入站控制均已退役；飞书保留告警、持续提醒、恢复、日报、重试和去重，卡片只作导航。全部 Go 测试、vet、路由契约、Compose 结构和 relay 当前运维契约通过。部署后仍需单独运行公网审计，本任务不执行部署。
