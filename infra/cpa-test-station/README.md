# CPA 测试站

## 当前管理入口（2026-10-12）

管理界面已改用官方 CPAMC v1.26.1（`cpamc/source.json` 记录制品来源和 SHA256），兼容既有 CPA v8.0.20。域名入口为 `https://cpa-test.xingqiaolab.top/management.html`，8317 端口提供相同页面，旧 18317 入口重定向到域名。登录使用 CPA Management Key；CPAMP Admin Key 和旧会话不再适用。Ticket Gateway 插件及 Keeper 保持独立运行。

`compose.yaml` 不再定义 CPAMP；其容器、SQLite 数据卷及历史采集数据在切换验证后删除。删除不涉及 CPA 凭证、Ticket Gateway、Keeper 或 Sub2API。旧 CPAMP 制品和路由配置仅保留作软件回退，历史数据库不备份、不恢复。浏览器旧 CPAMP 本地存储仍属各客户端，服务器无法远程清除所有设备。

后续部署只推广干净且已推送的 main；静态页面、Caddyfile 使用原子替换和 Caddy 平滑 reload，无需重启 CPA。切换流程见 `ops/switch-cpa-test-cpamc.py`。下方为旧部署说明，仅用于历史参考，不得据此重新启用 CPAMP。

## 当前票据网关（2026-10-14）

后端与边缘兼容页面均升级为官方 v1.6.0，来源和校验和记录在 `ticket-source.json`。`ticket-gateway-ui.html` 来自已校验的官方 Linux AMD64 插件包，保留已有 `enc::v2::` 原生会话适配。该页面由 Caddy 静态提供，因此后续插件升级必须同时核对并更新边缘页面，不能只验证后端注册版本。页面更新使用干净且已推送 main 的文件原子替换，无需重启 CPA 或 Caddy。

## 历史部署说明

目标为 `ubuntu@43.133.75.82`。独立 Compose project、network、数据卷和运行目录均以 `cpa-test-station` 命名。运行目录为 `/opt/cpa-test-station`。

复用本地版本的 Linux AMD64 制品：CPA v8.0.20、CPAMP v1.14.4、Ticket Gateway v1.5.4。镜像按 digest 固定；管理页使用 CPAMP v1.14.4 的官方 `management.html`，SHA256 为 `98476e174f8f74001c2d1b576091106a49f00b476843ed2a54d38a9f9f04e4ae`。插件下载地址为 `https://ticket.codingmiao.site/downloads/cliproxy-ticket/1.5.4/cliproxy-ticket-gateway-1.5.4-linux-amd64.store.zip`，压缩包 SHA256 为 `0739ece56aad758d4a87f7fdf3dd8d282711437a6192ae40c9e182c22e573dfa`，包内 `cliproxy-ticket-gateway.so` SHA256 为 `76fd4bde1a5d517ecfade6f6fcc779a73ee339e83aed7c20cf7f63142d7028c3`。

CPA 管理入口为 `http://43.133.75.82:8317/management.html`；完整 CPAMP 管理入口为 `http://43.133.75.82:18317/management.html`。分别使用 CPA Management Key 与 CPAMP Admin Key。服务器及云防火墙需要允许这两个 TCP 端口。

域名入口为 `https://cpa-test.xingqiaolab.top/`，使用 CPAMP Admin Key 登录，管理页和插件 API 均由已配置的 Manager Server 提供。独立 `compose.edge.yaml` 使用既有 Caddy v2.10.2 制品、仅发布 443，并通过 CPA 专用网络访问 Manager。DNS A 记录指向 43.133.75.82；首次签发证书时使用 DNS only，让 ACME TLS-ALPN-01 直接验证 443。证书和续期状态保存在独立卷内。将 Caddyfile 放到 `/opt/cpa-test-station/edge/Caddyfile` 后启动 edge project，原有 Sub2API 的 80 端口服务可继续运行。域名入口不依赖新增 8317/18317 公网端口。

## 初始化

