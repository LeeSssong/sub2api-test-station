# refactorUIUXv0.1 Sub2API 后端 Readiness 设计

> 日期：2026-09-20  
> 状态：已批准。批准依据：本任务是 PD-19 后阶段 2 已确定的后端 `/readyz` 阻断项；用户连续要求“继续”，且 `docs/project/native-sub-incremental-delivery-constraints.md` 2.3 授权发布总控代审既定队列内规格。  
> 基线：`origin/main@645ce06834698cebcd6707836a234d7096e0a081`，tree `d16f0f3f13d2f837587b73455d22aa3ac9eef540`。

## 1. 问题证据与当前行为

Sub2API 原生后端当前只在 `internal/server/routes/common.go` 注册 `/health`，固定返回 HTTP 200 JSON `{"status":"ok"}`。它表达进程存活，不检查 PostgreSQL 或 Redis。

后端没有 `/readyz` 路由。启用 `embed` 构建时，`internal/web/embed_on.go` 的前端中间件只绕过 `/health`，未知 `/readyz` 会落入 SPA fallback，返回 HTTP 200、`text/html` 和 `index.html`。独立测试站两份 Caddy 配置已经把 `/readyz` 精确反代至 API，因此当前外部 HTML 响应来自后端嵌入前端 fallback，不是 Caddy 路由缺失。

阶段 2 的测试站发布控制器合同要求 `/readyz` 为 HTTP 200、JSON Content-Type 且 `status=ready`；HTML、非 200、非法 JSON 或其他状态必须 fail closed。因此后端 readiness 是发布控制器进入真实发布前的独立阻断项。

基线额外存在两条与本任务无关的 `internal/server/routes` 全包测试失败：旧测试仍期待英文网关错误文案，而当前实现返回已本地化文案。本任务不修改这些网关合同，只运行直接相关的定向测试和编译验证。

## 2. 目标与非目标

### 2.1 目标

- 保留 `/health` 作为无依赖 liveness，行为不变。
- 新增原生后端 `/readyz`，以 PostgreSQL 与 Redis 的实时可达性判断 API 是否可接流量。
- readiness 探测使用请求级总超时，避免依赖卡死拖住 HTTP worker。
- 成功返回 HTTP 200、`application/json`、`{"status":"ready"}`。
- 任一依赖失败、超时或 readiness checker 不可用时返回 HTTP 503、`application/json`、`{"status":"not_ready"}`。
- 失败响应不暴露依赖名、连接串、主机、端口或底层错误。
- `embed` 和 legacy 嵌入前端路径都绕过 `/readyz`，确保路由处理器接管请求。
- Wire 生成代码与手写 provider 签名保持一致。

### 2.2 非目标

- 不修改数据库 schema、migration、Redis 数据、业务 API、前端页面或用户 UX。
- 不把 `/health` 改成依赖探针。
- 不检查 worker、detector、Caddy、外部上游、支付、邮件、对象存储或第三方 API；这些由 Compose 健康和发布控制器分别判断。
- 不修改主站根 Caddy 的 `/readyz` 所有权；生产根路径继续由 relay-ops 使用。
- 不修改独立测试站 Caddy；它已正确反代 `/readyz`。
- 不合并、推送或部署测试站/主站。
- 不顺带修复基线网关错误文案测试。

## 3. 影响范围与边界条件

直接影响对象是测试站发布控制器、容器编排和运维探针。普通用户不直接使用该接口。

边界条件：

- PostgreSQL 正常、Redis 正常：ready。
- PostgreSQL 失败：not ready；无需继续探测 Redis。
- PostgreSQL 正常、Redis 失败：not ready。
- 任一探测阻塞至总超时：not ready。
- 请求被客户端取消：not ready，并尽快结束。
- `/health` 在依赖失败时仍返回 `status=ok`，用于区分进程存活与接流量能力。
- `/readyz` 不受认证、管理员权限、面板限流或 SPA fallback 影响。

## 4. 方案比较

### 方案 A：由 Caddy 固定返回 readiness JSON

实现最少，但只能证明 Caddy 存活，无法证明 API、PostgreSQL 或 Redis 可用；会让发布控制器误判，拒绝。

### 方案 B：后端 `/readyz` 固定返回 ready

能消除 HTML fallback 并满足响应形状，但语义只是 `/health` 的别名，不能称为真实 readiness；依赖故障时仍会接流量，拒绝。

### 方案 C：后端对 PostgreSQL 与 Redis 做有界探测（采用）

复用应用已经持有的 `*sql.DB` 与 `*redis.Client`，不新增事实源或后台子系统。请求内执行低成本 ping，总超时后 fail closed；这与 API 的核心运行依赖一致，范围也保持最小。

## 5. 架构与组件

### 5.1 Readiness checker

在 `internal/server/routes` 内定义窄接口：

```go
type ReadinessChecker interface {
    Check(context.Context) error
}
```

原生实现持有两个窄 pinger：数据库的 `PingContext(context.Context) error` 与 Redis 的 `Ping(context.Context) *redis.StatusCmd`。生产构造函数接受现有 `*sql.DB`、`*redis.Client`。检查顺序固定为 PostgreSQL 后 Redis；首次错误立即返回，不记录或响应底层错误正文。

### 5.2 HTTP 合同

`RegisterCommonRoutes` 接收 `ReadinessChecker`。`GET /readyz` 从请求 context 派生固定 2 秒总超时：

