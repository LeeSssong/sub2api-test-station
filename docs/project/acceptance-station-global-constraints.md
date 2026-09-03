# 验收站与主站发布全局约束

> 生效日期：2026-09-03
>
> 适用范围：根线程、所有功能线程、审查线程、运维线程和发布线程。

> **重要纠偏：** 本文件中的“验收站”现在专指新建的独立测试站，不是主站上的历史 `/admin/lab` 路径。旧 `/admin/lab`、`sub2api-acceptance`、`/opt/sub2api/acceptance-live` 和旧验收 env 只保留为历史证据，不得再作为本轮验收入口、宿主或发布目标。

## 1. 验收站固定身份

验收站是一个从 0 开始、功能完整、可以真实商用的独立 Sub2API 实例。它与主站互不可见，所有数据库、Redis、对象存储、账户、会话、账单、支付订单、上游凭据、通知凭据、Docker project/network、运行目录和数据卷均独立。

固定入口与运行身份：

- 公网入口：`http://49.51.203.200/`（当前只保证 IPv4；IPv6 80 端口因宿主 Docker 绑定冲突暂未启用）
- 健康入口：`http://49.51.203.200/health`；就绪入口：`http://49.51.203.200/readyz`
- 根路径：独立站根路径 `/`，不经过主站域名、主站 Caddy 或 `/admin/lab` 路径
- 验收 API/登录入口：由独立站自身根路径提供，必须以该站页面和 API 实际响应为准；不得拼接旧 `/admin/lab/api/v1` 前缀
- 主站管理员页面：`https://api.xingqiaolab.top/admin/accounts`；该路径继续走主站，不属于验收站
- 宿主 SSH alias：`sub2api-test-station`（`ubuntu@49.51.203.200:22`）
- 验收宿主目录：`/opt/sub2api-test-station/`
- 当前活动 release：由宿主 `/opt/sub2api-test-station/release-state.json` 的 `source_commit/source_tree` 与运行容器 Compose 标签解析；已核对 source commit 为 `3f7d59df5aa6f511c64a8663a8f74b8c2b50b3ed`、tree 为 `e049847dc3aba0296ea4e84a66f5caeda5df9da1`，不得在全局规则中固定旧 release SHA
- Compose 文件：`<active-release>/infra/independent-test-station/compose.yaml`
- Compose project：`sub2api-test-station`
- Compose network：`sub2api-test-station-network`
- 独立 named volumes：`sub2api-test-station-app-data`、`sub2api-test-station-postgres-data`、`sub2api-test-station-redis-data`
- 验收服务：`test-station-api`、`test-station-worker`、`test-station-detector`、`test-station-postgres`、`test-station-redis`、`test-station-caddy`
- 运行 env：服务器 `/opt/sub2api-test-station/.env`；Compose 发布/核对使用 active release 内的 `.env`

注册默认关闭。验收站只允许独立管理员登录；“测试站可商用”不等于向公网开放注册。

## 2. 凭据与本地读取规则

任何线程需要登录、查看日志、执行验收发布或宿主运维时，使用以下受保护文件；不得把其中的密码、token、私钥、API key、支付密钥、上游 key 或 webhook 写入 Git、规格书、聊天消息、发布证据或普通日志：

- 测试站 SSH 私钥：`/Users/gongtengxinwen/Downloads/test_service.pem`，权限必须为 `0600`
- 测试站 SSH known_hosts：使用当前 SSH alias 配置的 `/Users/gongtengxinwen/.ssh/known_hosts`；若另设专用文件，必须先确认包含 `49.51.203.200` 的可信 host key
- 测试站运行 env：服务器 `/opt/sub2api-test-station/.env`，权限必须为 `0600`
- 旧验收 env：`/Users/gongtengxinwen/.config/sub2api/acceptance-20260827.env`，仅历史兼容，不得用于新独立测试站

线程可以读取非敏感配置名和值（站点、目录、project、network、端口、provider 类型），但不得用 `cat`、`env`、`docker inspect` 或日志命令打印完整 env。需要展示时只展示变量名、是否已设置、文件权限和脱敏摘要。

