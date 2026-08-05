# Cloudflare 免费边缘入口迁移设计

**日期：** 2026-08-05  
**状态：** 已获方案确认，待书面规格复核后实施

## 背景与目标

生产入口 `api.xingqiaolab.top` 当前由 DNSPod 权威 DNS 直接解析到首尔源站。受影响 Mac 到该域名的 TLS 1.2 在 ClientHello 阶段被重置，而同一源站、其他 SNI 以及源站本机访问均正常；站内同时出现客户端请求体未读完和客户端主动断开的 400/499。既有 nginx 前置层试验未改变故障，因此根因已收敛为客户端到源站之间的域名特异网络干预。

本次目标是在不更换正式域名、不购买付费套餐、不重启生产服务的前提下，将公网 TLS 入口迁到 Cloudflare Free 边缘代理，并用受影响网络上的真实 TLS、HTTP 和 Codex 流式请求判断是否保留。若稳定性或延迟不达标，立即恢复同域名源站直连。

## 已确认约束

- 正式入口保持 `api.xingqiaolab.top`，不创建或迁移到新的用户入口域名。
- 仅使用 Cloudflare Free，不购买 Argo、China Network、Business 或其他付费能力。
- 当前仅有一名实际用户，不设计用户流量灰度或分群。
- 迁移不得要求重启 Caddy、Sub2API、PostgreSQL、Redis、worker 或 relay-ops。
- 不删除 DNSPod 现有区域和记录；迁移后至少保留 48 小时，作为权威 DNS 回退依据。
- 不创建 Cloudflare API Token，不把密码、验证码、Cookie、API Key 或 DNS 凭据写入 Git、文档、终端输出或聊天。
- 生产源站防火墙暂不限制为 Cloudflare IP，以保留关闭代理后的快速直连回退。

## 方案选择

### 采用：同域名全区域迁移，激活后再开代理

1. 在 Cloudflare Free 添加 `xingqiaolab.top`。
2. 从 DNSPod 逐条核对 Cloudflare 自动导入的 A、AAAA、CNAME、MX、TXT、CAA 及其他现有记录；导入结果必须与 DNSPod 权威区域等价。
3. `api.xingqiaolab.top` 在 Cloudflare 区域激活前保持 DNS only，继续返回当前源站地址。
4. 在腾讯云域名管理中将权威 NS 改为 Cloudflare 分配的两台 nameserver。
5. 等待 Cloudflare 显示区域 Active，并确认 Universal SSL 边缘证书覆盖 `api.xingqiaolab.top`。
6. 将 `api` A 记录切换为 Proxied（橙色云），不改主机名、不改源站地址。
7. 完成受影响网络上的验收；通过则保留橙色云，不通过则切回 DNS only。

NS 传播期间，不同递归解析器可能分别查询 DNSPod 或 Cloudflare。两侧在此阶段返回相同源站地址，因此不存在设计上的计划停机。只有 Cloudflare 区域和边缘证书均已就绪后才启用代理，避免证书签发窗口造成 TLS 故障。

### 不采用：切 NS 时直接开启橙色云

该方式步骤更少，但 Cloudflare 边缘证书可能尚未就绪，会引入可避免的 TLS 不可用窗口。

### 不采用：Cloudflare Tunnel 或新测试域名

Tunnel 需要新增常驻连接器和故障面；新测试域名改变用户入口并违背已确认约束。当前问题只需要替换公网 TLS/网络入口，不需要改变源站拓扑。

## Cloudflare 配置边界

- 计划：Free。
- SSL/TLS 模式：Full (strict)。
- 最低 TLS：保留 TLS 1.2，同时允许 TLS 1.3。
- `api`：最终为 Proxied；其余记录按实际用途决定，迁移阶段默认保持 DNS only，避免扩大变更面。
- WebSockets：保持启用，以免影响兼容客户端。
- 动态 API：`/responses`、`/v1/*`、`/api/*` 不得配置缓存；不得使用 Cache Everything。
- API 路径不得启用交互式 Challenge、Under Attack Mode 或会阻断非浏览器客户端的规则。
- 不在本次启用 Argo、Cloudflare Tunnel、Workers、Load Balancing、Origin Rules 或源站 IP 白名单。
- Cloudflare Free 的 100 MB 单请求上限作为新入口硬上限；超过该值的请求不属于本次验收范围，不能误判为源站故障。

