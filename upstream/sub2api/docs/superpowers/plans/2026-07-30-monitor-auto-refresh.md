# 服务监控自动刷新 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让管理员统一配置服务监控页的自动刷新频率，并移除所有监控页的手动刷新入口。

**Architecture:** 使用现有全局 `settings` 键值存储和 `/admin/settings` 管理接口保存固定的刷新秒数。`/monitor-v2` 快照响应携带该设置，前端据此建立或停止定时器；刷新调度封装在监控视图内，并在组件卸载、隐藏页面和降级时彻底清理。

**Tech Stack:** Go 1.26、Gin、现有 SettingService、Vue 3 Composition API、TypeScript、Pinia、Vitest、Vue Test Utils。

## Global Constraints

- 管理员可选值严格为 `0 | 30 | 60 | 300 | 600` 秒，默认值为 `60` 秒。
- `0` 表示关闭周期刷新；非零值由页面统一执行，用户没有覆盖入口。
- 监控页不得渲染手动刷新按钮，管理员访问该页面也一样。
- 监控快照契约从版本 `2` 升级为 `3`，`refresh_interval_seconds` 是必需字段。
- 时间范围切换必须立即请求；它是筛选，不是手动刷新。
- 页面不可见时暂停定时器；重新可见时立即请求当前时间范围的数据。
- 不引入新依赖，不修改已有未提交的工作区文件。

---

## File Structure

- `backend/internal/service/domain_constants.go`：声明监控页刷新配置键、允许值与默认值。
- `backend/internal/service/settings_view.go`、`setting_parse.go`、`setting_update.go`：将配置加载、归一化和持久化到全局设置。
- `backend/internal/handler/dto/settings.go`、`backend/internal/handler/admin/setting_handler_update.go`：通过管理员设置 API 暴露并校验该字段。
- `backend/internal/service/monitor_v2.go`、`backend/internal/handler/monitor_v2_handler.go`：把已归一化的刷新秒数附加到版本 3 快照。
- `backend/internal/server/api_contract_test.go`、`backend/internal/service/setting_service_update_test.go`、`backend/internal/handler/admin/setting_handler_partial_payload_test.go`、`backend/internal/handler/monitor_v2_handler_test.go`：验证服务端默认、拒绝非法值、管理员写入和响应契约。
- `frontend/src/api/admin/settings.ts`、`frontend/src/types/index.ts`：声明管理员设置字段。
- `frontend/src/views/admin/SettingsView.vue`、`frontend/src/views/admin/__tests__/SettingsView.spec.ts`：在管理员设置界面显示固定下拉项并提交字段。
- `frontend/src/i18n/locales/{zh,en}/admin/settings.ts`：管理员设置的双语文案。
- `frontend/src/features/monitor-v2/{types.ts,api.ts,MonitorV2View.vue}`：验证版本 3 契约、移除按钮并实现自动刷新生命周期。
- `frontend/src/features/monitor-v2/__tests__/{api.spec.ts,MonitorV2View.spec.ts,MonitorV2RouteView.spec.ts}`：验证契约、定时行为、可见性和 UI。

## Task 1: Persist and validate the administrator refresh policy

**Files:**

- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/service/settings_view.go`
- Modify: `backend/internal/service/setting_parse.go`
- Modify: `backend/internal/service/setting_update.go`
- Modify: `backend/internal/handler/dto/settings.go`
- Modify: `backend/internal/handler/admin/setting_handler_update.go`
- Modify: `backend/internal/handler/admin/setting_handler_audit.go`
- Test: `backend/internal/service/setting_service_update_test.go`
- Test: `backend/internal/handler/admin/setting_handler_partial_payload_test.go`
- Test: `backend/internal/server/api_contract_test.go`

**Interfaces:**

- Produces `service.MonitorPageRefreshIntervalSeconds` in `service.SystemSettings`.
- Produces admin JSON field `monitor_page_refresh_interval_seconds`.
- Produces `service.NormalizeMonitorPageRefreshIntervalSeconds(raw string) int`, returning `60` for absent or malformed persisted values.

- [ ] **Step 1: Write the failing service tests**

Add focused cases to `setting_service_update_test.go` that assert `UpdateSettings` persists each permitted value and normalizes legacy data:

```go
func TestSettingServicePersistsMonitorPageRefreshIntervals(t *testing.T) {
    for _, want := range []int{0, 30, 60, 300, 600} {
        t.Run(strconv.Itoa(want), func(t *testing.T) {
            service, repo := newTestSettingService(t)
            err := service.UpdateSettings(context.Background(), &SystemSettings{
                MonitorPageRefreshIntervalSeconds: want,
            })
            require.NoError(t, err)
            require.Equal(t, strconv.Itoa(want), repo.Value(SettingKeyMonitorPageRefreshIntervalSeconds))
        })
    }
}

