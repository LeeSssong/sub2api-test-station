# 六阶段生产收口代理上下文合同

**生效日期：** 2026-08-05

**适用范围：** `docs/superpowers/plans/2026-08-05-six-stage-production-closure-deployment-units.md` 的所有实施代理、修复代理、任务审查代理和最终审查代理。

## 强制恢复入口

每个代理开始工作前必须完整读取：

1. 本文件。
2. 自己的 SDD task brief。
3. `docs/project/active-delivery-contract.md`。
4. `docs/project/account-monitor-v3-acceptance-contract.md`。
5. `docs/project/project-progress.md` 当前顶部登记。

代理报告第一段必须写出：

```text
CONTEXT_ACK=2026-08-05-six-stage-production-closure
TASK_BRIEF=$TASK_BRIEF_PATH
DEPLOYMENT_GATE=$DEPLOYMENT_GATE_VALUE
```

缺少 `CONTEXT_ACK` 的实现报告或审查报告无效，协调代理不得据此提交、部署或进入下一任务。

## 当前用户最高优先级指令

- 不丢失总方案上下文；聊天摘要不能代替本合同、task brief 和 SDD `progress.md`。
- 只执行本任务要求的验证；不扩展成全项目、全仓库或无关功能验证。
- 安全部署脚本自身要求的构建、迁移、健康、身份和回滚门禁属于本任务必要验证，不得删减。
- 拆分所有可独立部署单元；一个单元实现、审查、推送、生产部署并完成必要线上验证后立即停止。
- 每次生产部署后状态写为“等待用户验收”，只有用户明确确认后，协调代理才能派发下一个部署单元。
- 非部署准备任务可以连续完成，但不得越过最近的生产部署验收门。

## 不可漂移边界

- `/admin/accounts/monitor` 只承担账号服务质量、评分、排名和调度信息。
- 营收、用户计费、利润、利润率、账务、对账、流水、异常和运营汇总只进入独立 `/admin/revenue`。
- relay-ops 继续作为不可见后台服务，不恢复独立浏览器控制面。
- 真实上游扣费只能来自合法只读账单授权和真实交易/快照，不能由倍率、采购成本、`today_stats` 或前端估算替代。
- 飞书只保留主动告警、恢复通知和日报；不恢复入站命令或写控制。
- OpenAI 审计只保存最长 100 字符的实际响应模型字符串，不保存原始响应，不改变路由、计费或客户端响应。

## 基线事实

- 账号监控生产运行时来源提交：`05985e62ec88b04d1e647a815eecdb1cf1155776`。
- 账号监控收口远端分支：`origin/codex/account-monitor-completion@bbfe4a36d`。
- 2026-08-05 核对时：`origin/main@138d26efa`，两分支为 main 独有 4 个提交、账号监控分支独有 81 个提交。
- 本地 `main` 含未推送和未统一审查的提交，禁止整分支合并到生产基线。
- OpenAI 后端基础提交 `c53bbdf48`、`f20ab6a99`、`7518ac689` 已在账号监控分支；`45921782a`、`122d293db` 只能逐文件移植，不得整体盲目 cherry-pick。

## 状态与恢复规则

- 唯一持久化执行台账是本计划对应的 `.superpowers/sdd/.../progress.md`。
- 恢复时先核对台账最后一个 `Task N` 状态、Git 提交和生产 release-state，再决定是否派发。
- `awaiting_user_acceptance` 表示硬停止状态；任何代理不得自行改为 complete 或继续下一个部署单元。
- 用户确认后，协调代理在台账追加 `Task N: user accepted`，才可生成下一个 task brief。
- 所有代理只修改 task brief 列出的文件。发现无关缺陷只记录 concern，不修复、不验证、不扩展范围。
