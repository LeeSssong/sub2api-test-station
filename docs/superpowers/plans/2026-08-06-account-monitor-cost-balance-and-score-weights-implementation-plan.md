# 账号监控成本、余额与评分权重实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 恢复分组评分权重入口，以采购成本和预计可用额度统一混合分组成本评分，并为 OpenAI API Key 账号提供余额展示、6 小时 New API 倍率刷新和单卡强制刷新。

**Architecture:** 在账号表增加预计可用额度字段，监控服务按 OpenAI 账号类型解析统一成本倍率；现有倍率服务改用显式刷新选项，并新增独立版本化余额快照。前端复用评分权重弹窗，新建一个页面级轻量成本弹窗，账号卡片只负责显示和发起编辑。所有实现保持账号监控只读评分边界，不接入生产调度。

**Tech Stack:** Go 1.24、Ent/PostgreSQL、Gin、Vue 3、TypeScript、Tailwind CSS、Vitest、Playwright、Docker Compose、现有 Sub2API 零停机原子发布脚本与 `sub2api-prod` SSH 别名。

## Global Constraints

- 每个代理必须先读取 `docs/project/account-monitor-cost-balance-context.md` 并报告规定的 `CONTEXT_ACK`。
- 生产应用来源提交 `05985e62` 的 `upstream/sub2api` 子树与 `origin/main@69caeaf81` 相同；实现只在专用 worktree 进行。
- 只实现本功能，不增加灰度流量、长期观察、营收/账务 UI、调度自动改权重或无关重构。
- OpenAI API Key 只使用倍率；OpenAI 非 API Key 只使用采购成本除以预计额度；余额不参与评分。
- 缺预计额度时成本项为 0，账号仍按质量维度参与排名。
- New API 自动付费测量 TTL 固定为 6 小时；单卡刷新忽略 TTL 强制测量。
- `manual_override` 跳过自动付费测量；单卡强测只更新快照，不覆盖生效的手工倍率。
- 辅助刷新失败不得让健康探测失败，并必须保留最后一次有效值。
- 每个任务一个实施提交、一次独立审查；代理不得 push、deploy 或修改项目总账。
- 协调者只使用 `ssh -o BatchMode=yes sub2api-prod` 快速核验和发布，不输出凭据。
- 只授权零停机原子切换；任何 `downtime_required=true` 或停止服务要求都在生产变更前硬停止。
- Task 1 会新增迁移文件，因此候选迁移哈希预计与生产不同；现有发布器若按当前策略返回 `migration_set_changed`，本轮预期结果是代码实施并推送后停在生产变更前，不为绕过门禁扩展发布系统。

---

### Task 1: 预计额度字段与统一成本评分

