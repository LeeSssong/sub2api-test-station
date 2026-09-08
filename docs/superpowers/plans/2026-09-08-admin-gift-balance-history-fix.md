# 管理员赠送/扣除额度与用户记录弹窗修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将管理员额度调整收敛为赠送额度专用操作，并让用户充值/并发记录弹窗按原型显示双额度和竖向摘要布局，同时清理旧管理员余额调整记录。

**Architecture:** 管理员写入复用已有 `QuotaAccountingService` 的 `admin_gift` grant 与 `admin_gift_deduction` adjustment，不再调用 legacy paid-balance adjustment。历史接口扩展为统一记录 DTO，从 quota accounting、兑换码、支付订单和并发/订阅来源构造一套分页结果；前端只依赖该 DTO 的付费/赠送字段。新增一次性迁移只删除旧 `redeem_codes.type='admin_balance'` 记录，不修改钱包或用户余额。

**Tech Stack:** Go、Gin、Ent、PostgreSQL SQL migrations、Vue 3、TypeScript、Vitest、pnpm。

---

## 文件映射

- Modify: `upstream/sub2api/backend/internal/service/admin_service.go`：增加统一历史记录类型和服务契约。
- Modify: `upstream/sub2api/backend/internal/service/admin_user.go`：管理员赠送/扣除接入 quota accounting，统一历史来源编排。
- Modify: `upstream/sub2api/backend/internal/service/admin_balance_history_test.go`：覆盖统一记录合并、双额度字段和旧记录过滤。
- Modify: `upstream/sub2api/backend/internal/service/admin_service_update_balance_test.go`：覆盖管理员 gift-only 写入、扣除和失败语义。
- Modify: `upstream/sub2api/backend/internal/handler/admin/user_handler.go`：序列化统一历史 DTO。
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go` 或其管理员用户 DTO 文件：增加双额度历史字段。
- Modify: `upstream/sub2api/backend/internal/handler/admin/admin_service_stub_test.go`：同步测试 stub 接口。
- Modify: `upstream/sub2api/backend/internal/repository/redeem_code_repo.go`：保留兑换码来源 ID，并过滤旧管理员余额记录。
- Create: `upstream/sub2api/backend/migrations/236_remove_legacy_admin_balance_history.sql`：删除旧管理员余额调整兑换码记录。
- Create: `upstream/sub2api/backend/migrations/legacy_admin_balance_history_migration_test.go`：验证迁移只删除目标类型。
- Modify: `upstream/sub2api/frontend/src/api/admin/users.ts`：扩展 `BalanceHistoryItem` 双额度和来源字段。
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue`：按原型重排摘要和记录内容。
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`：把新余额预览明确拆成付费/赠送结果，并保持扣除 gift-only。
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.spec.ts`：覆盖新字段、标题、竖向布局和双额度展示。
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/overview.ts`、`upstream/sub2api/frontend/src/i18n/locales/en/admin/overview.ts`：补充记录来源、付费额度、赠送额度文案。

## Task 1: 固定后端统一历史 DTO 和旧记录过滤行为

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/admin_service.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_balance_history_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/user_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/admin_service_stub_test.go`

- [ ] **Step 1: 写失败测试，锁定 DTO 合同**

在 `admin_balance_history_test.go` 增加表驱动用例，要求每条额度记录都能表达付费和赠送增量：

```go
require.Equal(t, "10.00000000", item.PaidQuotaDeltaUSD)
require.Equal(t, "5.00000000", item.GiftQuotaDeltaUSD)
require.Equal(t, "payment_order", item.Source)
```

同时增加 `admin_balance` 旧记录不会进入统一历史结果的断言。