当前验收管理员账号由上述 env 的 `ACCEPTANCE_ADMIN_EMAIL` 和 `ACCEPTANCE_ADMIN_PASSWORD` 提供；不得在仓库中复制密码。密码轮换后只更新受保护 env，并重新执行登录探针。

## 3. 验收站日常查看与运维

所有线程先确认当前工作区和目标 commit，再执行只读检查。常用命令如下（命令中的 env 文件只能作为 `--env-file` 传给 Compose，不要把内容打印出来）：

```bash
acceptance_ssh='ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes sub2api-test-station'

# 服务状态
$acceptance_ssh 'sudo -n sh -c '\''config=$(docker ps --filter label=com.docker.compose.project=sub2api-test-station \
  --format "{{.Label \\"com.docker.compose.project.config_files\\"}}" | head -n 1); \
  release=${config%/infra/independent-test-station/compose.yaml}; \
  docker compose --project-name sub2api-test-station \
  --env-file "$release/.env" -f "$config" ps'\''

# 查看单个服务日志（只保留必要窗口，先脱敏再归档）
$acceptance_ssh 'sudo -n sh -c '\''config=$(docker ps --filter label=com.docker.compose.project=sub2api-test-station \
  --format "{{.Label \\"com.docker.compose.project.config_files\\"}}" | head -n 1); \
  release=${config%/infra/independent-test-station/compose.yaml}; \
  docker compose --project-name sub2api-test-station \
  --env-file "$release/.env" -f "$config" logs --tail=200 test-station-api'\''

# 验收入口与健康检查
curl --fail --silent --show-error http://49.51.203.200/health
curl --fail --silent --show-error http://49.51.203.200/readyz
```

允许查看独立测试站容器、Caddy、PostgreSQL、Redis 的运行状态和日志；涉及数据库时只做只读查询。禁止执行 `docker compose down -v`、删除 `sub2api-test-station-*` volume、复制主站数据或用主站 env/旧验收 env 覆盖测试站 env。

## 4. 独立测试站发布入口

独立测试站发布与主站共用同一来源底线：候选先合入并推送根目录 `main`，然后只能从该根目录干净的 `main` 执行发布。发布时必须同时满足：当前分支为 `main`、非 detached HEAD、工作树干净、`HEAD` commit/tree 与本地 `origin/main` 完全一致。

当前仓库的 `ops/release-sub2api-acceptance.sh`、`ops/deploy-sub2api-acceptance-host.sh` 和 `infra/compose.acceptance.yaml` 仍是旧 `/admin/lab` 拓扑的历史发布链，不能发布到 `sub2api-test-station`。在独立测试站发布控制器完成适配并经直接相关测试前，禁止用旧脚本发布新站；本轮只允许对新站做只读核对。适配后的控制器必须 fail-closed 校验 `main == origin/main`，使用 `<active-release>`/新站 Compose 文件，仅操作 `sub2api-test-station` project，并不得调用主站蓝绿链。

新站当前已部署的镜像/数据属于独立服务器上的既有运行态，不构成当前代码发布控制器已适配的证据。不得把旧 `/admin/lab` 健康结果、主站 Caddy 路由或旧验收脚本输出写成新独立站发布成功。

独立测试站发布成功只表示服务部署和基础健康检查完成，不代表真实充值、消费、支付、上游、通知或目标功能已经验收通过。真实功能必须由管理员在独立数据和独立凭据上人工验证。

旧 `/admin/lab` 历史入口不得作为新站别名；任何仍依赖它的任务必须先改写为新站根入口并重新记录目标 commit/tree。

## 5. 主站发布的两条唯一授权路径

任何线程都不得因为“代码已合并”“本地测试通过”“验收站部署成功”“用户说继续”或其他模糊表述而直接部署主站。主站发布只有以下两种授权语义：

### A. 常规路径：测试站验收通过

用户明确说“测试站验收通过，部署主站”或语义完全等价时，才允许常规主站发布。必须按以下顺序执行：