**Files:**
- Create: `upstream/sub2api/backend/migrations/197_account_estimated_usable_quota.sql`
- Create: `upstream/sub2api/backend/migrations/account_estimated_usable_quota_migration_test.go`
- Modify: `upstream/sub2api/backend/ent/schema/account.go`
- Regenerate: `upstream/sub2api/backend/ent/account.go`
- Regenerate: `upstream/sub2api/backend/ent/account/account.go`
- Regenerate: `upstream/sub2api/backend/ent/account/where.go`
- Regenerate: `upstream/sub2api/backend/ent/account_create.go`
- Regenerate: `upstream/sub2api/backend/ent/account_update.go`
- Regenerate: `upstream/sub2api/backend/ent/migrate/schema.go`
- Regenerate: `upstream/sub2api/backend/ent/mutation.go`
- Regenerate: `upstream/sub2api/backend/ent/runtime/runtime.go`
- Modify: `upstream/sub2api/backend/internal/service/account.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_service.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_account.go`
- Modify: `upstream/sub2api/backend/internal/service/account_procurement_cost_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_handler_procurement_cost_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/admin_service_stub_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Produces: `Account.EstimatedUsableQuotaUSD *float64` and JSON field `estimated_usable_quota_usd`.
- Produces: atomic procurement input carrying `procurement_cost_cny` plus `estimated_usable_quota_usd`.
- Produces: `func accountMonitorEffectiveCost(account Account, windowStart, windowEnd time.Time, baseCost float64) accountMonitorWindowCostResult`, classifying OpenAI accounts by `AccountTypeAPIKey` and returning a common multiplier.

**Review gate:** After the commit, the coordinator dispatches a fresh reviewer. The reviewer must read the context contract, inspect only this commit against its parent, verify the focused command output, and explicitly approve schema compatibility, request validation and mixed-group ranking before Task 2 starts.

- [ ] **Step 1: Write migration and handler tests that fail**

Add tests proving the migration is nullable with a positive-value constraint, a new procurement save requires both fields, clear sets both to null, and invalid quota values return 400.

```go
func TestUpdateAccountProcurementCostRequiresEstimatedQuota(t *testing.T) {
    body := `{"procurement_cost_cny":4,"estimated_usable_quota_usd":null}`
    response := runAccountUpdate(t, body)
    require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestUpdateAccountProcurementCostPersistsEstimatedQuota(t *testing.T) {
    body := `{"procurement_cost_cny":4,"estimated_usable_quota_usd":120}`
    account := runSuccessfulAccountUpdate(t, body)
    require.Equal(t, 120.0, *account.EstimatedUsableQuotaUSD)
}
```

- [ ] **Step 2: Run the new migration and handler tests and verify failure**

Run:

```bash
cd upstream/sub2api/backend
go test ./migrations ./internal/handler/admin -run 'EstimatedUsableQuota|ProcurementCost' -count=1
```

Expected: FAIL because the field, constraint and request binding do not exist.

- [ ] **Step 3: Add the schema, request contract and generated Ent code**

Use a nullable numeric column and reject non-finite or non-positive quota. A request that touches procurement configuration must send a complete pair, while clear sends both null.

```go
field.Float("estimated_usable_quota_usd").
    Optional().
    Nillable().
    SchemaType(map[string]string{dialect.Postgres: "numeric(14,2)"})

type ProcurementCostUpdate struct {
    Value                   *float64
    EstimatedUsableQuotaUSD *float64
}
```

Run `go generate ./ent` from `upstream/sub2api/backend`; do not hand-edit generated files beyond generated output.

- [ ] **Step 4: Write scoring tests that fail**

Cover `4/60`, `4/120`, API Key precedence over stale procurement fields, non-API Key precedence over `rate_multiplier`, mixed-group ranking, and missing quota retaining ranking with zero cost score.

```go
func TestAccountMonitorProcurementMultiplierUsesLifetimeQuota(t *testing.T) {
    got := accountMonitorEffectiveCost(Account{
        Platform: PlatformOpenAI, Type: AccountTypeOAuth,
        ProcurementCostCNY: ptr(4.0), EstimatedUsableQuotaUSD: ptr(60.0),
    }, time.Time{}, time.Time{}, 0)
    require.InDelta(t, 4.0/60.0, *got.EffectiveMultiplier, 1e-9)
}

func TestAccountMonitorMissingQuotaStillRanksOnQuality(t *testing.T) {
    row := projectOpenAIProcurementAccountWithoutQuota(t)
    require.Nil(t, row.EffectiveMultiplier)
    require.Zero(t, row.CostScore)
    require.NotNil(t, row.QualityScore)
    require.NotNil(t, row.GroupRank)
}
```

- [ ] **Step 5: Replace the OpenAI time-amortization path with type-based cost evidence**

Keep the current legacy calculation only for non-OpenAI accounts. For OpenAI, use this decision table:

```go
switch {
case account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey:
    return multiplierCost(account.RateMultiplier)
case account.Platform == PlatformOpenAI:
    return procurementQuotaCost(account.ProcurementCostCNY, account.EstimatedUsableQuotaUSD)
default:
    return legacyWindowCost(account, windowStart, windowEnd, baseCost)
}
```

Bump `AccountMonitorSchemaVersion` from 3 to 4 and project the new field. Do not use balance in any score function.

- [ ] **Step 6: Run focused backend tests and commit**

Run:

```bash
cd upstream/sub2api/backend
go test ./migrations ./internal/handler/admin ./internal/service -run 'EstimatedUsableQuota|ProcurementCost|AccountMonitor.*Cost|Mixed.*Rank' -count=1
```

Expected: PASS.

Commit:

```bash
git add upstream/sub2api/backend
git commit -m "feat: unify account monitor procurement scoring"
```

### Task 2: 余额快照与显式刷新策略

**Files:**
- Create: `upstream/sub2api/backend/internal/service/account_monitor_balance.go`
- Create: `upstream/sub2api/backend/internal/service/account_monitor_balance_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier.go`
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/account_repo_account_monitor_balance_cas_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`

**Interfaces:**
- Consumes: `AccountMonitorSchemaVersion=4` and account projection from Task 1.
- Produces: `AccountMonitorBalance`, `AccountMonitorRefreshOptions`, `ResolveBalance`, and versioned `account_monitor_balance` snapshot storage.
- Changes: `accountMonitorMultiplierResolver.Refresh(context.Context, *Account, AccountMonitorRefreshOptions) error`.

**Review gate:** A fresh reviewer must approve source detection, paid-request cadence, manual-policy precedence, CAS persistence and health/auxiliary failure isolation before Task 3 starts.

- [ ] **Step 1: Write parsing and persistence tests that fail**

Cover Sub2API `balance`/`remaining`, New API `total_available/quota_per_unit`, source selection, last-good-value retention, invalid payload rejection and compare-and-swap persistence.

```go
func TestDecodeNewAPIBalanceUSD(t *testing.T) {
    got, err := decodeNewAPIBalanceUSD([]byte(`{"data":{"total_available":600000}}`), 500000)
    require.NoError(t, err)
    require.Equal(t, 1.2, got)
}

func TestBalanceFailureRetainsLastGoodValue(t *testing.T) {
    snapshot := failedBalanceSnapshot(previousBalanceSnapshot(12.5), "upstream_http_error", now)
    require.Equal(t, 12.5, *snapshot.ValueUSD)
    require.Equal(t, AccountMonitorBalanceStatusFailed, snapshot.Status)
}
```

- [ ] **Step 2: Run the new balance tests and verify failure**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository -run 'AccountMonitorBalance|NewAPIBalance|Sub2APIBalance' -count=1
```

Expected: FAIL because balance types, decoders and persistence do not exist.

- [ ] **Step 3: Implement the versioned balance snapshot**

Use an independent extra key and never replace the last valid value on failure.

```go
const AccountMonitorBalanceExtraKey = "account_monitor_balance"

type AccountMonitorBalance struct {
    ValueUSD      *float64  `json:"value_usd,omitempty"`
    Source        string    `json:"source,omitempty"`
    Status        string    `json:"status"`
    ObservedAt    *time.Time `json:"observed_at,omitempty"`
    LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
    FailureCode   string    `json:"failure_code,omitempty"`
}
```

Sub2API is selected only when the declaration probe is supported; an explicitly unsupported declaration selects New API. Never request both balance sources in one cycle.

- [ ] **Step 4: Write refresh-policy tests that fail**

Test ordinary full runs, single-card runs, the 6-hour boundary, manual override, forced manual measurement without takeover, and health-success isolation from every auxiliary failure.

```go
type AccountMonitorRefreshOptions struct {
    RefreshDeclaration        bool
    RefreshBalance            bool
    MeasureNewAPIMultiplier   bool
    ForceNewAPIMeasurement    bool
}

type accountMonitorMultiplierCall struct {
    accountID int64
    options   AccountMonitorRefreshOptions
}

func TestSingleRunForcesNewAPIMeasurementInsideSixHours(t *testing.T) {
    monitorRepo := &accountMonitorRepoStub{}
    accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
        ID: 23, Status: StatusActive, Schedulable: true,
        Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
    }}}
    multiplier := &accountMonitorMultiplierStub{}
    service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, multiplier)

    _, err := service.RunOne(context.Background(), 1, 23)
    require.NoError(t, err)
    require.Len(t, multiplier.calls, 1)
    require.True(t, multiplier.calls[0].options.ForceNewAPIMeasurement)
}