插件页面兼容补丁 `ticket-gateway-ui.html` 来自已校验 Ticket Gateway v1.5.4 Linux AMD64 制品的 `/v0/resource/plugins/cliproxy-ticket-gateway/ui`，保留原页面和业务逻辑，仅增加 CPAMP v1.14.4 `enc::v2::` 原生会话格式读取。原始页面 SHA256 见发布记录。将该文件与 Caddyfile 一起安装到 edge 目录，边缘仅替换这一资源路径；插件 API 和后端鉴权继续由 Manager/CPA 提供，不另存管理凭据。插件仍要求原生登录时启用记住凭证。回归命令：`node --test tests/infra/ticket-plugin-session.test.cjs`。

只从已推送、干净且与 `origin/main` commit/tree 一致的根目录 main 推广 Compose 和 `infra/cpa-production/config.example.yaml`。将模板中的管理密钥替换为测试站独立随机密钥。使用全新 auths、插件状态目录、Manager 数据卷和独立密钥，密钥文件权限为 0600，目录权限为 0700。

先拉取固定镜像，准备并校验管理页和插件，再导入 CPA 连接：

```sh
docker compose -f /opt/cpa-test-station/compose.yaml run --rm --no-deps \
  -v /opt/cpa-test-station/secrets/cpa-management-key:/run/cpa-management-key:ro \
  cpa-manager store-cpa-connection \
  --cpa-base-url http://cpa-gateway:8317 \
  --management-key-file /run/cpa-management-key \
  --db-path /data/usage.sqlite --data-key-path /data/data.key
docker compose -f /opt/cpa-test-station/compose.yaml up -d --wait --wait-timeout 90
```

验证两容器健康、管理页校验和、两套密钥鉴权、CPAMP 到 CPA 的连接、插件 v1.5.4 注册及原有 Sub2API 健康。插件初装无默认网关地址；业务账号、网关绑定、CDK 激活和真实模型请求需使用测试站独立凭据另行配置和验收。

首次安装回退时仅停止该 Compose project，保留运行目录和数据卷；升级时保留前一版配置、插件和镜像，切回已验证兼容的制品。不得用删除卷的方式回退。

当前插件目录为可写挂载，支持原生插件商店安装。页面更新会先通过 `update/prepare` 等待当前请求结束；在途请求未排空时可能持续等待。本次用户授权暂时不可用，采用停止 CPA 网关、安装已校验版本化插件文件、只更新 `store.version`、同步边缘页面再启动的方式。保留原配置、安装身份、auth 和数据目录，以及上一版插件和页面用于失败恢复。后续升级仍须核对实时挂载权限和注册版本，不按历史只读说明推断。

## Keeper

Keeper v1.15.10 使用独立 `compose.keeper.yaml`、`cpa-test-station-keeper-data` 卷和 `/opt/cpa-test-station/secrets/keeper.env`（0600），仅通过专用 Docker 网络连接 CPA，不发布新公网端口。域名子路径为 `https://cpa-test.xingqiaolab.top/keeper/`。官方 Keeper v0.1.0 插件注册 `/v0/resource/plugins/keeper/open`，CPAMP 从插件菜单嵌入页面；Keeper 独立登录，插件不提供免密桥接。

制品来源和校验和见 `keeper-source.json`。从已推送的干净根目录 main 将本目录的 `compose.keeper.yaml`、`Caddyfile`、`keeper-source.json` 和 `ops/deploy-cpa-test-keeper.py` 打包上传；官方插件 ZIP 必须符合记录的 SHA256。宿主安装器同时持有测试站 API 发布锁和 CPA Keeper 发布锁，核对现有 Caddy 校验和、CPAMP subscribe transport，启动独立 Keeper、验证就绪、平滑加载 Caddy，再通过 CPA 原生配置 API 热加载插件，不重启 CPA/CPAMP。

安装器参数：`sudo python3 deploy-cpa-test-keeper.py <bundle-dir> <keeper.zip> <main-commit> <main-tree>`。每次安装在 `/opt/cpa-test-station/backups/keeper-<UTC>/release.json` 保存一份记录及原始配置。失败恢复旧 Caddy、禁用 Keeper 插件、停止 Keeper，保留数据、凭据、插件文件与制品。成功后的手动回退采用同样步骤；不删除卷，也不恢复旧业务数据库。

新安装不导入 CPAMP 历史。首次看到额度仅建立基线，预测需后续配额下降样本和有效模型价格。Keeper 的统计倍率不等于 Sub 用户扣费规则，默认不自动同步。