- [ ] **Step 2: 运行失败测试**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestMergeBalanceHistory|TestUnifiedBalanceHistory' -count=1
```

预期：FAIL，原因是统一 DTO 字段和来源合并逻辑尚不存在。

- [ ] **Step 3: 实现最小 DTO 和 handler 映射**

将服务接口从 `[]RedeemCode` 扩展为内部统一的 `[]AdminBalanceHistoryItem`，至少包含：

```go
type AdminBalanceHistoryItem struct {
    ID                int64
    Code              string
    Type              string
    Source            string
    Value             float64
    PaidQuotaDeltaUSD decimal.Decimal
    GiftQuotaDeltaUSD decimal.Decimal
    Status            string
    UsedBy            *int64
    UsedAt            *time.Time
    CreatedAt         time.Time
    Notes             string
    GroupID           *int64
    ValidityDays      int
    User              *User
    Group             *Group
}
```

DTO 序列化使用固定 8 位小数字符串，避免前端用二进制浮点重算。旧 `admin_balance` 项在统一编排处直接跳过。

- [ ] **Step 4: 运行后端聚焦测试**

```bash
go test ./internal/service ./internal/handler/admin -run 'TestMergeBalanceHistory|TestUnifiedBalanceHistory|TestAdmin.*BalanceHistory' -count=1
```

预期：PASS。

- [ ] **Step 5: 提交**

```bash
git add upstream/sub2api/backend/internal/service/admin_service.go upstream/sub2api/backend/internal/service/admin_balance_history_test.go upstream/sub2api/backend/internal/handler/admin/user_handler.go upstream/sub2api/backend/internal/handler/dto/types.go upstream/sub2api/backend/internal/handler/admin/admin_service_stub_test.go
git commit -m "feat: add unified balance history quota fields"
```

## Task 2: 将管理员 add/subtract 改为 gift-only 原子账务

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/admin_user.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_service_update_balance_test.go`
- Modify: `upstream/sub2api/backend/internal/service/quota_accounting.go` only if an existing exported result/helper is insufficient

- [ ] **Step 1: 写失败测试**

增加服务测试，构造初始 paid=20、gift=5：

```go
result := updateAdminBalance("add", 10)
require.Equal(t, "20.00000000", result.PaidQuotaBalanceUSD)
require.Equal(t, "15.00000000", result.GiftQuotaBalanceUSD)
```

增加 subtract 6 成功、subtract 6 在 gift=5 时失败且 paid 仍为 20 的测试；增加旧 `admin_balance` redeem code 不再创建的断言。

- [ ] **Step 2: 运行失败测试**

```bash
go test ./internal/service -run 'TestAdmin.*(Gift|Deduct|Balance)' -count=1
```

预期：FAIL，因为 `UpdateUserBalance` 当前仍调用 legacy `AdjustBalance`。

- [ ] **Step 3: 实现 gift-only 写入**

在 `UpdateUserBalance` 中：

1. 只接受本入口的 `add`、`subtract`；`set` 保留兼容错误或仅保留非本入口内部调用，不能由管理员赠送界面触发。
2. 校验正数、有限数值、用户和 operator ID。
3. `add` 调用 `CreateAdminGiftGrant`，幂等键使用稳定的管理员操作 key。
4. `subtract` 调用 `CreateAdminGiftDeduction`，使用已有 gift FIFO 和不足错误。
5. 统一失效用户余额、鉴权相关缓存并返回最新用户。
6. 删除生成 `redeem_codes(type='admin_balance')` 的代码和 affiliate admin recharge 逻辑。

管理员历史展示从 quota grant/adjustment 查询，而不是从新操作生成兑换码。

- [ ] **Step 4: 运行测试并检查付费额度保护**

```bash
go test ./internal/service -run 'TestAdmin.*(Gift|Deduct|Balance)' -count=1
go test ./internal/service ./internal/repository -run 'QuotaAccounting|AdminGift|GiftDeduction' -count=1
```

预期：PASS；失败扣除不会产生部分更新或付费额度变化。

- [ ] **Step 5: 提交**

```bash
git add upstream/sub2api/backend/internal/service/admin_user.go upstream/sub2api/backend/internal/service/admin_service_update_balance_test.go upstream/sub2api/backend/internal/service/quota_accounting.go
git commit -m "fix: make admin balance adjustments gift-only"
```

