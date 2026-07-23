# L1-9 运营与止损离线基线实施计划

> **执行约束：** 当前主助手在本任务内执行；不使用子代理，不提交 Git，不调用管理 API，不停止容器，不执行真实备份/恢复或外部付款。

**Goal:** 交付可验证的 OPS01 日常检查、告警分级、事故止损建议和备份/恢复离线基线。

**Architecture:** YAML 保存非敏感阈值和虚构快照；Ruby 标准库只输出告警与符号动作。真实操作留在运行手册和验收清单，由账户所有人在最终报告后确认。

**Tech Stack:** Ruby 标准库、YAML、JSON、Minitest、Markdown。

## Global Constraints

- `action_execution_mode` 固定为 `report_only`。
- 不允许 HTTP、Socket、进程控制、Docker 或数据库客户端调用。
- 真实快照只写 `*.local.yaml`，禁止凭据字段和值。
- Critical 先止损、再调查；动作只输出，不执行。
- 备份/恢复保持计划态，不接触现有容器和数据库。

## 文件职责

- `config/operations/OPS01.example.yaml`：节奏、阈值、备份选择和允许动作。
- `config/operations/daily-snapshot.example.yaml`：健康虚构快照。
- `ops/evaluate-operations-baseline.rb`：配置/快照校验和告警评估。
- `tests/operations/evaluate_operations_baseline_test.rb`：红绿测试。
- `docs/runbooks/operations-and-incident-response.md`：日/周/月和事故运行手册。
- `docs/superpowers/checklists/backup-and-restore-live-acceptance.md`：真实备份恢复验收。
- `docs/superpowers/reports/2026-07-15-operations-baseline-verification.md`：验证证据。
- `docs/project/final-recommendation-report.md`：全部假定资产与推荐配置汇总。

## 任务

- [x] 1. 写 OPS01、快照完整性、报告模式和秘密字段失败测试。
- [x] 2. 写账务、凭据、全上游、系统、备份、成本和渠道阈值失败测试。
- [x] 3. 写动作去重、顺序和 CLI 纯离线输出失败测试。
- [x] 4. 运行专项测试并确认实现缺失导致红灯。
- [x] 5. 实现最小校验器和运营评估器，使专项测试通过。
- [x] 6. 创建 OPS01/健康快照并运行 validate/evaluate/demo。
- [x] 7. 编写运营事故手册、备份恢复清单和验证报告。
- [x] 8. 生成最终推荐配置报告，更新采购、资产、主计划和项目状态。
- [x] 9. 运行专项、全量回归、基础设施、YAML/Markdown、秘密值和离线边界检查。

## 验证命令

- `ruby -w tests/operations/evaluate_operations_baseline_test.rb`
- `ruby ops/evaluate-operations-baseline.rb validate config/operations/OPS01.example.yaml`
- `ruby ops/evaluate-operations-baseline.rb evaluate config/operations/OPS01.example.yaml config/operations/daily-snapshot.example.yaml`
- `ruby ops/evaluate-operations-baseline.rb demo config/operations/OPS01.example.yaml config/operations/daily-snapshot.example.yaml`
- `for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done`
- `bash tests/infra/validate-baseline.sh`

## Acceptance

- [x] 规格第 10 节全部满足。
- [x] Critical 止损建议优先级和顺序可重复。
- [x] 所有验证通过且没有外部副作用。
- [x] 最终报告明确区分推荐、未购买和未验证。

## 风险

- 手工快照可能漏填；正式运营后应逐步接入只读采集并保留同一字段契约。
- 报告模式不能替代值班响应；真实用户放量前必须明确负责人和联系方式。
- `pg_dump` 成功不代表可恢复，必须在独立临时数据库做月度复验。