func TestFullRunSkipsManualOverridePaidMeasurement(t *testing.T) {
    account, upstream := newManualOverrideMeasurementFixture(t)
    service := newAccountMultiplierServiceForTest(t, upstream)
    require.NoError(t, service.Refresh(context.Background(), account, AccountMonitorRefreshOptions{
        RefreshDeclaration: true, RefreshBalance: true, MeasureNewAPIMultiplier: true,
    }))
    require.Zero(t, upstream.CompletionCalls())
}

func TestAuxiliaryFailureDoesNotFailHealthProbe(t *testing.T) {
    monitorRepo := &accountMonitorRepoStub{}
    accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
        ID: 23, Status: StatusActive, Schedulable: true,
        Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
    }}}
    multiplier := &accountMonitorMultiplierStub{err: errors.New("balance unavailable")}
    service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, multiplier)

    result, err := service.RunOne(context.Background(), 1, 23)
    require.NoError(t, err)
    require.Equal(t, int64(23), result.AccountID)
    require.Len(t, monitorRepo.results, 1)
}
```

Implement `newManualOverrideMeasurementFixture` and `newAccountMultiplierServiceForTest` in `account_multiplier_test.go` with the existing fake HTTP upstream pattern; the fixture must expose a counted `/v1/chat/completions` handler and an account whose policy extra is `manual_override`.

- [ ] **Step 5: Implement explicit refresh options and 6-hour TTL**

Set `AccountMultiplierMeasurementTTL = 6 * time.Hour`. Both full and single runs request declaration and balance refresh; full runs use due-only measurement, while single runs set `ForceNewAPIMeasurement=true`. Continue all auxiliary branches after one fails and log sanitized errors.

```go
func (s *AccountMonitorService) refreshAuxiliary(
    ctx context.Context,
    account *Account,
    options AccountMonitorRefreshOptions,
) {
    if err := s.multiplier.Refresh(ctx, account, options); err != nil {
        slog.Warn("account_monitor: auxiliary refresh failed", "account_id", account.ID, "error", err)
    }
}
```

Manual override skips only automatic paid measurement. A forced single-card measurement may store evidence, but `Resolve` must continue returning the manual value until policy is restored.

- [ ] **Step 6: Run focused service/repository tests and commit**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository -run 'AccountMonitorBalance|AccountMultiplier|RunAll|RunOne|Auxiliary' -count=1
```

