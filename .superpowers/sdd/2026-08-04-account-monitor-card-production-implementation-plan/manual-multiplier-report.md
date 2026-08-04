# Manual multiplier follow-up

日期：2026-08-04

## 根因与修复

- `AccountMultiplierService.Resolve` 只读取原生探测或 measured snapshot，缺少原生探测数据时不会投影账号已保存的手工倍率。
- 账号更新路径已有 `rate_multiplier_policy` 的显式策略字段和 managed probe 的条件更新保护；本轮补齐了倍率值必须为有限且 `>= 0` 的服务端校验，并让 manual override 在监控解析中优先于任何后续 managed probe。
- V3 卡片原有采购成本编辑态没有覆盖“无采购成本且无原生倍率”的账号。新增中文账号倍率录入/编辑、非负有限校验、异步完成回调、失败时保留草稿/焦点/局部错误；View 使用精确 payload `{ rate_multiplier, rate_multiplier_policy: 'manual_override' }`。

## RED 证据

```text
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMultiplierResolveUsesManualOverrideWithoutNativeProbe' -count=1
# FAIL: undefined: AccountMonitorMultiplierSourceManual
```

```text
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts -t 'manual multiplier'
# FAIL: 2 tests; card rendered "上游托管倍率" and had no manual-multiplier action/input.
```

## GREEN 证据

```text
cd upstream/sub2api/backend
go test ./internal/service -count=1
# PASS
go test ./internal/handler/admin ./internal/repository -count=1
# PASS
```

```text
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
# PASS: 2 files, 31 tests
pnpm typecheck
# PASS
```

## 文件

- `upstream/sub2api/backend/internal/service/account_multiplier.go`
- `upstream/sub2api/backend/internal/service/upstream_billing_rate_multiplier_sync.go`
- `upstream/sub2api/backend/internal/service/account_multiplier_test.go`
- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

## 约束与担忧

- 未执行 push、部署、生产访问或项目完成状态变更；交付仍需协调任务完成后续发布与线上验证。
- 浏览器测试仅有既有 pnpm/Browserslist 与 Node localStorage 警告，不影响测试结果。
- 手工倍率来源使用稳定 API 值 `source: "manual"`；采购成本非空时仍优先展示采购模式，账号倍率编辑入口只在倍率模式显示。

## Commit

代码提交：`1c739b1745719d79030c9d0d97ee740f840f79f9`（`fix(admin): support manual monitor multiplier`）。
