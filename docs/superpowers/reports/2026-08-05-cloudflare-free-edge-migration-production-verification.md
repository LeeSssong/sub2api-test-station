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

正式域名和源站保持不变；Cloudflare Free 迁移尚未进入 DNS 或代理变更阶段。下一步必须在不改变本基线边界的前提下完成 Cloudflare 记录逐条对账；在区域 Active、Universal SSL 覆盖正式域名前，不得修改权威 NS 或开启 `api` 橙色云。