1. 在候选 worktree 完成直接相关功能验证。
2. 将该候选合入根 `main`，在合并后 `main` 完成必要的冲突、直接回归、构建和发布预检，然后推送 `origin/main`。
3. 确认根工作区干净且 `main HEAD` commit/tree 与 `origin/main` 一致，从该 `main` 部署验收站。
4. 管理员在验收站完成并明确记录功能验收通过；真实支付、消费、上游、通知等若未验证，不能声称验收通过。
5. 确认主站发布候选仍为验收站已验收的同一 `main` commit/tree；若 `main` 已前进，新 commit 必须重新部署验收站并验收，不得直接发布主站。
6. 从该已验收的根 `main` 执行主站既有发布链，完成线上专项健康检查。
7. 主站发布成功后立即执行验收站同步/对账：使用同一 `main` commit/tree 确认验收站已运行该版本；若验收站不是该版本，立即从根 `main` 部署同一 commit。

### B. 紧急路径：快速部署到主站

用户明确说“快速部署到主站”或语义完全等价时，允许为紧急修复绕过“先完成测试站人工验收”这一前置步骤，但不能绕过本地最小直接验证、主站发布链自身的安全/停机门禁、健康检查和回滚保护。必须按以下顺序执行：

1. 记录紧急原因、目标 commit 和最小直接验证结果，将修复合入并推送根 `main`。
2. 确认根工作区干净且 `main HEAD` commit/tree 与 `origin/main` 一致，从该 `main` 执行主站发布。
3. 主站切换成功并完成健康检查后，立即使用**同一 commit**部署或对账验收站。
4. 在验收站同步完成前，禁止开始下一次主站发布；同步失败必须保留现场、记录失败原因并通知用户，不能宣称“主站与验收站一致”。

除上述 A、B 两类明确授权外，主站发布动作必须停止并等待用户明确指令。尤其是“部署”“上线”“推一下”“继续发布”“合并后处理”等表述，均不自动等价于主站授权。

### C. 明确不同比验收站的快速路径

用户明确说“快速部署主站，不同步验收站”或语义完全等价时，允许跳过验收站同步。该路径仍必须满足 B 路径的本地最小验证、根 `main` 来源门禁、主站发布链安全/停机门禁、健康检查和回滚保护。发布成功后必须记录：用户的不同步授权、目标 `main` commit/tree、主站宿主记录、健康检查结果，以及“验收站未同步”的明确状态；在下一次主站发布前，发布总控应再次提示当前主站/验收站可能存在版本差异。不得声称主站与验收站一致。

## 6. 主站与验收站同步合同

“立即同步”指主站发布成功后的同一发布窗口内，使用同一 source commit/tree（以及适用时同一镜像内容）完成验收站部署或版本对账；不能只同步配置文件、只同步前端或只同步数据库。

- 主站部署失败：不得同步新版本到验收站，保留失败证据并按主站回滚流程处理。
- 主站成功、验收站同步失败：主站不能被自动回滚，除非用户另行明确要求；B 路径必须将状态标记为“主站已生效、验收站同步失败”，阻止下一次主站发布并立即修复验收站同步。C 路径因用户明确授权不同步时，标记为“主站已生效、验收站按授权未同步”，不得伪造同步证据。
- 验收站已提前运行同一 commit：仍需执行一次只读版本核对并记录“无需重复部署”，满足同步证据要求。
- 任何同步过程不得导入主站数据、复制主站凭据或覆盖验收站独立卷。

## 7. 线程执行前检查清单

每个线程在涉及验收站或主站发布前，必须先回答并记录：

- 当前操作目标是验收站还是主站？
- 当前 commit/tree 是否明确？是否来自根目录干净的 `main`，且与 `origin/main` commit/tree 完全一致？
- 是否读取了本文件、`AGENTS.md`、`docs/project/native-sub-incremental-delivery-constraints.md` 和任务队列？
- 若目标是主站，用户授权是否明确匹配 A 或 B？
- 若是 A，是否有管理员“测试站验收通过”记录？
- 若是 B，是否记录紧急原因，并安排主站成功后的同 commit 验收站同步？
- 是否会触碰主站数据、验收站数据或凭据？是否保持完全隔离？
- 发布后是否完成健康检查、版本核对、日志/证据脱敏和状态登记？

任何一项无法回答时，停止任何部署；验收站和主站的只读检查可以继续，但不得扩大为发布动作。