func TestMonitorPageRefreshIntervalFallsBackToSixty(t *testing.T) {
    require.Equal(t, 60, NormalizeMonitorPageRefreshIntervalSeconds(""))
    require.Equal(t, 60, NormalizeMonitorPageRefreshIntervalSeconds("15"))
    require.Equal(t, 60, NormalizeMonitorPageRefreshIntervalSeconds("invalid"))
}
```

- [ ] **Step 2: Run service tests to verify they fail**

Run:

```bash
cd backend && go test ./internal/service -run 'TestSettingServicePersistsMonitorPageRefreshIntervals|TestMonitorPageRefreshIntervalFallsBackToSixty' -count=1
```

Expected: FAIL because the setting key, `SystemSettings` field, and normalization helper do not exist.

- [ ] **Step 3: Write the failing admin-handler and contract tests**

In the partial update handler test, submit a partial administrator request and assert an unsupported value is rejected while the field is omitted safely:

```go
body := strings.NewReader(`{"monitor_page_refresh_interval_seconds":15}`)
request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", body)
response := httptest.NewRecorder()
router.ServeHTTP(response, request)

require.Equal(t, http.StatusBadRequest, response.Code)
```

In `api_contract_test.go`, add `monitor_page_refresh_interval_seconds: 60` to the golden `/admin/settings` response/request schema and assert it is an integer.

- [ ] **Step 4: Run handler and contract tests to verify they fail**

Run:

```bash
cd backend && go test ./internal/handler/admin ./internal/server -run 'Test.*MonitorPageRefresh|TestAPIContract' -count=1
```

Expected: FAIL because the API has no refresh-policy field or validation.

- [ ] **Step 5: Implement the minimum backend settings path**

1. Add `SettingKeyMonitorPageRefreshIntervalSeconds = "monitor_page_refresh_interval_seconds"`, the default `60`, and a set-based validator accepting only `0, 30, 60, 300, 600`.
2. Add `MonitorPageRefreshIntervalSeconds int` to `service.SystemSettings`, `dto.SystemSettings`, and `UpdateSettingsRequest`; use pointer `*int` in the update request so an omitted partial update retains the prior value.
3. Add the default key to `defaultSettings`, parse through `NormalizeMonitorPageRefreshIntervalSeconds`, and always write the normalized value in `SettingService.UpdateSettings`.
4. In `SettingHandler.UpdateSettings`, validate a non-nil request field before building the merged `service.SystemSettings`; return the project's existing bad-request response for unsupported values.
5. Add the value to the updated API response and audit-field comparison.

```go
func IsMonitorPageRefreshIntervalSeconds(value int) bool {
    switch value {
    case 0, 30, 60, 300, 600:
        return true
    default:
        return false
    }
}
```

- [ ] **Step 6: Run focused backend tests to verify they pass**

Run:

```bash
cd backend && go test ./internal/service ./internal/handler/admin ./internal/server -run 'Test.*MonitorPageRefresh|TestAPIContract' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the backend policy**

```bash
git add backend/internal/service/domain_constants.go backend/internal/service/settings_view.go backend/internal/service/setting_parse.go backend/internal/service/setting_update.go backend/internal/handler/dto/settings.go backend/internal/handler/admin/setting_handler_update.go backend/internal/handler/admin/setting_handler_audit.go backend/internal/service/setting_service_update_test.go backend/internal/handler/admin/setting_handler_partial_payload_test.go backend/internal/server/api_contract_test.go
git commit -m "feat: configure monitor page refresh interval"
```

## Task 2: Publish the refresh policy in monitor snapshot contract v3

**Files:**

