# 管理员隔离测试站实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在当前域名的 `/admin/lab/` 路径下交付一个仅测试管理员可访问、完全隔离生产数据和外部副作用、可 reset 的 Sub2API 测试站。

**Architecture:** 复用当前 Sub2API 原生后端、管理员页面和 API 语义，新增独立 lab Compose project、独立 PostgreSQL/Redis/文件卷/密钥/服务身份，并由现有 Caddy 在生产路由前增加 `/admin/lab/` 专用入口。测试站所有支付、上游和通知均通过 fail-closed mock/录制适配器，生产服务不读取或写入 lab 资源；测试前端单独构建并固定 `/admin/lab/` base path。

**Tech Stack:** Go/Sub2API backend, Vue 3 + Vite + pnpm frontend, Docker Compose, PostgreSQL, Redis, Caddy, Bash contract tests, Vitest, Go focused tests。

**Spec:** `docs/superpowers/specs/2026-08-22-admin-lab-isolated-test-site-design.md`

## Global Constraints

- 只复用 Sub 原生后端、管理员 API、页面结构和迁移集合；不得新建第二套账务事实源或平行控制面。
- 测试站不得迁移、复制、回填或读取任何生产用户、订单、余额、usage log、账号或密钥。
- 测试站必须使用独立 PostgreSQL、Redis、Cookie 名称/Path、JWT/CSRF secret、管理员账号、文件卷和 Compose project/network。
- 真实支付、真实上游、真实通知、真实对象存储和生产出站默认 fail-closed；只能命中 mock/录制响应或测试 outbox。
- 生产 `/admin/`、`/api/`、SSE/WebSocket 和现有蓝绿发布链不得被测试站改写；不使用 GitHub Actions。
- 本计划不实现额度账本、充值包、经营分析、流式预占或退款业务；这些必须另立规格和任务包。
- 只有根发布总控可以修改根 `main`、全局队列/总账、推送、部署和线上验收；本任务候选不得自行合并或部署。
- 任何连接资源无法证明为 lab 资源时，启动、reset 和写操作必须拒绝继续。

## 文件地图

- Create: `infra/compose.admin-lab.yaml` — 独立 lab 前端/API/worker/PostgreSQL/Redis/mock 依赖编排。
- Create: `infra/.env.admin-lab.example` — 仅测试变量、独立命名和安全默认值模板，不含真实凭据。
- Create: `infra/admin-lab/Caddyfile.snippet` — `/admin/lab/` 路由、静态资源、API/SSE/WebSocket 代理和生产入口排除规则。
- Modify: `infra/Caddyfile` — 在生产 fallback proxy 前挂载 lab snippet；保持现有生产路由顺序和安全头。
- Create: `upstream/sub2api/frontend/vite.admin-lab.config.ts` — lab 前端固定 base/API/WS 前缀的构建配置。
- Create: `upstream/sub2api/frontend/src/admin-lab-entry.ts` — lab 入口和显式测试环境标识。
- Create: `upstream/sub2api/backend/internal/lab/guard.go` — lab 资源/endpoint/secret fail-closed 校验。
- Create: `upstream/sub2api/backend/internal/lab/guard_test.go` — 生产连接串、真实 endpoint、共享 secret 拒绝测试。
- Create: `tools/admin-lab/seed.go` 或 `tools/admin-lab/seed.sh` — 版本化、幂等种子和审计输出。
- Create: `tools/admin-lab/reset.sh` — 仅删除 lab project/database/Redis/volume 后重建并运行 seed。
- Create: `tools/admin-lab/mock-upstream` — 本地 mock/录制响应服务，拒绝非 lab 出站。
- Create: `tools/admin-lab/mock-payment` — mock 支付 provider 和 webhook，仅写 lab outbox。
- Create: `tests/admin_lab/compose_contract_test.sh` — Compose、env、network、volume、endpoint 静态合同。
- Create: `tests/admin_lab/caddy_route_test.sh` — Caddy 路径隔离和路由顺序合同。
- Create: `tests/admin_lab/auth_isolation_test.sh` — 未登录、普通用户、生产 Cookie、lab 管理员负向/正向矩阵。
- Create: `tests/admin_lab/seed_reset_test.sh` — seed 幂等、reset 隔离和 `LAB_ONLY` 标识验证。
- Create: `tests/admin_lab/mock_egress_test.sh` — 支付/上游/通知真实出站为 0 的验证。
- Create: `docs/handoffs/2026-08-22-t53-admin-lab-handoff.md` — 候选交接、验证、发布和回滚证据。

