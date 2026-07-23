# L1-8 路由与韧性离线基线验证

**日期：** 2026-07-15  
**范围：** ROUTE01 校验、上游资格/评分、安全重试、熔断状态机和容量建议  
**外部状态：** 未发送真实流量，未购买线路或节点，未读取凭据

## 结果

- ROUTE01 虚构配置通过校验，网络实测数组为空。
- 私有测试模式中，虚构 UP01 得分 80.12、UP02 得分 73.86；UP03 因人工禁用排除，UP04 只进入半开探测。
- 连接失败且请求体未发送的示例允许一次额外尝试；503 扣费状态未知的示例拒绝重试。
- 合成熔断序列为 `open -> half_open -> closed`，人工禁用不会自动恢复。
- 所有 CLI 输出均标记 `offline_simulation: true`、`real_traffic_sent: false` 和 `purchase_action_taken: false`。

## TDD 证据

1. 首次运行因 `ops/evaluate-routing-baseline.rb` 不存在而失败。
2. 最小实现后发现 Token 计量字段被秘密扫描误报，收紧为完整凭据字段名匹配。
3. 修正后补充 429 资格门槛、半开探测间隔和配置完整性测试；专项测试通过：15 tests / 62 assertions / 0 failures / 0 errors。

## 验证命令

```bash
ruby -w tests/routing/evaluate_routing_baseline_test.rb
ruby ops/evaluate-routing-baseline.rb validate config/routing/ROUTE01.example.yaml
ruby ops/evaluate-routing-baseline.rb score config/routing/ROUTE01.example.yaml gpt-test
ruby ops/evaluate-routing-baseline.rb demo config/routing/ROUTE01.example.yaml
ruby -c ops/evaluate-routing-baseline.rb
for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done
bash tests/infra/validate-baseline.sh
```

## 全量验证

- ROUTE01 专项：15 tests / 62 assertions / 0 failures / 0 errors。
- 全量 Ruby 回归：65 tests / 256 assertions / 0 failures / 0 errors。
- 基础设施契约：通过；未改动现有容器。
- 8 份示例 YAML 均可解析；项目 Markdown 围栏平衡。
- ROUTE01 本地文件规则已被 Git 忽略；受控配置和文档未发现真实秘密值模式或已填充凭据字段。
- 路由评估器未引入 HTTP、Socket 或其他网络客户端。

## 未验证

- 真实上游的商用/再分发状态、成本、余额、限额、成功率和计费失败语义。
- 中国电信、联通、移动的白天和晚高峰真实数据。
- Sub2API 管理界面的真实分组、优先级、冷却和故障切换。
- 2 GiB 主机的真实资源瓶颈、4 GiB 升级收益和第二节点经济性。

这些项目保留在 `docs/superpowers/checklists/routing-live-acceptance.md`，根据 D13 延后。
