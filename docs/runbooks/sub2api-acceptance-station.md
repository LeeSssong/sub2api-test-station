# Sub2API 独立准生产验收站运行手册

本验收站是一个长期存在、可真实商用的独立 Sub2API 实例。它只允许管理员登录，默认不对公网开放；站点、数据、凭据和运行资源均与主站隔离。部署成功只表示服务已启动，不表示功能验收通过，也不会自动部署主站。

## 运行边界

固定流程：

本地直接验证 -> 部署验收站 -> 管理员真实验收 -> 人工合入 main -> 人工部署主站

部署为单实例、串行更新。验收站不使用蓝绿槽、临时环境或自动晋级；明确不自动晋级。任何失败都保留验收站 PostgreSQL、Redis、应用数据和 Caddy named volumes，不得通过清库掩盖问题。只有管理员在验收记录中明确通过后，才可以由发布负责人从已验证的 main 单独执行主站发布。

旧 admin lab mock 方案（包括 /admin/lab/、PAYMENT_PROVIDER=mock、UPSTREAM_PROVIDER=mock-upstream 和 NOTIFICATION_TRANSPORT=lab-outbox）不属于验收站能力，禁止继续扩展或接入。

## 安装前置

由 operator 在验收宿主准备以下互不复用生产的值：

- 独立域名、DNS 记录和 TLS 证书路径；域名不得使用主站域名，例如 api.xingqiaolab.top 或 shop.xingqiaolab.top。
- 独立 SSH 用户、私钥和 known_hosts；宿主已允许该用户使用 sudo -n bash -s。
- 独立支付商户/回调配置、上游账号与 API key、通知通道；这些凭据只写入验收站 env 或后台，不提交 Git。
- 宿主防火墙仅允许管理人员来源访问 80/443 和 SSH；PostgreSQL、Redis、detector、API 和 worker 不发布宿主端口。DNS/防火墙限制是网络边界，不能由 backend_mode_enabled 替代。
- Docker Engine、Compose v2、curl、ssh、scp 和 sudo。宿主部署目录必须位于 /opt/sub2api/acceptance-*。

复制模板到仓库外的受保护路径，替换所有示例值，并确认文件是普通文件且权限为 0600：

    install -m 600 infra/.env.acceptance.example /secure/sub2api/acceptance.env
    $EDITOR /secure/sub2api/acceptance.env
    chmod 600 /secure/sub2api/acceptance.env
    stat -f '%Lp %N' /secure/sub2api/acceptance.env  # Linux 可用 stat -c '%a %n'

ACCEPTANCE_PROJECT_NAME 必须为 sub2api-acceptance，ACCEPTANCE_NETWORK_NAME 必须为 sub2api-acceptance-network，ACCEPTANCE_DEPLOY_ROOT 必须是 /opt/sub2api/acceptance-<name>。三个真实 provider 标识不能包含 mock/lab 值，并必须保留 ACCEPTANCE_REAL_FLOW_ACK=I_UNDERSTAND_REAL_CHARGES。示例密码和域名不可直接部署。

## 发布验收站

在本地当前干净的候选 worktree 完成直接相关验证后，确认 ACCEPTANCE_ENV_FILE 指向 0600 env，再执行唯一的验收发布控制器：

    ACCEPTANCE_ENV_FILE=/secure/sub2api/acceptance.env \
      RELEASE_WORKTREE="$PWD" \
      ops/release-sub2api-acceptance.sh

控制器会构建一个 Linux/amd64 候选镜像、计算归档 SHA-256，经 SSH/SCP 传输到验收宿主，并只调用 deploy-sub2api-acceptance-host.sh。它会在建立 SSH 连接前拒绝脏 worktree、生产身份、mock provider、错误 project/network、非 0600 env 或缺少真实消费确认。控制器输出不包含密码、token、cookie 或支付/上游密钥。无论成功或失败，控制器和宿主执行器都会删除远程 staging 中的 env、镜像归档及临时 bundle；仅保留脱敏失败证据和运行日志，不把真实凭据作为故障证据留在 /var/tmp。

严禁从候选分支直接运行主站发布脚本，严禁调用生产蓝绿链，严禁把验收 env 复制进仓库。

## 部署后检查

发布控制器返回 result=succeeded 只代表宿主部署和基础健康检查完成。管理员仍需在独立域名执行：

