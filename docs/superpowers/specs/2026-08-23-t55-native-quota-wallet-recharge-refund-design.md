# T55 原生额度钱包与手动充值退款账本设计

**状态：** DESIGNING，待用户书面审阅批准

**任务类型：** 原生 Sub2API 管理后台的跨前后端、计费余额与数据迁移任务

**基线：** `main@1af258ba776a9ff6e72248f36cb685d1e5a4a4a3`

**设计来源：** 用户提供的“开发包-星桥AL Link充值界面.zip”，其中的 `product-rules.md`、`data-and-api.md`、`acceptance-checklist.md` 和充值/退款原型；当前 Sub2API 原生用户、余额、支付、用量与管理员接口实现。

## 1. Goal

在不建立第二套计费事实源、不破坏 Sub 原生 `users.balance` 兼容语义的前提下，为管理员提供可审计的手动充值/退款录入，并将用户额度拆分为：

- 人民币充值账户余额 `cash_balance_cny`；
- 付费站内额度 `paid_quota_balance_usd`；
- 赠送站内额度 `gift_quota_balance_usd`；
- 对外展示的站内额度总额 `paid + gift`。

服务消耗必须按“付费额度优先、赠送额度其次”扣减；现有 `users.balance` 保留为兼容投影，始终等于新的付费额度与赠送额度之和。

## 2. Context and current behavior

### 2.1 当前原生能力

- `users.balance` 是当前 API 使用余额，数据库精度为 `decimal(20,8)`；`frozen_balance` 用于异步任务持有余额。
- 原生用量服务在写入 `usage_logs` 后，通过用户仓储扣除 `actual_cost`；现有扣费路径只知道一个总余额，不知道额度来源。
- 管理员接口 `POST /api/v1/admin/users/:id/balance` 支持 `set/add/subtract`，并通过 `admin_balance` 历史记录保留调整痕迹。
- 管理员余额历史 `GET /api/v1/admin/users/:id/balance-history` 当前合并兑换码、返利和管理员余额调整，前端由 `UserBalanceHistoryModal` 展示。
- `payment_orders` 已有真实支付订单和渠道退款流程，不能把手动录入伪装成支付订单，否则会污染真实支付统计和退款语义。

### 2.2 当前缺口

- 没有人民币现金余额字段；当前 `users.balance` 不能证明其对应可退款现金。
- 没有付费额度/赠送额度拆分；无法表达退款时清空赠送额度、付费优先消耗或来源审计。
- 现有余额调整 API 的 `set` 语义允许覆盖余额，不符合阶段一只允许“充值/退款”的产品约束。
- 现有余额历史记录只承载单一数值，无法保存现金、付费额度、赠送额度三类变化。

## 3. Product decisions confirmed by user

以下决策已经确认，实施不得重新解释：

1. 采用方案 B：新增原生钱包拆分和额度流水，保留 `users.balance` 兼容投影。
2. 消耗顺序固定为：先扣付费额度，再扣赠送额度。
3. 历史用户迁移规则固定为：
   - `cash_balance_cny = 0`；
   - `paid_quota_balance_usd = users.balance` 的迁移前值；
   - `gift_quota_balance_usd = 0`；
   - 不增加用户可见的“历史未拆分”标记。
4. 历史额度继续可用，但不会因为迁移自动产生可退款现金。
5. 新手动充值按 `¥1 = $1` 增加现金余额和付费额度；赠送额度单独增加。
6. 手动退款金额不能超过当前现金余额；退款同时扣减等额付费额度，并清空退款时剩余的全部赠送额度；赠送额度不折现、不增加退款金额。
7. 阶段一不做自动支付、自动退款渠道、冲正、订单化手动充值、赠送额度有效期和完整收入确认。

## 4. Goals and non-goals

### Goals

