# D04 首发用户非功能基线（第一阶段，无上游费用）

**日期：** 2026-07-21  
**结论：** `DONE_WITH_CONCERNS`  
**范围：** 仅入口 TLS/HTTP、无写页面、宿主机资源和容器健康。未发起任何 `/v1/*` 模型请求，未执行登录、注册或任何余额/路由/配置写入。

## 边界与测量位置

- 入口测量位置：Codex 本地 macOS 工作站，经公网访问 `api.xingqiaolab.top`。它反映该工作站到入口的观测，不代表中国三网、用户 SLA 或服务器到上游性能。
- 服务测量位置：`sub2api-prod` 的只读 SSH 会话。未读取 `.env`、容器环境、密钥、Cookie、日志正文、数据库或任何秘密文件。
- 时间：服务器资源快照为 `2026-07-21T01:06:14Z`；入口抽样在随后约四分钟内完成。
- 统计：延迟为 `curl` 的端到端 `time_total`，P50/P95 使用 nearest-rank；小样本时 P95 等于最大值。错误率仅针对具有完整分母的端点请求计算。

## 已测事实

### 入口 TLS 与 HTTP

| 检查 | 样本与结果 | 结论 |
|---|---:|---|
| HTTP `GET /health` | 1 次，`308`，重定向到 HTTPS | 明文 HTTP 强制升级存在。 |
| TLS 握手 | 1 次，证书验证 `OK`；TLS 1.3 / `TLS_AES_128_GCM_SHA256`；证书 CN 为入口域名 | 公网 TLS 在测量点有效。 |
| HTTPS `GET /health` | n=14，14 个 `200`，错误率 0%，P50 `4.405s`，P95 `5.112s`，范围 `1.583s-5.112s` | 健康端点可达；该延迟是本地公网观测。 |
| HTTPS `GET /pricing` | n=4，4 个 `200`，错误率 0%，P50 `4.506s`，P95 `4.738s`，范围 `4.350s-4.738s` | 无费用公开页可达。 |
| HTTPS `GET /ops` | n=2，2 个 `200`，错误率 0%，P50 `1.969s`，P95 `4.744s`，范围 `1.969s-4.744s` | 公共入口页可达；此处不执行认证或管理操作。 |
| HTTPS `GET /login` | n=1，`200`，错误率 0%，P50/P95 `4.698s` | 登录静态/渲染入口可达；未提交凭据。 |
| HTTPS `GET /register` | n=1，`200`，错误率 0%，P50/P95 `1.651s` | 注册静态/渲染入口可达；未提交表单或创建用户。 |

一次 5 路并发、每路拟采 12 次的低量 GET 抽样中，`curl` 记录了 **3 次** `SSL_ERROR_SYSCALL` TLS 握手失败。该执行在完整结果收集前中断，因而没有可信总分母，**不得**把它报告为错误率或成功率；它是需要复测的入口稳定性信号。

后续按相同强度完成了有完整分母的复测：5 路并发、每路 12 次，共 `60` 次 `GET /health`；`60/60` 为 HTTP 200，其他 HTTP 状态 `0`，TLS/连接传输错误 `0`。成功样本 P50 `1.779s`、P95 `4.779s`、范围 `1.563s-4.971s`。因此原 3 次 TLS 信号在本轮没有复现；它仍只代表单一测量位置的低并发入口观测，不构成模型容量或用户 SLA。

本轮功能收口后的只读复核中，第一次本地 `GET /health` 再出现 1 次 `SSL_ERROR_SYSCALL`，而 `/readyz`、`/pricing`、`/ops`、`/monitor` 同轮均为 200；随后顺序复测 `/health` 为 `12/12` HTTP 200。容器始终 healthy，未形成稳定复现或服务端故障证据，因此没有修改生产。该信号继续保留为入口常规观察项。

`HEAD /health` 返回 `404`，而上述 `GET /health` 连续成功；本报告把 D04 的只读健康验收定义为 GET，不将未声明支持的 HEAD 视作服务故障。

### 生产主机、容器与监听边界

