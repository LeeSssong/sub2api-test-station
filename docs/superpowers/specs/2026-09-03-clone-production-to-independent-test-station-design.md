# 独立测试服务器主站部署副本规格

## 1. 问题证据与当前行为

独立服务器 `49.51.203.200` 当前运行的是旧迁移栈：API/worker 使用镜像 `sub2api-acceptance:d8a306a16...`，Compose 仍使用 `ADMIN_LAB_*` 配置和旧测试站拓扑；前端返回 `Sub2API - AI API Gateway`，而主站根路径返回独立星桥首页。当前栈没有 source commit/tree 与镜像 digest 的绑定，无法证明它来自主站当前发布制品。

主站当前发布身份为 source commit `e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b`、source tree `ee671c292487d5e005beb696e02593e766146248`。主站运行 API、worker、model-detector 使用同一不可变镜像；Caddy 另提供首页静态制品、文档和管理路由。

## 2. 目标与非目标

目标：在独立服务器上重建主站的应用部署形态，使业务代码、前端首页、Caddy 路由、API、worker 和 model-detector 与主站当前运行基线内容对齐；根路径通过裸 IP 提供与主站一致的星桥首页；保留独立测试站的数据库、Redis、应用数据、管理员凭据、测试 provider/mock 配置和独立 Docker project/network/volumes。测试站发布源使用包含本任务部署代码的最终已推送 `main`，并记录该提交；业务版本一致性使用 Sub2API 与 homepage 源码子树哈希、制品哈希和路由合同证明，不能把不同 Git 提交伪称为同一提交。

非目标：不把测试站挂回主站 `/admin/lab`；不复制主站 PostgreSQL/Redis/对象存储、生产账号、支付/上游/通知凭据；不改主站；不使用 GitHub Actions；不删除旧测试栈或原始备份，直到新栈完成验证并由用户另行决定。

## 3. 方案比较与选择

方案 A（采用）：以主站生产 Compose/Caddy 为模板，在独立服务器建立同构但改名的 Compose project。应用镜像从最终根 `main` 的相同生产构建入口生成，homepage 从同一源码树构建独立镜像；所有测试站秘密、卷、网络和 provider 指向独立资源。对账同时记录测试站发布提交，以及它与主站当前 `e00c37e0e...` 的 Sub2API/homepage 子树哈希关系。优点是运行时和主站可逐项对账，且隔离边界清晰。

方案 B：继续修补现有测试站 Compose，仅替换 API/前端镜像。不能覆盖主站首页、Caddy 路由、detector 和生产拓扑差异，且保留旧镜像/旧配置漂移风险，不采用。

方案 C：直接复制主站生产 Compose、`.env`、卷和 secrets。表面一致但会复制生产身份与数据，违反验收站隔离和凭据边界，不采用。

## 4. 端到端控制流

1. 在根目录干净且已推送的最终 `main` 上确定 source commit/tree，确认其 Sub2API/homepage 子树相对主站运行基线是否有内容变化，并生成带 commit/tree/subtree/digest 标签的 Sub2API 与 homepage 制品。若业务子树已前进，则以发布时主站实际运行版本为目标基线重新判断，不把未部署到主站的新业务内容冒充主站版本。
2. 在独立服务器建立专用 Compose project、internal 应用网络、边缘网络、PostgreSQL/Redis/app-data 卷和受限配置目录；边缘代理仅监听该服务器的 80/IPv6 80。
3. 在切换前只读核对旧测试站容器、数据卷、备份 checksum，以及主站运行容器健康；停止或保留旧栈仅限测试站专用 project。
4. 将新制品、同构 Compose/Caddy、脱敏配置模板和独立测试站秘密传输到服务器；服务器端再次校验 checksum 与 source identity。
5. 启动数据库/Redis，恢复已确认属于测试站的备份；启动 detector、API、worker、homepage、gateway 和 Caddy，等待所有健康检查通过。
6. 通过 IPv4/IPv6 验证 `/`、`/login`、`/health`、`/readyz`、API 路由、静态首页 assets 和登录会话；只读核对主站健康与容器身份未改变。
7. 记录发布状态、source commit/tree、各镜像 digest、容器身份、备份 checksum、数据行数/容量和回滚点。

## 5. 接口与字段契约

