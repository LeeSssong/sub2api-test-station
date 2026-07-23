# D04 XM/Wawazz 公共非功能基线审计

**日期：** 2026-07-22（Asia/Shanghai）  
**执行者：** 非功能基线子 Agent  
**范围：** 只读复核当前工作树、公共 v3 benchmark、XM live directory 台账、现有生产只读报告和目标拓扑证据边界。  
**结果：** `IMPLEMENTATION PASS / TARGET TOPOLOGY NOT_READY`

## 1. 目标拓扑

```text
GPT-Plus -> XM API Plus primary -> Wawazz backup
GPT-Pro  -> XM API Pro primary  -> Wawazz backup
```

本次没有发起任何新请求、创建/读取 Key、创建候选、启用 probe、改路由、部署服务或写生产数据。XM 目录请求由前序授权阶段完成，证据只引用其无凭据摘要和运行 ID。

## 2. 要求与证据

| 要求 | 当前权威证据 | 判定 |
|---|---|---|
| XM Plus/Pro authenticated `/models` | `xm-plus` run `fa32a9e3-c817-4a81-a43c-2c41ae1273da`：HTTP 200、20 模型、15 文本；`xm-pro` run `94d3463f-7c07-4cd8-a0d7-3fec19a04be5`：HTTP 200、13 模型、10 文本；均 1 次目录、0 生成，状态 `partial / discovered_not_qualified` | 已满足目录发现；资格仍缺失 |
| XM 逐模型 sync/SSE | Responses 适配器和 dry-run 测试；没有 XM live 样本 | 缺失 |
| XM gateway primary | 没有隔离 Sub2API 账号/用户/Key 的目标角色样本 | 缺失 |
| XM billing | 没有 API usage、Sub2API debit、供应商实际账单三方对账 | 缺失 |
| XM sync/SSE/RPM 容量 | v3 阶梯为离线能力；Plus/Pro 尚无 live lower bound | 缺失 |
| Wawazz Plus backup | 现有证据是 GPT-Plus 的 primary 角色，不是 Wawazz backup 角色 | 缺失/角色不匹配 |
| Wawazz Pro backup | 没有 GPT-Pro 目标模型和 gateway backup 样本 | 缺失 |
| Wawazz shared backup | 没有两组隔离成员单测、等量并发、批准的 traffic mix 或公平性证据 | 缺失 |
| 24–72 小时目标角色观察 | 没有按 group/role/model/account 的连续窗口 | 缺失 |
| failover/failback | 只有既有 dry-run 路由控制和离线时间线 evaluator；没有隔离 drill 时间线 | 缺失 |
| 条款 | XM/Wawazz 转售、退款、调价、容量支持条款未知 | 缺失 |
| 当前生产安全边界 | 生产仍是 Pro→Neko、Plus→Wawazz、Aliu shared backup；relay-ops `read_only + dry_run` | 已满足安全边界，未达到目标拓扑 |
| 公网入口 | 既有单地点低并发 GET 基线 `60/60` HTTP 200 | 仅入口起点，不是模型容量或 SLA |

### XM 目录发现结果

目录结果来自 `docs/superpowers/reports/2026-07-22-upstream-live-directory-discovery.md` 及追加式 ledger：

| Channel | Run ID | HTTP | 目录耗时 | 模型总数 | 文本可测 | 非文本 |
|---|---|---:|---:|---:|---:|---|
| `xm-plus` | `fa32a9e3-c817-4a81-a43c-2c41ae1273da` | 200 | 2332 ms | 20 | 15 | audio 1、realtime 1、image 3 |
| `xm-pro` | `94d3463f-7c07-4cd8-a0d7-3fec19a04be5` | 200 | 1477 ms | 13 | 10 | image 3 |

目录证据不包含生成请求、价格/账单、网关角色或容量结论；临时 Key 已清理。

### Wawazz 风险上下文

当前报告中的 Wawazz 只读样本包含约 `$9.62` 余额、当日实际约 `$32.17`、7 天可用性 `94.70%` 和约 `34.49s` TTFT 长尾。它们不是目标 backup 角色的本站分母证据；但余额按该消耗速度明显不足 OPS01 的 3 天 runway，且可用性低于 `95%` 继承门槛，因此不能把 Wawazz 标记为已合格灾备。

## 3. v3 实现复核

已确认 benchmark 没有 XM、Wawazz、Neko、hostname、vendor 或模型特判。公共实现覆盖：

- 独立 sync/SSE/RPM 阶梯和 SSE 终止语义；
- barrier overlap、nearest-rank P50/P95、错误分类和成本/Token 汇总；
- `ExecutionBudget` 的请求、Token、货币、墙钟、总耗时和 queue stop；
- 样本 `run_id`、`recorded_at`、profile hash、role、group、account evidence 和测量位置绑定；
- shared pool 的 isolated/equal_demand/approved_mix 阶段、成员身份、两类请求、波次重叠和公平性；
- 观察窗口的每角色连续小时、身份、sync/SSE、成功率、429、5xx、TTFT、总耗时和 stream interruption 门禁；
- drill 的单调时间线、route read-after-write hash、backup/primary sync+SSE 验证和 primary recovery window；
- topology evidence 的 `scenario_id` 与 `scenario_hash` 绑定，防止跨 scenario 复用。