- `Check` 返回 nil：HTTP 200，JSON `status=ready`。
- `Check` 返回错误、context deadline/cancel 或 checker 为 nil：HTTP 503，JSON `status=not_ready`。

`GET /health` 保持原实现。

### 5.3 依赖注入

`server.ProvideRouter` 与 `SetupRouter` 增加 `*sql.DB` 参数，并在注册路由时构造 readiness checker。`cmd/server/wire_gen.go` 必须通过 Wire 重新生成，不能只手工改生成物；生成结果应把现有 `db` 传入 `ProvideRouter`。

### 5.4 嵌入前端边界

`shouldBypassEmbeddedFrontend` 增加精确路径 `/readyz`。`FrontendServer.Middleware` 和 legacy `ServeEmbeddedFrontend` 共用该函数，因此一次规则修改覆盖两条路径。只精确绕过 `/readyz`，不把任意相似 SPA 路径排除。

## 6. 端到端控制流

```text
GET /readyz
  -> Caddy 精确反代至 test-station-api:8080
  -> embedded frontend middleware 识别 /readyz 并 c.Next()
  -> Gin /readyz handler 创建 2 秒 context
  -> PostgreSQL PingContext
  -> Redis Ping
  -> 全部成功：200 application/json {"status":"ready"}
  -> 任一步失败/超时：503 application/json {"status":"not_ready"}
  -> 发布控制器再执行连续三次成功门禁
```

`/health` 不进入上述依赖流，始终只证明 HTTP 进程可响应。

## 7. 接口与字段契约

成功：

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{"status":"ready"}
```

失败：

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json; charset=utf-8

{"status":"not_ready"}
```

响应只包含一个 `status` 字段。不得加入 `database`、`redis`、`error`、`reason`、DSN 或连接详情。发布控制器只依赖状态码、Content-Type 与 `status`。

## 8. 失败与安全语义

- readiness 默认 fail closed；nil checker、依赖错误和超时均为 503。
- 不把底层错误回显给未认证公网请求。
- 不在 readiness handler 中重连、修复、迁移、写数据或触发后台任务。
- 每次请求只执行只读 ping；不缓存成功结果，避免陈旧 ready。
- 使用总超时防止串行探测无限延长；第二项只能使用剩余 context 时间。
- 不改变现有认证、CORS、安全头和请求日志中间件顺序。

## 9. 兼容性与迁移

无数据库或配置迁移。`ProvideRouter`、`SetupRouter` 与 `RegisterCommonRoutes` 是仓库内部构造接口，调用点需同步编译更新。外部 API 只新增 `/readyz`，不修改任何已有响应。

主站根 Caddy 已把 `/readyz` 分配给 relay-ops，因此本任务不会改变生产公网 `/readyz` 的现有含义。独立测试站 Caddy 已将它转发至 Sub2API API，候选部署后会得到新合同。

## 10. 验收矩阵

| 场景 | `/health` | `/readyz` | 预期正文 |
|---|---:|---:|---|
| DB 与 Redis 可达 | 200 | 200 | `ok` / `ready` |
| DB 不可达 | 200 | 503 | `ok` / `not_ready` |
| Redis 不可达 | 200 | 503 | `ok` / `not_ready` |
| checker 超时 | 200 | 503 | `ok` / `not_ready` |
| embed 构建请求 `/readyz` | 不适用 | 由 Gin handler 接管 | 绝不返回 HTML |
| legacy embed 请求 `/readyz` | 不适用 | 由 Gin handler 接管 | 绝不返回 HTML |
| 普通 SPA 路径 | 不变 | 不适用 | 继续返回 index.html |

## 11. 测试策略

按 TDD 实施：

1. 为 common routes 新增单元测试，先证明缺少 `/readyz`、依赖失败/超时合同尚未满足。
2. 为 checker 新增数据库与 Redis 成功/失败短路测试。
3. 在 `embed` build tag 下把 `/readyz` 加入 modern 与 legacy bypass 表；先验证当前失败，再修改实现。
4. 运行 readiness 定向测试、`go test -tags embed ./internal/web`、`go test ./internal/server` 和 `go test ./cmd/server`，确保依赖注入和生成代码编译。
5. 运行 `go test ./internal/server/routes` 时预期仍可能命中已确认的两条无关基线文案失败；若失败集合扩大，任务不得收口。

## 12. 发布、线上验证与回滚条件

候选完成状态只能是 `READY_FOR_ROOT_REVIEW`。不得合并、推送或部署。

未来合入并推送根 `main` 后，测试站发布前必须由干净根 `main == origin/main` 发起。线上验证至少包括：

- API 容器内 `/health` 为 JSON `status=ok`。
- API 容器内 `/readyz` 为 JSON `status=ready`。
- 经测试站 Caddy 的 `/readyz` Content-Type 与 JSON 合同一致。
- 发布控制器连续三次 readiness 门禁通过。
- 临时阻断 PostgreSQL 或 Redis 时 `/readyz` 返回 503，而 `/health` 仍为 200；仅在受控测试环境执行。

回滚为恢复上一应用 release；不删除或恢复 PostgreSQL、Redis、app-data named volumes。若旧 release 没有真实 `/readyz`，补强发布控制器的回退 readiness 复验会 fail closed，但仍保留回退应用与证据，需人工确认。

`downtime_required=false`：只新增读探针与路由，无 schema、配置或数据变化。

## 13. 未决事项

无产品未决项。基线两条网关错误文案测试失败不属于本任务，保留为独立修复事项，不阻断 readiness 定向候选，但必须在最终报告中列出。
