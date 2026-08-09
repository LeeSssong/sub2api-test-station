# 生图账号固定上游成本设计

## 问题

生产“生图”分组的图片流水没有写入 `account_stats_cost`。账号成本因此回退为本站客户标准成本乘通用账号倍率；该倍率来自仅覆盖 Token 计费的上游探测，不能表达按图片尺寸固定收费。现有 92 条图片流水全部命中该回退，但本次不修改历史数据。

现有链路还有第二个约束：报表按 `COALESCE(account_stats_cost, total_cost) * account_rate_multiplier` 聚合，账号配额扣减和通知也独立使用 `total_cost * account_rate_multiplier`。因此即使图片规则写入 `$0.06`，倍率为 `0.5` 的账号仍会被记成 `$0.03`。固定图片成本必须作为已解析的最终账号成本保存并贯穿所有消费者，不能只修一个报表字段。

## 目标

从新请求开始，为“生图”分组所有现有及未来账号按 USD/张记录固定上游成本：

| 图片计费档位 | 固定上游成本 |
|---|---:|
| 1K | $0.06 |
| 2K | $0.08 |
| 4K | $0.10 |

客户标准成本 `total_cost`、客户实际扣费 `actual_cost`、用户图片倍率以及调度行为均保持不变。

## 非目标

- 不回补现有 92 条历史图片流水。
- 不修改各上游的 Token 倍率探测协议。
- 不把图片固定成本写入 `accounts.rate_multiplier`。
- 不修改“生图”分组的 `image_price_1k/image_price_2k/image_price_4k`，因为这些字段属于客户计费。
- 不为单个上游账号建立不同价格。

## 当前链路

1. 图片转发结果确定 `image_count` 和规范化计费档位 1K/2K/4K。
2. 客户计费生成 `total_cost` 和 `actual_cost`。
3. `applyAccountStatsCost` 尝试从渠道账号统计规则生成 `account_stats_cost`。
4. 当前函数只传模型、Token 和请求次数，没有传图片档位；`BillingModeImage` 最终只读取默认 `per_request_price`，忽略已持久化的尺寸 intervals。
5. “生图”分组没有渠道绑定和账号统计规则，步骤 3 返回 `nil`。
6. 管理员账号成本回退为 `total_cost * account_rate_multiplier`，导致图片现金成本错误。
7. 账号配额扣减、配额通知、盈利统计和多处用量聚合分别重复该公式；即使步骤 3 得到固定价格，也仍会被 Token 倍率二次缩放。

## 设计

### 1. 图片档位进入账号统计定价

扩展账号统计成本解析入口，使其接收本次用量的 `billingMode` 和图片尺寸。档位复用现有严格分类器 `ClassifyImageBillingTier`，只在能够确认 1K/2K/4K 时返回档位，不增加另一套分类逻辑，也不对未知值调用带默认档位的归一化。仅当本次用量和规则定价的 `billing_mode` 都为 `image` 时，按 `sizeTier` 查找同名 interval；匹配 interval 且 `per_request_price > 0` 时，成本为：

```text
interval.per_request_price * image_count
```

Token 和普通 `per_request` 规则保持现有行为。图片规则没有匹配档位、档位为空或价格无效时返回 `nil`，继续走现有账号成本回退，不猜测相邻档位。

### 2. 最终账号成本快照

新增可空字段 `usage_logs.account_cost NUMERIC(20,10)`，表示本次请求已经解析完成、可直接用于现金成本统计和账号配额的最终上游成本。新版本为每条新请求写入最终结果（包括正常回退或零成本），不回补历史流水；`NULL` 因而只表示迁移前历史数据或异常缺失。

账号成本解析返回结构化结果，至少包含规则计算值和该值是否已经是最终成本：

- 命中本次固定图片档位规则：`account_stats_cost = interval price * image_count`，最终 `account_cost` 使用同一值，不应用 `account_rate_multiplier`。
- 现有 Token、普通按次、自定义账号统计规则和模型文件定价：保持当前倍率语义，最终 `account_cost = account_stats_cost * account_rate_multiplier`。
- 没有账号统计覆盖：保持现有回退，最终 `account_cost = total_cost * account_rate_multiplier`。

`account_stats_cost` 保留规则计算结果和既有管理端可见性；旧消费者不再把它单独当成最终现金成本。所有账号成本查询统一改为：

```text
COALESCE(
  usage_logs.account_cost,
  COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)
)
```

后半段仅用于迁移前历史流水兼容，因此现有 92 条记录仍按历史口径展示。本次不通过把图片流水倍率改为 `1`、按倍率反向除价或覆盖账号倍率来规避问题；`account_rate_multiplier` 继续真实记录账号 Token 倍率快照。

### 3. 账号配额与通知使用同一最终值

账号统计定价和 `account_cost` 必须在构建统一扣费命令前完成。统一事务扣费、legacy 降级扣费以及账号配额通知均接收同一个已解析账号成本：

- `UsageBillingCommand.AccountQuotaCost = usageLog.account_cost`（仅在原有配额守卫满足时使用）。
- legacy `IncrementQuotaUsed` 使用同一值。
- `notifyAccountQuota` 使用同一值报告本次增量。
- 用户余额、订阅、API Key 配额和客户限流仍使用客户侧 `actual_cost`，不读取 `account_cost`。