| 项目 | 只读快照 | 基线判断 |
|---|---|---|
| 宿主机运行时间与负载 | 已运行 `4d 22h`；load average `0.00/0.01/0.03` | 测量时无 CPU 压力证据。 |
| 内存与 swap | 3,723 MiB 总内存，2,765 MiB available；swap 1,987 MiB，已用 0 | 有余量，未见 swap 压力。 |
| 根分区 | 59 GiB，总已用 15 GiB，剩余 42 GiB，使用率 26% | 有足够短期磁盘余量。 |
| 服务容器 | `sub2api`、PostgreSQL、Redis、relay-ops 均 `healthy`；Caddy 运行中且未声明 healthcheck | 当前服务健康状态正常。 |
| 单次容器资源快照 | relay-ops `0.09%` CPU / `7.324 MiB`；Caddy `0.32%` / `14.66 MiB`；Sub2API `0.91%` / `65.38 MiB`；PostgreSQL `0.44%` / `98.72 MiB`；Redis `2.97%` / `5.336 MiB` | 单瞬时快照，不等于持续容量或 CPU 上限。 |
| 公网监听 | IPv4 与 IPv6 的 `80`、`443` 均为 LISTEN | 与 Caddy 入口边界一致；本次未检查或更改其他服务配置。 |

### 新版 D04 生产只读复核（2026-07-21 11:05-11:08 UTC）

本轮只发送无认证 `GET`；未登录、注册、写余额、创建邀请码、发送模型请求或读取秘密。服务器命令只读取 Docker 运行状态、资源和监听端口。

| 范围 | 原始摘要 | 可作出的结论 |
|---|---|---|
| TLS 与 HTTP 入口 | TLS 1.3、`TLS_AES_128_GCM_SHA256`、证书验证 `OK`；`HTTP GET /healthz` 为 `308` 到 HTTPS | 入口 HTTP 升级和 TLS 在单一测量位置可用。 |
| 公网并发入口 | `GET /healthz`，5 个 worker x 12 次，`n=60`；`200=60`、其他/传输失败 `0`；P50 `1.747s`、P95 `2.182s`、范围 `1.546s-4.882s` | 这是 Caddy/公共入口低并发 GET 基线，**不是**模型容量、SSE 容量、上游延迟或 SLA。 |
| D04 容器 | `sub2api-d04-internal-test-service-1` 为 `healthy`，restart `0`，OOM `false`，启动于 `2026-07-21T10:19:00Z` | D04 进程的 Docker healthcheck 与容器存活通过。 |
| Caddy 容器 | restart `0`，OOM `false`，启动于 `2026-07-21T10:24:05Z`；Caddy 没有 Docker healthcheck | Caddy 未重启；运行状态不能替代代理到 D04 的端到端证明。 |
| 资源（并发抽样后） | Caddy `0.00%` CPU、`19.1 MiB / 3.637 GiB`；D04 `0.00%` CPU、`4.941 MiB / 192 MiB`；主机可用内存 `2,754 MiB`、swap 已用 `7 MiB`、根盘 `29%` 已用 | 这是一个静态、低负载瞬时快照，未见资源压力；不构成 15 用户或长连接容量证据。 |

**D04 经 Caddy 的路径异常/限制：** 本轮公网 `GET /healthz` 返回 `200`，但响应为通用 HTML（`content-type: text/html`），不是本地 D04 服务预期的 JSON health 响应，故上表的 60 次延迟不能归因于 D04。为避免任何写操作而尝试的既有只读路径 `GET /internal-test/api/join/nonexistent-d04-baseline`、`GET /internal-test/api/checkin` 与 `GET /internal-test/join/nonexistent-d04-baseline` 均得到空响应 `404`；它们没有给出 D04 handler 的 JSON `404` 或 `405` 语义。该观察仅表明这些路径在当前公网路由下未被证实进入 D04 服务，**不推断其是否符合新版规格，也不修改 Caddy**。因此新版 D04 的“经 Caddy 公网路径延迟”仍为未知，等待主 Agent 提供已批准、无写的新版健康/只读路径后再测。

本轮执行摘要（均无认证头、请求体或模型名）：

```sh
# 单次 TLS/HTTP 与路由辨识
openssl s_client -connect api.xingqiaolab.top:443 -servername api.xingqiaolab.top -brief </dev/null
curl --proto '=https' --tlsv1.2 -o /dev/null -w '%{http_code} %{time_total}\n' https://api.xingqiaolab.top/healthz
curl -o /dev/null -w '%{http_code} %{redirect_url} %{time_total}\n' http://api.xingqiaolab.top/healthz

# 公网并发：5 个并行 worker，每个执行 12 次上述 HTTPS GET /healthz；汇总状态、P50、P95、最小和最大值。

# 服务器只读资源/重启快照
ssh -o BatchMode=yes sub2api-prod \
  'docker ps; docker stats --no-stream; docker inspect <caddy-and-d04-container>; free -m; df -h /; ss -ltn'
```

## 执行命令