- 管理员从用户列表或用户详情打开充值/退款弹窗；弹窗不重新选择用户。
- 充值模式：现金余额和付费额度按金额 1:1 增加，赠送额度可填写。
- 退款模式：支持全退，退款金额超限时前端提示且后端拒绝，赠送额度自动清零。
- 用户列表和详情展示现金余额、付费额度、赠送额度、额度总额。
- 保存可分页、可筛选的手动充值/退款流水，包含操作人、备注、前后快照和三类变化。
- 充值/退款/消费更新具备事务原子性、并发保护、幂等和缓存失效。
- 旧的 `users.balance`、API 鉴权和原生用量统计继续工作。

### Non-goals

- 不替换 `payment_orders`，不新增支付渠道，不改变真实支付回调或渠道退款。
- 不改变用户实际扣费价格、`usage_logs.actual_cost`、Sub 原生经营页美元口径或利润公式。
- 不引入新的外部控制面、第二账务源、汇率服务或人工收入确认表。
- 不为历史用户推导现金余额，不回填历史充值订单，不制造历史赠送额度。
- 不开放通用“手动调整”类型，不允许负数输入绕过退款规则。
- 不实现赠送额度有效期、分组独立钱包、用户侧退款申请或批量充值。

## 5. Proposed architecture

### 5.1 兼容投影原则

新增钱包表是额度来源和人民币现金的事实源；`users.balance` 是兼容投影，不再由业务代码独立计算或覆盖。

```text
wallet.paid_quota_balance_usd
  + wallet.gift_quota_balance_usd
  = users.balance
```

每次充值、退款和实际用量扣费必须在同一事务内更新钱包与 `users.balance`。禁止先更新钱包、再异步“尽力”同步 `users.balance`。

### 5.2 钱包模型

新增 `user_wallets`（expand-only）：

| 字段 | 类型 | 语义 |
|---|---|---|
| `user_id` | bigint PK/FK | 用户唯一钱包 |
| `cash_balance_cny` | decimal(20,8) | 可用于手动退款校验的人民币余额，历史迁移为 0 |
| `paid_quota_balance_usd` | decimal(20,8) | 当前未消耗的付费额度 |
| `gift_quota_balance_usd` | decimal(20,8) | 当前未消耗的赠送额度 |
| `created_at` / `updated_at` | timestamptz | 审计时间 |
| `version` | bigint | 钱包更新版本，用于乐观读回和诊断 |

不增加 `legacy_unsegmented` 或其他用户可见状态字段；历史迁移事实由迁移脚本和流水/审计证据保留，不在用户页面显示标记。

### 5.3 额度流水模型

新增 `user_quota_ledger_entries`：

| 字段 | 类型 | 语义 |
|---|---|---|
| `id` | bigint/uuid | 流水主键 |
| `user_id` | bigint | 用户 |
| `record_type` | enum/text | `recharge`、`refund`、`usage_consumption`、`legacy_balance_adjustment`、`payment_fulfillment`、`redeem_credit`、`affiliate_credit`、`migration_projection`（仅内部审计，不展示为充值） |
| `cash_delta_cny` | decimal(20,8) | 现金变化 |
| `paid_quota_delta_usd` | decimal(20,8) | 付费额度变化 |
| `gift_quota_delta_usd` | decimal(20,8) | 赠送额度变化 |
| `cash_before_cny` / `cash_after_cny` | decimal(20,8) | 现金快照 |
| `paid_before_usd` / `paid_after_usd` | decimal(20,8) | 付费额度快照 |
| `gift_before_usd` / `gift_after_usd` | decimal(20,8) | 赠送额度快照 |
| `reference_type` / `reference_id` | text/null | 关联 usage log、人工请求或迁移批次 |
| `idempotency_key` | text/null | 写操作幂等键，按用户和操作域唯一 |
| `note` | text | 管理员备注或系统说明 |
| `operator_id` | bigint/null | 管理员；系统消费为 null 或系统 actor |
| `status` | text | 阶段一只允许 `confirmed`；失败不落流水 |
| `created_at` | timestamptz | 发生时间 |

