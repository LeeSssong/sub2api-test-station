# Project collaboration constraints

- Once an implementation plan has been approved, execute it with subagents by default: assign each plan task to a fresh implementer subagent, require an independent task review after each task, and run a final whole-branch review before completion.
- Continue through approved plan tasks without repeated approval prompts unless execution is genuinely blocked, the plan conflicts with itself, or a new decision would materially change the approved scope.
- Explicit instructions in the current user request override these defaults.

## 项目进度总账（强制）

- 实施前必须登记 `docs/project/project-progress.md`，状态为“进行中”；实施中持续更新。
- 只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才能标记“已完成”。
- 仅本地/离线测试、代码合并或报告完成不能标记“已完成”；历史离线成果可归为“准备完成”，但它是未完成的非终态分类，不计入完成数。
- 仍需部署、线上验证或受外部条件阻塞的事项保持“进行中”。
