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
- 飞书命令模式：`RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`
- 镜像：`sub2api-relay-ops:quality-report-read-only-20260722-v1`
- AMD64 image ID：`sha256:b7977f9cb850d020dba66443a920c186772649edecd12d80023825552dd84b8e`
- 运行用户：`10002:10002`
- 根文件系统：只读；仅 `/tmp` 使用 tmpfs
- 网络：只暴露 Docker 内部端口 `8100`，宿主机不发布端口
- 数据库：独立 `relay_ops` database/schema
- 已安装：relay-ops 数据库凭据、Sub2API Admin API Key
- 已安装：现有飞书 App 凭据和告警群引用；异常、恢复和日报通过 App Bot 主动投递
- 未安装：真实 Agent API Key、候选上游 Key、上游用量会话秘密
- 当前候选记录：0；当前付费探测记录：0
- 质量报告表已安装，当前记录为 0；`read_only` 下不创建 `candidate-fast:*` 调度任务

`read_only` 会同步 Sub2API 公开分组和免费页面证据，但不会发送候选 API 请求。真实 Agent 未配置时，事件分析使用确定性回退并继续向飞书投递，不影响 Sub2API 服务。

公开 HTML 倍率只有在“倍率 / multiplier / rate”明确文本上下文、显式 `data-multiplier`/`data-rate` 属性或结构化 JSON 字段中才采信。`unparseable` 表示页面没有可验证的倍率/模型字段，不表示上游故障；此时继续使用 Sub2API 配置基线，并等待登录会话或账单辅助证据，禁止从终端尺寸、CSS、脚本或编码片段猜倍率。

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
5. 独立低额度监测 Key。

性能页 URL 可选；站点类型、模型目录和代表模型由采集器识别。管理员在 `/ops` 的“录入候选上游”表单中一次性填写 Key。Key 通过同源 HTTPS 提交后写入独立 `0600` 文件，数据库只保存文件引用、SHA-256 指纹和末四位；响应、日志、飞书和 Agent 输入不包含 Key。提交完成后页面立即清空密码框。

生产首次部署先创建托管目录：

```bash
sudo install -d -m 0700 -o 10002 -g 10002 /opt/sub2api/production/secrets/candidate-managed-keys
```

该目录只挂载到 `/var/lib/relay-ops/candidate-keys`，是容器唯一新增的读写秘密目录。旧 `/run/secrets/candidates` 继续只读，兼容历史文件引用记录。生产模式保持 `read_only` 时只运行既有公开信息采集；候选同步/SSE 付费 probe 仍需单独批准。

## 6. 告警规则

正常重复结果不发飞书。只有以下状态变化触发通知：

- 新事件达到确认门槛。
- 已有事件升级或出现新证据。
- 故障恢复。
- 上游模型价格或倍率发生语义变化。
- 候选站在价格、TTFT 和服务质量综合条件下明显优于当前分组。
- 已批准的 `probe` 模式候选任务生成新的质量结论；报告先入库，再绑定稳定 incident 并通过既有持久化去重器发送无切换按钮的飞书卡片。
- 每日摘要。
- 上游用量页面登录会话失效，通知中附对应登录链接。

消息必须回答：做了什么、得到什么结果、相对基线有什么变化、管理员是否需要处理、在哪里查看证据。Agent 只接收脱敏结构化事件，不能执行工具或生产写操作。

质量卡的去重证据不包含 run ID 或时间，结论语义等价时不重复投递。`failed` 投递可在原记录上重试；`reserved` 和 `delivered` 仍必须抑制重复。生产保持 `read_only` 时，只允许保存免费读取得到的证据，不会因卡片功能自动启用候选 fast run。

### 告警验收边界

`/ops` 已改为只读状态页，不再提供合成告警、测试日报或质量预览按钮，对应浏览器 HTTP 写接口返回 `404`。既有合成告警、重复抑制、恢复和日报证据继续保留；不要删除 incident/notification/去重记录，也不要为了视觉验收制造故障或重复事件。

日报继续由现有调度器按上海日期生成并通过持久化发送器去重。告警与恢复继续由自然监控状态转换触发。需要复核时读取既有投递、事件和报告，不从 `/ops` 发起写操作。

## 7. 登录与会话恢复

### `/ops` 管理员登录

`/ops` 读取浏览器已有的 Sub2API `auth_token`，relay-ops 只把 Bearer、浏览器 User-Agent 和 Caddy 可信代理 IP 链转发给 `/api/v1/auth/me`。Cookie、Admin API Key 和其它请求头不会转发。

如果管理员访问 `/ops` 后进入 404：

1. 在同一浏览器重新登录 Sub2API 管理后台。
2. 若公网 IP 或 User-Agent 已变化，完成一次新的 2FA 登录。
3. 重新打开 `/ops`。

不要关闭 Sub2API 会话绑定，也不要复制浏览器 Token 到文档、聊天或 shell 历史。

### 上游用量页面会话

`/ops` 不再登记供应商 Cookie、Token 或秘密文件引用。上游账号、Base URL、Key、分组和调度只在 Sub2API 原生管理后台维护；供应商余额若无法由 Sub2API 提供，则作为门禁外部只读证据单独采集，不通过网页录入凭据。

## 8. 回滚

只回滚 relay-ops 镜像，不重启 Sub2API、PostgreSQL、Redis 或 Caddy：

```bash
cd /opt/sub2api/production
cp compose.yaml compose.yaml.before-relay-ops-rollback
cp -p compose.yaml.bak-quality-report-read-only-20260722-v1 compose.yaml
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --no-deps --force-recreate relay-ops
```

该备份回滚到 `candidate-admin-intake-20260721-v2`，保留共享 Aliu dry-run 命令控制、原生告警/恢复/日报闭环、候选站管理员录入、`/ops`、`/pricing` 和原生 `/monitor` 引用，仅缺少本次质量报告增量。回滚后仍要核对 `read_only + dry_run`、候选/probe/notification 计数和路由哈希。不要删除 `quality_reports`、incident、notification 或去重记录；不要删除 `relay_ops` 数据库或秘密文件。

## 9. 禁止事项

- 不把 `RELAY_OPS_MODE` 自动改为 `probe`。
- 不让 Agent 修改路由、价格、余额、账号、Key 或数据库。
- 不读取或输出秘密文件内容。
- 不直接修改 Sub2API PostgreSQL。
- 不因飞书、Agent 或上游账单页失败而停止 Sub2API。
- 不把候选站的名称、采购倍率、余额和故障历史展示给普通用户。
