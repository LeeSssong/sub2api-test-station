# T33 经营页账号卡片与搜索交接

- 状态：`READY_FOR_ROOT_REVIEW`
- REFRESH_REQUIRED 基线 `main` SHA：`584b37bba6ed05d86a5a152160d37a9f92fefc9c`
- 刷新后候选 merge SHA：`aedccc218`
- 功能提交 SHA：`fa6a2b33cd35f4d6cb76a2c857e69b5ce0055049`
- 当前 worktree：`/Users/gongtengxinwen/.codex/worktrees/e4db/sub2api搭建`（`codex/t33-account-profitability-cards`）
- 规格：`docs/superpowers/specs/2026-08-19-t33-profitability-account-cards-search-design.md`
- 计划：`docs/superpowers/plans/2026-08-19-t33-profitability-account-cards-search.md`

## 实现

- `AccountProfitabilityView.vue` 的 USD 表格改为每账号独立卡片，桌面 `lg:grid-cols-2`、窄屏 `grid-cols-1`；固定两列字段网格保留内部运营消耗、业务消耗、业务营收、总消耗、净利润、利润率及 active/historical 状态。
- CNY 自购表格改为每账号独立卡片，固定两列字段网格保留采购成本（CNY）、预计额度（USD）、标准消耗（USD）、利用率、确认成本（CNY）、待摊（CNY）、采购损失（CNY）、营收（CNY）、净利润（CNY）、利润率、运行/成本状态及录入/编辑/确认失效入口。
- 共享 `account-search` 在两个视图本地过滤账号名、ID、平台、类型和状态/成本状态；无匹配显示空态，不触发 API；清空恢复。
- 继续复用 `accountFinancial`、`selfPurchasedProfitability` 和 `AccountMonitorCostDialog`，未修改后端财务公式、采购保存链路、调度、账号数据或生产数据。
- 明确零流水和 `cost_pending` 的展示：标准消耗 `0.00 USD`，采购成本/成本状态显示“成本待录入”，其余 null 金额保持 `—`。

## 变更文件

- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- `docs/superpowers/specs/2026-08-19-t33-profitability-account-cards-search-design.md`
- `docs/superpowers/plans/2026-08-19-t33-profitability-account-cards-search.md`
- 本交接文件

## 直接相关测试

- `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`：29/29 通过。
- `cd upstream/sub2api/frontend && pnpm exec vitest run src/api/admin/__tests__/accountFinancial.spec.ts src/api/admin/__tests__/selfPurchasedProfitability.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts`：33/33 通过。
- `cd upstream/sub2api/frontend && pnpm typecheck`：通过。
- `cd upstream/sub2api/frontend && pnpm build`：通过。
- `git diff --check`：通过。

刷新后上述直接门禁全部重新执行并通过；本次又将 T32 已验证生产 `main@584b37bba` 合入候选，未覆盖本任务文件。

测试覆盖 USD/CNY 卡片、搜索名称/ID/平台/类型/状态、无匹配空态、长账号名换行、390px 单列、成本待录入、零流水、原生排序/分组/刷新/错误和共享采购操作。

## 未验证项

- 未做浏览器登录态截图或生产线上专项验收；由根总控在合并部署后执行。
- USD 后端报表未提供实时 status，卡片/搜索使用 `historical ? historical : active` 本地映射；CNY 使用原生 `status/cost_status`。

## 迁移、配置与发布

- 无数据库迁移、无配置变化、无依赖变化、无生产数据写入、无 GitHub Actions。
- `downtime_required=false`（本候选仅前端展示变更；最终以根合并后的发布预检为准）。
- 未合并到根 `main`、未推送、未部署；本 worktree 通过刷新 merge 引入 T32 已验证 `main`，未修改生产状态或发布证据。

## 回滚与剩余风险

- 回滚：根总控恢复上一已验证 `main` 提交/镜像；本候选 worktree 保留供修复。
- 剩余风险：真实生产数据量和长名称样式需线上登录态确认；状态搜索的 USD active/historical 是现有字段可证明的本地兼容映射，不改变后端事实。