迁移初始化不为每个历史用户伪造充值流水；通过迁移批次审计记录初始化范围、数量和源列哈希即可。这样不会把历史余额错误标记成一次可退款充值。若实现需要逐用户可追溯性，使用 `migration_projection` 内部记录且不在普通额度流水 UI 中显示为充值，也不增加用户可见的历史标记。

### 5.4 消耗算法

给定实际扣费 `amount > 0`：

```text
paid_deduct = min(paid_before, amount)
gift_deduct = amount - paid_deduct
assert gift_deduct <= gift_before
paid_after = paid_before - paid_deduct
gift_after = gift_before - gift_deduct
cash_after = cash_before
users.balance_after = paid_after + gift_after
```

如果总额度不足，沿用当前 Sub 原生余额不足语义：不得产生负余额，不写入成功的 `usage_consumption` 流水，不提交扣费事务。若现有请求流程允许先写 usage 再失败，需要在本任务内确保事务回滚或使用现有失败路径保持一致，不能留下“流水已扣但请求失败”的新不一致。

### 5.5 充值算法

请求只接受非负金额，阶段一实际确认按钮应拒绝零金额空操作：

```text
cash_delta = +recharge_amount_cny
paid_delta = +recharge_amount_cny
gift_delta = +gift_quota_amount_usd
```

结果必须更新钱包、`users.balance` 和一条 `recharge` 流水。充值不触发真实支付订单、返利订单或渠道统计，也不触发 affiliate rebate；手动赠送只记录在额度流水中，不能被统计为真实支付充值。

### 5.6 退款算法

```text
assert 0 < refund_amount_cny <= cash_before_cny
cash_delta = -refund_amount_cny
paid_delta = -refund_amount_cny
gift_delta = -gift_before
cash_after = cash_before - refund_amount_cny
paid_after = paid_before - refund_amount_cny
gift_after = 0
```

退款需要额外校验 `refund_amount_cny <= paid_before`。如果现金余额大于付费额度，不能产生负付费额度；服务端返回明确业务错误，前端不以本地预览替代后端结果。赠送额度被清零但不折算现金，也不改变 `refund_amount_cny`。

## 6. API contract

### 6.1 查询额度摘要

```http
GET /api/v1/admin/users/:id/quota-summary
```

响应：

```json
{
  "user_id": 37,
  "cash_balance_cny": 100.00,
  "paid_quota_balance_usd": 100.00,
  "gift_quota_balance_usd": 20.00,
  "total_quota_balance_usd": 120.00,
  "wallet_version": 4,
  "updated_at": "2026-08-23T12:00:00Z"
}
```

### 6.2 创建手动充值/退款

```http
POST /api/v1/admin/users/:id/quota-ledger
Idempotency-Key: admin-quota-<uuid>
```

充值请求：

```json
{
  "record_type": "recharge",
  "amount_cny": 50,
  "gift_quota_usd": 10,
  "note": "活动赠送"
}
```

退款请求：

```json
{
  "record_type": "refund",
  "amount_cny": 40,
  "note": "服务问题退款"
}
```

响应返回最新用户兼容投影、钱包摘要和已确认流水 ID。相同 `Idempotency-Key` 且请求体相同必须返回第一次结果；相同 key 请求体不同必须返回幂等冲突，不得执行第二次操作。

### 6.3 查询额度流水

```http
GET /api/v1/admin/users/:id/quota-ledger?page=1&page_size=20&type=recharge
```

只返回管理员可见的脱敏字段；不得返回密码、API Key、支付渠道敏感配置或完整请求体。现有 `/balance-history` 保持兼容，不在本任务中删除或改变其既有兑换码/返利/并发语义；新的额度流水以独立 Tab 或同一弹窗中的独立区块展示。

## 7. Frontend design

### 7.1 用户列表

在现有 `UsersView` 中：