- Modify: `backend/internal/service/monitor_v2.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/cmd/server/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Modify: `backend/internal/handler/monitor_v2_handler.go`
- Test: `backend/internal/handler/monitor_v2_handler_test.go`
- Test: `backend/internal/server/routes/monitor_v2_routes_test.go`
- Modify: `frontend/src/features/monitor-v2/types.ts`
- Modify: `frontend/src/features/monitor-v2/api.ts`
- Test: `frontend/src/features/monitor-v2/__tests__/api.spec.ts`

**Interfaces:**

- Consumes `service.SystemSettings.MonitorPageRefreshIntervalSeconds`.
- Produces `MonitorV2Snapshot.RefreshIntervalSeconds int` and JSON `refresh_interval_seconds`.
- Produces frontend `MonitorV2Snapshot['refresh_interval_seconds']` constrained to `0 | 30 | 60 | 300 | 600`.

- [ ] **Step 1: Write the failing Go snapshot-contract test**

Extend `monitor_v2_handler_test.go` to expect version 3 and the configured refresh interval:

```go
require.Equal(t, "3", response.ContractVersion)
require.Equal(t, 300, response.RefreshIntervalSeconds)
```

Make the service stub return a snapshot whose `RefreshIntervalSeconds` is `300`.

- [ ] **Step 2: Run the Go handler test to verify it fails**

Run:

```bash
cd backend && go test ./internal/handler -run TestMonitorV2HandlerReturnsVersionedNoStoreContract -count=1
```

Expected: FAIL because the snapshot and response do not expose version 3 or the interval.

- [ ] **Step 3: Implement snapshot contract v3**

1. Change `MonitorV2ContractVersion` to `"3"`.
2. Add `RefreshIntervalSeconds int` to the service snapshot and response DTO.
3. Add a narrow `MonitorV2SettingsReader` interface with `GetAllSettings(context.Context) (*SystemSettings, error)`, add it to `MonitorV2Service`, and pass the existing `settingService` from `ProvideMonitorV2Service`.
4. Update the provider signature in `backend/internal/service/wire.go` and both generated server wiring files to pass `settingService`; regenerate the Wire output if the repository's existing generator is available, otherwise make the matching generated call signature explicit.
5. Resolve the setting once per `Snapshot` call; if loading it fails, use the safe default `60` rather than failing the monitoring page.
6. Serialize `refresh_interval_seconds` alongside `generated_at`.

```go
snapshot.RefreshIntervalSeconds = service.NormalizeMonitorPageRefreshIntervalSeconds(
    strconv.Itoa(settings.MonitorPageRefreshIntervalSeconds),
)
```

- [ ] **Step 4: Run Go monitor tests to verify they pass**

Run:

```bash
cd backend && go test ./internal/handler ./internal/server/routes ./internal/service -run 'TestMonitorV2' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write the failing frontend contract test**

In `frontend/src/features/monitor-v2/__tests__/api.spec.ts`, assert a version-3 payload without `refresh_interval_seconds` is rejected and each allowed value is accepted:

```ts
expect(() => validateMonitorV2Snapshot({ ...payload, refresh_interval_seconds: 15 }))
  .toThrow('refresh_interval_seconds is unsupported')
```

- [ ] **Step 6: Run the frontend API test to verify it fails**

Run:

```bash
cd frontend && npm run test:run -- src/features/monitor-v2/__tests__/api.spec.ts
```

Expected: FAIL because the frontend still expects contract version 2 and does not validate the refresh field.

- [ ] **Step 7: Implement frontend contract parsing**

Set `MONITOR_V2_CONTRACT_VERSION` to `'3'`, add the literal union type, add `refresh_interval_seconds` to `MonitorV2Snapshot`, and validate it using an allowed-value set:

```ts
const REFRESH_INTERVALS = new Set([0, 30, 60, 300, 600])
const refreshIntervalSeconds = integer(source.refresh_interval_seconds, 'refresh_interval_seconds')
if (!REFRESH_INTERVALS.has(refreshIntervalSeconds)) {
  throw new MonitorV2ContractError('refresh_interval_seconds is unsupported')
}
```

- [ ] **Step 8: Run frontend API tests to verify they pass**

Run:

```bash
cd frontend && npm run test:run -- src/features/monitor-v2/__tests__/api.spec.ts
```

Expected: PASS.

- [ ] **Step 9: Commit the contract change**