本轮新增的最小 TDD 修复：

- 拒绝未知 `topology_phase` 和缺失 `wave_id`；
- 拒绝缺失观察窗口 TTFT、总耗时或 SSE 完整性指标；
- 拒绝 shared-pool 成员缺失 TTFT/总耗时；
- 拒绝绑定到其他 scenario 的 offline evidence。

代码和测试保持 vendor-neutral，未增加 live executor 或路由写命令。

## 4. 验证结果

```text
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb
32 runs / 160 assertions / 0 failures / 0 errors / 0 skips

ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
32 runs / 194 assertions / 0 failures / 0 errors / 0 skips

ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
10 runs / 44 assertions / 0 failures / 0 errors / 0 skips

ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb
18 runs / 63 assertions / 0 failures / 0 errors / 0 skips

ruby -c ops/upstream-benchmark-nonfunctional.rb       PASS
ruby ops/upstream-benchmark-v2.rb validate            PASS
```

公共 v3 profile hash 为：

```text
03cb79b0fc91b70f2dba01953db42f7b50245bf946dd15d002e7db5c86ba0390
```

完整阶段按 `HTTP = D + 2M + 70 + K`、`generation = 2M + 70 + K` 计算。下面第一张表保留目录阶段（`D=1`），第二张表是目录已完成后的资格阶段（`D=0`）。

| Channel | M | K=0（含目录）HTTP / generation | K=4（含目录）HTTP / generation | K=4 最大输出 Token |
|---|---:|---:|---:|---:|
| `xm-plus` | 15 | `101 / 100` | `105 / 104` | `832` |
| `xm-pro` | 10 | `91 / 90` | `95 / 94` | `752` |
| 合计 | 25 | `192 / 190` | `200 / 198` | `1584` |

已完成目录后，下一阶段应使用 `--exclude-discovery`：

| Channel | M | K=0（不含目录）HTTP / generation | K=4（不含目录）HTTP / generation |
|---|---:|---:|---:|
| `xm-plus` | 15 | `100 / 100` | `104 / 104` |
| `xm-pro` | 10 | `90 / 90` | `94 / 94` |
| 合计 | 25 | `190 / 190` | `198 / 198` |

对应 dry-run 均输出 `requests_sent=0`、`network_sent=false`。默认 `M=3,K=4` 的历史示例仍为 `81/80`，不能用于 XM 的实际预算。

此前的默认 v3 dry-run（`M=3`、目录发现、`K=4`）输出：

```text
maximum_http_requests=81
maximum_generation_requests=80
requests_sent=0
network_sent=false
```

## 5. 安全下一步与授权

### A. 完整资格评测（目录阶段已完成）

必须绑定具体 profile/scenario ID 和 hash，使用 `--exclude-discovery`，按 direct、gateway primary、gateway backup、shared pool 分阶段授权。每一阶段都要有最大请求/Token/货币/墙钟预算；Plus 当前 K=0 上界 `100/100`，Pro 当前 K=0 上界 `90/90`。K=4 时分别为 `104/104`、`94/94`。这些是每渠道/角色上界，不能把它们误当作整个双组拓扑预算。

目录发现已经证明可测模型集合，但没有授权自动进入 compatibility；后续仍需重新确认具体 `K`、最大总 Token、货币、墙钟和停止阈值。

停止条件还包括首个 429/5xx/认证/余额/协议错误、SSE 未终止、未知账单、overlap 不足、队列或总耗时阈值、跨上游重试风险和清理失败。

### B. 观察与 drill

只有兼容性、计费、容量和 shared-pool 阶段通过后，才批准 24 小时观察（出现错误或退化再扩展到 72 小时），以及隔离环境 failover/failback drill。生产仍不能进入 `enabled`；只有生成 secret-free proposal 并由用户明确采纳 proposal ID/hash 后，才可讨论生产拓扑变更。

## 6. 当前结论

公共非功能 benchmark 的离线实现和证据门禁已通过；XM 目录发现已完成，但两条记录明确仍是 `discovered_not_qualified`。XM primary 的 sync/SSE、gateway、billing、capacity、条款和 proposal 证据仍缺失；Wawazz Plus/Pro backup、shared pool、观察和 drill 证据仍缺失。Wawazz 余额 runway 还构成现有 OPS01 门槛风险。当前状态应保持：

```text
relay-ops = read_only + dry_run
production routing = unchanged
target topology = NOT_READY
```

下一主线是基于 `M_plus=15`、`M_pro=10`，先提交不含目录阶段的完整资格评测预算；不得直接切换 XM primary 或把现有 Wawazz primary 历史证据复用于 backup 角色。
