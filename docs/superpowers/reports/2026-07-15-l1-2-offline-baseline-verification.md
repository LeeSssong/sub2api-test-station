# L1-2 Sub2API 离线部署基线验证报告

**日期：** 2026-07-15  
**范围：** 本地 Docker Compose、镜像版本、服务健康、端口、安全默认值、低内存参数和数据卷持久化。  
**结论：** 离线部署基线通过；不代表生产部署或真实商业闭环通过。

## 验证结果

| 项目 | 结果 | 证据 |
|---|---|---|
| 静态契约 | 通过 | `bash tests/infra/validate-baseline.sh` 输出 `PASS: infrastructure baseline contracts` |
| Compose 解析 | 通过 | `.env.example` 和随机本地 `.env` 均通过 `docker compose config --quiet` |
| Sub2API 版本 | 通过 | 锁定摘要启动输出 `Sub2API 0.1.155`，commit `41cec0db059ffb82d0efdcfcf07a24ab51fbfe97` |
| Caddy 配置 | 通过 | Caddy 2.10.2 `validate` 返回 `Valid configuration` |
| 四服务启动 | 通过 | PostgreSQL、Redis、Sub2API 为 healthy，Caddy running；Compose `--wait` 退出 0 |
| HTTP 入口 | 通过 | 经 Caddy 请求 `/health` 返回 HTTP 200 和 `{"status":"ok"}`，首页返回 200 |
| 端口隔离 | 通过 | 只有 Caddy 发布 80/443；PostgreSQL 5432、Redis 6379、Sub2API 8080 仅容器网络可见 |
| PostgreSQL 参数 | 通过 | `max_connections=60`、`shared_buffers=128MB`、`effective_cache_size=512MB`、`maintenance_work_mem=32MB` |
| Redis 参数 | 通过 | `maxclients=1000`，密码健康检查通过 |
| 应用连接池 | 通过 | 启动日志显示 `max_open=20`、`max_idle=5` |
| 请求体限制 | 通过 | 启动日志显示全局上限 `16777216 bytes (16.00 MB)`；网关环境值同为 16777216 |
| 可信代理 | 通过 | `SERVER_TRUSTED_PROXIES=172.16.0.0/12` 生效，重建后不再出现 trusted proxies 为空警告 |
| 密钥文件 | 通过 | 生成器创建五个独立 64 位随机值，文件权限 600，Git 忽略命中，重复执行拒绝覆盖 |
| 数据持久化 | 通过 | 重建 Sub2API/Caddy 后 PostgreSQL 中管理员记录数仍为 1，未重新初始化管理员 |
| 测试清理 | 通过 | 测试容器和网络已 `down`；命名卷保留供后续恢复测试 |

## 观察项

- Docker Desktop 主机的 Redis 日志提示 `vm.overcommit_memory` 未启用；真实 Ubuntu 主机必须设置为 1，运行手册已加入命令和验收。
- 首次启动提示本地价格 fallback 文件不存在，随后成功在线下载 214 个模型；不是启动失败。生产部署仍需检查定价更新时间和异常时行为。
- CORS allowed origins 为空会拒绝跨域浏览器请求；当前前后端同源，因此保留该安全默认。未来拆分前端域名时再添加精确 origin，不使用 `*`。
- 本地使用 `http://localhost` 跳过 ACME，仅验证反代；真实域名证书签发和 HTTP 到 HTTPS 跳转未验证。

## 明确未验证

- 服务器购买、Ubuntu 主机、SSH、防火墙、swap、`vm.overcommit_memory` 和境外网络。
- 真实域名、DNS A 记录、Cloudflare DNS only 和公网 TLS。
- 现有上游 Base URL、API Key、模型、余额、限额、流式/非流式请求和错误透传。
- Token 日志、成本、用户余额扣费、人工充值账本和毛利。
- 管理员真实邮箱、TOTP 2FA 和恢复码存储。
- 支付渠道、Webhook、真实客户款项和商户对账。
- K12、Plus、Pro 等订阅账号的购买、授权、可用性和单位成本。

## 外部支出确认

本轮未执行付款、充值、购买、收款、实名、商户开通或付费账户登录。SRV01、DOM01、ACC01 和 PAY01 继续保持未购买/未开通状态。

## 下一节点

进入 L1-0.2/L1-3 的离线部分：建立现有上游的非敏感参数模板、模型白名单候选、成本/限额字段和模拟渠道验收清单。真实 Base URL 和 Key 到位后再录入受控环境，不写入 Git 或普通文档。
