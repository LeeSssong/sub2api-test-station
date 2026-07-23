# L1-8 路由与韧性离线基线实施计划

> **执行约束：** 当前主助手在本任务内执行；不使用子代理，不提交 Git，不连接真实上游，不购买线路/节点，不读取凭据。

**Goal:** 交付可验证的 ROUTE01 上游评分、熔断、安全重试、网络测量和扩容离线基线。

**Architecture:** 保留 Sub2API v0.1.155 为唯一运行时调度器。Ruby 标准库工具只读取非敏感 YAML，执行资格过滤、评分和状态机模拟，不代理请求也不改变运行中容器。

**Tech Stack:** Ruby 标准库、YAML、JSON、Minitest、Markdown。

## Global Constraints

- 所有上游、网络和容量数据均为虚构样本或空模板。
- `real_traffic_sent` 固定为 `false`。
- 最多建议一次额外尝试；响应已开始或扣费未知时禁止重试。
- 人工禁用高于自动恢复；半开渠道只允许探测。
- D10/D11 保持未触发，线路和第二节点均不采购。

## 文件职责

- `config/routing/ROUTE01.example.yaml`：非敏感策略和虚构候选。
- `ops/evaluate-routing-baseline.rb`：校验、资格/评分、重试、熔断和容量决策。
- `tests/routing/evaluate_routing_baseline_test.rb`：策略的红绿测试。
- `docs/project/routing-and-resilience.md`：运营映射、阈值和推荐结论。
- `docs/superpowers/checklists/routing-live-acceptance.md`：真实上游、三网和扩容验收。
- `docs/superpowers/reports/2026-07-15-routing-resilience-verification.md`：离线验证证据。

## 任务

- [x] 1. 写完整配置、资格过滤、评分和秘密字段的失败测试。
- [x] 2. 写重试拒绝/允许、熔断、半开恢复和人工禁用的失败测试。
- [x] 3. 写扩容阈值和 CLI 非联网输出的失败测试。
- [x] 4. 运行专项测试，确认因实现缺失红灯。
- [x] 5. 实现最小校验器和策略模拟器，使专项测试通过。
- [x] 6. 创建 ROUTE01 虚构配置并运行 validate/score/demo。
- [x] 7. 编写项目说明、真实验收清单和验证报告。
- [x] 8. 更新采购建议、资产台账、主计划和项目当前状态。
- [x] 9. 运行专项、全量回归、基础设施契约、YAML/Markdown 和秘密值检查。

## 验证命令

- `ruby -w tests/routing/evaluate_routing_baseline_test.rb`
- `ruby ops/evaluate-routing-baseline.rb validate config/routing/ROUTE01.example.yaml`
- `ruby ops/evaluate-routing-baseline.rb score config/routing/ROUTE01.example.yaml gpt-test`
- `ruby ops/evaluate-routing-baseline.rb demo config/routing/ROUTE01.example.yaml`
- `for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done`
- `bash tests/infra/validate-baseline.sh`

## Acceptance

- [x] 规格第 11 节全部满足。
- [x] 虚构渠道评分、熔断和扩容建议可解释且可重复。
- [x] 所有测试、结构和秘密值检查通过。
- [x] 未发送真实流量，未购买或启用任何外部资产。

## 风险

- 离线阈值不能替代真实三网、上游计费和晚高峰数据。
- Sub2API 不一定提供本规格的全部跨上游指标，正式接入时需要从日志、余额查询或外部采集补齐。
- 未确认是否计费的 POST 重试可能产生重复成本，因此默认拒绝。
