# 日常运营与事故止损手册

**当前状态：** 离线准备完成，生产未部署
**执行原则：** 先止损、再调查；OPS01 只给建议，不调用管理 API 或控制容器

## 1. 每日检查

真实资产到位后，将示例复制为 Git 忽略的本地文件并填非敏感数据：

```bash
cp config/operations/daily-snapshot.example.yaml config/operations/daily-snapshot.local.yaml
ruby ops/evaluate-operations-baseline.rb evaluate \
  config/operations/OPS01.example.yaml \
  config/operations/daily-snapshot.local.yaml
```

数据来源按以下顺序核对：

1. Sub2API/Caddy/PostgreSQL/Redis 健康、证书、磁盘、备份和管理登录。
2. 上游余额、成功率、429、5xx、TTFT、流中断和可调度状态。
3. 账号池可用数、错误数和最近到期时间。
4. 收款、站内加款、退款、人工调整、用户消耗和上游成本。
5. 全站和单用户日成本、请求 ID 覆盖率。

不把 API Key、Cookie、OAuth Token、Authorization 头、原始提示词或完整用户响应写入快照。

## 2. 告警处理

### Critical

先在管理后台人工复核对象，再按报告顺序执行：

- 凭据暴露：停用受影响账号/渠道，吊销旧凭据，轮换，保留非敏感时间线。
- 账务异常：保持 PAY01 关闭或关闭可见支付方式，冻结人工余额调整，停受影响渠道，保存账本和请求 ID，再对账。
- 核心服务/全上游不可用：保持注册关闭，停受影响模型组，发布状态说明，保存日志。

Sub2API v0.1.155 已验证存在 `registration_enabled` 设置以及账号、分组、API Key 的 active/disabled 状态。支付止损使用 Provider 禁用和支付宝/微信可见方式关闭；不要仅隐藏前端按钮而保留可用回调。

### High

当日处理：备份过期、磁盘压力、管理登录失败、全站/单用户成本越线、账号池无可用账号。成本异常先暂停异常 Key 或模型，不先提高限额。

### Warning

进入当日或周复盘：证书、本机账户备份新鲜度、请求 ID、上游余额/质量、账号到期和毛利。证书由 Caddy 自动续期，但剩余天数异常仍要检查 DNS、80/443、Caddy 数据卷和 ACME 日志。

## 3. 账本与利润

真实账本只使用 Git 忽略的 `*.local.jsonl`：

```bash
ruby ops/manual-ledger.rb verify --ledger config/ledger/production.local.jsonl
ruby ops/manual-ledger.rb summary --ledger config/ledger/production.local.jsonl --format json
```

每周按上游、账号类型、模型和用户汇总：收入、上游成本、账号摊销、支付费、补偿/退款和固定成本。售价仍通过已有试算器生成，不在后台凭感觉改倍率：

```bash
ruby ops/calculate-pricing.rb \
  --upstream config/upstreams/UP01.local.yaml \
  --scenario config/pricing/MVP.local.yaml \
  --format markdown
```

完全成本毛利低于 20% 时，先核对成本和异常补偿，再决定提价、换渠道或收窄模型。

## 4. 路由与供应复盘

每个主模型使用最近同口径窗口运行 ROUTE01。未达到样本量、条款未知、余额不足、429 过高或半开状态的渠道不进入付费用户流量。

```bash
ruby ops/evaluate-routing-baseline.rb score config/routing/ROUTE01.local.yaml MODEL
```

账号池在到期 72 小时前进入复核；首样 ACC01 未完成真实验收前，不根据卖家理论额度计算利润，不批量续购。

## 5. 轻量本机账户备份

账户数据是当前最重要的备份对象。每日在生产服务器执行一次：

```bash
cd /opt/sub2api/production
D04_BACKUP_ROOT=/opt/sub2api/production/backups/d04-account-data \
SUB2API_COMPOSE_FILE=/opt/sub2api/production/compose.yaml \
D04_IMAGE=sub2api-internal-test:d04-lightweight-launch-20260722-v2 \
D04_VOLUME=sub2api_d04_internal_test_data \
./ops/backup-d04-account-data.sh
```

每个备份集包含完整 Sub2API PostgreSQL 自定义格式逻辑备份、D04 SQLite 在线一致性快照、`SHA256SUMS` 和无秘密 metadata。目录权限为 `0700`，文件为 `0600`；脚本只有在两份归档非空且 SHA-256 校验通过后才原子发布。保留最近 3 个已验证集合，失败运行不替换上一份成功备份。