以下命令均未携带认证头、请求体、模型名或秘密值：

```sh
curl --connect-timeout 8 --max-time 20 -o /dev/null \
  -w 'http_status=%{http_code} redirect=%{redirect_url} total_s=%{time_total}\n' \
  http://api.xingqiaolab.top/health

curl --proto '=https' --tlsv1.2 --connect-timeout 8 --max-time 25 \
  -o /dev/null -w '%{http_code} %{time_total}\n' \
  https://api.xingqiaolab.top/{health,pricing,ops,login,register}

openssl s_client -connect api.xingqiaolab.top:443 \
  -servername api.xingqiaolab.top -brief </dev/null

ssh -o BatchMode=yes sub2api-prod \
  'date -u; uptime; free -m; df -h /; docker ps; docker stats --no-stream; ss -ltn'
```

## 自动化非功能证据（本地 Fake Provider）

2026-07-21 新增并通过的测试均使用临时 SQLite 与 Fake Sub2API，不读取生产秘密、不访问服务器、不触发模型或上游费用：

- `internal/http/nonfunctional_test.go`：12 个同日、同步起跑的原生登录代理请求全部返回 `200`；Fake Provider 余额只有一次 `$20` 效果且余额历史只有 1 条。跨源 `OPTIONS` 返回 `405` 且不提供 CORS allow header，D04 响应带 `nosniff`、`no-store` 和受限 CSP。认证代理的未知端点、非首发用户和 grant 失败保持原生响应边界由 `internal/authproxy` 覆盖。
- `internal/store/locking_test.go`：两个 Store 打开同一个临时 WAL SQLite；第一个 Store 保持写事务时，第二个写入受生产一致的 5 秒 SQLite `busy_timeout` 约束。测试使用 7 秒外层门禁并要求锁等待不超过 6.5 秒；释放锁后写入成功，双 Store 关闭并重开后数据仍可读取。250 ms Go context 不被误写成 modernc SQLite 的立即中断承诺。
- 验证命令：`docker run --rm -e GOMAXPROCS=1 -e CGO_ENABLED=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./... -p 1 -race -count=1`、同环境 `go vet ./...`，以及 `bash tests/internal_test/validate_internal_test_contract.sh && bash tests/infra/validate-baseline.sh`。修正测试口径后的 fresh run 均 exit `0`；锁测试实测约 `5.14s` 后有界失败，与 5 秒 `busy_timeout` 一致。

`golang:1.24-alpine` 在本机以 `CGO_ENABLED=0` 运行，不能执行 Go race detector；因此 race 验证使用已有的标准 `golang:1.24` 容器。该差异仅影响本地测试运行器，不是 D04 生产镜像或服务失败。

## 未知项与限制

- 当前入口样本来自一个位置，且 `/pricing`、`/ops`、`/login`、`/register` 的样本量分别仅为 4、2、1、1。它们是首发前的可达性基线，不是延迟目标、可用性承诺或容量证据。
- 原三次并发 TLS 握手失败已通过 `60/60` HTTP 200、传输错误 `0` 的完整分母低并发复测补充取证；单次复测不能证明长期零错误，后续只需纳入常规入口观测，不再作为当前功能阻塞。
- 未测真实登录、注册 POST、每日登录余额调整或调度器写路径。新的公开注册/每日登录版本已经以 `read_only` 部署；D04 仍必须保持 `read_only`，直到得到单独的低额 write 验收批准。
- 未测客户端到上游、模型排队、TTFT、SSE 完整性、上游扣费、三方账单或 XM API 可用性。本阶段模型请求数为 **0**，上游费用为 **0**。
- 容器资源仅为一次无压力快照；没有 15 名用户、SSE 连接保持、长尾、RPM 或 24 小时观察证据。

## XM API PLUS/PRO 后续有界测试计划

本计划只在主 Agent 完成 XM 账号映射、用户明确批准一个费用上限后执行。它遵循 V2 的“先 proposal、后单独采纳”原则；未获采纳不得修改生产路由、账号、倍率、价格、余额、Key、数据库、Compose 或服务。

### 公共 v3 非功能工具更新

2026-07-21 规格批准后，公共评测工具已新增独立 sync/SSE/RPM 容量证据、严格身份绑定、共享容量池公平性、连续观察窗口和 failover/failback 时间线的离线评估。该实现没有 XM、Wawazz、域名、模型或渠道特判；供应商只作为 scenario/profile 数据实例。

