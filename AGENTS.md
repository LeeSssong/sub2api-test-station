# Project collaboration constraints

- 本轮原生 Sub 小步发布计划的所有根线程、派生线程和审查线程，开始任何工作前必须完整阅读 `docs/project/native-sub-incremental-delivery-constraints.md` 与 `docs/project/native-sub-task-package-queue.md`；两份文件是本轮任务边界、线程交接、串行合并和部署停机门禁的共同事实源。
- 所有线程在涉及验收站登录、日志、运维、发布或主站部署前，必须完整阅读 `docs/project/acceptance-station-global-constraints.md`。该文件是验收站固定入口/宿主身份、敏感凭据读取方式、验收站发布命令，以及主站仅允许“测试站验收通过，部署主站”“快速部署到主站”或用户明确授权“快速部署主站，不同步验收站”三种路径的共同事实源；不同步路径必须记录版本差异，不得声称主站与验收站一致。密码、token、私钥和支付/上游/通知凭据只能从本机 0600 受保护文件读取，不得写入仓库或聊天。
- 所有环境的部署（包括验收站、主站、预演、relay-ops、旧 admin lab 和官方更新）只允许从根目录干净的 `main` 发起，且 `HEAD` commit/tree 必须与已推送的 `origin/main` 完全一致。禁止从功能 worktree、候选分支、临时 checkout 或 detached HEAD 构建或发布任何部署制品。发布链内部自动回滚到上一已验证槽位是失败保护，不是从非 `main` 发起新部署；人工重新发布旧版本必须先在 `main` 上形成明确的 revert 或前向修复提交并推送。

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
- 允许多个互不依赖的功能 worktree 同时处于 `DESIGNING`、`IMPLEMENTING` 或 `REVIEWING`；同一个 worktree 同时只能有一个实现写入者，reviewer 必须严格只读。
- 状态机统一为：`BACKLOG -> DESIGNING -> IMPLEMENTING -> REVIEWING -> READY_FOR_ROOT_REVIEW -> REFRESH_REQUIRED -> INTEGRATING -> DEPLOYING -> VERIFYING -> DONE`，异常终态或暂停态为 `FROZEN` / `BLOCKED`。
- 多任务可以并行准备，但同时只能有一个任务处于 `INTEGRATING`、`DEPLOYING` 或 `VERIFYING`。只有发布总控可以发出 `AUTHORIZE_MERGE_TO_MAIN`、推送、部署和线上验收指令。
- 已冻结的窗口、分支和 worktree 仅作只读证据；没有发布总控的明确解冻指令，不得恢复代理、继续写入、合并、推送、部署或清理。

## 工作区生命周期约束（强制）

- 每次开启新任务，必须先在 `docs/project/project-progress.md` 登记任务、范围、当前工作区和状态“进行中”；实施过程中在设计确认、实现完成、合并、部署、验证或阻塞等实质状态变化时更新同一条记录。
- 实施前必须扫描全部已注册的非 `main` worktree，比较其分支提交、工作区状态和生产发布证据；若某个非 `main` worktree 已完成且领先 `main`，必须先将其审查并合并到 `main`，再从更新后的 `main` 创建新任务 worktree。不得以“当前任务无关”跳过领先变更的盘点。
- 只有用户在当前任务中明确点名的活动 worktree 才可作为保护例外；保护例外必须写入总账并保留其未提交内容，不得把“所有脏 worktree”作为默认忽略范围。本轮明确保护的线程为“新建运营界面”和“优化账号卡片”。
- 每次准备部署，必须先把候选 worktree 合并到根目录 `main`、推送 `origin/main`，再在该干净且 commit/tree 一致的 `main` 上完成冲突检查、直接相关回归、必要构建/类型检查、迁移与发布预检。验收站、主站及其他任何环境的部署和部署制品构建都只能从该 `main` 执行。官方 Sub2API 更新适用上方官方更新测试例外，但不适用发布来源例外：冲突解决结果仍必须先进入并推送 `main` 后才可发布。
- 只有同时满足 `main` 已推送到服务端、部署成功且线上验证生效，才能删除对应候选 worktree 和本地分支。删除前必须确认没有未提交、未归档或未解决冲突的内容，并在总账或整合清单记录删除结果与恢复证据位置。
- 合并、构建、部署或线上验证失败时，必须保留候选 worktree、失败证据和未提交修复，在同一候选上继续修复；修复后重复“合并到 `main` → 并版回归 → 推送 → 部署 → 线上验证”循环，禁止直接覆盖、清理或删除失败候选。

## 项目进度总账（强制）