Expected: PASS.

Commit:

```bash
git add upstream/sub2api/backend
git commit -m "feat: refresh account balance and multiplier evidence"
```

### Task 3: 恢复分组评分权重入口

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes: existing `AccountMonitorGroupScoreDialog` and `adminAPI.accountMonitor.updateGroupScoreWeights/resetGroupScoreWeights`.
- Produces: the eighth group-summary item and page-level save/reset state.

**Review gate:** A fresh reviewer must verify that only the selected group exposes the entry, existing seven fields remain intact, save/reset reload the active range, and no scheduling behavior changes before Task 4 starts.

- [ ] **Step 1: Write view tests that fail**

Test that a selected group shows the eighth field and edit button, all-site hides it, save calls PUT, reset calls DELETE, and successful operations reload the active range.

```ts
it('restores group score weight editing and reloads the active range', async () => {
  const wrapper = mountView()
  await flushPromises()
  await wrapper.get('[data-test="group-tab-3"]').trigger('click')
  expect(wrapper.findAll('[data-test="group-summary-field"]')).toHaveLength(8)
  await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
  expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(true)
})
```

- [ ] **Step 2: Run the view test and verify failure**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected: FAIL because the eighth item and dialog wiring are absent.

- [ ] **Step 3: Restore the dialog and group-summary wiring**

