# relay-ops 只读生产验收报告

## 范围

本报告覆盖 relay-ops 本地实现、假上游端到端流程、容器安全、生产只读部署、管理员登录复用、公开价格和既有仓库回归。未执行真实候选 API 探测，未配置飞书或 Agent 真实凭据，未修改 Sub2API 路由、余额或用户价格。

## 结果

| 项目 | 结果 |
|---|---|
| Go race 测试 | PASS；全部包通过，含 PostgreSQL 端到端场景 |
| Go vet | PASS |
| Ruby 全量回归 | PASS；`118 runs / 472 assertions / 0 failures / 0 errors` |
| relay-ops Compose/Caddy 契约 | PASS |
| 基础设施契约 | PASS |
| D04 独立服务契约 | PASS |
| `git diff --check` | PASS |
| 桌面/移动页面截图 | PASS；无空白、重叠或认证数据泄露 |
| 生产 `/healthz`、`/readyz` | PASS |
| 生产 `/ops` 管理员登录复用 | PASS |
| 生产 `/pricing` | PASS；6 个模型、3 个 `>272k` 阶梯行，USD/1M 单位正确 |
| 生产 `/monitor` | PASS；仍由 Sub2API 原生页面负责 |
| 生产付费探测记录 | `0` |

## 关键验证

### 候选状态机与费用门禁

一次性 PostgreSQL、假价格页、假候选探测器、假飞书和假 Agent 场景验证：

- `0.07x -> 0.10x` 倍率变化经过确认后只生成一次通知和一次 Agent 分析。
- 重复不变结果不重复通知。
- `read_only` 运行不调用候选探测器。
- 显式允许 probe 后只调用一次假探测器。
- 事件升级、恢复、证据变化和通知去重由 focused tests 覆盖。

### 管理员会话绑定

初版 relay-ops 代请求 `/api/v1/auth/me` 时使用 Go 默认 User-Agent 和 relay-ops 容器 IP，触发 Sub2API IP/UA 会话绑定 401。修复后只转发白名单字段：Bearer、浏览器 User-Agent、`X-Forwarded-For` 和 `X-Real-IP`；Cookie、Admin API Key 和其它头不会转发。

生产 `SERVER_TRUSTED_PROXIES=172.16.0.0/12` 覆盖 Caddy 与 relay-ops 的 Docker 网络。真实已登录浏览器访问 `/ops` 后成功显示公开分组、原生监控入口、候选、事件和 Agent 分析，不再跳回管理控制台。

### 公开价格单位与阶梯

生产验收发现 Sub2API Admin API 返回每 Token USD，而页面标题为每 1M Token。修复后统一乘以 `1,000,000` 并保持必要小数位，同时读取 `model_pricing.intervals`：

- `gpt-5.6-sol` 基础输入/输出显示 `$5.00 / $30.00`。
- `gpt-5.6-sol >272k` 显示 `$10.00 / $45.00`。
- Terra/Luna 的 `>272k` 阶梯同样显示。
- 页面不展示 Neko、采购倍率、余额或 Key。

## 生产部署

- 模式：`read_only`
- 镜像：`sub2api-relay-ops:665fa0a`
- 摘要：`sha256:87cc0340ed181fcc0ee4bab51bc63f4943470b32ea230c6bbb95d5c0febb1141`
- 架构/大小：`amd64`，约 44.6 MB
- 容器：非 root、只读根文件系统、`cap_drop: ALL`、`no-new-privileges`、无宿主机端口
- 数据：独立 `relay_ops` database/schema

两次 relay-ops 滚动更新期间，Sub2API、PostgreSQL、Redis 和 Caddy 容器 ID 均未变化。最终容器健康，重启后 `/readyz` 立即返回 ready。

## 尚未验证

- 飞书 Webhook 和 OpenAI-compatible Agent 真实凭据尚未安装，因此未发送真实通知或真实模型分析。
- Neko/wawazz 尚未录入 relay-ops 上游表，当前候选列表为空。
- `RELAY_OPS_MODE=probe` 未获本轮明确低成本批准，真实同步/SSE 请求数为 0。
- 已确认的 `GPT-Pro` / `GPT-Plus` 目标配置尚未切换；生产实际仍是公开 `GPT 0.15x`、专属旧分组 `1.0x`、Neko 账号倍率 `0.07x`，wawazz 尚未接入 Sub2API。
- 正式域名仍在审核流程；当前只完成临时 HTTPS 入口技术验收。

## 后续门禁

1. 先完成 GPT-Pro/GPT-Plus、Neko `0.10x`、wawazz `0.05x` 的生产配置和隔离验证。
2. 把 Neko/wawazz 的公开价格、用量页和服务器秘密引用录入 relay-ops，先保持 `read_only`。
3. 安装飞书/Agent 独立秘密并验证一条假事件通知。
4. 报告候选 probe 的预计 2 请求/候选/周期和最多 `$0.002` 费用，取得明确批准后执行一次真实验收。
5. 域名审核通过后迁移正式 DNS/TLS，再开放首批用户。
