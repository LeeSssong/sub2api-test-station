# T79 独立准生产验收站设计规格

## 1. 问题证据与当前行为

当前仓库有两类不适合作为正式验收站的部署形态：

- 生产 Compose/生产 Caddy 使用 `sub2api` project、生产域名、生产数据库/Redis/对象存储和生产凭据，不能承担隔离验收。
- 旧 `infra/compose.admin-lab.yaml` 与 `/admin/lab/` 是 mock 环境：`PAYMENT_PROVIDER=mock`、`UPSTREAM_PROVIDER=mock-upstream`、`NOTIFICATION_TRANSPORT=lab-outbox`，并通过生产 Caddy 网络暴露，不能证明真实充值、消费、支付或上游链路。

本任务把旧 admin lab 退役，新增一个完整但不对外开放的验收实例。验收站本身仍是可商用的真实 Sub2API：真实上游、支付、通知和账户数据均由验收站自己的配置提供。

## 2. 目标

1. 新功能在本地完成直接相关验证后，可由管理员将候选版本串行部署到固定验收站。
2. 验收站拥有独立 Caddy、Docker Compose project/network、PostgreSQL、Redis、对象存储/应用数据目录、账户/会话、支付/上游/通知凭据和管理员身份；对外复用主站域名 `https://api.xingqiaolab.top/admin/lab/` 路径，生产 Caddy 仅作外层入口。
3. 验收站只允许管理员登录：`backend_mode_enabled=true` 且 `registration_enabled=false`，非管理员用户路由和注册被原生后端拒绝。
4. 验收站支持真实充值、消费、支付、上游请求和通知，不含 mock provider/service。
5. 串行重复部署时保留验收站数据和 named volumes；失败只回滚镜像/配置，不清理数据。
6. 发布链简单快速：本地镜像构建 -> 传输到验收站 -> Compose 更新 -> bootstrap 管理员模式 -> 健康检查；不引入蓝绿、临时环境、自动晋级或额外审批。

## 3. 非目标

- 不修改生产 Compose/Caddy、生产数据库、生产 Redis、生产凭据或生产账户。
- 不新增域名；不复用生产 Docker project/network、生产 volumes 或生产环境文件。生产 Caddy 只共享 TLS/外层路径反代，并由 `ACCEPTANCE_LAB_ALLOWED_IPS` 默认拒绝未授权来源，不接入验收内部网络或数据。
- 不保留、扩展或迁移 `/admin/lab/` mock 路由和 mock 服务。
- 不实现自动从验收站晋级生产、不实现候选 per-branch 环境、不实现蓝绿槽。
- 不猜测验收站宿主、域名、支付/上游凭据；这些值由 operator 在独立宿主、本地 0600 env 文件或验收站原生后台配置中安装。
- 不扩大到全仓回归或强制新的质量门禁；直接相关测试和发布链保护即可。

## 4. 方案比较与选择

### 方案 A：固定独立单实例（采用）

一个长期存在的 `sub2api-acceptance` Compose project，API/worker/detector/PostgreSQL/Redis/Caddy 均为独立服务和命名资源。每次部署只替换 API/worker/detector 镜像与配置，保留数据库和 Redis volumes。优点是实现最小、启动快、数据可持续、最贴合串行人工验收；缺点是同一时刻只有一个候选。

### 方案 B：每个候选创建临时验收站

隔离更强但需要动态域名、更多资源、数据清理和生命周期管理，增加部署时间与失败面，不符合小步快跑。

### 方案 C：在生产 Compose 中增加验收服务

表面上复用基础设施，但会共享 Caddy、网络或凭据边界，无法证明真实隔离，且容易误写生产数据，排除。

## 5. 架构与端到端控制流

```text
本地直接测试
    │
    ▼
release-sub2api-acceptance.sh
    ├─ 校验 0600 验收 env、非生产标识、真实 provider 声明
    ├─ 从当前 worktree 构建一个不可变 Linux/amd64 镜像并保存归档
    ├─ 传输 bundle、镜像归档、env 到验收宿主 staging
    ▼
deploy-sub2api-acceptance-host.sh
    ├─ 安装 root-owned compose/Caddy/env（旧数据卷不动）
    ├─ docker compose --project-name sub2api-acceptance up -d --wait
    ├─ acceptance-bootstrap（一次性、幂等）写入本实例 settings：
    │     backend_mode_enabled=true, registration_enabled=false
    ├─ 检查 API/worker/detector/PostgreSQL/Redis/Caddy healthy
    └─ 通过主站 `/admin/lab/` 路径检查 /health 与管理员登录入口
    ▼
管理员真实验收
    ├─ 独立管理员身份
    ├─ 真实上游请求/消费
    ├─ 真实支付/充值回调
    └─ 真实通知链路
    ▼
人工合入 main → 使用既有生产发布链部署生产
```