```bash
git add backend/internal/service/monitor_v2.go backend/internal/service/wire.go backend/cmd/server/wire.go backend/cmd/server/wire_gen.go backend/internal/handler/monitor_v2_handler.go backend/internal/handler/monitor_v2_handler_test.go backend/internal/server/routes/monitor_v2_routes_test.go frontend/src/features/monitor-v2/types.ts frontend/src/features/monitor-v2/api.ts frontend/src/features/monitor-v2/__tests__/api.spec.ts
git commit -m "feat: publish monitor refresh policy"
```

## Task 3: Add the administrator-only settings control

**Files:**

- Modify: `frontend/src/api/admin/settings.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/admin/SettingsView.vue`
- Modify: `frontend/src/i18n/locales/zh/admin/settings.ts`
- Modify: `frontend/src/i18n/locales/en/admin/settings.ts`
- Test: `frontend/src/views/admin/__tests__/SettingsView.spec.ts`

**Interfaces:**

- Consumes `/admin/settings` field `monitor_page_refresh_interval_seconds: number`.
- Produces a settings update payload with `monitor_page_refresh_interval_seconds` only from the fixed option values.

- [ ] **Step 1: Write the failing settings-view test**

Add a test that loads a `60`-second response, finds the select control, and asserts the save request preserves a changed `300` value:

```ts
expect(wrapper.get('[data-test="monitor-page-refresh-interval"]').element.value).toBe('60')
await wrapper.get('[data-test="monitor-page-refresh-interval"]').setValue('300')
await wrapper.get('[data-test="save-settings"]').trigger('click')
expect(updateSettings).toHaveBeenCalledWith(
  expect.objectContaining({ monitor_page_refresh_interval_seconds: 300 }),
)
```

- [ ] **Step 2: Run the view test to verify it fails**

Run:

```bash
cd frontend && npm run test:run -- src/views/admin/__tests__/SettingsView.spec.ts
```

Expected: FAIL because the form field and save payload do not exist.

- [ ] **Step 3: Implement typed settings form support**

1. Add the new field to the admin API `SystemSettings` and `UpdateSettingsRequest` interfaces and the shared `SystemSettings` type where it mirrors the API.
2. Initialize the SettingsView form at `60`; when loading settings, assign the returned value after defending against invalid legacy values with a local fixed-values helper.
3. Add a select in the existing monitoring-related settings section:

```vue
<select
  v-model.number="form.monitor_page_refresh_interval_seconds"
  data-test="monitor-page-refresh-interval"
  class="input"
>
  <option :value="0">{{ t('admin.settings.monitorPageRefresh.off') }}</option>
  <option :value="30">{{ t('admin.settings.monitorPageRefresh.seconds', { count: 30 }) }}</option>
  <option :value="60">{{ t('admin.settings.monitorPageRefresh.minute') }}</option>
  <option :value="300">{{ t('admin.settings.monitorPageRefresh.minutes', { count: 5 }) }}</option>
  <option :value="600">{{ t('admin.settings.monitorPageRefresh.minutes', { count: 10 }) }}</option>
</select>
```

4. Include the field in the existing save payload and apply the returned value back to the form.
5. Add Chinese and English title, description, and option labels stating that users cannot adjust this setting.

- [ ] **Step 4: Run the settings-view test to verify it passes**

Run:

```bash
cd frontend && npm run test:run -- src/views/admin/__tests__/SettingsView.spec.ts
```

Expected: PASS.

- [ ] **Step 5: Run locale compilation**

Run:

```bash
cd frontend && npm run test:run -- src/i18n/__tests__/localesMessageCompile.spec.ts
```

Expected: PASS with the new Chinese and English messages compiled.

- [ ] **Step 6: Commit the administrator control**

```bash
git add frontend/src/api/admin/settings.ts frontend/src/types/index.ts frontend/src/views/admin/SettingsView.vue frontend/src/views/admin/__tests__/SettingsView.spec.ts frontend/src/i18n/locales/zh/admin/settings.ts frontend/src/i18n/locales/en/admin/settings.ts
git commit -m "feat: add monitor refresh setting control"
```

## Task 4: Enforce automatic-only refresh behavior on the monitoring page

**Files:**

