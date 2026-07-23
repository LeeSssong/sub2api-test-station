# L1-9 运营与止损离线基线规格

**日期：** 2026-07-15  
**状态：** 已批准的 L1-9 离线实施细化；根据 D13 不触发外部操作

## 1. 问题

前序阶段已经准备部署、上游、定价、账本、订阅账号、支付和路由基线，但仍缺少一个统一的日常判断入口。若只靠临时查看后台，余额差异、重复入账、上游同时故障、凭据暴露、备份过期和成本失控可能使用不同口径，事故发生时也容易先调查、后止损。

## 2. 目标

建立 OPS01 离线基线：读取一份不含凭据的虚构日快照，输出按 `critical/high/warning/info` 分级的告警和推荐动作，同时固定 `action_execution_mode: report_only`，确保工具不会修改 Sub2API、容器、数据库、DNS、支付或上游。

覆盖：

- 服务、证书、磁盘、备份、恢复演练和管理登录。
- 上游余额、成功率、429、5xx、TTFT、流中断和全渠道不可用。
- 收款/余额差异、重复入账、无法解释的人工调整和日成本上限。
- 凭据暴露、请求 ID 覆盖和事故证据保留。
- 每日检查、每周毛利/供应复盘和每月恢复演练的工作节奏。

## 3. 非目标

- 不部署 Prometheus、Grafana、Loki 或外部告警服务。
- 不登录 Sub2API，不调用管理 API，不停止运行中的容器。
- 不执行真实备份或恢复，不读取 `infra/.env`。
- 不自动禁用用户、Key、账号、分组、注册或支付。
- 不把虚构告警结果当作真实生产状态。

## 4. 方案选择

### 采用：离线快照评估 + 人工确认止损

用 YAML 保存阈值和虚构快照，Ruby 标准库输出统一告警。真实部署后可先由 AI/管理员每日填本地快照；数据量和收入证明需要时，再把同一字段接到定时采集或监控系统。

### 未采用：立即部署完整监控栈

可视化更强，但会占用 2 GiB 主机资源并扩大维护面；首版请求量、资产和真实 SLA 都不存在。

### 未采用：自动调用管理 API 止损

响应更快，但需要保存高权限凭据并承担错误禁用、越权和恢复失败风险。首版只输出动作顺序，由管理员在真实后台复核执行。

## 5. 数据契约

### OPS01 策略

- `action_execution_mode` 必须是 `report_only`。
- `thresholds` 保存对账、备份、证书、磁盘、登录、上游和成本门槛。
- `cadence` 保存日/周/月节奏。
- `backup_policy` 保存 `pg_dump -Fc`、restic、Cloudflare R2 Standard、保留期、异地加密和每月恢复演练选择；当前 `dry_run_only: true`。
- `stop_loss_actions` 只保存允许输出的符号动作，不保存 URL、Token 或命令凭据。

### 日快照

只允许 `status: fictional` 的示例进入 Git。真实数据写入 `config/operations/*.local.yaml`：系统健康、证书/磁盘/备份、账务差异、成本、请求 ID 覆盖、上游指标和事故标志。

## 6. 告警与动作

### Critical

- 凭据暴露：`disable_affected_channel -> revoke_exposed_credential -> rotate_credential -> preserve_evidence`。
- 余额差异绝对值超过 USD 0.01、重复入账或无法解释的人工调整：`disable_recharge -> freeze_balance_adjustments -> disable_affected_channel -> preserve_evidence -> reconcile_ledger`。
- 所有上游不可用或核心服务不健康：`disable_registration -> disable_affected_models -> publish_status_notice -> preserve_evidence`。

### High / Warning

- 备份超过 24 小时、磁盘超过 80%、日总成本超过 USD 20、单用户日成本超过 USD 5、管理登录失败 1 小时达到 5 次：High。
- 证书不足 14 天、恢复演练超过 31 天、请求 ID 覆盖不足 100%、上游余额不足 3 天、成功率低于 95%、429 超过 15%、5xx 超过 5%、TTFT P95 超过 5000 ms 或流中断超过 1%：Warning。

告警只建议动作，不执行。相同动作去重，并保留触发规则、对象和非敏感证据。

## 7. 运营节奏

- 每日：系统/证书/磁盘/备份、上游、账号池、成本、账本差异和异常登录。
- 每周：按上游、账号类型、模型、用户复盘收入、完全成本和毛利；调整建议仍通过定价和 ROUTE01 工具验证。
- 每月：在独立临时数据库执行恢复演练，复核上游价格、账号采购、售后率、支付费率和补偿率。
- 事故：先执行最小止损，再保存时间线、请求 ID、影响、恢复和补偿；秘密值不进入时间线。

## 8. 备份与恢复

PostgreSQL 使用自定义格式 `pg_dump -Fc`，每日保留 7 份本地，并用 restic 客户端加密写入 Cloudflare R2 Standard，保留 30 天；Backblaze B2 是 R2 账户/结算不可用时的回退。每份需记录 SHA-256、大小、开始/结束时间和版本；月度使用 `pg_restore` 恢复到独立临时数据库，完成表数、管理员记录、关键配置和只读查询抽查后销毁临时库。

Caddy 会自动申请和续期证书，但仍监控剩余天数和 Caddy 数据卷；备份范围还包括 Compose、Caddyfile、已脱敏配置和版本清单。任何恢复演练不得指向当前生产数据库。

## 9. 接口

```text
ruby ops/evaluate-operations-baseline.rb validate config/operations/OPS01.example.yaml
ruby ops/evaluate-operations-baseline.rb evaluate config/operations/OPS01.example.yaml config/operations/daily-snapshot.example.yaml
ruby ops/evaluate-operations-baseline.rb demo config/operations/OPS01.example.yaml config/operations/daily-snapshot.example.yaml
```

输出固定包含 `report_only: true`、`real_action_executed: false` 和 `external_system_contacted: false`。

## 10. 验收标准

- [x] 完整 OPS01 和虚构日快照通过校验，真实本地快照被 Git 忽略。
- [x] 健康快照不产生 Critical/High 告警。
- [x] 账务差异、凭据暴露、全上游不可用和核心服务异常产生正确 Critical 止损顺序。
- [x] 备份、恢复、证书、磁盘、登录、成本和上游阈值均有测试。
- [x] 输出动作去重且从不执行。
- [x] 专项、全量、YAML、Markdown、秘密值和无网络/进程控制检查通过。

## 11. 依据

- Sub2API v0.1.155 源码：`registration_enabled` 查询失败时安全默认关闭；账号、分组、API Key 使用 active/disabled 状态；支付可见方式默认关闭。
- [PostgreSQL 18：SQL Dump](https://www.postgresql.org/docs/18/backup-dump.html)：`pg_dump` 一致性导出、自定义格式与 `pg_restore`。
- [Caddy Automatic HTTPS](https://caddyserver.com/docs/automatic-https)：自动申请、续期和证书存储。
- [Cloudflare R2 Pricing](https://developers.cloudflare.com/r2/pricing/)：Standard 免费层、存储/操作和出网价格。
- [restic：Preparing a new repository](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html)：密码仓库和 S3 兼容存储。
- [Google SRE Workbook：Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/)：按症状、严重度和时间窗区分即时处理与工单。
- 前序公开商业站样本的状态页、透明用量和单一客服入口做法。
