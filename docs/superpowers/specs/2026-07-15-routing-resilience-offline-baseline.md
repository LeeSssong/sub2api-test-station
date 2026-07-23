# L1-8 路由与韧性离线基线规格

**日期：** 2026-07-15  
**状态：** 已批准的 L1-8 离线实施细化；根据 D13 不购买线路、服务器或上游额度

## 1. 问题

Sub2API v0.1.155 已提供账号优先级、粘性会话、负载感知、并发槽位和针对 401/403/429/529 的暂停机制，但项目仍缺少跨上游统一比较、故障分类、安全重试边界、线路测量格式和扩容经济阈值。没有这些约束时，接入第二家中转 API 后容易只按低价路由、重复计费、放大故障或过早购买优化线路。

## 2. 目标

建立一个不连接真实上游的 ROUTE01 离线基线，使 AI 能用同一份非敏感数据完成：

- 判断上游是否具备参与用户流量或半开探测的资格。
- 按成功率、成本、TTFT、容量余量和支持响应计算可解释的 0-100 分。
- 模拟 `CLOSED -> OPEN -> HALF_OPEN -> CLOSED`，并让人工禁用始终优先。
- 只在未向用户输出且不会造成不受控重复计费时允许一次额外尝试。
- 区分“用户到入口”和“入口到上游”的网络数据。
- 只在观测阈值和经济阈值同时满足时建议升级内存、优化线路或第二节点。

## 3. 非目标

- 不修改 Sub2API 源码，不在 Caddy 增加运行时故障转移。
- 不向任何上游发请求，不读取 API Key，不测试真实服务器或三网线路。
- 不购买 CN2 GIA、CMI/CMIN2、联通优化线路或第二节点。
- 不承诺 SLA，不把虚构样本当成真实供应商质量。

## 4. 方案选择

### 采用：Sub2API 原生调度 + 离线策略护栏

Sub2API 继续执行实际账号/渠道选择；ROUTE01 只负责在配置前比较候选、模拟故障状态和给出人工操作建议。这个边界可以快速落地，也不会形成两个互相冲突的运行时调度器。

### 未采用：只写运维文档

成本最低，但公式、阈值和状态转换无法自动复验，接入新上游时容易出现同名字段和口径漂移。

### 未采用：新增 Envoy/Nginx 或自研代理层

能够做运行时熔断，但会增加 SSE、计费、重试和观测的第二套状态；当前单节点 MVP 没有数据证明需要这层复杂度。

## 5. ROUTE01 数据契约

配置只保存非敏感事实和假定值：

- `policy`：评分权重、归一化边界、切换滞后和最小样本。
- `retry_policy`：额外尝试次数、允许的失败类型、状态码和重复计费保护。
- `circuit_breaker`：滚动窗口、连续失败、失败比例、冷却、半开探测和退避上限。
- `capacity_policy`：纵向扩容、优化线路和第二节点的观测/经济阈值。
- `upstreams`：代号、模型、条款状态、成本、成功率、TTFT、429 比例、余额天数、并发余量和人工状态。
- `network_measurements`：运营商、时间段、网络分段、延迟、丢包、TLS、TTFT 和流中断；未有真实资产时为空。

禁止保存 Base URL 查询参数、鉴权头、API Key、Cookie、OAuth Token 或任何凭据值。

## 6. 资格与评分

用户流量资格先于评分：渠道必须启用、未人工禁用、条款不是 `forbidden`、支持模型、熔断为 `closed`、样本数达标、余额天数和并发余量不低于底线。`half_open` 只允许探测，不参与用户流量；`open` 和 `manual_open` 直接排除。

默认分数：成功率 30%、成本 25%、TTFT 20%、容量余量 15%、客服响应 10%。各指标按配置边界线性归一化到 0-100；最终分数保留两位。当前渠道不会因为新候选高出很小幅度立即切换：挑战者至少高 8 分并连续 3 个观测窗口领先，才建议变更主渠道。