- Modify: `frontend/src/features/monitor-v2/MonitorV2View.vue`
- Modify: `frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
- Modify: `frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`

**Interfaces:**

- Consumes `initialSnapshot.refresh_interval_seconds`.
- Calls `getMonitorV2Snapshot(currentWindow, AbortSignal)` only on initial load, time-range selection, restored visibility, and administrator-configured intervals.
- Emits existing `fatal` only for non-abort snapshot failures.

- [ ] **Step 1: Write the failing component tests**

Use fake timers in `MonitorV2View.spec.ts`; update the fixture to version 3 with `refresh_interval_seconds: 60`, then add:

```ts
it('does not render a manual refresh control', () => {
  expect(mountView().find('[data-test="monitor-refresh"]').exists()).toBe(false)
})

it('refreshes using the administrator interval', async () => {
  vi.useFakeTimers()
  getMonitorV2Snapshot.mockResolvedValue(snapshot)
  const wrapper = mountView()
  await vi.advanceTimersByTimeAsync(60_000)
  expect(getMonitorV2Snapshot).toHaveBeenCalledWith('7d', expect.any(AbortSignal))
  wrapper.unmount()
})

it('does not schedule an interval when administrator disables refresh', async () => {
  vi.useFakeTimers()
  mountView({ ...snapshot, refresh_interval_seconds: 0 })
  await vi.advanceTimersByTimeAsync(600_000)
  expect(getMonitorV2Snapshot).not.toHaveBeenCalled()
})
```

Add tests that dispatch `document.visibilitychange`: when hidden, no timed call occurs; when visible again, one current-window request occurs. Add an unmount assertion showing no call after timers advance. Add an interval-change test where the first periodic response returns `300` and the next request occurs after five minutes rather than one.

- [ ] **Step 2: Run the component tests to verify they fail**

Run:

```bash
cd frontend && npm run test:run -- src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
```

Expected: FAIL because the page still renders a manual button and has no timed-refresh lifecycle.

- [ ] **Step 3: Implement the automatic-refresh lifecycle**

1. Remove the refresh button and its `Icon` import; retain the time-range selector.
2. Keep `reload(window)` private and call it only from `selectWindow`, the timer, and the visibility-restoration handler.
3. Store `refreshTimer: ReturnType<typeof window.setTimeout> | null`, then schedule a single next run only after a successful snapshot:

```ts
function scheduleRefresh(intervalSeconds: number) {
  clearRefreshTimer()
  if (intervalSeconds === 0 || document.visibilityState === 'hidden') return
  refreshTimer = window.setTimeout(async () => {
    await reload(currentWindow.value)
  }, intervalSeconds * 1_000)
}
```

4. After every accepted snapshot response, call `scheduleRefresh(next.refresh_interval_seconds)` so a server-side administrator change takes effect on the next poll.
5. Register a `visibilitychange` handler on mount. On `hidden`, clear the timer. On a transition back to `visible`, call `reload(currentWindow.value)`; its successful response schedules the next run.
6. On unmount, abort the active request, clear the timer, and remove the visibility listener. Do not schedule a new timer after an aborted or failed request.

- [ ] **Step 4: Run focused monitor view tests to verify they pass**

Run:

```bash
cd frontend && npm run test:run -- src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
```

Expected: PASS.

- [ ] **Step 5: Run frontend verification**

Run:

```bash
cd frontend && npm run test:run -- src/features/monitor-v2/__tests__/api.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/views/admin/__tests__/SettingsView.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts
npm run typecheck
npm run build
```

Expected: all commands PASS.

- [ ] **Step 6: Commit automatic-only monitoring refresh**

```bash
git add frontend/src/features/monitor-v2/MonitorV2View.vue frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
git commit -m "feat: auto refresh monitor page"
```

## Final Verification and Review

- [ ] **Step 1: Run backend verification**

```bash
cd backend && go test ./internal/service ./internal/handler ./internal/handler/admin ./internal/server ./internal/server/routes -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

```bash
cd frontend && npm run test:run -- src/features/monitor-v2/__tests__/api.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/views/admin/__tests__/SettingsView.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts
npm run typecheck
npm run build
```

Expected: PASS.

- [ ] **Step 3: Review the completed diff**

```bash
git diff HEAD~4..HEAD --check
git status --short
```

Expected: no whitespace errors and no staged or unstaged files outside this feature; preserve the pre-existing unrelated report modification.
