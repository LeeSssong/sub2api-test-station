# Task 3 实施报告：双维度摘要、权重入口与账号分区

## 状态

本任务代码与本地验证已完成，尚未推送、部署或进行线上验证。项目进度台账保持“进行中”，未新增或修改台账条目。

## 实施内容

- 为账号监控健康摘要 TypeScript 契约补充 `monitoring_accounts`，并把 `monitor_bucket` 收紧为五个固定值。
- 将全站展示拆分为“全站经营数据”和“全站账号数据”；分组展示拆分为“分组经营数据”和“分组服务数据”。经营数据保留真实上游成本、利润、覆盖率、待对账、历史与异常入口。
- 在分组摘要下新增同域操作行：显示可用、不可用、成本不合格、待确认、暂停五类范围内计数，展示当前 15/45/20/20 等权重，并保留显著的“评分权重”动作。
- 按 `monitor_bucket` 将账号互斥分区。分组的可用账号按组内质量分、排名、账号 ID 排序；其他分区和全站均按账号 ID 稳定排序。全站不再展示分组质量分。
- 空分组与搜索无结果时保留分组经营摘要、服务摘要和权重操作行；账号空态置于摘要之后。
- 状态筛选改为五个中文分区标签；账号历史协议状态改为中文展示。

## TDD 记录

RED：先只增加视图与筛选器的聚焦测试，然后执行：

```sh
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts
```

结果：2 个测试文件失败，22 个测试中 4 个失败、18 个通过。该 RED 运行之后补充了搜索零结果回归测试，因此最终 GREEN 套件为 37 个测试。失败项覆盖独立摘要区域、五分区、空分组保留摘要与权重行、五类中文筛选项；符合改动前预期。

GREEN：

```sh
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts
pnpm run typecheck
cd ../../..
git diff --check
```

结果：4 个测试文件、37 个测试全部通过；`vue-tsc --noEmit` 通过；`git diff --check` 通过。

## 文件清单

- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- `.superpowers/sdd/2026-08-02-account-monitor-product-completion-implementation-plan/task-3-report.md`

## 自审

- 未修改路由、后端、对账 API、调度器或评分接口；账号调度仍仅使用 `priority`。
- 已验证全站范围使用投影唯一账号集合，分组优先使用 `group.accounts`（包括显式空数组）。
- 五类账号分区使用唯一的 `monitor_bucket` 作为键，分组状态摘要使用未过滤的分组范围，避免搜索/筛选篡改范围计数。
- 分组质量评分角标仅在已选分组的“可用”分区展示；全站维持账号 ID 稳定顺序。

## 关注项

- 本任务只完成本地工程验证，尚未推送、部署或进行生产页面验收；不能将项目事项标记为“已完成”。
- Vitest 输出包含现有的 Browserslist 数据过期与 pnpm `overrides` 配置告警，不影响本任务测试结果。