验收站 Compose 不加入任何生产或共享外部网络；API、worker、detector 和 Caddy 仅额外接入专属 `sub2api-acceptance-egress-network` 以访问真实上游、支付和通知。Caddy 只反代 `acceptance-api:8080`；PostgreSQL、Redis 只在 `sub2api-acceptance-network` 内可达。API/worker/detector 使用同一个候选镜像，避免验收站出现代码漂移。

## 6. 文件与接口契约

### 新增文件

- `infra/compose.acceptance.yaml`：独立 project、服务、named volumes 和内部网络。
- `infra/Caddyfile.acceptance`：验收站专用 edge HTTP 反代和 15 分钟慢上传窗口，接收 `/admin/lab/` 前缀并剥离后转发。
- `infra/.env.acceptance.example`：只列 operator 必填/可选变量，示例值明确不可直接部署。
- `ops/release-sub2api-acceptance.sh`：本地构建、校验、打包和 SSH/SCP 发布控制器。
- `ops/deploy-sub2api-acceptance-host.sh`：验收宿主安装、Compose 更新、bootstrap、健康检查和可恢复回滚。
- `tests/acceptance_station/compose_contract_test.sh`：Compose 隔离和真实 provider 合同。
- `tests/acceptance_station/release_delivery_contract_test.sh`：控制器/宿主执行器/禁区合同。
- `tests/acceptance_station/auth_mode_contract_test.sh`：管理员模式 bootstrap 和注册关闭合同。

### 环境契约

`infra/.env.acceptance.example` 必须包含：

- `ACCEPTANCE_SITE_ADDRESS=api.xingqiaolab.top`、`ACCEPTANCE_LOOPBACK_PORT`、`ACCEPTANCE_DEPLOY_ROOT`、`ACCEPTANCE_PROJECT_NAME`、`ACCEPTANCE_NETWORK_NAME`；生产 Caddy 另需受保护的 `ACCEPTANCE_LAB_ALLOWED_IPS` 来源网段配置。
- `ACCEPTANCE_IMAGE`（由发布控制器写入候选镜像标签）和 `ACCEPTANCE_POSTGRES_IMAGE`、`ACCEPTANCE_REDIS_IMAGE`、`ACCEPTANCE_CADDY_IMAGE`。
- 独立 `ACCEPTANCE_DB_*`、`ACCEPTANCE_REDIS_PASSWORD`、`ACCEPTANCE_ADMIN_EMAIL`、`ACCEPTANCE_ADMIN_PASSWORD`、`ACCEPTANCE_JWT_SECRET`、`ACCEPTANCE_TOTP_ENCRYPTION_KEY`。
- `ACCEPTANCE_PAYMENT_PROVIDER`、`ACCEPTANCE_UPSTREAM_PROVIDER`、`ACCEPTANCE_NOTIFICATION_TRANSPORT` 和 `ACCEPTANCE_REAL_FLOW_ACK=I_UNDERSTAND_REAL_CHARGES`。三个 provider 值禁止 `mock`、`mock-upstream`、`lab-outbox`。
- `ACCEPTANCE_SSH_TARGET`、`ACCEPTANCE_SSH_PORT`、`ACCEPTANCE_SSH_KEY`、`ACCEPTANCE_SSH_KNOWN_HOSTS` 由 operator 在 shell 中提供，不写入仓库。

发布控制器必须拒绝：`shop.xingqiaolab.top`、生产根 `/opt/sub2api/production`、生产 project `sub2api`、生产 network `sub2api_default`、生产 env 路径和所有 mock provider 值；允许且必须使用 `api.xingqiaolab.top` 的 `/admin/lab/` 路径入口。

### 管理模式契约

`acceptance-bootstrap` 只连接 `acceptance-postgres`，使用本实例数据库账号执行幂等 SQL：

```sql
INSERT INTO settings (key, value) VALUES
  ('backend_mode_enabled', 'true'),
  ('registration_enabled', 'false')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
```

不调用生产 API、不连接生产数据库、不写生产 Redis。管理员后续仍可在验收站后台按业务需要配置真实支付/上游/通知实例。

## 7. 失败、安全与回滚语义