---

### Task 1: 建立任务候选与原生接口基线

**Files:**
- Modify: `docs/project/project-progress.md`（仅发布总控登记；任务 worktree 不得直接改全局总账）
- Create: `docs/handoffs/2026-08-22-t53-admin-lab-baseline.md`
- Test: `tests/admin_lab/native_reuse_inventory.sh`

**Interfaces:**
- Consumes: 当前 `upstream/sub2api/backend/internal/server/routes`、认证 middleware、管理员 handler/DTO、`upstream/sub2api/frontend/src/api`、现有 `infra/compose.yaml` 和 `infra/Caddyfile`。
- Produces: 原生接口复用表，明确每个页面/API/认证/迁移能力是直接复用、小幅扩展还是必须新增；列出禁止重复建设的事实源。

- [ ] **Step 1: 盘点原生入口和复用点**
  - 记录管理员登录、管理员 API 前缀、SSE/WebSocket 路径、静态资源构建方式、数据库迁移入口、Redis 使用点、支付 provider 注册点和通知 outbox。
  - 对每个目标能力写明“原生复用 / 原生扩展 / lab 新增隔离层”以及代码证据路径和行号。

- [ ] **Step 2: 写基线合同测试**
  - 测试脚本必须在生产代码仍未修改时通过，并检查不存在第二套余额/账务聚合实现。
  - 失败信息必须指出缺少的原生复用点，而不是笼统提示。

- [ ] **Step 3: 运行并提交基线证据**
  - Run: `bash tests/admin_lab/native_reuse_inventory.sh`
  - Expected: PASS，输出原生复用清单和当前 `main` SHA。
  - Commit: `docs: baseline native reuse for admin lab`

---

### Task 2: 创建独立 lab Compose、环境模板和 fail-closed guard

**Files:**
- Create: `infra/compose.admin-lab.yaml`
- Create: `infra/.env.admin-lab.example`
- Create: `upstream/sub2api/backend/internal/lab/guard.go`
- Create: `upstream/sub2api/backend/internal/lab/guard_test.go`
- Test: `tests/admin_lab/compose_contract_test.sh`

**Interfaces:**
- Consumes: Task 1 原生服务入口；Sub 原生迁移和启动参数。
- Produces: Compose project `sub2api-admin-lab`；服务 `admin-lab-api`、`admin-lab-worker`、`admin-lab-frontend`、`admin-lab-postgres`、`admin-lab-redis`、`admin-lab-mock-upstream`、`admin-lab-mock-payment`；Go `lab.ValidateConfig(cfg) error`。

- [ ] **Step 1: 写 Compose/env RED 合同**
  - `docker compose --project-name sub2api-admin-lab --env-file infra/.env.admin-lab.example -f infra/compose.admin-lab.yaml config` 应在模板值下成功。
  - 合同先断言独立 project/network/volume、独立 service name、非生产数据库/Redis host、非生产 cookie/JWT secret、mock-only provider 和禁止真实 endpoint。

- [ ] **Step 2: 实现最小 guard**
  - `ValidateConfig` 拒绝：生产数据库 host/DB 名、生产 Redis host、`sub2api` 生产 service alias、生产 Cookie 名称、空或复用 JWT/CSRF secret、非 mock payment、非 mock upstream、生产域名 allowlist、未设置 `LAB_ONLY` 标识。
  - 错误必须包含字段名和拒绝原因，不能打印 secret 值。