## 7. 故障分类与熔断

- 400/404/409/422 等用户或请求错误：不惩罚渠道，不重试。
- 401/403：凭据或权益隔离，进入人工检查，不自动半开。
- 402：余额/账单暂停，待人工确认，不自动重试。
- 429：优先遵守 `Retry-After`；没有可信值时使用 30 秒容量冷却，不计为普通 5xx 熔断失败。
- 529：按 Sub2API 默认语义使用 10 分钟过载冷却。
- 连接/TLS/超时和 5xx：计入滚动熔断；连续 5 次失败，或至少 20 个样本中失败率达到 50%，转为 `open`。
- 冷却 60 秒后进入 `half_open`，每 30 秒只放 1 个合成/受控探测；连续 2 次成功后关闭，任一失败重新打开。重复打开按 60/120/240 秒退避，最高 900 秒。
- 人工禁用不会被计时器自动恢复。

## 8. 安全重试边界

最多 1 次额外尝试，并同时满足：

1. 尚未向用户发送响应头或流内容。
2. 没有已确认的上游扣费。
3. TLS/连接失败发生在请求体发送前；或失败状态被明确允许且上游确认未计费；或请求使用上游支持的幂等键。
4. 目标渠道与原渠道不同且当前可参与用户流量。

上游是否已经处理请求或是否计费为 `unknown` 时默认不重试。已经开始 SSE 后绝不跨渠道续传，避免内容拼接和重复扣费。

## 9. 网络与扩容决策

网络记录必须拆为 `client_to_entry` 和 `entry_to_upstream`，并区分中国电信/联通/移动与白天/20:00-23:00。没有每格至少 20 个真实样本时，不输出优化线路采购建议。

2 GiB 主机满足任一资源阈值才建议升 4 GiB：24 小时内 OOM/非人为容器重启至少 1 次，或 `MemAvailable < 300 MiB`、swap > 512 MiB、CPU > 70%、PostgreSQL 连接 > 80% 持续 15 分钟。优化线路还要求入口段是主要瓶颈。第二节点还要求 4 GiB 纵向扩容已完成、入口自身 7 日可用率低于 99.5% 或持续并发达到 30，并且预计月故障损失不低于新增节点月成本的 1.5 倍。

## 10. 接口

```text
ruby ops/evaluate-routing-baseline.rb validate config/routing/ROUTE01.example.yaml
ruby ops/evaluate-routing-baseline.rb score config/routing/ROUTE01.example.yaml gpt-test
ruby ops/evaluate-routing-baseline.rb demo config/routing/ROUTE01.example.yaml
```

CLI 只输出非敏感 JSON，明确标注 `offline_simulation: true` 和 `real_traffic_sent: false`。

## 11. 验收标准

- [x] 完整的虚构 ROUTE01 通过校验，真实网络样本保持为空。
- [x] 资格过滤和五项评分可重复计算并解释。
- [x] 熔断打开、半开成功恢复、半开失败退避和人工禁用均有测试。
- [x] 已开始响应、扣费未知、用户错误和已耗尽尝试均拒绝重试。
- [x] 只在所有观测和经济条件满足时输出扩容建议。
- [x] 专项、全量、YAML、Markdown 和秘密值检查通过。

## 12. 依据

- Sub2API v0.1.155 源码：账号优先级、粘性/负载感知调度、401/403/429/529 暂停。
- [One API](https://github.com/songquanpeng/one-api)：公开实现优先级、权重、自动禁用和可配置重试。
- [New API](https://github.com/QuantumNous/new-api)：公开实现渠道加权随机、失败重试和跳过重试标记。
- [AWS：Timeouts, retries and backoff with jitter](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/)。
- [AWS：Circuit breaker pattern](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/circuit-breaker.html)。
- [Envoy：Circuit breaking](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/circuit_breaking)。
- [NGINX：proxy_next_upstream](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_next_upstream)。