- 根首页：`GET /`、`GET /home/` 返回主站同一 homepage HTML/asset manifest；不得返回 Sub2API 默认登录首页。
- 应用入口：`GET /login`、`/admin/*`、`/api/*`、`/ws/*` 按主站 Caddy 路由语义工作，但使用独立测试站 cookie 命名空间和独立 API base URL。
- 健康：`GET /health`、`GET /readyz` 返回 HTTP 200；应用和 detector 健康检查不得访问主站网络或生产数据。
- 发布身份：Sub2API 应用镜像必须带 `com.xingqiao.sub2api.source.commit`、`source.tree`、`tested.tree`、`migrations.sha256` 标签；homepage 镜像必须记录 source commit/tree 和 homepage subtree hash；运行记录必须保存这些字段及 image digest。另保存与主站基线对账的 Sub2API/homepage subtree hash，明确区分“部署代码提交”和“业务内容基线”。
- 配置：测试站使用独立变量命名空间和独立 secrets 文件；不得将生产 `.env`、API key、JWT/TOTP/支付/通知 secret 写入仓库或日志。

## 6. 失败、安全与回滚

构建、传输、身份校验、数据恢复、健康检查、首页 manifest 对账或隔离检查任一失败时，发布停止，不切换新入口；保留新栈现场、日志和 checksum。新栈验证失败时仅停止独立测试 project，并从原始备份恢复旧测试栈；不得执行全局 `docker compose down`、删除卷或触碰主站 project。

裸 IP 管理入口在未绑定 HTTPS/VPN/IP 限制前只作为短期测试入口；不把该入口宣称为生产安全入口。所有远程命令使用受保护 SSH key/known_hosts，秘密只从本机 0600 文件读取。

## 7. 兼容性与迁移

迁移采用 expand/restore 语义：保留测试站现有 PostgreSQL、Redis、`/app/data` 和上传文件数据；不回填或重算生产账务，不导入主站数据。若主站当前 migration set 与测试备份不一致，先执行应用迁移并只在测试站卷上恢复；迁移失败则回滚到旧测试栈和原始备份。

## 8. 场景化验收矩阵

| 场景 | 通过条件 |
|---|---|
| 版本身份 | 测试站发布 source commit/tree 为最终已推送 `main`；Sub2API/homepage 子树、制品与主站实际运行基线内容一致，digest 可追溯 |
| 首页 | 裸 IP `/` HTML、标题、`/home-assets/*` manifest 与主站一致 |
| 登录与路由 | `/login`、管理员路由、API、WebSocket 均可用，资产路径无 `/admin/lab` 前缀 |
| 健康 | API、worker、detector、PostgreSQL、Redis、Caddy healthy，三项健康端点 200 |
| 数据 | 测试用户/账号/账单/文件恢复行数和 checksum 与测试备份一致，主站数据未读取 |
| 隔离 | project/network/volume/container/env/凭据均为测试站专用；无主站网络或卷挂载 |
| 重启 | 新栈重启后数据和健康状态保持，旧栈可按回滚步骤恢复 |
| 双栈影响 | 主站 API/worker/detector/Caddy 持续 healthy，运行 source/digest 不变 |

## 9. 测试策略

实施前运行现有 Compose/Caddy 合同与来源门禁测试；实施中对构建产物做 source/tree/digest、HTML/asset manifest、配置渲染和网络/卷检查；部署后执行 IPv4/IPv6 HTTP 探针、登录态功能验收、健康检查、只读数据库对账和主站不变性检查。禁止用真实生产支付、上游消费或通知流量制造验收数据。

## 10. 发布、验证与回滚条件

发布只能从根目录干净且 `HEAD == origin/main` 的 `main` 发起；当前候选需先合入并推送。独立服务器发布不等同于主站授权，不触发主站部署。只有新栈全部场景通过后，才可将其标记为已部署；若失败，保留旧栈、备份和新栈证据，不删除候选或恢复材料。

## 11. 待决事项与批准记录

实施时记录而非产品待决项包括：测试备份的最终 checksum/行数、主站 homepage 运行 manifest、独立服务器 Docker/Compose 版本和 IPv6 防火墙状态。homepage 固定从最终已推送 `main` 的 `homepage/` 使用仓库既有生产构建方式生成，并与主站运行 manifest 做内容对账。业务范围已由用户于 2026-09-03 明确批准：独立测试服务器应完全克隆主站部署形态和差异内容，同时保持测试站数据、凭据和运行资源独立。