## 数据流

迁移前：

```text
Codex / API 客户端 -> api.xingqiaolab.top -> 首尔 Caddy -> 活动 Sub2API API 槽 -> 上游
```

迁移后：

```text
Codex / API 客户端 -> Cloudflare 边缘 TLS -> 首尔 Caddy -> 活动 Sub2API API 槽 -> 上游
```

Cloudflare 只替换客户端到源站的公网入口，不参与 Sub2API 到上游模型的连接、账号调度、计费、数据库或 worker 流程。

## 实施前基线

切换前从受影响 Mac 固定采集：

- 当前 A/AAAA、NS、TLS 证书和响应头。
- TLS 1.2 与 TLS 1.3 各 20 次握手结果。
- `/health` 20 次状态码、连接时间、TLS 时间和总耗时。
- 最近 10 分钟 `ops_error_logs` 中 `/responses` 的 400、499、5xx 分类。
- Caddy、活动 API 容器健康、重启次数和宿主机资源快照。

基线只发送无认证健康请求；真实流式验收沿用现有本机客户端和现有有效 Key，密钥原文不进入命令输出或报告。

## 验收标准

Cloudflare 代理生效且 DNS 已解析到 Cloudflare 后执行：

1. DNS：`api.xingqiaolab.top` 不再公开解析为源站地址，并出现 Cloudflare 响应标识。
2. TLS：TLS 1.2 与 TLS 1.3 各连续 20 次握手全部成功。
3. 健康：`/health` 连续 20 次返回 HTTP 200，无 TLS reset、超时、403、413、502、522 或 524。
4. 流式：使用当前 Codex Desktop 完成至少 5 次真实 `/responses` 流式请求，每次收到首个事件并正常结束。
5. 站内日志：验收请求不得产生 `Failed to read request body`、客户端异常 499、Cloudflare 相关 4xx/5xx 或重复计费迹象。
6. 资源：Caddy、活动 API 槽、worker、PostgreSQL、Redis 和 relay-ops 保持健康且重启计数不增加。
7. 性能：在成功率和断流率达标前提下，Cloudflare `/health` 中位总耗时不得比可用的直连样本恶化超过 30%。直连样本无法完成 TLS 时，只比较成功率，不用失败样本计算延迟。

只有以上条件全部满足，才保留橙色云并将项目事项标记为“已完成”。

## 回退

### 一级回退：关闭代理

若 TLS、HTTP、流式、计费或延迟任一门禁失败，将 Cloudflare 中 `api` 记录从 Proxied 切回 DNS only。正式域名和源站地址不变，客户端恢复直连。随后重复 `/health`、TLS 和站内日志检查，确认故障没有扩大。

### 二级回退：恢复 DNSPod 权威 NS

只有 Cloudflare 权威 DNS 自身出现持续解析错误、记录缺失或账户级不可用时，才将腾讯云注册商 NS 恢复为原 DNSPod nameserver。DNSPod 区域及记录至少保留 48 小时，确保该回退无需重建区域。

回退不修改生产 Compose、Caddy、数据库或应用镜像。

## 错误处理与安全门禁

- Cloudflare 自动导入记录不完整：停止，不修改 NS，先完成逐条对账。
- Cloudflare 要求付费套餐：停止，保持 Free，不接受付费升级。
- 出现 CAPTCHA、验证码、重新认证或新的条款接受：由用户接管相应步骤。
- Universal SSL 未覆盖正式域名：保持 DNS only，等待或排查，不开启代理。
- DNSSEC 当前若已启用：必须先核对 DS 状态并按 Cloudflare 指引迁移，禁止保留指向旧 DNS 提供商的失效 DS。
- 任何无法解释的 MX/TXT/CAA 差异都视为阻塞，不以“只影响 API”为由忽略。

## 项目记录与完成口径

实施开始前在 `docs/project/project-progress.md` 登记为“进行中”。实施过程中记录基线、DNS 对账、NS 修改、证书激活、代理切换和验收结果。只有配置已经在线生效、正式域名经过真实 TLS/HTTP/流式验收且代码或文档提交已推送时，才能标记“已完成”；仅添加 Cloudflare 区域、修改 NS 或本地测试不能标记完成。
