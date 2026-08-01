# 2026-08-01 main 蓝绿生产发布验证

## 结论

- `main@754ab2efeebd87771ac3eb945c125a7b538e30ff` 已推送、部署并完成生产验收。
- 遗留单实例已切换为 `sub2api-blue`、`sub2api-green`、唯一 `sub2api-worker`、`relay-ops`、Caddy、PostgreSQL、Redis 七服务拓扑。
- 最终维护窗口从 `2026-08-01T11:42:18Z` 到 `2026-08-01T11:42:50Z`，持续 32 秒。
- PostgreSQL 和 Redis 使用原命名卷且容器身份未改变；旧单实例容器已删除，旧镜像保留用于回滚。

## 发布身份

- source commit：`754ab2efeebd87771ac3eb945c125a7b538e30ff`
- source/tested tree：`e7c905fd2d17635a952269f59ec988f65d7905a2`
- migrations hash：`176e6659b45bffbf11f5e1fce7dfbaf60906fe974553d7156fdc516231f4f5d0`
- Sub2API：`xingqiao-sub2api@sha256:426d253b8b358120db361ec7e7c82ec9f35b97b8cdbb76aa1164febbfc88b465`
- relay-ops：`xingqiao-relay-ops@sha256:c88f58e4f9cbee2338dc6b607fa3e1f4f54fa8adbb32790f29411d3a5f224c66`
- Caddy/homepage：`xingqiao-caddy@sha256:1508c9687082148932ad79cda479d1bc29e9bc27d5f94b61269b406613d72603`

## 运行身份

- blue：`f4869cba569885c0772a14e95c75b1687dc6777228b67a815b826191e2ea86e7`，`SERVER_PROCESS_ROLE=api`，healthy。
- green：`dec6de67a4d14296c74600143241084e3af3fd8881b4360b28255e799abf170e`，`SERVER_PROCESS_ROLE=api`，healthy。
- worker：`27c3b43c2388f7bc2e0e2e5a0b9bb2721e7020d3872027fd40e716506cfbbeea`，`SERVER_PROCESS_ROLE=worker`，healthy，运行基数为 1。
- relay-ops：`2a9faf7482d139055a48bc142c871fcc34012f4bad1c83a1f78a34939ff0bfd0`，healthy。
- Caddy：`4b81becdbed01509e7d7f7871392e7f81623f62216384021cb715c443ab98a85`，实际活动上游为 `sub2api-blue:8080`。
- PostgreSQL：`2db52788ad733522b3398f3ba9c0ff4c45a418c360a57424a9e115feb43d4db6`，与发布前一致。
- Redis：`c45202c0d9e64f27d21191e87681c3ccb70e927555b74a4b9a47eb701afaa475`，与发布前一致。

## 验收证据

- `docker compose config -q` 通过；三个 Sub2API 服务均由 `release.env` 的 immutable digest 驱动。
- TLS 1.2 和 TLS 1.3 请求 `/health` 均返回 `status=ok`；API 站点证书已轮换为 RSA 2048。
- TLS 1.2 和 TLS 1.3 携带有效网关 Key 请求 `/v1/usage` 均返回 200；因此早期拟议的 nginx TLS front 已被更小的 RSA Caddy 根因修复取代，未引入第二层反向代理。
- 首页、管理员版本接口、管理员 Monitor 接口、带生产网关 Key 的 `/v1/models` 和 relay-ops `/healthz` 均通过。
- Caddy Admin API 解析到唯一活动上游 `sub2api-blue:8080`。
- 最近 10 分钟日志未出现 panic、fatal、migration failure、checksum mismatch、重复 worker 或配置加载失败。
- host executor 已安装到 `/usr/local/libexec/deploy-sub2api-blue-green-host.sh`，`release-state` 位于 `/var/lib/sub2api/release-state`，语法、权限和 schema 检查通过。
- 本地 focused 回归通过：release controller 测试、host executor final-review 测试、shell syntax 和 `git diff --check`。

## 2026-08-01 对账发布追加验收

- 主干后续发布提交：`6f0609799`（包含生产主机 GNU `stat` 门禁兼容、预加载镜像支持和 curl header 读取修复）；均已推送到 `origin/main`。
- Sub2API immutable image：`xingqiao-sub2api@sha256:0387df2e011e12aa18e2fce525c0967d08d16810b669507e220697c242faad66`，活动槽切换为 green；blue 保留用于回滚。
- relay-ops immutable runtime image：生产容器使用 `xingqiao-relay-ops:release-48244833b`，容器 healthy；对账迁移已在 `relay_ops` 数据库创建/确认 `upstream_cost_attempts`、`upstream_reconciliation_exceptions` 及 `accounting_daily_snapshots.reconciliation_status`。
- 唯一 worker 仍为 1，PostgreSQL、Redis、Caddy 容器身份与发布前一致；未重建共享服务。
- 通过 Caddy 公网入口验证账号监控对账 summary、exceptions 和 refresh 管理 API 返回 200；TLS 兼容策略及 `/health`、`/v1/models`、relay-ops healthz 复验通过。

## 备份与回滚

- 拓扑切换备份：`/opt/sub2api/production/backups/blue-green-bootstrap-20260801T110506Z`
- TLS 证书轮换备份：`/opt/sub2api/production/backups/api-cert-pre-rsa-20260801T120500Z`
- 两次 relay-ops 配置门禁失败均在切换脚本内自动恢复旧单实例；最终根因是通知策略新增字段和策略文件权限，修正为显式关闭未接线通道及 `0640 root:10002` 后离线配置检查通过。
- 生产网关验收 Key 已刷新为现有有效管理员 Key；密钥内容未写入 Git 或本报告。
