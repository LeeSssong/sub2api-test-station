# 生图账号固定上游成本

本流程只影响部署后的新请求。历史 `usage_logs.account_cost IS NULL` 流水不回填，继续按旧公式展示。

## 前置条件

1. 候选代码已合并到 `main` 并通过完整验证。
2. 数据库已先应用 `201_usage_log_account_cost.sql`。
3. 部署环境变量指向已审核的 Compose 项目、env 文件和镜像 overlay。

## 检查与应用

默认命令只执行只读检查：

```bash
bash ops/configure-image-account-costs.sh
```

显式写入模式在一个 SERIALIZABLE 事务中创建或复用“生图固定上游成本”渠道、绑定唯一“生图”分组，并配置规则：

```bash
bash ops/configure-image-account-costs.sh --apply
```

规则固定为 `gpt-image-*` / `openai` / `image`，价格为 1K `$0.06`、2K `$0.08`、4K `$0.10`。渠道不得包含客户模型定价，`apply_pricing_to_account_stats=false`。

## 验证

应用后再次运行默认检查，并用只读 SQL 核对渠道、唯一分组绑定、唯一规则、模型定价和三个区间。等待自然产生的图片请求，不主动制造付费请求；核对新流水的 `account_stats_cost`、`account_cost`、账号配额增量、通知输入和管理员用量展示。未出现自然流量的档位保持待验证。

## 回滚

回滚只删除本流程拥有的三个区间、模型定价、规则和“生图”分组绑定；确认渠道无其他绑定后再删除渠道。不得更新或删除 `usage_logs`，不得修改账号倍率或客户价格。代码回滚时保留 `account_cost` 可空列，避免收缩迁移影响现有数据。
