# Task 2 Report: 默认全站 Tab 与无隐式筛选

## RED 证据

命令：

```sh
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts
```

工作目录：`upstream/sub2api/frontend`

预期失败：现有实现首屏自动选择首个分组、过滤掉暂停账号、账务请求带 `group_id`，并保留分组下拉。

结果：2 个测试文件失败。视图测试无法找到 `all-site-tab`，渲染的首个 `group-tab-3` 为选中状态且仅显示 1 张卡片；筛选器测试发现 `group-filter` 仍存在。之后为“无分组仍显示全站 Tab”新增的 RED 用例也失败，确认旧模板以 `sortedGroups.length` 条件渲染整个 Tab 区域。

## GREEN 证据

命令：

```sh
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts
pnpm run typecheck
```

工作目录：`upstream/sub2api/frontend`

结果：Vitest 通过，2 个测试文件、17 个测试全部通过；`vue-tsc --noEmit` 退出码为 0。

## 文件清单

- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
  - 对齐 Task 1 投影的 `management_state`、`service_state`、`group_eligibility`、`monitor_bucket` 字段。
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
  - 新增固定中文“全站”首 Tab，以 `activeGroupId === null` 表示全站。
  - 首屏保留完整账号投影；删除默认首分组和附加分组筛选。
  - 全站账务请求不传 `group_id`；仅分组 Tab 选择后传该范围。
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
  - 删除重复的分组下拉和事件。
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
  - 覆盖全站首屏、多分组/未分组/暂停账号、无 `group_id`、无分组边界、分组切换与搜索/服务状态组合。
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.spec.ts`
  - 覆盖仅保留搜索、平台和服务状态筛选。

## 自审

- 已确认首屏不再以 `status === 'active' && schedulable` 裁剪账号。
- 已确认全站账户账务范围为 `{ account_id }`，没有 `group_id: undefined` 或具体分组值。
- 已确认分组范围只由 Tab 改变，筛选器不再暴露分组选择。
- 已确认未改写 `accounts.priority`、调度逻辑、评分保存逻辑、摘要重排或五类视觉分区。
- `git diff --check` 无输出。

## Concerns

- 无功能性 concern。
- 验证环境仍提示 pnpm overrides 配置弃用、Node localStorage experimental 和 Browserslist 数据过期；这些均为既有非阻塞告警，未在本任务范围内修改。