- 现金余额使用 `¥`；
- 站内总额度使用 `$`；
- 付费/赠送拆分放在详情或 tooltip 中，避免桌面表格过宽；
- 充值按钮仍从用户行打开，用户由行上下文确定；
- 操作成功后以服务端返回的新摘要更新当前行，不以本地计算覆盖服务端值。

### 7.2 充值/退款弹窗

复用现有 `UserBalanceModal` 的 BaseDialog、加载、错误和提交状态，但改为：

- 顶部用户信息和操作前摘要；
- 记录类型只显示“充值”和“退款”；
- 充值：金额、只读自动计算付费额度、可编辑赠送额度、备注；
- 退款：金额、只读扣减付费额度、“全退”按钮、赠送额度禁用并说明自动清零、备注；
- 顶部预览只显示操作后的单组余额，不展示两套完整前后余额卡片；
- 退款超限既要在输入时提示，也要处理后端并发冲突后的业务错误；
- 提交按钮在金额为 0、正在提交、用户不存在或摘要未加载时禁用。

### 7.3 额度流水

在 `UserBalanceHistoryModal` 中增加“额度流水”视图或独立 Tab：

- 充值：现金 `+¥`、付费额度 `+$`、赠送额度 `+$`；
- 退款：现金 `-¥`、付费额度 `-$`、赠送额度清零；
- 消耗：展示付费/赠送实际扣减来源；
- 展示备注、操作人、时间、流水号和前后余额快照；
- 保留原有兑换码、返利、管理员调账和并发/订阅历史，不混淆记录类型。

## 8. Transaction, concurrency, and cache rules

所有写路径使用数据库事务并锁定 `user_wallets` 行；历史没有钱包行时由受控初始化函数在同一事务中创建，不允许两个并发请求各自插入不同初始余额。兑换码、返利转入、真实支付入账和原生用量扣费等现有余额写路径在 T55 中统一改为调用同一钱包协调器；本任务不改变它们各自的业务来源、支付状态或统计语义，只增加对应的内部流水类型和 `users.balance` 投影同步。任何未接入协调器的余额写入口都必须在发布前禁用或改为显式失败，不得带着旁路写入上线。

事务顺序：

1. 解析并校验请求和幂等键。
2. 获取幂等记录/锁，确认没有请求体冲突。
3. `SELECT ... FOR UPDATE` 读取钱包和用户兼容投影。
4. 重新计算并校验余额变化。
5. 更新钱包。
6. 更新 `users.balance` 为付费+赠送。
7. 写入额度流水。
8. 写入现有审计日志。
9. 保存幂等结果并提交。
10. 提交后失效 auth/billing balance cache；缓存失效失败不得回滚已提交账务，但必须记录可诊断日志。

禁止“先 GET 摘要、再 POST 旧余额覆盖写回”。禁止使用浮点前端结果作为服务端事实；金额在服务端按 decimal/定点规则计算，响应序列化保持现有 JSON 数字兼容。

## 9. Migration and compatibility

### 9.1 Expand-only migration

迁移新增钱包/流水/幂等所需表、索引和约束，不删除或重命名 `users.balance`、`redeem_codes`、`payment_orders` 或 `usage_logs` 字段。

初始化 SQL 逻辑：

```sql
INSERT INTO user_wallets (
  user_id,
  cash_balance_cny,
  paid_quota_balance_usd,
  gift_quota_balance_usd,
  version,
  created_at,
  updated_at
)
SELECT id, 0, balance, 0, 1, NOW(), NOW()
FROM users
WHERE deleted_at IS NULL
ON CONFLICT (user_id) DO NOTHING;
```

迁移必须在初始化前锁定/串行化受影响的用户余额读取，或使用可证明不会覆盖并发扣费的迁移窗口；发布预检若判定 `downtime_required=true`，必须停在用户授权门禁。迁移不得生成伪造充值流水，不得把历史余额计入 `cash_balance_cny`。