当前轻量方案不建设异地备份，也不设置按天数计算的留存门禁。D04 开放只要求最新完整备份不超过 24 小时；建议在实际开放前额外执行一次。

## 6. 手动恢复

恢复不是周期开放门禁。只有账户数据损坏、服务器迁移或人工核验需要时才执行：

1. 在备份集目录运行 `sha256sum -c SHA256SUMS`。
2. 将 `sub2api.dump` 恢复到不同数据库名或空的新 PostgreSQL 实例，禁止指向当前生产数据库。
3. 停止 D04 写入后，把 `d04.sqlite` 安装到新卷，再以 `read_only/registration=false` 启动。
4. 核对用户数量、管理员数量、D04 roster/grant 数量和注册 `403 D04_REGISTRATION_CLOSED`。
5. 确认无误后再决定是否切换；不把临时恢复实例接入生产路由。

## 7. 事故时间线

每次事故记录：

```text
incident_id:
detected_at:
detected_by:
affected_users_models_channels:
first_stop_loss_at:
request_ids:
financial_exposure:
root_cause:
recovery_at:
customer_notice_or_compensation:
follow_up_owner_and_due_date:
```

时间线只保存非敏感引用。若计费或余额无法解释，只有完成三方对账、修复验证和第二人复核后才恢复充值/受影响渠道。

## 8. D04 内测自动化

D04 默认使用独立 `compose.d04-read-only.yaml`，保持 `D04_MODE=read_only` 和 `D04_REGISTRATION_OPEN=false`。它复用 Sub2API 原生邮箱密码注册、登录和用户中心；最多 15 个首发用户，注册后的自动登录算当天首次登录，此后每个上海自然日首次成功登录发放 `$20`。首发阶段不开放邀请、推荐、affiliate 奖励或手动签到。

生产已经完成一个隔离用户、一次 `$20`、同日幂等和三方余额对账。不要重复该 grant 来证明既有写路径。后续开放使用 `D04-LIGHTWEIGHT-LAUNCH-v2`，先创建本机账户备份并生成 Git 忽略的无秘密快照：

```bash
./ops/backup-d04-account-data.sh
ruby ops/evaluate-d04-lightweight-launch-readiness.rb evaluate \
  config/operations/D04-lightweight-launch-readiness-v2.yaml \
  config/operations/d04-lightweight-launch-snapshot.local.yaml
```

`approvals.launch_approved` 是唯一开放批准。它为 `true` 且同一快照返回 fresh `go` 后，操作员可以应用 `compose.d04-launch.yaml`，不再要求第二份预算或时间窗口批准。准备策略固定为：15 人、每日 `$20`、总成本风险上限 `$100`、当前活动上游成本因子 `1000bps`。

开放硬门禁：

- 当前活动上游余额至少 `$5`，余额证据不得超过 20 分钟；不再计算消耗覆盖天数。
- 最近 15 分钟自然生产流量至少 20 个样本，成功率不低于 95%，错误率不高于 5%。
- TTFT P95 不高于 5 秒，总耗时 P95 不高于 45 秒。
- 本机账户备份不超过 24 小时、SHA-256 已验证，且同时包含 Sub2API PostgreSQL 与 D04 SQLite。
- 六个服务健康、无 OOM、磁盘使用不超过 80%，D04 余额漂移为 0。
- 主值守和飞书支持群已明确，注册回滚已验证。

任何未知或越线都保持 `NO-GO`，不得通过调低阈值获得通过。开启注册不是 relay-ops 或飞书命令的职责；relay-ops 始终保持 `read_only + dry_run`。

### 8.1 开放前渲染

```bash
docker compose -f infra/compose.d04-read-only.yaml \
  -f infra/compose.d04-launch.yaml config --quiet
```

渲染成功不等于允许应用。只有包含唯一 `launch_approved: true` 的当前快照返回 `go` 才允许应用。

### 8.2 停止与回滚

出现预算、余额、质量、健康、备份、漂移或对账失败时，立刻只重建 D04 到只读基线：

```bash
cd /opt/sub2api/production
docker compose -f compose.d04-read-only.yaml config --quiet
docker compose -f compose.d04-read-only.yaml \
  up -d --no-deps --force-recreate internal-test-service
```

回滚后必须确认 D04 healthy、重启 0、`D04_MODE=read_only`、`D04_REGISTRATION_OPEN=false`，且同源注册返回 `403 D04_REGISTRATION_CLOSED`。保留用户、grant、余额历史、SQLite 和审计记录；不要直接修改 Sub2API PostgreSQL 或 D04 SQLite。

完整门禁见 `docs/superpowers/checklists/2026-07-22-d04-controlled-launch-readiness.md`。