Import and mount `AccountMonitorGroupScoreDialog`; add `showScoreDialog`, `savingScoreWeights`, and `scoreWeightsError`. The eighth field uses a compact edit icon and current four weights. Change the desktop summary grid to eight stable tracks without changing the existing seven values.

```ts
async function saveScoreWeights(weights: EditableWeights) {
  if (!activeGroup.value) return
  await adminAPI.accountMonitor.updateGroupScoreWeights(activeGroup.value.id, weights)
  await load(activeRange.value)
  showScoreDialog.value = false
}
```

- [ ] **Step 4: Run the focused view/dialog/API tests and commit**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/api/__tests__/admin.accountMonitor.spec.ts
```

Expected: PASS.

Commit:

```bash
git add upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
git commit -m "fix: restore account monitor score weight controls"
```

### Task 4: 轻量成本弹窗与余额卡片

**Files:**
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCostDialog.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCostDialog.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes: `estimated_usable_quota_usd`, `balance`, schema version 4 and existing account update API.
- Produces: page-level `AccountMonitorCostDialog` events `saveProcurement`, `saveMultiplier`, `restoreAuto`, `clear`, and card event `editCost`.

**Review gate:** A fresh reviewer must verify account-type dispatch, no manual mode selector, draft-only default 60, conditional balance rendering, error retention and mobile/desktop layout before Task 5 starts.

- [ ] **Step 1: Write modal and card tests that fail**

Cover account-type dispatch, no manual mode selector, 60 USD draft-only default, derived multiplier, paired save, manual multiplier save, restore auto, error retention, stale balance and non-API-Key no-placeholder behavior.

```ts
it('shows procurement fields for OpenAI non-api-key accounts', async () => {
  const wrapper = mountDialog(openAIAccount({ account_type: 'oauth', estimated_usable_quota_usd: null }))
  expect(wrapper.get('[data-test="procurement-cost-input"]').exists()).toBe(true)
  expect(wrapper.get<HTMLInputElement>('[data-test="estimated-quota-input"]').element.value).toBe('60')
  expect(wrapper.find('[data-test="cost-mode-select"]').exists()).toBe(false)
})

it('retains a last good failed balance as delayed', () => {
  const wrapper = mountCard(apiKeyAccount({ balance: { value_usd: 12.5, status: 'failed', source: 'newapi' } }))
  expect(wrapper.get('[data-test="balance-metric"]').text()).toContain('$12.50')
  expect(wrapper.get('[data-test="balance-metric"]').text()).toContain('数据延迟')
})
```

- [ ] **Step 2: Run component/view tests and verify failure**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCostDialog.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected: FAIL because the modal, balance types and events do not exist.

- [ ] **Step 3: Implement typed API contracts and the lightweight modal**

Extend monitor types with `estimated_usable_quota_usd` and `balance`. Change the procurement helper to send both fields atomically.

```ts
export async function updateProcurementCost(
  id: number,
  cost: number | null,
  estimatedQuotaUSD: number | null,
): Promise<AccountWithProcurementCost> {
  return update(id, {
    procurement_cost_cny: cost,
    estimated_usable_quota_usd: estimatedQuotaUSD,
  })
}
```

Use `BaseDialog` with account-type-derived content. Validate multiplier `>= 0`, procurement cost `>= 0`, and quota `> 0`. The initial 60 is local draft state only.

- [ ] **Step 4: Remove card inline editors and wire page-level saves**

The card keeps the cost display and one edit icon, emits the selected account, and conditionally renders the balance metric only for OpenAI API Key accounts. The view owns one dialog and performs API calls:

```ts
await adminAPI.accounts.update(accountID, {
  rate_multiplier: value,
  rate_multiplier_policy: 'manual_override',
})

