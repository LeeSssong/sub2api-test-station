# relay-ops 监控运维手册

## 1. 职责边界

`relay-ops` 是 Sub2API 的只读补充服务，不替代网关本身：

| 路径/能力 | 负责人 |
|---|---|
| `/monitor`、生产 Channel Monitor、Ops、Usage | Sub2API 原生能力 |
| `/pricing` | relay-ops，从 Sub2API 渠道定价生成匿名投影 |
| `/ops` | relay-ops，复用 Sub2API 管理员登录态 |
| 候选站价格页、倍率变化、低成本同步/SSE | relay-ops |
| 路由、用户价格、余额、充值、Key 和账号配置写入 | 仅人工批准后的 Sub2API 操作 |

relay-ops 不连接 Sub2API PostgreSQL，不保存生产 API Key，不自动切换上游，也不修改价格或余额。

## 2. 当前生产状态

- 部署模式：`RELAY_OPS_MODE=read_only`
- 镜像：`sub2api-relay-ops:665fa0a`
- 镜像摘要：`sha256:87cc0340ed181fcc0ee4bab51bc63f4943470b32ea230c6bbb95d5c0febb1141`
- 运行用户：`10002:10002`
- 根文件系统：只读；仅 `/tmp` 使用 tmpfs
- 网络：只暴露 Docker 内部端口 `8100`，宿主机不发布端口
- 数据库：独立 `relay_ops` database/schema
- 已安装：relay-ops 数据库凭据、Sub2API Admin API Key
- 未安装：飞书 Webhook、Agent API Key、候选上游 Key
- 当前候选记录：0；当前付费探测记录：0

`read_only` 会同步 Sub2API 公开分组和免费页面证据，但不会发送候选 API 请求。飞书和 Agent 未配置时，事件分析使用确定性回退内容并保留在内部证据中，不影响 Sub2API 服务。

## 3. 健康检查

在生产目录执行：

```bash
docker compose --env-file .env -f compose.yaml ps relay-ops
docker inspect -f '{{.State.Health.Status}}' sub2api-relay-ops-1
docker exec sub2api-caddy-1 wget -qO- http://relay-ops:8100/healthz
docker exec sub2api-caddy-1 wget -qO- http://relay-ops:8100/readyz
```

正常结果分别为 `healthy`、`{"status":"alive"}` 和 `{"status":"ready"}`。`/readyz` 依赖 PostgreSQL 和最近一次成功的 Sub2API 读取，不依赖飞书或 Agent。

外部检查：

- `/pricing` 匿名返回 `200`，金额按 USD/1M Token 展示，并包含 Sub2API 阶梯价。
- `/ops` 未登录时跳转 Sub2API 登录；管理员登录后显示公开分组、候选、事件和 Agent 分析。
- `/monitor` 必须继续由 Sub2API 返回，不能被 relay-ops 接管。

## 4. 调度与费用边界

- 生产页面和 Sub2API 元数据：每 5 分钟检查；内容哈希未变化时不重复解析或告警。
- 候选站：每 6 小时一个采集周期。
- `read_only`：只抓取允许的 HTTPS 页面，不调用候选 API。
- `probe`：每个候选使用最多 1 个代表模型，发送 1 次同步和 1 次 SSE，共最多 2 个 Chat 请求；每次请求最多 8 个输出 Token。
- 当前代码费用上限：每次请求按 `$0.001` 估算，即每候选每周期最多 `$0.002`。

切换到 `probe` 前必须向管理员报告：候选名称、用途、代表模型、预计请求数、费用上限和恢复方式，并取得明确批准。页面变化不会额外触发付费探测。

## 5. 候选上游录入

最少信息：

1. 管理员显示名称。
2. OpenAI-compatible HTTPS Base URL。
3. HTTPS 模型定价/倍率页 URL。
4. HTTPS 用量/账单页 URL。
5. 独立低额度监测 Key 的服务器秘密文件。

性能页 URL 可选；站点类型、模型目录和代表模型由采集器识别。Key 原文不进入数据库、请求 JSON、日志或 Agent 输入。

先把 Key 安装到宿主机：

```bash
sudo install -d -m 0700 -o 10002 -g 10002 /opt/sub2api/production/secrets/candidate-keys
sudo install -m 0600 -o 10002 -g 10002 /path/from-password-manager/key /opt/sub2api/production/secrets/candidate-keys/<candidate>
```

录入时只提交容器内引用 `/run/secrets/candidates/<candidate>`。服务会读取一次以生成 SHA-256 指纹和末四位，数据库只保存这些元数据。生产模式保持 `read_only` 时可以先验证价格页采集，Key 不会被用于 API 请求。

## 6. 告警规则

正常重复结果不发飞书。只有以下状态变化触发通知：

- 新事件达到确认门槛。
- 已有事件升级或出现新证据。
- 故障恢复。
- 上游模型价格或倍率发生语义变化。
- 候选站在价格、TTFT 和服务质量综合条件下明显优于当前分组。
- 每日摘要。
- 上游用量页面登录会话失效，通知中附对应登录链接。

消息必须回答：做了什么、得到什么结果、相对基线有什么变化、管理员是否需要处理、在哪里查看证据。Agent 只接收脱敏结构化事件，不能执行工具或生产写操作。

## 7. 登录与会话恢复

### `/ops` 管理员登录

`/ops` 读取浏览器已有的 Sub2API `auth_token`，relay-ops 只把 Bearer、浏览器 User-Agent 和 Caddy 可信代理 IP 链转发给 `/api/v1/auth/me`。Cookie、Admin API Key 和其它请求头不会转发。

如果管理员访问 `/ops` 被送回登录页：

1. 在同一浏览器重新登录 Sub2API 管理后台。
2. 若公网 IP 或 User-Agent 已变化，完成一次新的 2FA 登录。
3. 重新打开 `/ops`。

不要关闭 Sub2API 会话绑定，也不要复制浏览器 Token 到文档、聊天或 shell 历史。

### 上游用量页面会话

上游用量 Cookie/Token 过期只会把真实账单证据标记为不可用，不影响质量、公开价格或路由。收到告警后按消息中的登录链接重新登录，并替换对应服务器秘密文件；不在 relay-ops 中保存上游账号密码。

## 8. 回滚

只回滚 relay-ops 镜像，不重启 Sub2API、PostgreSQL、Redis 或 Caddy：

```bash
cd /opt/sub2api/production
cp compose.yaml compose.yaml.before-relay-ops-rollback
sed -i 's/sub2api-relay-ops:665fa0a/sub2api-relay-ops:e923055/' compose.yaml
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --no-deps relay-ops
```

`e923055` 保留管理员会话绑定修复，但公开价格仍存在旧的每 Token 显示问题，只用于紧急回滚。若 relay-ops 整体不可用，应恢复生产目录中的 relay-ops 前 Compose/Caddy 备份，并只重建受影响的 relay-ops/Caddy 容器；不要删除 `relay_ops` 数据库或秘密文件。

## 9. 禁止事项

- 不把 `RELAY_OPS_MODE` 自动改为 `probe`。
- 不让 Agent 修改路由、价格、余额、账号、Key 或数据库。
- 不读取或输出秘密文件内容。
- 不直接修改 Sub2API PostgreSQL。
- 不因飞书、Agent 或上游账单页失败而停止 Sub2API。
- 不把候选站的名称、采购倍率、余额和故障历史展示给普通用户。
