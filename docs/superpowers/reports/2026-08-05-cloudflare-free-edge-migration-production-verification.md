# Cloudflare Free 公网入口迁移：直连基线

**采集结论：** 迁移事项保持进行中。`api.xingqiaolab.top`、首尔源站、Cloudflare Free 计划边界和生产容器拓扑均未改变；本次仅完成只读直连基线采集，未修改 DNSPod、Cloudflare 权威 NS 或代理状态，未重启生产服务。

## 采集窗口

- **UTC：** `2026-08-05T17:49:26Z` 至 `2026-08-05T17:50:08Z`
- **入口：** `api.xingqiaolab.top`
- **范围：** 公共 DNS/TLS/HTTPS 健康探针，以及生产 `sub2api-prod` 上的只读容器和错误聚合查询。

## DNS 基线

- **NS：** `golf.dnspod.net.`、`train.dnspod.net.`
- **A：** `43.133.75.82`
- **AAAA：** 无记录
- **DS（DNSSEC）：** 无记录
- **CAA：** 无记录
- **变更边界：** DNSPod 仍为权威 DNS；Cloudflare 区域尚未切换，`api` 未开启橙色云代理。

## TLS 基线

使用 SNI `api.xingqiaolab.top` 对公网入口连续执行各 20 次握手：

| 协议 | 成功 | 失败 |
|---|---:|---:|
| TLS 1.2 | 0 | 20 |
| TLS 1.3 | 20 | 0 |

TLS 1.2 的 20 次失败与既有受影响客户端直连故障基线一致；TLS 1.3 的 20 次握手全部成功。

## `/health` 基线

- **样本：** 20 次直连 HTTPS 请求
- **HTTP 200 成功：** 0/20
- **连接重置/失败：** 20/20（均未完成 TLS，返回码 `000`）
- **成功样本总耗时中位数：** 不适用（无成功样本）
- **说明：** 探针未记录客户端地址；失败样本不用于延迟中位数。

## 生产容器与资源快照

以下七个带 `com.docker.compose.project=sub2api` 标签的容器均保持运行；未执行重启或重建：

| 容器 | 短 ID | 状态 |
|---|---|---|
| `sub2api-relay-ops-1` | `342d58ebdec7` | Up 2 hours (healthy) |
| `sub2api-sub2api-worker-1` | `7e218a28a62d` | Up 24 hours (healthy) |
| `sub2api-sub2api-green-1` | `9171b00cd77a` | Up 24 hours (healthy) |
| `sub2api-sub2api-blue-1` | `cfbaea1abd30` | Up 24 hours (healthy) |
| `sub2api-caddy-1` | `ace4a23b9650` | Up 2 days |
| `sub2api-postgres-1` | `2db52788ad73` | Up 2 weeks (healthy) |
| `sub2api-redis-1` | `c45202c0d9e6` | Up 2 weeks (healthy) |

资源快照（CPU、内存、PIDs）已采集用于后续对比；本报告不记录宿主机或客户端地址。

## 最近 10 分钟错误聚合

数据库只读聚合输出按 `status_code | error_phase | error_type | error_message | count` 返回，已移除请求 ID、余额细节及其他敏感内容：

| 状态码 | 阶段 | 类型 | 脱敏消息分类 | 次数 |
|---:|---|---|---|---:|
| 502 | request | upstream_error | 上游余额不足 | 38 |
| 200 | upstream | upstream_error | 恢复请求：上游余额不足 | 1 |
| 200 | upstream | upstream_error | 恢复请求：上游预扣费失败 | 1 |
| 200 | upstream | upstream_error | 恢复请求：上游访问被拒绝 | 1 |
| 499 | upstream | api_error | 空错误消息 | 1 |

该聚合仅用于迁移前对比，不代表 Cloudflare 已介入请求链路。

## 当前门禁结论

正式域名和源站保持不变；Cloudflare Free 区域已经创建并处于等待注册商 nameserver 传播的 Pending 阶段。当前权威 DNS 仍为 DNSPod，`api`/`shop` 在 Cloudflare 中保持 DNS only。下一步在修改权威 NS 前仍须执行 Task 3 的最终门禁与操作时确认；在区域 Active、Universal SSL 覆盖正式域名前，不得开启 `api` 橙色云。

## Task 2：Cloudflare Free 区域与 DNS 对账证据（2026-08-06）