## Task 3: 实现 quota accounting、兑换码和支付订单的统一历史投影

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/admin_user.go`
- Modify: `upstream/sub2api/backend/internal/repository/redeem_code_repo.go`
- Add focused repository/service tests in existing history test files.

- [ ] **Step 1: 写失败测试**

构造四类记录并断言排序、分页和双额度：

```go
require.Equal(t, "10.00000000", history[0].PaidQuotaDeltaUSD)
require.Equal(t, "5.00000000", history[0].GiftQuotaDeltaUSD)
require.Equal(t, "redeem_code", history[0].Source)
require.Equal(t, "admin_gift_deduction", history[1].Source)
require.Equal(t, "-10.00000000", history[1].GiftQuotaDeltaUSD)
```

- [ ] **Step 2: 运行失败测试**

```bash
go test ./internal/service ./internal/repository -run 'Test.*BalanceHistory.*(Quota|Redeem|Payment|Gift)' -count=1
```

预期：FAIL，当前历史查询只合并兑换码和 affiliate 记录。

- [ ] **Step 3: 实现来源读取和合并**

按 `created_at/granted_at/adjusted_at` 降序合并：

- `user_quota_grants.grant_type='payment_order'`：从 grant 取 paid/gift，并通过关联订单保留支付来源。
- `grant_type='redeem_code'`：从 grant 取 paid/gift，并通过关联兑换码保留 code。
- `grant_type='admin_gift'`：取 gift 正值，source 为 `admin_gift`，展示“余额赠送（管理员）”。
- `user_quota_adjustments.adjustment_type='admin_gift_deduction'`：取 `applied_gift_quota_usd` 的负值，source 为 `admin_gift_deduction`，展示“余额扣除（管理员）”。
- 并发、订阅、affiliate 记录保留现有行为；不适用的双额度字段为 `0.00000000`。

总充值统计改为仅统计普通兑换码/支付订单产生的付费额度，不能把管理员赠送计入充值金额。

- [ ] **Step 4: 运行历史接口和分页测试**

```bash
go test ./internal/service ./internal/repository ./internal/handler/admin -run 'Test.*BalanceHistory' -count=1
```

预期：PASS。

- [ ] **Step 5: 提交**

```bash
git add upstream/sub2api/backend/internal/service/admin_user.go upstream/sub2api/backend/internal/repository/redeem_code_repo.go upstream/sub2api/backend/internal/service/admin_balance_history_test.go
git commit -m "feat: project quota sources into balance history"
```

## Task 4: 增加并验证历史旧管理员记录清理迁移

**Files:**
- Create: `upstream/sub2api/backend/migrations/236_remove_legacy_admin_balance_history.sql`
- Create: `upstream/sub2api/backend/migrations/legacy_admin_balance_history_migration_test.go`
- Modify: migration checksum/runner tests only if the repository requires explicit registration.

- [ ] **Step 1: 写迁移测试**

测试 SQL 必须包含精确目标：

```sql
DELETE FROM redeem_codes
WHERE type = 'admin_balance';
```

并断言迁移不会删除 `admin_concurrency`、`balance`、`concurrency`、`subscription` 或 quota accounting 表记录。

- [ ] **Step 2: 运行失败测试**

```bash
go test ./migrations -run 'Test.*LegacyAdminBalance' -count=1
```

预期：FAIL，因为迁移文件不存在。

- [ ] **Step 3: 实现可审计迁移**

迁移只执行一次明确的 `DELETE ... WHERE type='admin_balance'`，不更新钱包、不更新 `users.balance`，不删除普通兑换码、支付订单、管理员并发记录和 quota accounting 事实。迁移文件增加注释说明用户已确认旧管理员余额记录不保留。

- [ ] **Step 4: 运行迁移合同检查**

```bash
go test ./migrations -run 'Test.*LegacyAdminBalance|TestLatestMigrationBaseline|TestMigrationChecksum' -count=1
git diff --check
```

预期：PASS。

- [ ] **Step 5: 提交**

```bash
git add upstream/sub2api/backend/migrations/236_remove_legacy_admin_balance_history.sql upstream/sub2api/backend/migrations/legacy_admin_balance_history_migration_test.go
git commit -m "feat: remove legacy admin balance history"
```

## Task 5: 前端记录字段与原型排版

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/users.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/overview.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/overview.ts`

- [ ] **Step 1: 写失败组件测试**

增加带双额度 mock 的记录，断言弹窗包含：