### 9.2 兼容旧路径

- `users.balance` 继续由 API 鉴权、用量扣费和既有用户 DTO 使用，但所有写入统一经过钱包协调器。
- 旧管理员 `/balance` 接口保留用于兼容已有前端/脚本；新充值 UI 不调用它。其 `add/subtract` 写操作统一映射为 `legacy_balance_adjustment` 并经过钱包协调器：变化只作用于 `paid_quota_balance_usd`，`cash_balance_cny` 不随之变化，`users.balance` 仍由 `paid + gift` 回写；`set` 解释为设置新的总额度目标，服务端计算 `paid_delta = target_total - (paid_before + gift_before)`，仅当 `target_total >= gift_before` 时允许执行，否则返回业务错误；每次操作必须带管理员审计记录。该接口不再允许直接更新 `users.balance`，并在响应中标记为兼容接口。
- 真实支付订单仍由 `payment_orders` 管理；支付完成后的余额入账接入钱包协调器并记录内部 `payment_fulfillment`（可作为 `record_type` 扩展），但不改变支付订单状态机、渠道退款或经营统计。手动充值仍不创建 `payment_orders`。
- 现有兑换码/返利充值接入钱包协调器并保留其原有来源和历史展示；兑换码默认增加付费额度，返利默认增加赠送额度，具体来源字段和统计映射沿用现有语义。这样所有余额增加路径都满足 `users.balance = paid + gift` 恒等式。
- `frozen_balance` 在 T55 保持现有“总额度冻结”语义，不拆分为付费冻结和赠送冻结；冻结/解冻操作只读取并更新兼容总额投影，实际可用额度校验仍由钱包协调器按付费优先、赠送其次执行。新增冻结拆分属于后续独立任务。

## 10. Failure and security semantics

- 非管理员不可访问新 API；越权返回现有管理员鉴权错误。
- 用户不存在、软删除用户、钱包初始化失败、幂等冲突、金额非法、退款超现金、退款超付费额度分别返回稳定业务错误码。
- 任何数据库错误都回滚钱包、`users.balance`、流水和幂等记录。
- 缓存失效失败不得向用户伪造“未入账”；接口在提交成功后返回最新数据库读值，并记录异步失效失败。
- 不记录完整 API Key、密码、支付渠道密钥、Authorization、完整请求体或敏感个人信息。
- 备注需要长度限制和输出转义；前端列表/详情防止 HTML 注入。
- 幂等键必须绑定管理员、用户、操作域和请求体哈希；不可跨用户重放。

## 11. Acceptance matrix

| 场景 | 预期 |
|---|---|
| 历史用户 balance=100 初始化 | cash=0，paid=100，gift=0，users.balance=100，无用户可见历史标记 |
| 充值 50、赠送 10 | cash=50，paid=150，gift=10，total=160，写一条 recharge 流水 |
| 付费 20、赠送 10 消耗 25 | paid 减 20，gift 减 5，total 减 25，写一条 usage_consumption 流水 |
| 退款 40，cash=100，paid=100，gift=20 | cash=60，paid=60，gift=0，total=60 |
| 全退 | 退款金额自动为当前现金，结果三类余额均不小于 0 |
| 退款超过 cash | 前端提示，后端拒绝，余额和流水不变 |
| 退款超过 paid | 后端拒绝，不能产生负付费额度，余额和流水不变 |
| 充值赠送额度为 0 | 允许正常充值；gift 不变 |
| 充值/退款金额为 0 | 确认提交禁用或后端拒绝，不产生空流水 |
| 相同幂等键重复请求 | 返回第一次结果，不重复加款/扣款 |
| 相同幂等键不同请求体 | 返回幂等冲突，不改变余额 |
| 两个并发退款 | 至多一个成功到达可用现金边界，另一个按最新锁定余额失败 |
| 事务中途失败 | wallet、users.balance、ledger、idempotency 全部回滚 |
| 旧余额鉴权/用量扣费 | 继续读取与校验兼容投影，不出现总额与拆分和不一致 |
| 非管理员访问 | 新摘要、写入、流水接口均拒绝 |
| 移动端 390px | 弹窗可滚动、金额单位可读、无页面级横向溢出 |