- **Cloudflare 区域状态：** Free `$0` 方案已选中，`Continue to activation` 已成功提交；`xingqiaolab.top` 区域已创建，当前页面显示等待注册商 nameserver 传播（Pending）。
- **Cloudflare 分配 NS：** `brian.ns.cloudflare.com.`、`gabriella.ns.cloudflare.com.`。本任务仅记录 Cloudflare 分配结果，没有在腾讯云/DNSPod 修改权威 NS。
- **Cloudflare 导入记录：** DNS Records 页面可见 8 条记录，按类型为 `A 2`、`MX 2`、`TXT 4`，无 `AAAA`。`api.xingqiaolab.top` 和 `shop.xingqiaolab.top` 的 A 目标均为 `43.133.75.82`；MX 目标分别为根域 `mxbiz1.qq.com`（优先级 10）与 `send` 子域 `feedback-smtp.ap-northeast-1.amazonses.com`（优先级 10）。TXT 仅记录名称/数量（根域、`_dmarc`、`send`、`resend._domainkey`），未记录任何 TXT 内容。
- **代理状态：** 主线程已在确认页将 `api` 与 `shop` 两条 A 记录改为 `DNS only`；本子任务未再次修改，也未开启橙云。
- **DNSPod 逐条元数据核验：** 公共 DNS 只能交叉确认名称、类型、目标和 MX 优先级；以下 DNSPod 管理台字段仍未能读取，不能据此证明“每条已启用记录”的完整对账：

  | 主机记录 | 类型 | 目标/值记录口径 | 线路/线路类型 | TTL | 启用状态 |
  |---|---|---|---|---|---|
  | `api` | A | `43.133.75.82` | 默认（DNSPod 截图可见） | 600（截图可见） | 已启用（绿色指示） |
  | `shop` | A | `43.133.75.82` | 默认（DNSPod 截图可见） | 600（截图可见） | 已启用（绿色指示） |
  | `send` | MX | `feedback-smtp.ap-northeast-1.amazonses.com`，优先级 10 | 未核验 | 未核验 | 未核验 |
  | `@` | MX | `mxbiz1.qq.com`，优先级 10 | 未核验 | 未核验 | 未核验 |
  | `_dmarc` | TXT | 仅记录名称，不记录值 | 未核验 | 未核验 | 未核验 |
  | `qcloudhk2048._domainkey` | TXT | 仅记录名称，不记录值 | 默认（DNSPod 截图可见） | 600（截图可见） | 已启用（绿色指示） |
  | `send` | TXT | 仅记录名称，不记录值 | 未核验 | 未核验 | 未核验 |
  | `@` | TXT | 仅记录名称，不记录值 | 未核验 | 未核验 | 未核验 |

  因此目前只能确认部分 DNSPod 行的线路/TTL/enabled state，不能宣称所有 8 条记录均已完成字段级对账；未记录任何 TXT 值。截图还确认实际启用的 DNSPod DKIM 名称是 `qcloudhk2048._domainkey`，而既有 Cloudflare 导入证据记录的是 `resend._domainkey`。该名称不一致是明确的 parity blocker，除非 Cloudflare 记录表复核后证明已经纠正。
- **DNSSEC：** 公共 DNS `DS xingqiaolab.top` 查询无结果，说明当前没有注册商侧 DS 阻塞；但 Cloudflare DNSSEC 设置页的只读导航在本轮超时，未能证明 Cloudflare 控制台明确显示 Disabled/关闭。
- **Task 2 判定：** **阻塞，保持进行中。** Free/Pending、两台 Cloudflare nameserver、`api`/`shop` DNS-only 状态和公共 DS 结果已记录；其余 DNSPod 行的线路/TTL/enabled state、Cloudflare DNSSEC Disabled，以及 `qcloudhk2048._domainkey` 与 Cloudflare 既有 `resend._domainkey` 的名称差异仍未解决。Task 3 不得替换权威 NS；本任务未修改腾讯云/DNSPod NS、未开启橙云、未重启生产服务。

### Fix round 1 review evidence (2026-08-06)

- DNSPod record-management URL reached: `https://console.dnspod.cn/dns/list/detail/xingqiaolab.top/records` (logged-in Chrome page title: `我的域名 - DNSPod-免费智能DNS解析服务商-电信_网通_教育网,智能DNS`).
- Read-only attempts to obtain the table via `dom_cua.get_visible_dom()` and CDP `Runtime.evaluate` timed out before returning row metadata; no TXT content was emitted.
- Cloudflare DNSSEC URL attempted: `https://dash.cloudflare.com/a2e1d7e8a44f705df43758078c7289e9/xingqiaolab.top/dns/settings`; read-only navigation timed out before a Disabled/Enabled control could be observed.
- Safe public checks retained for context:

  ```text
  dig +short NS xingqiaolab.top
  train.dnspod.net.
  golf.dnspod.net.

  dig +short DS xingqiaolab.top
  (no output)
  ```

### Fix round 2 review evidence (2026-08-06)

- User-provided current DNSPod screenshot proves the visible rows have green enabled indicators, line `默认`, TTL `600`; it visibly confirms `api` and `shop` A targets `43.133.75.82` and the enabled TXT record name `qcloudhk2048._domainkey`. No TXT value was copied or persisted.
- This contradicts the earlier Cloudflare inventory evidence naming `resend._domainkey`; record counts/types alone are therefore insufficient, and Task 3 remains blocked pending a Cloudflare record-table recheck/correction.
- The logged-in Chrome tab remained on `https://console.dnspod.cn/dns/list/detail/xingqiaolab.top/records`, but screenshot capture, DOM reads, and page scroll all timed out before the remaining rows could be observed.
- Cloudflare DNSSEC Disabled was not inferred: no explicit Disabled control was observed in this round, so that gate remains open.