- [ ] **Step 3: 实现独立 Compose**
  - API/worker 使用独立 image tag 或本地构建上下文；数据库、Redis、卷和网络全部带 `admin-lab` 前缀。
  - API/worker 的数据库和 Redis 仅引用 `admin-lab-postgres`、`admin-lab-redis`；所有外部 URL 默认指向 compose 内 mock 服务。
  - healthcheck 必须从 lab 服务自身检查 `/health`，不能通过生产 Caddy 反查。

- [ ] **Step 4: 运行合同测试**
  - Run: `go test ./internal/lab -count=1`
  - Run: `bash tests/admin_lab/compose_contract_test.sh`
  - Expected: PASS；将真实生产连接串、生产 service alias 或真实 provider 注入模板时必须 FAIL-CLOSED。
  - Commit: `feat: add isolated admin lab compose and fail-closed guard`

---

### Task 3: 挂载 `/admin/lab/` 路由并隔离前端 base/API/WS

**Files:**
- Create: `infra/admin-lab/Caddyfile.snippet`
- Modify: `infra/Caddyfile`
- Create: `upstream/sub2api/frontend/vite.admin-lab.config.ts`
- Create: `upstream/sub2api/frontend/src/admin-lab-entry.ts`
- Test: `tests/admin_lab/caddy_route_test.sh`
- Test: `upstream/sub2api/frontend/src/admin-lab-entry.spec.ts`

**Interfaces:**
- Consumes: Task 2 的 `admin-lab-frontend:4173`、`admin-lab-api:8080` 和独立 session 配置。
- Produces: `/admin/lab/`、`/admin/lab/api/...`、`/admin/lab/assets/...`、`/admin/lab/ws/...` 的可预测路由；生产 `/admin/...`、`/api/...` 和默认 fallback 不被 lab matcher 截获。

- [ ] **Step 1: 写路由 RED 合同**
  - 断言 lab matcher 位于生产 fallback `reverse_proxy {$SUB2API_ACTIVE_UPSTREAM...}` 之前。
  - 断言 `/admin/lab/api/*`、SSE、WebSocket 使用 lab upstream，`/api/v1/*` 不被改写到 lab。

- [ ] **Step 2: 实现 Caddy snippet**
  - `/admin/lab` 308 到 `/admin/lab/`；`/admin/lab/assets/*` 只服务 lab 静态资源。
  - API 请求剥离外层 `/admin/lab/api` 前缀后转发到 lab API；转发时保留 Host/Forwarded headers，但不转发生产 Cookie 到 lab。
  - SSE/WS 单独设置 flush、长响应和 Upgrade 所需 transport；不得改变生产相同配置。

- [ ] **Step 3: 实现 lab 前端构建入口**
  - Vite `base: '/admin/lab/'`；API 默认 `/admin/lab/api/v1`；WS 默认 `/admin/lab/ws`。
  - 页面首屏显示“测试环境 / LAB_ONLY”，不写生产导航配置，不生成根路径资源。

- [ ] **Step 4: 验证**
  - Run: `bash tests/admin_lab/caddy_route_test.sh`
  - Run: `pnpm --dir upstream/sub2api/frontend exec vitest run src/admin-lab-entry.spec.ts`
  - Run: `docker run --rm -v "$PWD/infra/Caddyfile:/etc/caddy/Caddyfile:ro" -e SITE_ADDRESS=api.example.com caddy:2.10.2-alpine caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null`
  - Expected: PASS；生产 route contract 既有测试不回归。
  - Commit: `feat: mount isolated admin lab route`

---

