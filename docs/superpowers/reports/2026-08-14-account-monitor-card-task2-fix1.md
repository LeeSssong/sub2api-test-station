# 账号监控卡片 Task 2 Fix Round 1

日期：2026-08-14

## 修复范围

- 详情入口使用代际令牌，只允许最后一次入口请求打开账号信息、编辑、删除或更多界面；相同账号仍共享单个详情请求，不同账号乱序返回不会覆盖当前操作目标。
- `更多` 菜单从实际触发按钮的 `DOMRect` 计算浮层位置，使用项目已有 `getFloatingPanelPosition` 规则。
- 复制加入原生同账号进行中去重；隐私设置按原生 `privacy_mode` 结果区分成功、Cloudflare 阻断和失败；恢复状态、配额重置、刷新凭据、Spark 影子账号使用与账号管理页一致的成功/失败语义和 i18n key。
- 只读账号信息壳不再展示原始 `error_message`，测试使用包含凭据样式文本的错误值证明不会渲染。
- 未修改 `AccountsView.vue`、`AccountMonitorAccount` DTO、后端、迁移、配置、总账、任务队列或生产。

## TDD 证据

- RED：新增跨账号乱序返回与隐私结果测试后，聚焦测试出现 2 项预期失败：旧实现把账号 10 覆盖到账号 11 的编辑目标；Cloudflare 阻断结果未调用错误提示。
- GREEN：
  - `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts --reporter=dot`：3 files，70 tests passed。
  - `pnpm vitest run src/components/admin/account/__tests__/AccountActionMenu.spark_shadow.spec.ts src/views/admin/__tests__/AccountsView.sparkShadow.spec.ts --reporter=dot`：2 files，19 tests passed。
  - `pnpm typecheck`：PASS。
  - `pnpm build`：PASS；仅有仓库既有的动态/静态 import 与 Browserslist 警告。
  - `git diff --check`：PASS。

## 交付状态

等待独立 scoped re-review。尚未合并、推送、部署或触碰生产。
