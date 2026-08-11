# Project collaboration constraints

- 本轮原生 Sub 小步发布计划的所有根线程、派生线程和审查线程，开始任何工作前必须完整阅读 `docs/project/native-sub-incremental-delivery-constraints.md` 与 `docs/project/native-sub-task-package-queue.md`；两份文件是本轮任务边界、线程交接、串行合并和部署停机门禁的共同事实源。

- Once an implementation plan has been approved, execute it with subagents by default: assign each plan task to a fresh implementer subagent, require an independent task review after each task, and run a final whole-branch review before completion.
- Continue through approved plan tasks without repeated approval prompts unless execution is genuinely blocked, the plan conflicts with itself, or a new decision would materially change the approved scope.
- Explicit instructions in the current user request override these defaults.
- Sub2API release preparation and production deployment must not use GitHub
  Actions. Keep release discovery, qualification, publishing, staging, source
  advancement, and blue-green promotion in the reviewed local/host script
  chain; do not add scheduled or manually dispatched release workflows under
  `.github/workflows/`.

## 工作区生命周期约束（强制）

- 每次开启新任务，必须先在 `docs/project/project-progress.md` 登记任务、范围、当前工作区和状态“进行中”；实施过程中在设计确认、实现完成、合并、部署、验证或阻塞等实质状态变化时更新同一条记录。
- 实施前必须扫描全部已注册的非 `main` worktree，比较其分支提交、工作区状态和生产发布证据；若某个非 `main` worktree 已完成且领先 `main`，必须先将其审查并合并到 `main`，再从更新后的 `main` 创建新任务 worktree。不得以“当前任务无关”跳过领先变更的盘点。
- 只有用户在当前任务中明确点名的活动 worktree 才可作为保护例外；保护例外必须写入总账并保留其未提交内容，不得把“所有脏 worktree”作为默认忽略范围。本轮明确保护的线程为“新建运营界面”和“优化账号卡片”。
- 每次准备部署，必须先把候选 worktree 合并到 `main`，在合并后的 `main` 上完成冲突检查、专项回归、构建/类型检查、迁移与发布预检；生产推送和部署只能从该已验证的 `main` 执行。
- 只有同时满足 `main` 已推送到服务端、部署成功且线上验证生效，才能删除对应候选 worktree 和本地分支。删除前必须确认没有未提交、未归档或未解决冲突的内容，并在总账或整合清单记录删除结果与恢复证据位置。
- 合并、构建、部署或线上验证失败时，必须保留候选 worktree、失败证据和未提交修复，在同一候选上继续修复；修复后重复“合并到 `main` → 并版回归 → 推送 → 部署 → 线上验证”循环，禁止直接覆盖、清理或删除失败候选。

## 项目进度总账（强制）

- 实施前必须登记 `docs/project/project-progress.md`，状态为“进行中”；实施中持续更新。
- 只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才能标记“已完成”。
- 仅本地/离线测试、代码合并或报告完成不能标记“已完成”；历史离线成果可归为“准备完成”，但它是未完成的非终态分类，不计入完成数。
- 仍需部署、线上验证或受外部条件阻塞的事项保持“进行中”。