- 缺少 env、env 非 0600、路径为符号链接、生产标识、mock provider 或未确认真实消费时 fail-closed，不能开始传输或启动容器。
- Compose 配置、镜像加载、bootstrap、服务健康或验收站 URL 检查失败时，执行器恢复上一份 compose/Caddy/env（若存在）并重新拉起上一版本；不删除 named volumes、不清空数据库、不删除 Redis 数据。
- 首次安装失败只停止本次服务并保留 volumes；执行器和控制器会尝试删除远程 staging 中的 env、镜像归档和 bundle，清理失败时明确告警并由 operator 立即处理；正常情况下只保留脱敏日志供人工修复后重试。
- 验收 Caddy 只监听同宿主的专用 `ACCEPTANCE_LOOPBACK_PORT` edge 端口；生产 Caddy 继续拥有公网 80/443。该端口由宿主防火墙限制为生产 Caddy/管理员来源；API、worker、detector、数据库和 Redis 不发布宿主端口。
- env、镜像归档和 staging 目录使用 0600/0700；宿主运行文件由 root 安装，远程 staging 在退出时清除。脚本输出不得打印密码、token、cookie 或支付/上游密钥。
- 站点“不对外开放”由主站路径的管理员来源 ACL/防火墙、受限 acceptance edge 端口与仅管理员登录共同保证；代码不把管理员模式误当作网络 ACL，operator 必须在生产 Caddy/宿主层限制来源。

## 8. 兼容性与迁移

- 不新增数据库迁移；`settings` 表由 Sub 原生初始化，bootstrap 只更新本实例已有键值。
- 不改变生产 Compose、生产 Caddy 或现有生产发布链。
- 验收站在后续版本升级时继续使用同一 named volumes 和独立 env，数据跨部署保留。
- 旧 mock lab 文件保留为历史只读证据；生产 Caddy 的 `/admin/lab/` 路由在本任务中改为指向验收站 loopback，旧 mock stack 退役动作由 operator 在新站稳定后单独执行。

## 9. 场景化验收矩阵

| 场景 | 期望 |
|---|---|
| 缺少验收 env/权限不是 0600 | 发布控制器在本地失败，不连接宿主 |
| 使用生产域名、project、network 或 deploy root | 发布控制器 fail-closed |
| provider 值为 mock/lab-outbox | 发布控制器和 Compose 合同失败 |
| 完整首次安装 | 6 个核心服务 healthy，bootstrap 成功，主站路径 `/admin/lab/health` 200 |
| 匿名访问登录入口 | 可看到登录页，但注册/自助流程被后端拒绝 |
| 管理员登录 | 独立管理员可登录并访问后台 |
| 非管理员 token 请求用户路由 | 原生 backend mode 返回 403 |
| 重复部署新候选 | API/worker/detector 更新，PostgreSQL/Redis/Caddy 身份和数据卷保留 |
| 新候选启动失败 | 回滚上一配置/镜像，数据卷不删除 |
| 真实上游请求/消费 | 请求只使用验收站自己的账号、余额和上游凭据 |
| 真实支付/充值回调 | 回调只写验收站数据库；生产数据库无变化 |
| 人工验收通过后进入主站 | 由管理员单独执行合入与既有生产发布，不由验收脚本自动晋级 |

## 10. 测试与发布策略

- 先运行三份 acceptance_station 合同测试和 `docker compose ... config --quiet`；再运行脚本 `bash -n`、`git diff --check`。
- 发布控制器只做本地构建、归档校验和宿主部署；不调用 GitHub Actions。
- 本任务不运行全仓测试、压力测试或浏览器穷举；真实支付/上游专项由管理员在验收站人工执行。
- 预期 `downtime_required=false`：单实例更新允许短暂重启，但不停止生产服务；若 operator 要求零停机，应另立任务，不在本任务引入蓝绿。
- 回滚条件：任一服务不健康、bootstrap 未验证管理员模式、站点 URL 非 200、发现生产标识或发现 mock 依赖。

## 11. 发布与回滚操作边界

验收站发布脚本需要明确的 operator SSH 目标、主站域名路径与 loopback 端口和 0600 env；仓库不提供真实值。脚本成功只表示验收站部署成功，不表示功能验收通过，也不表示允许合入主站。只有管理员人工验收通过后，才可从已经验证的 `main` 执行现有生产发布链。

## 12. 待决事项与批准记录

- 待 operator 提供：验收站宿主或独立运行目录、loopback 端口/生产 Caddy ACL、真实支付实例、真实上游账号、真实通知通道、SSH key/known_hosts。
- 本规格不等待用户逐份批准；用户已在 2026-08-26 明确授权“规格书写完直接实施”，并确认测试站与主站完全独立、真实可商用、管理员专用、串行部署和敏捷小步发布。