### Task 4: 独立认证、管理员访问控制和敏感边界

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/auth*.go` 或现有认证配置入口（以 Task 1 盘点结果为准）
- Create: `upstream/sub2api/backend/internal/lab/auth_policy.go`
- Create: `upstream/sub2api/backend/internal/lab/auth_policy_test.go`
- Test: `tests/admin_lab/auth_isolation_test.sh`

**Interfaces:**
- Consumes: 原生管理员认证 middleware、`sub2api_lab_session`、独立 JWT/CSRF secret。
- Produces: `RequireLabAdmin(next http.Handler) http.Handler`；生产 session、普通用户身份和未登录请求均返回 401/403；lab 管理员只能访问 lab API。

- [ ] **Step 1: 写认证矩阵 RED 测试**
  - 覆盖：未登录页面/API、普通用户页面/API、生产 Cookie API、lab Cookie 非 admin、lab admin 页面/API、错误 Path Cookie、伪造 Authorization header。
  - 预期分别为 401/403/404，且不创建 lab session 或写入数据库。

- [ ] **Step 2: 实现 cookie/session 隔离**
  - 使用 `sub2api_lab_session`，`Path=/admin/lab`，独立签名密钥和过期时间；生产 Cookie 名称和 secret 不作为 fallback。
  - 反向代理层可叠加 Basic Auth/IP allowlist，但应用层鉴权不能省略。

- [ ] **Step 3: 实现 lab admin guard**
  - 所有 lab API、SSE、WS handler 在路由注册处统一包裹 `RequireLabAdmin`。
  - 任何 auth 解析失败都 fail-closed；日志只记录 reason code，不记录 cookie/token 值。

- [ ] **Step 4: 验证**
  - Run: `go test ./internal/lab ./internal/handler -run 'Lab|Auth' -count=1`
  - Run: `bash tests/admin_lab/auth_isolation_test.sh`
  - Expected: 正向仅 lab admin 成功，生产 Cookie 无法跨域复用。
  - Commit: `feat: isolate admin lab authentication`

---

### Task 5: Mock 支付、Mock 上游、通知 outbox 和出站阻断

**Files:**
- Create: `tools/admin-lab/mock-upstream/*`
- Create: `tools/admin-lab/mock-payment/*`
- Create: `upstream/sub2api/backend/internal/lab/egress_guard.go`
- Create: `upstream/sub2api/backend/internal/lab/egress_guard_test.go`
- Test: `tests/admin_lab/mock_egress_test.sh`

**Interfaces:**
- Consumes: 原生 payment provider interface、gateway upstream client、notification/outbox interface。
- Produces: mock provider names `lab-mock-payment`、`lab-mock-upstream`；所有通知写 `admin_lab_outbox`；`ValidateEgressTarget(url) error` 阻断非 lab endpoint。

- [ ] **Step 1: 写失败出站测试**
  - 注入真实支付 URL、真实上游 URL、生产 SMTP/Webhook/对象存储 URL，要求请求在 DNS/HTTP 连接前被拒绝。
  - mock 支付成功/失败/退款预留状态只返回确定性 fixture。

- [ ] **Step 2: 实现 mock-upstream**
  - 支持正常响应、上游失败、流式中断和固定 token/成本 fixture；每次请求带 `LAB_ONLY` trace 标识。
  - 默认只监听 Compose 内网，不发布宿主端口。

- [ ] **Step 3: 实现 mock-payment 与 outbox**
  - 支持创建订单、支付成功/失败、退款预留状态；webhook 只回调 lab API。
  - 通知事件写 lab outbox 表/日志，不调用生产 Feishu、SMTP、短信或 webhook。

- [ ] **Step 4: 验证**
  - Run: `bash tests/admin_lab/mock_egress_test.sh`
  - Expected: mock 用例通过，真实外部出站计数为 0，敏感 URL/凭据不进入日志。
  - Commit: `feat: add lab mocks and egress fail-closed controls`

---

### Task 6: 版本化种子、reset 和测试审计

**Files:**
- Create: `tools/admin-lab/seed.go` 或 `tools/admin-lab/seed.sh`
- Create: `tools/admin-lab/reset.sh`
- Create: `tools/admin-lab/seed-manifest.yaml`
- Create: `upstream/sub2api/backend/internal/lab/audit.go`
- Create: `upstream/sub2api/backend/internal/lab/audit_test.go`
- Test: `tests/admin_lab/seed_reset_test.sh`

**Interfaces:**
- Consumes: Task 2 lab database/Redis、Task 4 lab admin、Task 5 mock provider。
- Produces: 幂等 `seed --version v1`、安全 `reset`、`LAB_ONLY` 记录和 reset/seed/mock 账务审计事件。

- [ ] **Step 1: 写 seed manifest 和幂等 RED 测试**
  - manifest 固定测试管理员、用户 A-D、两个分组、三个模型、正常/失败/流式中断/跨额度 usage log、支付订单、充值记录、账号有效成本样本。
  - 两次 seed 的业务主键和记录数量必须一致，不产生重复订单或 usage log。

- [ ] **Step 2: 实现 seed**
  - 只连接 `admin-lab-postgres`；所有记录带 `LAB_ONLY` 来源标记，邮箱/API key/request ID 全为假值。
  - 用户 A/B/C/D 分别覆盖付费+赠送、仅赠送、待补缴、旧余额转付费额度场景；账本字段仅使用后续任务定义的接口，不在本任务实现账务规则。

- [ ] **Step 3: 实现 reset**
  - 校验 Compose project、数据库标识、Redis 标识和 `LAB_ONLY` 环境后，停止 lab 服务、删除 lab 数据库/Redis/卷、重新启动迁移并执行 seed。
  - 输出 JSON：`project`、`database`、`redis`、`seed_version`、`reset_at`、`result`；失败返回非零且不执行生产清理。

- [ ] **Step 4: 验证**
  - Run: `bash tests/admin_lab/seed_reset_test.sh`
  - Expected: reset 前后测试场景一致；生产数据库/Redis探针调用为 0；审计记录完整。
  - Commit: `feat: add reproducible lab seed and reset`

---

### Task 7: 集成启动、管理员 smoke 和生产路由回归

**Files:**
- Modify: `infra/compose.admin-lab.yaml`（仅集成修正）
- Modify: `infra/Caddyfile`（仅集成修正）
- Create: `tests/admin_lab/smoke_admin_lab.sh`
- Create: `docs/handoffs/2026-08-22-t53-admin-lab-handoff.md`

**Interfaces:**
- Consumes: Tasks 2–6 的 Compose、路由、认证、mock、seed/reset。
- Produces: 可重复启动/停止/reset 的完整测试站，管理员页面和关键写操作可用，生产入口无行为变化。

- [ ] **Step 1: 启动 lab 栈**
  - 使用独立 project：`docker compose --project-name sub2api-admin-lab --env-file infra/.env.admin-lab -f infra/compose.admin-lab.yaml up -d --wait`。
  - 启动前运行 guard；任何生产连接串、共享卷或真实 endpoint 直接终止。

- [ ] **Step 2: 执行管理员 smoke**
  - 登录 `/admin/lab/`，验证测试环境标识、管理员 dashboard、用户/分组/模型读取、mock 请求、支付 fixture、usage log 查询和 reset 入口。
  - 使用浏览器或 Playwright 只访问 lab 路径，保存成功和负向截图/响应证据。

- [ ] **Step 3: 执行生产路由回归**
  - 验证生产 `/healthz`、`/readyz`、`/health`、`/admin/`、`/api/v1/...`、既有 SSE/WS 路径仍指向生产 upstream。
  - 验证关闭 lab 栈后生产路由仍 200，`/admin/lab/` 返回 404/503 且不影响生产。

- [ ] **Step 4: 记录交接**
  - 交接文件必须包含基线 main SHA、候选 SHA、变更文件、测试命令/结果、迁移变化、配置变化、`downtime_required`、回滚方式、未验证项和证据路径。
  - Commit: `docs: hand off isolated admin lab`

---

### Task 8: 候选收口、发布预检和回滚演练（不自行部署）

**Files:**
- Modify: `docs/handoffs/2026-08-22-t53-admin-lab-handoff.md`
- Test: `tests/admin_lab/*.sh`
- Test: `tests/operations/deploy_sub2api_blue_green_host_test.sh`（只读调用，不改生产）

**Interfaces:**
- Consumes: 完整 lab 候选和所有直接相关测试。
- Produces: `READY_FOR_ROOT_REVIEW` 交接包，供根发布总控合并、预检、部署和线上/宿主专项验收。

- [ ] **Step 1: 运行最小直接验证矩阵**
  - Go focused tests：`go test ./internal/lab -count=1`。
  - Frontend focused tests：`pnpm --dir upstream/sub2api/frontend exec vitest run src/admin-lab-entry.spec.ts`。
  - Infra contracts：`bash tests/admin_lab/*.sh`、`git diff --check`。
  - Compose/Caddy：`docker compose ... config`、`caddy adapt`。
  - 不运行无关全仓压力、长时间 soak 或真实支付/上游验收。

- [ ] **Step 2: 生成发布预检输入**
  - 在候选 worktree 记录是否有迁移、Caddy 路由变化、Compose 服务变化、需不需要宿主停机；不得从候选直接部署。
  - 预检返回 `downtime_required=false` 才能进入根总控快速发布车道；若为 `true`，在任何停服/切换前暂停等待授权。

- [ ] **Step 3: 做本地回滚演练**
  - 停止 lab 服务、移除 lab Caddy 路由、重新加载生产 Caddy；确认生产健康端点和管理员入口恢复。
  - 回滚不得执行生产数据库恢复或迁移逆操作。

- [ ] **Step 4: 标记候选状态**
  - 交接结论只能写 `READY_FOR_ROOT_REVIEW`，不能写 `DONE`、“已部署”或“已生效”。
  - Commit: `chore: close admin lab candidate for root review`

## 验收总表

| 验收项 | 通过标准 |
|---|---|
| 路由 | `/admin/lab/` 独立可达；生产 `/admin/`、`/api/`、SSE/WS 不被截获 |
| 认证 | 仅 lab admin 成功；未登录、普通用户、生产 Cookie 均拒绝 |
| 数据 | 生产 DB/Redis 无连接、无写入；lab reset 可重复 |
| 外部副作用 | 真实支付、上游、通知、对象存储出站为 0 |
| 前端 | base/API/WS 均带 `/admin/lab/`；显示 LAB_ONLY |
| 种子 | 用户 A-D、分组/模型、usage/payment/cost fixture 可重复重建 |
| 安全 | secret、Cookie、API key、生产 URL 不进入日志 |
| 发布 | 候选仅交根总控；预检明确 `downtime_required`；回滚只移除 lab 路由和服务 |

## 计划自审

- **规格覆盖：** 目标/非目标由 Task 1–2 覆盖；路由和前端契约由 Task 3 覆盖；认证由 Task 4 覆盖；外部副作用由 Task 5 覆盖；种子/reset/审计由 Task 6 覆盖；生命周期、smoke、生产回归由 Task 7 覆盖；发布/停机/回滚由 Task 8 覆盖。
- **占位扫描：** 计划不使用未定义事项或“后续再补”的实现占位；所有步骤给出具体文件、命令、预期结果和提交边界。
- **接口一致性：** `ValidateConfig`、`RequireLabAdmin`、`ValidateEgressTarget`、`seed --version v1` 和 `reset` 在相邻任务中保持同名；前端 API/WS 前缀固定为 `/admin/lab/api/v1`、`/admin/lab/ws`。
- **范围检查：** 额度账本和经营分析不在任何任务中实现，只提供脱敏场景数据承载能力；无第二账务事实源、无生产数据迁移、无真实外部调用。
