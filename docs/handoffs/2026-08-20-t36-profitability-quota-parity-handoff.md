# T36 经营页 CNY/USD 额度关系文案交接

- 任务：`T36`
- 状态：`READY_FOR_ROOT_REVIEW`
- 刷新后基线 `main` SHA：`180ddd25b`
- 刷新后候选整合提交 SHA：`629e3265732602ed8d84a2c783c221c1a9fd39e1`
- 刷新后候选 tree：`8dc322789064b14ed05fb7eba1d6084f2464bef6`
- 候选 worktree：`/Users/gongtengxinwen/.codex/worktrees/e649/sub2api搭建`
- 候选分支：`codex/t36-profitability-quota-parity`
- 规格：`docs/superpowers/specs/2026-08-20-t36-profitability-quota-parity-copy-design.md`
- 计划：`docs/superpowers/plans/2026-08-20-t36-profitability-quota-parity-copy.md`

## 实现

- 在 USD/CNY segmented view 下新增 `p[data-test="quota-parity-note"]`，使用现有 `useI18n()` 渲染 `admin.accountProfitability.quotaParityNote`。
- zh 文案：`额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）`。
- en 文案：`Quota basis: 1 USD quota = 1 CNY quota (for understanding the quota relationship only; not an exchange-rate conversion)`。
- 文案节点位于现有 view 切换控件之后、USD/CNY 分支之外，两个视图和 loading/空数据状态均可见。
- 保留现有 `() => import('@/views/admin/AccountProfitabilityView.vue')` 路由懒加载、API 请求、账务字段、采购保存/结算与搜索/卡片行为。

## 变更文件

- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`
- `docs/superpowers/specs/2026-08-20-t36-profitability-quota-parity-copy-design.md`
- `docs/superpowers/plans/2026-08-20-t36-profitability-quota-parity-copy.md`
- `docs/handoffs/2026-08-20-t36-profitability-quota-parity-handoff.md`

## 直接验证

- RED：新增组件合同先于实现运行，30 项中 1 项按预期因缺少 `quotaParityNote` 源码/i18n 合同失败。
- GREEN：`cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`：30/30 通过。
- 页面 + locale：`pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts`：32/32 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过；既有 dynamic-import/chunk-size/Browserslist 警告仍存在，未引入新的路由或手工分包。构建产物中可检出中英文固定文案，并生成独立 `AccountProfitabilityView-*.js` 懒加载包。
- `git diff --check`：通过。

## 范围、迁移与发布

- 无后端、API、SQL、查询、账务公式、采购保存、账号数据、数据库迁移、配置、依赖或生产数据变化。
- 无全局队列、项目进度总账、生产证据或部署文件变化。
- 预期 `downtime_required=false`；最终以根总控合并后的发布预检为准。
- 未合并根 `main`、未推送、未部署、未执行线上写入。

## 回滚与剩余风险

- 回滚：根总控恢复上一已验证 `main` 提交/镜像；不涉及数据恢复。
- 根总控负责确认目标 `main` 未漂移、授权合并、发布链、线上健康验证和最终队列/总账收口。
- 未做登录态浏览器截图；当前用户指令规定真机视觉验收不阻塞候选收口。