## 12. Test strategy

### Backend

- 迁移/schema 测试：字段、唯一约束、历史初始化、重复迁移幂等。
- 钱包服务单测：充值、退款、付费优先消耗、赠送额度清零、零/负数/超限。
- 仓储集成测试：`FOR UPDATE` 并发退款、事务回滚、`users.balance` 投影守恒。
- API handler 测试：管理员鉴权、请求校验、幂等重复/冲突、错误码和响应字段。
- 原生用量回归：实际扣费同时更新 wallet 与 `users.balance`，usage log 数学值不变。
- 缓存失效测试：成功写入后调用现有 auth/billing invalidation；失效失败不吞掉账务结果。

### Frontend

- `UserBalanceModal` 充值/退款模式、实时预览、全退、赠送禁用、错误回显。
- 用户列表现金/总额度/拆分展示和成功后读回。
- `UserBalanceHistoryModal` 额度流水 Tab、分页、空态、错误态和既有历史类型兼容。
- 390px 视口无横向溢出，深浅色主题可读，键盘/屏幕阅读器可操作。

### Required validation commands

实际命令以实施 worktree 的现有项目脚本为准，至少包括：

```bash
go test ./backend/internal/service/... ./backend/internal/repository/... ./backend/internal/handler/...
go build ./backend/cmd/server
pnpm --dir frontend test --run
pnpm --dir frontend typecheck
pnpm --dir frontend build
git diff --check
```

如果后端目录的 `go.mod` 或前端脚本路径与当前树不同，实施计划必须先映射实际命令，不得执行不存在的占位命令。

## 13. Release, rollback, and done-when

- T55 作为一个独立任务包，从当时最新干净 `main` 创建独立 worktree；不得修改 T54-R1 worktree。
- 规格书获批准后才可编写实施计划和实现代码。
- 候选完成标准为：功能实现 + 直接相关测试通过 + 必要类型检查/构建 + diff 自查；不要求额外形式审查，真实失败/范围冲突/高风险问题除外。
- 合并前必须确认迁移集合、目标 `main` SHA 和部署车道；任何目标漂移都要求候选刷新。
- 发布预检返回 `downtime_required=false` 时按项目全局规则继续蓝绿发布；返回 `true` 时在任何停服、迁移、重启或切换前暂停并请求用户授权。
- 部署失败或线上验收失败时保留候选、失败证据和恢复依据，在同一 worktree 修复；不得覆盖或删除失败候选。
- 回滚优先使用上一已验证镜像/活动槽，禁止通过删除钱包流水或反向写账抹平失败；任何人工补偿另立流水并保留审计。

**Done when：** 新充值/退款 UI、钱包摘要、付费优先消耗、赠送额度退款清零、历史余额迁移、原子事务、幂等、缓存失效、管理员审计和旧 `users.balance` 兼容路径均有直接相关验证；候选已合入并推送 `main`、部署成功且线上专项验收生效。

## 14. Resolved implementation boundaries

以下边界已在规格阶段明确，实施计划不得重新解释：

1. 兑换码、返利转入、真实支付入账和原生用量扣费全部经过钱包协调器；本任务只保持其来源、状态机和统计语义不变，并补齐内部流水与兼容投影同步。
2. `frozen_balance` 保持总额度冻结语义，不新增冻结拆分字段；付费优先/赠送其次只适用于实际消费扣减。
3. 旧 `/balance` 的 `set/add/subtract` 通过 `legacy_balance_adjustment` 受审映射到付费额度，现金余额保持不变，禁止任何直接库写入；新 UI 只调用新额度流水 API。