由此，同一请求在流水、账号配额、通知、盈利统计和管理员报表中不会出现不同的上游成本。

### 4. 生产分组级规则

为“生图”分组建立专用账号成本渠道。渠道不配置客户模型定价，不开启 `apply_pricing_to_account_stats`，因此不会改变客户价格解析。渠道只包含一条账号统计规则：

- 匹配条件：生产“生图”分组 ID；不枚举账号 ID，未来加入分组的账号自动生效。
- 平台：`openai`。
- 模型：`gpt-image-*`。
- 计费模式：`image`。
- intervals：1K `$0.06`、2K `$0.08`、4K `$0.10`。

新流水同时写入规则计算结果 `account_stats_cost` 和最终快照 `account_cost`。即使账号 Token 倍率不是 `1`，最终快照仍等于对应图片固定价乘图片张数。

### 5. 可审查的生产配置脚本

新增本地/宿主脚本，以事务方式按稳定名称解析“生图”分组并幂等创建或更新专用渠道、分组绑定、账号统计规则、模型定价和三个 intervals。脚本必须：

- 默认只读检查，显式 `--apply` 才写入。
- 要求目标分组唯一存在，否则写入前失败。
- 拒绝覆盖同名但结构不兼容的渠道或规则。
- 写入后在同一事务内重读并验证模型、模式、三档价格和分组绑定。
- 不更新既有 `usage_logs`、账号倍率、分组客户图片价格或历史记录。
- 不使用 GitHub Actions；随已验证的 `main` 通过现有本地/宿主蓝绿发布链执行。

应用配置后由部署重启/新版本加载刷新渠道缓存；脚本的只读复验同时确认数据库持久状态。

## 错误处理

- 图片档位不在 1K/2K/4K 中：不写固定成本，保留现有回退并允许后续监控发现。
- 图片数量小于等于 0：不生成图片固定成本。
- 渠道、规则或 interval 加载失败：不静默使用部分规则，沿用现有错误/回退行为并由测试覆盖。
- 生产配置目标不唯一或现有对象冲突：脚本在事务写入前失败，不做部分修改。
- 新字段迁移失败：应用版本不得晋级；旧版本继续按旧公式运行，不允许在缺列状态启动新查询。

## 测试策略

1. TDD 单元测试分别锁定 1K、2K、4K 一张和多张图片的账号统计成本与最终账号成本。
2. 以账号倍率 `0.5`、`1` 和非整数倍率验证固定图片成本完全不变；Token 和普通按次规则仍应用倍率。
3. 单元测试锁定未知档位返回固定规则未命中并使用旧回退、图片数量无效不生成固定成本。
4. 统一扣费、legacy 扣费和通知测试确认三者使用相同 `account_cost`；客户余额、订阅、API Key 配额和限流仍使用 `actual_cost`。
5. 仓储与聚合测试确认新流水优先读取 `account_cost`，历史空值仍使用旧公式；盈利统计、趋势、看板和管理员账号统计口径一致。
6. 仓储测试确认账号统计 intervals 创建、加载和排序保持三档价格。
7. 脚本合同测试覆盖默认只读、显式写入、幂等重跑、目标缺失、冲突拒绝和禁止历史更新。
8. 运行后端聚焦测试、迁移测试、`go test ./...`、`go vet ./...`、脚本语法和 diff 检查。
9. 合并到 `main` 后执行迁移/发布预检、蓝绿部署和数据库只读配置复验。
10. 不主动生成付费测试图；等待第一条自然 1K/2K/4K 请求后核对新流水的 `account_stats_cost`、`account_cost` 和账号配额增量。未覆盖档位保持待验证，不伪造线上完成。

## 验收标准

- 新 1K 图片流水的 `account_stats_cost = 0.06 * image_count`。
- 新 2K 图片流水的 `account_stats_cost = 0.08 * image_count`。
- 新 4K 图片流水的 `account_stats_cost = 0.10 * image_count`。
- 上述流水的 `account_cost` 与对应 `account_stats_cost` 相等，不受 `account_rate_multiplier` 影响。
- 账号配额扣减、配额通知、盈利统计和管理员报表均使用同一个 `account_cost`。
- 规则覆盖“生图”分组所有现有及未来账号。
- 客户 `total_cost`、`actual_cost` 和分组客户图片价格不因本次改动变化。
- 现有 92 条历史流水保持原值，`account_cost` 为空并由查询兼容旧公式。
- 代码、配置脚本、并版回归、部署健康检查和至少一个自然流量档位验证通过；其余尚无自然样本的档位明确保持待验证。

## 风险与回滚

- 风险：把账号成本规则误配置成客户渠道定价。防护是专用渠道不含 `channel_model_pricing` 且 `apply_pricing_to_account_stats=false`。
- 风险：部分成本消费者继续重复计算旧公式。防护是集中最终账号成本解析，并为统一扣费、legacy、通知和所有聚合表达式建立回归测试与全仓搜索检查。
- 风险：未来出现新图片档位。未知档位 fail open 到旧回退，并在后续单独定价。
- 回滚：先由配置脚本删除专用分组绑定/规则，再回滚应用版本；数据库新增列保留以兼容已写入的新流水。历史流水无需恢复，因为本次不回补旧数据，新流水保留其发生时的最终成本快照。
