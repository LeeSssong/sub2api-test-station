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

## 5. 注册与邀请码

注册和邀请码完全由 Sub2API 原生设置控制：

1. 管理员在 Sub2API 管理后台开启注册。
2. 管理员在同一后台开启并配置邀请码注册。
3. Caddy 只将注册和公开设置请求转发给 Sub2API；没有额外的注册代理、人数上限、预算门禁、每日额度或只读模式。

需要停止新注册时，在 Sub2API 管理后台关闭注册即可。邀请码的生成、禁用和核验也只通过 Sub2API 原生能力完成。

## 6. 数据备份与恢复

账户数据仍应以 Sub2API PostgreSQL 和应用数据卷为边界备份。恢复前先校验备份完整性，并恢复到隔离的数据库或实例中核验用户、管理员、分组、账号和 API Key；不要把临时恢复实例接入生产路由。

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
