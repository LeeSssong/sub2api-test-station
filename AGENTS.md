# Project collaboration constraints

- 本轮原生 Sub 小步发布计划的所有根线程、派生线程和审查线程，开始任何工作前必须完整阅读 `docs/project/native-sub-incremental-delivery-constraints.md` 与 `docs/project/native-sub-task-package-queue.md`；两份文件是本轮任务边界、线程交接、串行合并和部署停机门禁的共同事实源。
- 所有线程在涉及验收站登录、日志、运维、发布或主站部署前，必须完整阅读 `docs/project/acceptance-station-global-constraints.md`。该文件是验收站固定入口/宿主身份、敏感凭据读取方式、验收站发布命令，以及主站仅允许“测试站验收通过，部署主站”或“快速部署到主站”两种明确授权路径、主站成功后同 commit 立即同步验收站的共同事实源。密码、token、私钥和支付/上游/通知凭据只能从本机 0600 受保护文件读取，不得写入仓库或聊天。

- 自 2026-08-16 用户最新指令起，计划内任务以“功能实现完成 + 直接相关功能测试通过”为完成门槛；不再强制逐任务独立复审、scoped re-review 或全分支终审，也不为形式扩大验证。仍可用 fresh implementer 隔离写入；只有发现真实功能失败、范围冲突或高风险问题时才追加针对性复核。
- Continue through approved plan tasks without repeated approval prompts unless execution is genuinely blocked, the plan conflicts with itself, or a new decision would materially change the approved scope.
- 只有先满足 `docs/project/acceptance-station-global-constraints.md` 定义的两种主站明确授权之一后，发布预检返回 `downtime_required=false` 才表示无需再次请求同一次部署授权，可继续既有本地/宿主蓝绿发布和线上验证；`downtime_required=false` 本身绝不构成主站发布授权。返回 `downtime_required=true` 时，仍必须在任何停服、迁移、重启或切换前暂停并取得用户明确授权。
- Explicit instructions in the current user request override these defaults.
- Sub2API release preparation and production deployment must not use GitHub
  Actions. Keep release discovery, qualification, publishing, staging, source
  advancement, and blue-green promotion in the reviewed local/host script
  chain; do not add scheduled or manually dispatched release workflows under
  `.github/workflows/`.
- 官方 Sub2API 更新是最高优先级插队任务。官方更新只要求把最新稳定版合入当前定制树、人工解决全部冲突并通过既有本地/宿主发布链使其生效；主站发布仍必须先满足验收站全局约束定义的两种用户明确授权之一。不运行功能测试、回归测试、类型检查、构建验证或上线专项验收。发布链为原子切换而内置的候选构建、迁移/停机保护和运行就绪检查不属于额外测试或验收，不得绕过，也不得改用 GitHub Actions。

## 唯一发布总控与并发边界（强制）

- 本项目同时只能有一个发布总控。只有发布总控可以修改根目录 `main`、`docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、发布证据和生产状态记录。
- 功能任务窗口只维护自己的规格、计划、实现、测试、复审报告和交接文件；不得在自己的 worktree 修改全局队列或项目进度总账。
- 最多允许两个互不依赖的功能 worktree 同时处于 `DESIGNING`、`IMPLEMENTING` 或 `REVIEWING`；同一个 worktree 同时只能有一个实现写入者，reviewer 必须严格只读。
- 状态机统一为：`BACKLOG -> DESIGNING -> IMPLEMENTING -> REVIEWING -> READY_FOR_ROOT_REVIEW -> REFRESH_REQUIRED -> INTEGRATING -> DEPLOYING -> VERIFYING -> DONE`，异常终态或暂停态为 `FROZEN` / `BLOCKED`。
- 多任务可以并行准备，但同时只能有一个任务处于 `INTEGRATING`、`DEPLOYING` 或 `VERIFYING`。只有发布总控可以发出 `AUTHORIZE_MERGE_TO_MAIN`、推送、部署和线上验收指令。
- 已冻结的窗口、分支和 worktree 仅作只读证据；没有发布总控的明确解冻指令，不得恢复代理、继续写入、合并、推送、部署或清理。

## 工作区生命周期约束（强制）

- 每次开启新任务，必须先在 `docs/project/project-progress.md` 登记任务、范围、当前工作区和状态“进行中”；实施过程中在设计确认、实现完成、合并、部署、验证或阻塞等实质状态变化时更新同一条记录。
- 实施前必须扫描全部已注册的非 `main` worktree，比较其分支提交、工作区状态和生产发布证据；若某个非 `main` worktree 已完成且领先 `main`，必须先将其审查并合并到 `main`，再从更新后的 `main` 创建新任务 worktree。不得以“当前任务无关”跳过领先变更的盘点。
- 只有用户在当前任务中明确点名的活动 worktree 才可作为保护例外；保护例外必须写入总账并保留其未提交内容，不得把“所有脏 worktree”作为默认忽略范围。本轮明确保护的线程为“新建运营界面”和“优化账号卡片”。
- 每次准备部署，必须先把候选 worktree 合并到 `main`，在合并后的 `main` 上完成冲突检查、专项回归、构建/类型检查、迁移与发布预检；生产推送和部署只能从该已验证的 `main` 执行。官方 Sub2API 更新适用上方官方更新例外：只解决冲突并直接发布生效，不执行额外测试或验收，但保留发布链不可绕过的原子切换保护。
- 只有同时满足 `main` 已推送到服务端、部署成功且线上验证生效，才能删除对应候选 worktree 和本地分支。删除前必须确认没有未提交、未归档或未解决冲突的内容，并在总账或整合清单记录删除结果与恢复证据位置。
- 合并、构建、部署或线上验证失败时，必须保留候选 worktree、失败证据和未提交修复，在同一候选上继续修复；修复后重复“合并到 `main` → 并版回归 → 推送 → 部署 → 线上验证”循环，禁止直接覆盖、清理或删除失败候选。

## 项目进度总账（强制）

- 实施前必须登记 `docs/project/project-progress.md`，状态为“进行中”；实施中持续更新。
- 只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才能标记“已完成”；官方 Sub2API 更新在发布链成功切换后即可标记完成，不另做功能验收。
- 仅本地/离线测试、代码合并或报告完成不能标记“已完成”；历史离线成果可归为“准备完成”，但它是未完成的非终态分类，不计入完成数。
- 仍需部署、线上验证或受外部条件阻塞的事项保持“进行中”。