```ts
expect(wrapper.text()).toContain('付费 10')
expect(wrapper.text()).toContain('赠送 5')
expect(wrapper.text()).toContain('余额赠送（管理员）')
expect(wrapper.find('.quota-summary-secondary-row').exists()).toBe(true)
```

断言管理员扣除表单的最大值来自 `gift_quota_balance_usd`，并不显示旧“充值/退款”入口。

- [ ] **Step 2: 运行失败测试**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/user/UserBalanceHistoryModal.spec.ts
```

预期：FAIL，当前 DTO 没有双额度字段，摘要也没有稳定的竖向第二行标记。

- [ ] **Step 3: 实现前端最小改动**

1. 扩展 `BalanceHistoryItem`：`paid_quota_delta_usd`、`gift_quota_delta_usd`、`source`。
2. 摘要改为：可用额度一整行；第二行左右/分列显示充值额度和赠送额度，使用稳定 class 便于测试和视觉核验。
3. 记录项中额度类记录显示 `付费 X`、`赠送 Y`，0 值仍显示 `0`，管理员扣除显示负赠送额度。
4. 管理员标题映射为“余额赠送（管理员）”“余额扣除（管理员）”，不再依赖旧 `admin_balance` 的正负余额语义。
5. 保持普通兑换码和支付订单来源标识，管理员记录显示“管理员调整”。
6. 预览区域同时显示变更后的付费额度和赠送额度，管理员操作不会预览付费额度变化。

- [ ] **Step 4: 运行前端聚焦测试和静态检查**

```bash
pnpm vitest run src/components/admin/user/UserBalanceHistoryModal.spec.ts
pnpm typecheck
pnpm build
```

预期：全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add upstream/sub2api/frontend/src/api/admin/users.ts upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin/overview.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/overview.ts
git commit -m "fix: align balance history with paid and gift quotas"
```

## Task 6: 集成前直接相关验证和交接

**Files:**
- No new product files; inspect all changed files and test artifacts.

- [ ] **Step 1: 运行后端直接相关测试**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository ./internal/handler/admin ./migrations -run 'Test(Admin|Unified|Merge|Quota|Gift|BalanceHistory|LegacyAdminBalance)' -count=1
go build ./cmd/server
```

预期：受影响包测试和 server build PASS。

- [ ] **Step 2: 运行前端直接相关测试**

```bash
cd ../frontend
pnpm vitest run src/components/admin/user/UserBalanceHistoryModal.spec.ts
pnpm typecheck
pnpm build
```

预期：聚焦组件测试、类型检查和生产构建 PASS。

- [ ] **Step 3: 检查范围和敏感文件**

```bash
cd ../..
git diff --check
git status --short
git diff --stat main...HEAD
git diff --name-status main...HEAD
rg -n "\.env|password|token|secret|BEGIN .*PRIVATE KEY|Authorization: Bearer" --glob '!**/node_modules/**' --glob '!**/dist/**' $(git diff --name-only main...HEAD)
```

预期：无格式错误、无凭据或运行数据文件、改动只在规格范围内。

- [ ] **Step 4: 写交接摘要**

记录基线 `481fc8e2`、最终提交 SHA、变更文件、测试结果、迁移 236、无配置变化、无生产数据写入、无部署授权、回滚代码方案和迁移删除不可逆风险。

- [ ] **Step 5: 提交最终交接**

```bash
git add docs/handoffs/2026-09-08-admin-gift-balance-history-fix-handoff.md
git commit -m "docs: hand off admin gift balance history fix"
```

## 计划自审

- 规格覆盖：管理员 gift-only、双额度历史、原型布局、旧管理员记录清理、测试和发布门禁均有对应任务。
- 占位扫描：无 `TBD`、`TODO` 或“后续补充”步骤；每一步包含文件、命令和预期。
- 类型一致性：统一历史 DTO 在 Task 1 定义，Task 3 后端来源映射、Task 5 前端字段和 Task 6 验证均使用同一 `paid_quota_delta_usd`、`gift_quota_delta_usd`、`source` 契约。
- 风险边界：迁移只删除旧 `admin_balance` 兑换码，不能通过代码回滚恢复；部署前必须保留删除计数和恢复证据，且未取得主站授权不得发布。