- 实施前必须登记 `docs/project/project-progress.md`，状态为“进行中”；实施中持续更新。
- 只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才能标记“已完成”；官方 Sub2API 更新在发布链成功切换后即可标记完成，不另做功能验收。
- 仅本地/离线测试、代码合并或报告完成不能标记“已完成”；历史离线成果可归为“准备完成”，但它是未完成的非终态分类，不计入完成数。
- 仍需部署、线上验证或受外部条件阻塞的事项保持“进行中”。

## Sub2API 双环境与唯一发布来源约束（全局）

本项目包含两个完全独立的环境：主站（正式生产环境）和验收站（测试、人工验收环境）。不同客户端或 AI 可能同时修改验收站，因此必须始终区分“验收站当前运行状态”和“允许推广到主站的代码变更”。最高原则：验收站不是主站代码来源；经过 Git 审计的 commit 才是代码来源；主站根目录 `main` 是唯一发布来源。

### 代码变更与来源

- 所有可推广代码改动必须从最新 `origin/main` 创建独立分支；每次改动必须形成 Git commit 并推送远程分支。
- 必须记录作者、时间、基线 commit、目标 commit 和变更文件。
- 禁止把服务器目录、容器内文件、Docker volume、运行镜像或人工复制文件直接作为主站发布来源。
- 验收站服务器临时修改只能用于排查，不能自动视为正式代码。发现未提交服务器修改时，必须停止推广，先导出脱敏差异，并将修改重建为 Git commit 后再审核。

### 环境隔离

禁止从验收站复制到主站：数据库数据、用户/账号/API Key、订单/充值/余额/账务数据、Redis 缓存、上传文件/对象存储内容、日志敏感信息、密码/token/私钥/上游凭据/支付凭据/webhook 以及任何生产运行数据。

允许推广的内容仅包括：已审核的代码 commit、明确审核过的 migration、明确审核过的配置变化，以及可追溯到 Git commit 的必要静态资源和构建产物。不得通过删除、覆盖、强制 reset 或复制服务器目录来“整理干净”。

### 验收站修改与交接

每次修改验收站必须依次完成：读取并确认运行中的 `source_commit`、`source_tree`、`image_digest`；从最新 `origin/main` 创建独立分支；实施修改；运行直接相关测试；检查 `git diff`、`git status` 和敏感文件；提交并推送 Git commit；输出交接报告；等待主站维护 AI 审核和合并。

交接报告必须包含：验收站原始版本 commit/tree、新分支名称、新提交 SHA、`git log` 摘要、`git diff --stat`、`git diff --name-status`、测试结果、migration 变化、配置变化、是否触碰数据/凭据、回滚方式和未验证项。

### 主站推广流程

收到验收站分支后，主站维护 AI 必须先执行并记录：

```bash
git fetch origin
git log --oneline <branch> ^origin/main
git diff --stat origin/main...<branch>
git diff --name-status origin/main...<branch>
```

只有确认来源清晰、没有运行数据和凭据、范围明确且测试通过后，才能合并到 `main`。合并后必须在根目录干净的 `main` 上完成必要验证，推送 `origin/main`，并且只从该 `main` 发布验收站或主站；必须记录 `source_commit`、`source_tree`、`image_digest`。发布失败时保留候选分支、失败证据和修复内容。

### 发布授权与停机门禁

“代码写好了”“验收站通过了”“继续”“部署一下”“上线”“推一下”均不构成主站发布授权。主站发布只允许以下明确授权：

1. `测试站验收通过，部署主站`
2. `快速部署到主站`
3. `快速部署主站，不同步验收站`

使用第 3 种授权时，必须记录用户明确授权不同步、主站 commit/tree、验收站当前 commit/tree、两边可能存在版本差异，并不得声称两边一致。发布预检返回 `downtime_required=true` 时，必须停止，不得自行停服、迁移、重启或切换。

### 版本、差异与异常处理

每次环境发布必须记录环境名称、`source_commit`、`source_tree`、`image_digest`、发布时间、发布结果、健康检查结果，以及是否与另一个环境一致。判断“该推哪些”只能依据 Git 提交历史、Git diff、migration 文件、明确批准的配置差异和发布记录；不得依据服务器文件时间、容器内人工修改、数据库当前内容、日志业务结果、镜像标签或“看起来像改过”的文件。

遇到以下任一情况，必须停止推广并报告：验收站未提交代码修改、来源不明、分支非最新 main 派生、main 漂移、代码混入凭据或生产数据、migration 未说明、配置变化未说明、测试失败、发布授权不明确，或需要停机但没有明确授权。

### 任务最终交付格式

每次任务结束必须明确写出：本次改动；可推广的 commit；明确不可推广的内容；主站当前 commit/tree；验收站当前 commit/tree；两边是否一致；测试结果；发布结果；回滚方案；未解决问题。