默认 v3 阶梯的每渠道短测上界为 HTTP `2M+71+K`、生成 `2M+70+K`，其中 `M` 是该渠道实际文本模型数，`K` 是单独配置的 topology 验证请求数。旧 V2 的 `2M+42/2M+41` 只适用于旧同步容量流程，不能作为 sync/SSE 分离后的预算。`capacity-dry-run` 以 `M=3/K=4` 实测得到 `81/80`，并明确输出 `requests_sent=0`、`network_sent=false`。

工具就绪不改变本报告结论：XM authenticated `/models`、同步、SSE、计费、容量、共享灾备和 24–72 小时观察均未执行；目标拓扑仍为 `NOT_READY / NONFUNCTIONAL_EVIDENCE_INCOMPLETE`。详见 `docs/superpowers/reports/2026-07-21-upstream-sse-capacity-and-topology-nonfunctional-verification.md`。

### 前置等待字段

1. `XM API PLUS` 与 `XM API PRO` 各自的非秘密账号/分组标识、实际公开模型 ID、目标 Base URL 和协议兼容范围。
2. 每个账号的已配置并发上限、调度角色、出站 User-Agent 策略，以及是否共享底层额度或账号池。
3. 通过服务器端秘密引用安装的低额度临时测试凭据，及其余额/单次/总费用上限；Key 原文不进入聊天、命令、报告或台账。
4. 用户明确同意的测试工作负载：模型、每请求最大输出 Token、超时、总请求数与货币费用上限。
5. 每个模型的价格、可见账单取证路径、再分发条款状态，以及用于三方对账的精度限制。
6. 经批准的隔离下游用户/Key/分组/账号对象、清理责任人和停止阈值。未获批准时不创建这些对象。

### 阶梯与停止规则

1. **零费用预检：** V1 registry、Responses V2 profile 和两个 XM `--dry-run` 已通过；两次 dry-run 都是 `requests_sent=0`、`network_sent=false`。
2. **目录发现单独授权：** PLUS 和 PRO 各执行 1 次带临时凭据的 `GET /models`。在目录未知前不批准生成请求；先据实际文本模型数 `M` 计算下一阶段的固定请求上限。
3. **逐模型兼容性：** 公共 V2 对每个发现的文本模型执行 1 次同步和 1 次 SSE，最大输出均为 8 Token；每个渠道为 `2M` 个生成请求。记录状态、用量、TTFT、总时长和干净终止，不保存正文。
4. **同步容量阶梯：** 只对首个稳定文本模型执行并发 `1/2/3/5/8/10`，最多 `29` 请求；随后在 10 秒窗口执行目标 RPM `6/12/20/30`，实际最多 `1+2+4+5=12` 请求。当前公共 V2 不执行 SSE 并发阶梯，不能从同步阶梯推断 SSE 容量。
5. **旧 V2 上界：** legacy V2 每个渠道完整成功运行最多 `1` 次目录请求、`2M` 次逐模型生成和 `41` 次同步容量请求，即 HTTP 请求总数 `2M+42`、生成请求数 `2M+41`。新 v3 同时包含独立 SSE 容量阶梯，必须使用上文 `2M+71+K / 2M+70+K`；任一停止条件触发会提前结束，目录发现后仍须把 `M`、`K` 和货币费用上限重新提交用户确认。
6. **停止：** 任一 `429`、`5xx`、连接/TLS/协议错误、超时、SSE 未完成、未知/重复计费、余额阈值触发，或该阶的队列/总时长明显劣化时立即停止。已开始响应、SSE 或扣费状态未知时禁止跨上游重试，先对账。
7. **结论门槛：** 每阶记录样本数、P50/P95、错误率、测量位置和费用；最高稳定阶只能表述为“至少该级”。建议生产账号并发为最后稳定阶的 70%-80% 向下取整，且不得高于已映射的账号并发和共享池限制。SSE 并发保持 `unknown`，除非另行批准并执行公共的 SSE 容量扩展。
8. **清理与报告：** 禁用临时下游 Key、删除或停用经批准的隔离对象，复核原生产路由不变，再生成 secret-free proposal。只有用户对指定 proposal ID/hash 明确回复“采纳”后，才可进入隔离应用和后续生产变更流程。

## 首发判定

第一阶段确认了入口可达、TLS、HTTP 升级、服务健康和基础资源余量；完整分母复测为 `60/60` HTTP 200、传输错误 `0`，足以作为 D04 只读首发前的非功能起点。由于测量位置单一、静态页样本仍少，且 XM 映射/预算尚未就绪，本报告仍不授予任何入口 SLA、模型容量结论或 XM API 并发资格。
