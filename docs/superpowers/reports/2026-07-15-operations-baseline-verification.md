# L1-9 运营与止损离线基线验证

**日期：** 2026-07-15  
**范围：** OPS01 策略/快照校验、告警分级、止损动作、运营和备份恢复文档  
**外部状态：** 未调用管理 API，未停止容器，未执行备份/恢复或任何外部操作

## 结果

- OPS01 固定为 `report_only`，备份策略固定为 `dry_run_only`。
- 健康虚构快照得到 0 Critical、0 High、0 Warning。
- 事故演示识别全上游不可用、余额差异和凭据暴露共 3 个 Critical，并给出去重的止损动作。
- 所有 CLI 输出均标记 `real_action_executed: false` 和 `external_system_contacted: false`。

## TDD 证据

1. 首次运行因 `ops/evaluate-operations-baseline.rb` 不存在而失败。
2. 实现后发现当前 Ruby 不支持 `Hash#filter_map`，改为兼容的显式遍历。
3. 补充快照完整性、零请求日和账号池错误测试后，专项通过：14 tests / 72 assertions / 0 failures / 0 errors。

## 全量结果

- 全量 Ruby 回归：79 tests / 328 assertions / 0 failures / 0 errors / 0 skips。
- 基础设施静态契约通过；只运行 `docker compose config`，没有启动、停止或重建容器。
- 10 份示例 YAML 全部可解析，Markdown 围栏平衡，`config/operations/*.local.yaml` 已被 Git 忽略。
- OPS01 受控数据和文档没有高置信秘密值；评估器没有网络、数据库、Docker 或进程控制调用。

## 验证命令

```bash
ruby -w tests/operations/evaluate_operations_baseline_test.rb
ruby ops/evaluate-operations-baseline.rb validate config/operations/OPS01.example.yaml
ruby ops/evaluate-operations-baseline.rb evaluate config/operations/OPS01.example.yaml config/operations/daily-snapshot.example.yaml
ruby ops/evaluate-operations-baseline.rb demo config/operations/OPS01.example.yaml config/operations/daily-snapshot.example.yaml
ruby -c ops/evaluate-operations-baseline.rb
for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done
bash tests/infra/validate-baseline.sh
```

## 未验证

- 生产健康、证书、磁盘、登录、上游、账号池、账本、成本和请求 ID 数据采集。
- 管理后台止损动作的真实权限、顺序和恢复。
- PostgreSQL 每日备份、加密异地副本和独立恢复演练。
- 真实用户通知、客服和值班响应。

这些项目保持在运行手册和真实验收清单中，根据 D13 延后。
