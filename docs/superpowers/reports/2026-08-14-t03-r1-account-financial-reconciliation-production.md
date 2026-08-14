# T03-R1 账号财务与异常核对生产验收

日期：2026-08-14（Asia/Shanghai）

## 发布身份

- 生产源提交：`210d0397e647b91be080f0c7252da39a6e61d71d`
- 源码树 / 已测试树：`4e2b7be29191894a8e7fac7e7af21cb0cf4adb21`
- 迁移集合哈希：`6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`
- 发布记录：`/var/lib/sub2api/release-records/20260814T051143Z-production-2876774.json`
- 发布结果：`succeeded/promoted`，`rolled_back=false`，活动槽 `green`
- 用户已明确授权停机维护发布；受审维护链的 API/worker 不可用硬上限为 300 秒。发布成功后 PostgreSQL、Redis、Caddy 容器身份保持不变。

## 运行健康

2026-08-14 13:30（+08:00）重新只读核验：

- `sub2api-sub2api-green-1` 与 `sub2api-sub2api-worker-1` 均为 `healthy`，重启计数均为 `0`，镜像 ID 均为 `sha256:0a5468769321bdff32b42f5d53ab2bf4f78913dd4891543a78575bf3397578a5`。
- PostgreSQL、Redis、relay-ops 均为 `healthy`、重启计数 `0`；Caddy 运行中、重启计数 `0`。
- 公网 `/healthz`、`/readyz`、`/health` 均返回 HTTP `200`。
- 迁移 222 的四张新增表均存在；官方 `usage_logs` 未增加 T03-R1 成本持久化字段。

## 功能启用与自然流水

- `account_financial_settings.key=t03_r1_account_financial` 的 `enabled_at` 为 `2026-08-14 13:17:21.831859+08`；只观察该时间之后的新流水，没有历史回填。
- 首次稳定样本窗口内共有 15 笔启用后官方流水：3 笔生成一对一 evidence，均为账号 `23` 的 `newapi/confirmed`；12 笔没有 evidence，均来自账号 `183`，因没有明确持久化的 Sub/New 原生账本身份而投影为 `unavailable/evidence_not_registered`，没有猜测来源、补查、重试或估算。
- 已确认的自然样本为 usage `104709`（成本 `0.106698`）、`104711`（成本 `0.028844`）和 `104713`（成本 `0.043400`）；均在管理员打开详情前由现有响应后 usage 异步链自动登记。
- 自然流量继续发生时，管理员异常 API 的 pending 数从 12 增至 13，证明查询使用实时本地事实而非静态快照。

## 管理员 API 与隔离

2026-08-14 13:30（+08:00）的管理员只读请求：

- 今日财务：83 个账号；营收 `0.2236755` 元，上游扣费 `0.178942` 元，利润 `0.0447335` 元，利润率约 `19.9993%`；异常 13 笔，受影响营收 `0.39722277` 元；全站用户未消费金额 `331.95014002` 元。
- usage `104713` 本地详情约 `0.047` 秒返回，`source=newapi`、`evidence_status=confirmed`、`normalized_cost_cny=0.0434`，并带精确上游请求 ID。
- 财务首页、异常列表、本地成本详情三条管理员路由在未认证时均返回 HTTP `401`。
- 管理员详情代码已移除同步上游 fallback；线上详情读取本地持久化事实，不触发 Sub/New 请求。

## OAuth 与未覆盖自然样本

- 今日与 31 天报告均识别出 9 个字面 `oauth` 账号；本次所选自然窗口没有产生“有营收但当日成本待填写”的 OAuth 样本，因此没有执行任何成本写入或人工核对。
- OAuth 待填写、填写及汇总语义由已审定向测试覆盖；本次生产验收没有伪造数据来制造样本。

## 回滚与剩余风险

- 应用回滚：切回发布前 blue 镜像；独立新增表保留，不删除账务证据。
- 数据回滚：迁移为 expand-only；不对 `usage_logs` 做破坏性回退。
- 已知业务风险：没有明确持久化 Sub/New 原生账本身份的 API-key 账号会进入 `evidence_not_registered` 异常列表，需要管理员逐笔或批量核对；系统按硬合同不根据其他字段猜测来源。

结论：T03-R1 已完成推送、停机维护发布和线上专项验收；不得重复发布同一 SHA。T05 只能在独立用户可见顶层任务中按串行门禁继续。