await adminAPI.accounts.update(accountID, {
  rate_multiplier_policy: 'upstream_managed',
})
await adminAPI.accountMonitor.runOne(accountID)
```

Only close after save plus current-range reload succeeds. Preserve dialog input and show the extracted API error on failure.

- [ ] **Step 5: Run frontend tests, typecheck and build, then commit**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCostDialog.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts src/api/__tests__/admin.accountMonitor.spec.ts
pnpm run typecheck
pnpm run build
```

Expected: PASS.

Commit:

```bash
git add upstream/sub2api/frontend
git commit -m "feat: add account cost dialog and balance display"
```

### Task 5: 整体回归、视觉验收与生产门禁

**Files:**
- Create: `.superpowers/sdd/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan/final-verification.md`
- Modify: `.superpowers/sdd/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan/progress.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: Tasks 1-4 reviewed commits and the complete task contract.
- Produces: clean reviewed branch, focused verification evidence, pushed branch, and either verified zero-downtime production deployment or an explicit pre-mutation downtime stop.

- [ ] **Step 1: Run final focused backend and frontend verification**

Run only the feature and required build gates:

```bash
cd upstream/sub2api/backend
go test ./migrations ./internal/service ./internal/handler/admin ./internal/repository -count=1
go vet ./internal/service ./internal/handler/admin ./internal/repository

cd ../frontend
pnpm exec vitest run src/api/__tests__/admin.accountMonitor.spec.ts src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/components/admin/account-monitor/AccountMonitorCostDialog.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm run lint:check
pnpm run typecheck
pnpm run build
```

Expected: every command exits 0.

- [ ] **Step 2: Perform desktop and mobile visual checks**

Use the Playwright workflow with deterministic mocked monitor data at `1440x1000` and `390x844`. Capture a selected mixed group, API Key cost dialog and procurement cost dialog. Verify eight summary cells, two desktop cards/one mobile column, no overlap, no horizontal scroll, correct conditional balance tile, and browser console 0 errors.

- [ ] **Step 3: Run independent whole-branch review and close findings**

The reviewer reads the task contract first, compares `origin/main...HEAD`, and checks scope, correctness, error isolation, paid-request cadence, migration compatibility, security and test coverage. Any finding is fixed in a separate focused commit and re-reviewed before proceeding.

- [ ] **Step 4: Commit verification records and push the feature branch**

Record exact commands, commits, screenshots and reviewer result. Keep the project ledger “进行中（本地实现完成，待生产）”. Then:

```bash
git push -u origin codex/account-monitor-cost-balance
```

- [ ] **Step 5: Apply the zero-downtime production gate over SSH**

Read production state directly through the saved alias, without rediscovering credentials:

```bash
ssh -o BatchMode=yes sub2api-prod 'sudo cat /var/lib/sub2api/release-state'
```

Compare the candidate migration hash with production before building or mutating production. If they differ and the existing controller classifies the release as `downtime_required=true`, record the exact reason and stop immediately. Do not stop services, apply SQL manually, edit release-state, add a migration bypass or create a new deployment mechanism.

If the controller proves the candidate is zero-downtime compatible, write canonical `0600` test evidence and use the existing atomic inactive-slot release in 0%/100% mode. This is a direct switch, not traffic gray deployment.

- [ ] **Step 6: Verify only this feature online and update the ledger**

After a successful zero-downtime release, verify public health plus authenticated `/admin/accounts/monitor` behavior for score-weight save/reset, one Sub2API API Key balance, one New API API Key balance/forced refresh, and 60/120 USD procurement examples. Do not expose credentials or raw account payloads.

Mark the ledger “已完成” only when push, deployment and online verification all succeed. If Step 5 stops on downtime, keep “进行中（代码已推送，部署需要停机授权）” and return the gate evidence to the user.
