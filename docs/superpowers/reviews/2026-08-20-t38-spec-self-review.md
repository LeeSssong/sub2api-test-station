# T38 规格自审

- 审查对象：`docs/superpowers/specs/2026-08-20-t38-schedulable-native-probe-score-retention-design.md`
- 基线：`main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`
- 结论：`READY_FOR_SPEC_APPROVAL`

## 完整性检查

- [x] 问题证据定位到 `projectAccountMonitorProbe`、`accountMonitorScoreStatus` 与 `accountMonitorWindowEvidence` 的双重新鲜度耦合。
- [x] 现有 focused tests 可稳定复现 unavailable/stale 清空评分资格。
- [x] 明确复用 `account_monitor_results`、`Account.IsSchedulable()` 与既有评分公式。
- [x] 明确“可调度”不是 raw `Schedulable` 字段，而是原生方法的完整当前语义。
- [x] 可评分证据最低条件包含至少一条成功原生探测，纯失败样本不会仅凭成本得分。
- [x] latest failure 与 stale 两条边界均有独立验收合同。
- [x] 当前状态与评分资格允许形成 `unavailable|stale + scored/ranked` 组合。
- [x] 24h/7d/30d 不跨窗口回退，避免混合范围排名。
- [x] T32 暂停账号语义保持原路径，T38 新分支仅覆盖当前 `Account.IsSchedulable()==true`。
- [x] 无新字段、迁移、配置、历史回填或生产写入。
- [x] 测试、发布、回滚、未验证项与剩余风险已列明。

## 矛盾与歧义检查

- 状态“不可用/过期”与评分“有历史窗口证据”已经拆成两个问题，不再互相覆盖。
- “最近一次评分”没有实现为浏览器缓存；统一解释为所选窗口最近聚合，最新失败也进入成功率分量。
- “从未有有效证据”落实为当前所选范围没有成功原生探测；切换范围后按该范围独立判断。
- `eligible` 明确为评分资格，不承诺 scheduler 选择或当前健康。

## 范围检查

- 预计代码范围集中在账号监控 service 及直接测试；仅当前端合同暴露强耦合时增加最小组件测试/修正。
- 保持评分数学、权重、调度、runner、Monitor V2、账务与生产数据边界。
- 本任务工作区未触碰全局队列、项目进度总账、根 `main`、发布证据或生产。

## 自审结论

规格具备实施计划所需的输入与验收闭环，等待唯一发布总控返回 `APPROVE_SPEC_T38`。