1. 访问 /health 和 /auth/login，确认 HTTPS、登录入口和站点标识正确；确认注册入口关闭。
2. 使用验收站独立管理员登录后台，确认 backend_mode_enabled=true、registration_enabled=false；非管理员 token 访问受保护用户路由应被原生后端拒绝。
3. 核对 acceptance-api、acceptance-worker、acceptance-detector、acceptance-postgres、acceptance-redis、acceptance-caddy 六个服务均 healthy。
4. 确认 Compose project 为 sub2api-acceptance，网络为 sub2api-acceptance-network，named volumes 为 sub2api-acceptance-*；不要执行 docker compose down -v。

宿主只读核对命令：

    docker compose --project-name sub2api-acceptance \
      --env-file /opt/sub2api/acceptance-example/.env \
      -f /opt/sub2api/acceptance-example/compose.acceptance.yaml ps
    curl --fail --silent --show-error https://<验收域名>/health
    curl --fail --silent --show-error https://<验收域名>/auth/login

## 管理员真实验收清单

使用验收站自己的账户、余额和凭据，逐项记录结果、时间和请求编号：

- 管理员登录、退出、注册关闭、权限边界和会话刷新。
- 真实上游请求：至少验证一个目标模型/协议的成功请求、失败请求和用量记录；确认扣费来自验收站余额。
- 真实充值/支付：使用验收支付实例完成一笔测试充值或回调，确认只写入验收站数据库和账务；不要使用主站订单号、回调密钥或生产账户。
- 真实通知：验证验收站配置的通知通道收到预期事件，确认没有发送到生产群组或生产 webhook。
- 目标新功能的成功、失败、刷新/重试和权限场景；保留必要截图、请求编号和日志摘要，但脱敏后再归档。

发现任何功能、支付、上游、通知、数据隔离或权限问题，结论为不通过，保留现场并在同一验收站修复后重新部署；不得清除数据后重新验收。

## 回滚与恢复

宿主执行器在替换 compose/Caddy/env 前会保存上一份运行配置。镜像加载、bootstrap、健康检查或 URL 检查失败时，会自动恢复上一份配置并重新拉起上一版本；named volumes 和验收数据不会删除。首次安装失败则停止本次服务并保留 volumes；远程 staging 会被删除，失败原因只保留在脱敏日志中。

需要人工恢复上一版本时，在本地保留或创建上一已验证提交的干净 worktree，并使用同一份验收站 0600 env 重新运行发布控制器：

    ACCEPTANCE_ENV_FILE=/secure/sub2api/acceptance.env \
      RELEASE_WORKTREE=/path/to/previous-verified-worktree \
      /path/to/previous-verified-worktree/ops/release-sub2api-acceptance.sh

这会构建并部署上一已验证提交，同时继续使用当前验收站的独立数据库和 named volumes。不要执行 docker compose down -v、删除 sub2api-acceptance-* volumes、重置数据库或把验收数据导入主站。若没有上一份配置，先停止服务并保留数据，修正候选后重新发布。

## 通过后的主站边界

管理员验收记录必须明确通过、候选 commit/tree、验收范围和未验证项。发布负责人随后人工合入 main，在合并后的 main 上按主站既有发布链部署主站并做线上专项验证。验收发布控制器不会合入、推送或部署主站，也不会因验收站成功而触发任何自动晋级。

## 退役旧 admin lab

新验收站稳定运行并完成首轮真实验收后，由 operator 单独安排旧 mock lab 退役：

1. 先确认没有任务仍依赖 /admin/lab/，导出需要保留的只读日志/证据，并通知相关管理员。
2. 停止并禁用旧 compose.admin-lab.yaml stack，移除生产 Caddy 中指向 /admin/lab/ 的路由；不得把该路由转发到验收站。
3. 在确认无恢复需求后，按变更记录归档旧 lab 配置和 mock outbox；删除动作必须由 operator 明确执行，不能由验收发布脚本代办。
4. 用生产 Caddy 配置检查 /admin/lab/ 不再可达，并确认主站和验收站域名、project、network、数据库、Redis、对象存储及凭据仍完全独立。

旧 lab 的退役不等于删除验收站数据，也不改变本手册定义的串行人工验收流程。
