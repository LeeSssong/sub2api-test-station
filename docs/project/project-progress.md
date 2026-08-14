# 项目全局进度总账

**T05 用量页移除外部控制面状态重新启动（2026-08-14）：** 状态：进行中（`DESIGNING`，等待用户澄清边界）。账号监控卡片已完成合并、推送、无停机蓝绿部署、登录态线上验收和候选归档，串行门禁已解除。唯一发布总控以最新干净 `main@6b606c731faed504f9368730a910489d08ee8ab3` 为基线创建新的用户可见顶层任务和独立 worktree；T05 已完成生产只读审计并通过 9 文件/89 项基线测试，确认用量页仍显示外部控制面状态且请求 `/api/v1/xingqiao/externalization/pages/accounting` 返回 401。当前唯一待用户确认的问题是是否只从 `UsageView.vue` 移除状态条、accounting decision/ledger 调用及覆盖逻辑，而保留共享 `controlPlane.ts`、`ReadModelStatus.vue`、外部化配置与测试给后续 T06；未经确认不得 writing-plans、实施、合并或部署。

**账号监控卡片原生账号复用生产收口（2026-08-14）：** 状态：已完成。候选 `codex/account-monitor-card@62eb1029f08cb6530d2e2b9fcdc2d8c197681a75` 经根授权无冲突合并为 `main@39520d2cfdc47760fad57613616aabe5c045afa0`，并已推送到 `origin/main`。合并后专项验收：账号监控相关 71 项 Vitest、原生账号操作回归 19 项、`pnpm typecheck`、`pnpm build`、`git diff --check` 均通过；无后端/API、数据库迁移、配置或 GitHub Actions 变化，`downtime_required=false`。预加载蓝绿发布记录为 `/var/lib/sub2api/release-records/20260814T102354Z-production-3093579.json`，结果 `succeeded/promoted`、`rolled_back=false`，活动槽 `blue`，运行镜像绑定 source commit `39520d2cfdc47760fad57613616aabe5c045afa0`、source tree `3b88529643b135d9bd39a1c343deb1a0e7615e3d`；公网 `/healthz`、`/readyz`、`/health` 均返回 200，API/worker/PostgreSQL/Redis/Caddy 健康。登录态账号监控页面已验证账号信息、原生更多菜单入口正常；账号信息弹窗不展示凭据原文、token 或 `error_message`；页面请求未出现 `/api/v1/xingqiao/*` 或其他控制面 API 调用。当前无发布候选；下一步仅可从最新干净 main 重建 T05 用户可见顶层任务。

**多指挥窗口与并发任务治理收敛（2026-08-14）：** 状态：进行中（治理收敛和根目录清理已完成，账号卡片进入唯一候选恢复阶段）。唯一发布总控已由根任务 `/Users/gongtengxinwen/Documents/sub2api搭建` 正式接管；治理提交 `2396df5a5` 已写入唯一总控、全局文件权限、最多两个独立准备 worktree、单 worktree 单写入者、reviewer 只读、统一状态机和单车道发布门禁。“调度优化-指挥”“调度优化（2）”已完成只读交接、停止全部代理并改名归档；S1 顶层任务已确认 `FROZEN_FOR_REBASE`，冻结 `69a93343c`、五份未提交文档、Task 5 的 2 个 Important/1 个 Minor、迁移 `220` 碰撞和测试/报告证据；T05 已确认 `FROZEN`，旧 detached `a71c675b1` 只作启动审计。根目录 `AccountMonitorCard.spec.ts` 草稿中的有效入口测试已确认存在于账号卡片提交 `c83006cd9`，候选 worktree 对应测试文件完整保留；根目录唯一额外差异只是错误缩进，已用精确补丁清除，没有 reset、checkout、覆盖或数据丢失。当前根 `main` 工作树干净，下一步恢复 `codex/account-monitor-card@72e063bca` 作为唯一发布候选，完成现有 `AccountMonitorCard.vue` 修复、复审、刷新、合并、部署和线上验收；旧维护/官方升级/韧性规格 worktree 继续只作证据，不得抢占发布位。

**T05 用量页移除外部控制面状态（2026-08-14）：** 状态：进行中。已确认 T03-R1 已完成生产验收，当前从干净 `main@263a2de748269b3c96057f500eda5426fe1c013e` 创建用户可见独立顶层任务；范围仅限原生管理员用量页移除外部控制面状态/banner/调用，保留原生流水、错误请求和管理员详情。T06–T09 继续排队，未启动。

**T03-R1 维护发布迁移门禁补全（2026-08-14）：** 状态：已完成。最终 `main@210d0397e647b91be080f0c7252da39a6e61d71d` 已推送；`0600` 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-14-main-210d0397e647b91be080f0c7252da39a6e61d71d-t03-r1-maintenance-v1.json` 绑定 tree `4e2b7be29191894a8e7fac7e7af21cb0cf4adb21`、迁移哈希 `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`。用户已明确授权停机，宿主维护记录 `/var/lib/sub2api/release-records/20260814T051143Z-production-2876774.json` 返回 `succeeded/promoted`，活动槽 `green`，生产 `release-state` 与源 SHA/tree/迁移哈希一致；公网 `/healthz`、`/readyz`、`/health` 均 `200`，API、worker、PostgreSQL、Redis、Caddy 健康。功能启用时间为 `2026-08-14 13:17:21.831859+08`；启用后首个稳定窗口的 15 笔自然流水中，账号 `23` 的 3 笔 NewAPI 流水无需管理员打开详情即自动登记 `confirmed`，账号 `183` 的 12 笔因无明确持久化 Sub/New 身份保持 `evidence_not_registered`，未猜测来源。今日财务与异常、本地详情、未认证隔离均在线验证通过，完整证据见 `docs/superpowers/reports/2026-08-14-t03-r1-account-financial-reconciliation-production.md`。已合并旧候选的可恢复归档为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-14-t03-r1-maintenance-transition-cd22be537.bundle`（0600，`git bundle verify` 通过），旧 `t03-r1-maintenance-transition` worktree/分支已删除；用户可见维护任务 `019ffe60-c370-7290-a310-0f811e8d09ae` 仍保留并因根 `main` 漂移停止在 `BLOCKED_FOR_ROOT_RECONCILIATION`，未将其宣称为合规交付。技术变更以已审根主线和生产证据为准。

**T03-R1 维护发布迁移门禁补全（2026-08-14，实施更新）：** 状态：进行中（本地实现和定向验证已完成，待独立复审、全分支审查、根任务授权合并、推送、维护发布及线上验收）。宿主执行器现仅新增并允许 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc` 到 `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71` 的第七组完整哈希转换；TDD 已记录改动前 `migration_set_changed`，改动后同源的变异/未知目标仍在任何生产变更前 fail-closed。未连接生产、未推送、未部署。

**本轮登记（2026-08-13）：** 账号监控卡片复用原生账号管理的账号级信息与操作，状态：进行中（需求已受理，受 T03-R1 串行发布门禁阻塞，尚未实施）。用户要求不改动账号管理页，只在账号监控的每张卡片增加低干扰的账号信息入口，并复用原生“编辑 / 删除 / 更多”及其弹窗、确认和操作逻辑；仅纳入账号级能力，不搬迁列设置、批量操作、导入导出、自动刷新等全局能力。本轮按 AGENTS.md 将“优化账号卡片”视为明确保护方向；当前只做现状核对与需求排队，不修改账号管理或账号监控运行时代码。

**本轮登记（2026-08-13）：** 监控卡片 TTFT/总延迟 P95 数据异常排查与最快投产修复方案，状态：已完成。生产只读 SQL 已证明截图中的 GPT-Pro `121.08s/131.11s`、GPT-Plus `88.85s/447.49s`、GPT-特惠分组 `18.09s/96.58s` 分别等于过去 7 天“小时 P95 的最大值”，根因是监控卡片使用 Ops `auto` 预聚合路径并跨小时对 P95/P99 取 `MAX`。热修仅将监控卡片切换到已有 raw 精确分位数路径，不改通用 Ops 大盘预聚合、不迁移、不回填。生产运行提交 `fc75b62d503a730f7562c058b3fcd9dc991c4fcb` 已通过预加载蓝绿链无停机部署，宿主记录 `/var/lib/sub2api/release-records/20260813T182624Z-production-2417881.json`，`downtime_required=false`，活动槽 blue、容器 healthy、重启 0；公网 `/healthz`、`/readyz`、`/health` 均为 200。上线后 7 天原始精确值为 GPT-Pro TTFT/总延迟 `20.60s/38.63s`、GPT-Plus `19.07s/40.26s`、GPT-特惠分组 `10.79s/19.31s`。生产镜像明确不包含随后并发合入 `main` 的 T03-R1，T03-R1 继续独立推进。

**监控 P95 紧急热修（2026-08-13）：** 状态：已完成。用户明确授权在不暂停 T03-R1 的前提下插队实施并立即上线；候选 `3957ca4b7` 经任务审查和最终全分支审查均 `APPROVE`，合并后发布树为 `fc75b62d5`。RED→GREEN 合同测试、聚焦服务测试、完整 `internal/service`、MonitorV2 race 测试和 `git diff --check` 均通过；证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-13-main-fc75b62d5-monitor-p95-v1.json` 绑定 tree `309ba66ae5684a133d1a3feea8880d81f9ecfaa3`，迁移哈希不变。该树已推送并完成 `downtime_required=false` 蓝绿部署及线上健康/数据验收；未使用 GitHub Actions。并发操作期间发现 T03-R1 暂存内容短暂进入根目录索引，发布门禁在构建前阻止脏树；其内容已保存在恢复分支 `codex/t03-r1-recovery-dde8d61a1`，`main` 恢复为纯热修树后才发布，故生产未混入 T03-R1。

**T03-R1 Task 5（2026-08-13，deleted trace fix round 3）：** 状态：准备完成（本地实现、定向验证与独立 scoped re-review 均通过；未合并、未推送、未部署、未线上验证）。当前工作区 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`。提交 `c0f471588bd860e42cff3c711a65a9c118c2a087` 严格限定为修复已软删除账号的历史 exception 仍可列出、但本地 evidence/detail 被默认软删除查询拦截的不一致：`GetUsageEvidence` 使用 `SkipSoftDelete` 读取 immutable account name/type，保留既有 usage/evidence/provider/request trace 语义；新增该详情回归测试。独立复审报告 `task-5-contract-rereview-soft-delete.md` 为 Spec APPROVE、Quality APPROVE、open findings 0。未修改 schema、migration、frontend、main、网络/上游、stash、推送、部署或生产。

**T03-R1 Task 6（2026-08-13）：** 状态：进行中。当前工作区 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，基线 `2944a36d11fec648930ac0fef8321a44a66cd377`。Task 5 scoped re-review 已通过；本任务严格限定为把既有管理员账号盈利入口升级为本地财务首页：消费 Task 5 API，展示六项全站事实、统一 `generated_at`、60 秒自动/手动刷新、今日可编辑营收/成本/OAuth 成本、其他范围只读，以及携带范围/账号的异常流水跳转。不得修改后端、schema、migration、Task 7+、main、其他 worktree、生产或 GitHub Actions。

**T03-R1 Task 5（2026-08-13）：** 状态：准备完成（fix round 3 已验证，等待独立复审；未合并、未推送、未部署、未线上验证）。当前工作区 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，范围为管理员财务/本地证据 API（handler、admin routes、wire、测试），不改 schema、usage_logs、frontend、生产；fix round 2 严格只修复 correlation fallback 与 `RequestLogger` 的有效 UTF-8/64 字节一致性，以及五个财务 mutation handler 经真实 service 到 audit recorder 的关联 ID 覆盖。复现发现原测试将带参数 handler 注册为具体静态路径，导致 `one/oauth/override` 在参数校验处提前返回；fix round 3 已将测试拆分为 Gin route template 与 request path，完整 admin handler、财务审计 service、RequestLogger 聚焦测试及 `git diff --check` 均通过。

**本轮 T03-R1 Task 4 fix round 3（2026-08-13）：** 状态：准备完成（等待独立复审；未合并、未推送、未部署、未线上验证）。当前工作区为 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，基线 `957d4c94d42e3dbb1b0341a0a046af1c55eb95ab`。范围严格限定为修复 `task-4-rereview-r2.md` 指出的 `ReviewSelected` 后续 `validateMoney` 失败路径：已通过 `CreateReview` 提交的前序 rows 必须逐行 audit，并单独记录失败行；仅修改 service implementation/test 与本轮报告，禁止触碰 Task 5/API/UI/schema、合并、推送、部署或生产。实现与专项验证见 `task-4-fix3-report.md`。

**本轮 T03-R1 Task 4 fix round 1 接管（2026-08-13）：** 状态：进行中。当前工作区为 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，接管基线 `5a8d830b2de8063e2b99876e13e044b0e1930cdb`。原 implementer 留下的未提交 Task 4 修复、`account_financial_repo_integration_test.go` 与 opt-in `SUB2API_TEST_POSTGRES_TMPFS` harness 支持必须原样保留、审查并在其上完成；禁止 reset、丢弃或重做。范围仅关闭独立复审列出的 3 个 Critical 和 5 个 Important：canonical activation、post-enable 非 OAuth pending-only review eligibility、所有 mutation 的真实 old/new 审计、北京 00:00、OAuth 今日/7/31 语义、异常列表过滤/持久化原因、override old/new，以及过滤核对的原子 freeze/recheck/count/无部分写入。不得修改 schema/migration/generated Ent/`usage_logs`/Task 5+，不得上游 HTTP、合并、推送、部署或触碰生产。

Fix round 1 实现提交 `962e468c7858bb753fb1a47ebdbeae45891211fa`，SDD ledger 记录提交 `e0cb75a1c61cc671542bdd954636e3a73e0fc251`。原始 GREEN、focused 新测试、compile、`go vet`、`git diff --check` 均通过；fresh PostgreSQL integration 已按仓库 harness 尝试，但在 TestMain 启动阶段因 `panic: rootless Docker not found` 阻断，未进入迁移或测试。等待独立 fix re-review，仍未合并、推送、部署或线上验证。

**本轮 T03-R1 Task 3 fix round 2 接管（2026-08-13）：** 状态：进行中。当前工作区为 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，接管基线 `f41f4682c575231f72a82ec3c98fa44a7a12b661`。接管时共享工作区已有 `internal/service/{sub_upstream_cost.go,usage_cost_evidence.go,usage_cost_evidence_test.go}` 的未提交 eligibility 修复，必须原样保留、审查并在其上完成，禁止 reset 或丢弃。fix round 2 仅关闭 scoped re-review 的两个 Important：以持久化正证据识别 Sub/New ledger（probe `ok` 或余额 source `sub2api` 为 Sub，余额 source `newapi` 为 New；unknown、unsupported-only、direct official provider 均零 ledger HTTP/零证据），并新增真正经过代表性成功非流式与成功流式 handler response 分支、最终观察 Gateway/OpenAI `RecordUsage` 调用的回归测试。完成后运行 Task 3 原 GREEN 矩阵、focused handler 测试、server 编译和 diff check，自审并提交单一 fix round 2 commit；不得改 schema/migration/`usage_logs`、Task 4+、`main`、生产，不推送或部署。

**本轮 T03-R1 Task 2 Ent 唯一约束生成一致性修复（2026-08-13）：** 状态：进行中（本地根因修复与验证完成，待根任务后续审查/合并/部署/线上验证）。目标 worktree 为 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，修复基线 `bdf278ce653bd642b9b6bb0922db8b179e6fce4d`，修复已收敛为本任务单一提交。根因是关联字段级 `.Unique()` 没有作为独立 unique index 进入 Ent 迁移表元数据；已按仓库 Ent 约定在 `UsageUpstreamCostEvidence` 与 `UsageCostReview` 的 `Indexes()` 增加显式唯一 `usage_log_id` 索引，并运行 `make generate`，生成的 `ent/migrate/schema.go` 现包含 `usage_upstream_cost_evidence_usage_log_id_key` 与 `usage_cost_reviews_usage_log_id_key`（`Unique: true`），与手写 SQL 的 `BIGINT NOT NULL UNIQUE` 默认 PostgreSQL 约束名和既有查询索引一致。新增 schema 加载回归先真实 RED、修复后 GREEN；Task 2 migration 测试、`go test ./ent/schema -count=1`、生成幂等和 `git diff --check` 通过。`TestMigrationsSchema` 的真实 PostgreSQL 集成用例在 Colima 默认数据卷因 `No space left on device` 无法启动；未清理或影响其他容器，改用一次性 tmpfs 挂载与仓库支持的 `postgres:15-alpine` 后同一用例通过，临时 harness 修改已还原。提交后再次运行 `make generate`、Task 2 定向测试、`go test ./ent/schema -count=1`、`git diff --check`，均通过。`UsageLog` 源文件哈希保持 `5adc345325675a4d439524f20603412f69971c3f024913240996df7eb7e461f9`，手写 SQL 未修改，无 Task 3+ 路径变化；不推送、不合并、不部署、不触碰其他 worktree。

**本轮 T03-R1 Task 1（2026-08-12）：** 状态：进行中。目标 worktree 为 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，起始 HEAD `b73b4aa7dc9f40eb1be2a83561d591380f21f8ef`。严格 TDD 已完成：先创建 `upstream/sub2api/backend/migrations/t03_r1_legacy_usage_log_fields_absent_test.go`，指定 RED 命令 `go test ./migrations -run TestT03R1LegacyUsageLogFieldsAreAbsent -count=1` 真实失败，证明遗留字段/迁移仍存在；随后按逆依赖顺序显式 revert `ce5691527a54cb2e7f8b3dabf624eb65e93fc177`，生成 `26da1cd72`，再 revert `1c3e8768458c7c46725725e9f828fbcaba403f16`，生成 `48770ff65`，无冲突。GREEN 命令 `go test ./migrations -run TestT03R1LegacyUsageLogFieldsAreAbsent -count=1`、`go test ./internal/repository -run 'TestUsageLog.*(Insert|Detail|RequestType|SessionID)|TestMigrationsSchema' -count=1` 和 `git diff --check` 均通过；确认无 direct `usage_logs` 成本字段、无 `221_usage_log_upstream_cost_persistence.sql` 残留，旧规格/旧计划保留。待提交本 Task 测试与总账更新；不推送、不合并、不部署、不触碰其他 worktree。

**本轮 T03-R1 计划阶段更新（2026-08-12）：** 状态：进行中（现行书面规格已批准，旧候选终审为 **REJECT**，正式实施计划已起草并等待计划审查；尚未开始实现、合并、推送或部署）。当前工作区为 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`，登记时 HEAD `856016a0be040d4141d356cec1028a055d5a30bb`。实施前已盘点注册非 `main` worktree：`codex/official-v0175-fast-merge` 干净且相对 `main` 领先 1 个提交，须先按其独立流程审查整合；`codex/upstream-resilience-hardening` 有 6 个未提交规格/队列/总账修改，作为已登记保护例外原样保留；本工作区干净。待审计划为 `docs/superpowers/plans/2026-08-12-t03-r1-account-financial-reconciliation.md`：首任务以显式 `git revert`/等价可审查提交移除 `1c3e87684` 与 `ce5691527` 的冻结实现，同时保留规格历史；后续使用独立 Ent evidence/review/账号日值/启用边界表，响应完成后一次性 Sub/New 登记，无历史回填、延迟补查或上游重试；管理员只读本地证据、异常 Tab、批量核对、财务首页和 60 秒刷新，普通用户完全隔离。发布只能在根任务授权合并后的 `main` 进行；迁移预检必须 expand-only 且真实输出 `downtime_required=false`，否则在任何迁移、停服、重启或蓝绿切换前停止。T05 继续暂停。

**本轮 T03-R1 账号财务、逐笔上游成本与异常核对 MVP（2026-08-12）：** 状态：进行中（现行正式规格已由总统筹依据用户授权完成最终书面审查并批准，进入新实施计划阶段）。任务从干净 `main@747c7fb14d1ded243794a77984778babece7c799` 创建用户可见独立顶层任务、独立 worktree 与分支 `codex/t03-r1-upstream-cost-persistence`，并已回合并官方 `v0.1.175` 生产验证后的 `main@72b4dfcdba47075df2795465957a9225f7e8594c`。生产最近 500 条自然流水精确 `upstream_request_id` 覆盖 500/500（Sub 244、New 256），但本地持久化上游成本/利润为 0/500，根因是管理员详情仍即时查询上游。旧规格/计划及直接扩展 `usage_logs` 的候选 `1c3e87684`、`ce5691527` 已冻结，终审再次确认其不符合现行规格，必须以显式 revert 从候选树移除。新批准方向：官方 `usage_logs` 保持不变；独立一对一证据模型登记功能启用后的 Sub/New 原生账单；正常非零证据立即纳入人民币营收/支出/利润，零成本或不可用证据进入使用记录的异常 Tab，人工核对后才纳入；字面 `oauth` 类型不查询上游，由管理员填写北京自然日人民币成本；现有账号盈利页升级为管理员财务首页，显示全站与账号营收、本站支出、利润、利润率、异常和未删除用户余额快照，查询时实时汇总并每 60 秒刷新；今日可直接覆盖营收/成本且以截止点保护后续增量；无历史回填、无延迟补查、无上游重试、无估算、无汇率或采购分摊。新有效规格为 `docs/superpowers/specs/2026-08-12-t03-r1-account-financial-reconciliation-design.md`；旧规格和旧计划只作历史追溯。T05 继续暂停。

**本轮官方 `v0.1.175` Task 3 资格、合并与生产发布（2026-08-12）：** 状态：已完成。工作区为根目录 `/Users/gongtengxinwen/Documents/sub2api搭建`；官方升级运行树为 `main@350e050575377d8e31ed153624bb19da3591517f`，最终全分支审查 `011bf2b0c48f775e624c82bff3bceddf8e12c69a` 为 `APPROVE`。并版后 `internal/handler`/`internal/service` Go 测试与 vet、UsageView/UsageTable 39/39 Vitest、前端 typecheck/build、版本/官方来源和 `git diff --check` 均通过；`0600` 资格证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-12-main-350e050575377d8e31ed153624bb19da3591517f-v1.json` 绑定 tree `47e98861f921bebb6d62e41e8a44c142d4d7fe4f`，官方合并无 migration 路径变化，迁移哈希保持 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`。Task 3 的首次 controller 调用因未加载 operator-controlled profile 在生产变更前安全停止；后续获授权发布链加载既有 profile 后完成预加载蓝绿发布，宿主记录 `/var/lib/sub2api/release-records/20260812T212526Z-production-1519752.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`。活动槽为 green，API/worker 使用 `release-350e0505…-2f73be08…` 镜像且 healthy；旧 blue 保留可回退，PostgreSQL、Redis、Caddy 身份未变化。公网 `/healthz`、`/readyz`、`/health` 均为 200，管理员版本接口为 `0.1.175`，Usage stats/list 为 200 且有数据。生产验收与正式报告均已推送至 `origin/main`；T03-R1 与 `upstream-resilience-hardening` 保护 worktree均未修改。正式报告为 `docs/superpowers/reports/2026-08-13-official-v0175-production.md`，T03-R1 的新生产代码基线为 `350e050575377d8e31ed153624bb19da3591517f`。

**官方 `v0.1.175` 修复轮 3（2026-08-12）：** 状态：进行中。第三次范围化独立复审已 `REJECT`；本地修复与规定门禁已完成，待新一轮范围化独立复审。起点精确为 `ce4e2d320701dfa9fde58b024a005b208d22a766`；真实 RED 证明 Responses/Messages × 非池/池状态覆盖排除认证 × 401/403 共八种安全请求均错误停在 `[9910]`，未到健康备用账号。提交 `d3fc44fa88359ab110897908e7a987e608df54c0` 将“语义可重放”与“同账号池重试资格”分离，提交 `731dedc641c6c2100848813212cb1d760994ed2b` 显式区分端点协议夹具；Responses 测试仅为稳定真实路径使用 `force_responses`，Messages 保留原 `openai_passthrough` 语义，未改变生产账号 `Extra` 行为。最终聚焦矩阵通过：安全非池/覆盖 `[9910,9911]`、配置化池 `[9910,9910,9911]`、tools/function output/tool result `[9910]`、输出后 `[9930]` 及既有 502/transient；完整 `internal/handler`、`go vet ./internal/handler`、gofmt 与 `git diff --check` 均通过。运行日期由根任务统一为 2026-08-12；规格中既有 2026-08-13 历史批准文字与目录名保持不变。未改迁移或配置，未改 `main`、推送、部署、生产、秘密、T03-R1、其他 worktree 或 GitHub Actions；`downtime_required` 留待根任务合并后预检判定，未满足推送部署与线上验证前继续保持“进行中”。

**官方 `v0.1.175` 修复轮 2（2026-08-12）：** 状态：进行中，修复轮 2 已完成本地聚焦验证，待第三次范围化独立复审。第二次独立审查已 `REJECT`，唯一代码范围是 `openAIRequestHasSideEffects` 未识别原始 Responses `input[].type=function_call_output` 之外的 Anthropic Messages `messages[].content[].type=tool_result`；旧候选的 Messages tool-result 401/403 RED 实测为 `[9910, 9910, 9911]`，证明同账号与跨账号均会错误重放。回归提交 `e4cb5363ed7d84451729e3909027c89520dac701` 与最小 helper 修复 `ada3a69dfb22bcdff3922549042afffbed6fba1a` 后，聚焦 GREEN 证明 Responses function-call-output 与 Messages tool-result 在 401/403 下均只调用 `[9910]`，既有安全请求 `[9910, 9910, 9911]` 与顶层 tools `[9910]` 保持通过。本次文档收口不新增或虚报全套测试结果；候选尚未第三次审查、合并 `main`、推送、部署或线上验证。候选 worktree 为 `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/official-v0175-fast-merge`，分支 `codex/official-v0175-fast-merge`，起点 `6690b282878e026ad45b34c74d057e36045f53a9`；不得改 `main`、推送、部署、T03 或调度优化 worktree。最终候选 SHA 在本次文档提交成功后产生，并由提交后的 `git rev-parse HEAD` 精确绑定。

**本轮官方 `v0.1.175` 快速升级实施（2026-08-12）：** 状态：进行中（Task 2 首次独立审查已 `REJECT`，三个 Important 阻断分别为 Responses 带 tools/function 风险请求仍可跨账号重放、Messages 缺少对称安全切换/禁止重放证明、健康备用账号使用了官方 `v0.1.175` 语义下不能代表成功的空 `response.completed` 夹具。修复轮次已严格完成 RED/GREEN：恢复未修生产代码后，Responses/Messages 的 401/403 tools 用例均真实失败为 `[主账号, 备用账号]`；官方 unit 语义证明空 completed 必须 502 failover、带真实输出才成功。最小修复后，仅无输出且无副作用的配置化池认证失败按 `[主账号, 主账号, 备用账号]` 重试切换，tools/function 请求保持 `[主账号]`；健康 JSON/SSE 夹具均包含真实 `output_text=ok`。新鲜通过两端点 401/403 聚焦回归、既有输出后禁止重放、完整 `internal/handler`、gofmt 与 `git diff --check`；新 HEAD 待范围化独立复审，尚未合并 `main`、推送、部署或线上验证）。用户已批准 `docs/superpowers/specs/2026-08-12-official-v0175-fast-merge-design.md`，并授权立即按“提取定制行为 -> 升级官方源码 -> 语义回接”实施。候选 worktree 为 `.worktrees/official-v0175-fast-merge`，分支 `codex/official-v0175-fast-merge`；其工作区起点为包含批准规格记录的干净 `main@acb8d1891a8c2b3be3de8c2f0288313c6af2644a`，官方源码三方合并锁定使用任务包指定父基线 `main@82e97418bd1480e03a869856e0cc872194839477`。9 个冲突文件已逐函数/组件完成语义合并，保留 Xingqiao 计费、利润门、尝试元数据、流恢复与管理员详情契约，并接入官方请求 ID、service tier 定价、池模式重试和流错误修复；官方 `v0.1.175` 的 114 个变更路径已全部覆盖，来源记录和服务版本已推进至 `0.1.175`。官方迁移目录相对 `v0.1.173` 无变化。未实施 T03-R1，未触碰生产、秘密、GitHub Actions、推送或部署；`downtime_required` 留待根任务在合并后发布预检中判定。Task 1 的发布元数据和精确 conflict stage 证据继续保存在权限 `0700` 的本地临时目录，T03-R1 与调度优化 worktree 均按本任务明确边界保持原样。

**本轮官方最新版本升级请求（2026-08-12）：** 状态：进行中（官方候选准备被冲突门禁阻塞，尚未合并、推送、部署或线上验证）。用户明确要求先基于当前生产版本更新官方版本，再让 T03-R1 在新基线上继续。已锁定官方稳定版 `v0.1.175`，发布于 `2026-08-12T11:07:29Z`，annotated tag object 为 `b898c60c422d1de059968c56aca22f6643f1fed4`，source commit 为 `93c32fa1a2450351561abc46156d2e28cb5f74ca`；Linux amd64/arm64 SHA-256 分别为 `690b99607b6e318bc667ccf3593cc7059ccc9f91cd27706e52e8bfb83004fd4e`、`cf0e643d6241038b94fd045d790c658513437cc31853e76dc44989ba4679650a`。在基线 `main@86abfde64` 执行 `ops/merge-sub2api-release.sh` 时，官方变更与当前 `0.1.173` 定制生产基线发生未覆盖内容冲突，脚本已 fail-closed 并自动恢复根目录干净；冲突范围包含 `backend/internal/handler/openai_gateway_handler.go`、`backend/internal/handler/openai_gateway_handler_test.go`、`backend/internal/service/account_stats_pricing.go`、`backend/internal/service/account_stats_pricing_test.go`、`backend/internal/service/openai_gateway_scheduling.go`、`backend/internal/service/openai_stream_read_error.go`、`frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`、`frontend/src/views/admin/UsageView.vue`、`frontend/src/views/admin/__tests__/UsageView.spec.ts`。根据全局约束，禁止猜测解决、自动部署或让 T03-R1 基于未合并候选继续；T03-R1 worktree `codex/t03-r1-upstream-cost-persistence@ce5691527` 保持原样，等待人工确认冲突处理方案。

**本轮生产耗时、TTFT 与分组账号优先级分析（2026-08-12）：** 状态：准备完成（生产只读分析，不属于需部署的完成态）。已完成北京时间 `2026-08-10 00:00` 至 `2026-08-12 21:41` 的 `usage_logs` / `ops_error_logs` / 账号与分组元数据只读联合采集，完成按天、分组、账号的耗时、TTFT、失败阶段与当前可调度状态分析，并输出账号名称、ID 和建议优先级。TTFT 仅对 `first_token_ms` 非空样本统计；自然业务请求与主动探测分开；不输出凭据、完整 API Key、请求体或原始上游敏感响应，未修改生产数据、账号状态、分组绑定、优先级、调度、配置、容器或发布状态。工作区为 `/Users/gongtengxinwen/.codex/worktrees/latency-analysis-20260812/sub2api搭建`，分支 `codex/latency-analysis-20260812`，基线 `main@747c7fb14`；已安全清理本轮只读查询临时文件。

**本轮 T03-R1 上游扣费缺失与异步持久化修复（2026-08-12）：** 状态：进行中（仅完成根任务只读事实核对和启动登记，尚未创建顶层任务或开始 brainstorming）。用户在 T04 完成后复验确认仍有大量流水没有上游扣费信息，并要求先修复该问题再进入 T05。根任务核对当前生产源码 `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`、T03 实现提交 `fe9d04c9ccd8e40faf1e58ff137004f57964392b` 及其规格/计划，确认现状只有管理员访问 `/api/v1/admin/usage/:id/upstream-cost` 时由 `SubUpstreamCostService.GetByUsageID` 同步读取本地流水、即时请求 Sub `/api/v1/usage/records` 或 New API `/api/log/token` 与 `/api/status`、精确匹配后临时返回上游扣费和利润；T03 只修改该服务、处理器测试和文档，没有新增数据库迁移、上游扣费持久化字段、后台任务、outbox、worker 或延迟重试链。因此此前约定的“异步持久化登记”实际未落地，T03 原先的完成结论只证明即时查询中 confirmed 样本有值，不能证明全部流水最终持久化。当前严格暂停 T05，下一步从最新干净 `main` 创建 T03-R1 用户可见独立顶层任务；该任务必须先只读量化生产缺失比例/原因和账单延迟，再完整执行 brainstorming、2–3 方案比较、逐节批准、正式规格书及用户明确批准，之后才能 writing-plans 和实施。禁止估算、模糊匹配、以 `account_cost` 冒充上游实际扣费、引入 external-primary 或无界历史回填。

**本轮 T04 账号监控移除外部控制面状态（2026-08-12）：** 状态：已完成。T04 从当时最新干净 `main@eaa73610238f1effc253dc84a98eae4965390525` 创建用户可见独立顶层任务 `019ff559-b4d6-7411-81c7-d0e631584d70`、独立 worktree 与分支 `codex/t04-account-monitor-native-only`，完整执行 brainstorming、方案比较、逐节批准、正式规格书及用户批准、实施计划、fresh implementer、独立任务复审和全分支终审。设计提交为 `5b58197e90063e92793793b1326770068843c3c2`，计划提交为 `e123fc472`，实现候选为 `8f14556dfc327ab56d7004fbe33c0fb8cfb965bd`，最终候选为 `63019434684a53b7b856a6acea5605e3e8b4aede`；根任务按精确目标 `main@e8f93a57a1e59aedff694bf3591bdf3dcbd4a03a` 授权后，保留双方总账记录并合并为 `main@be9e124d65c7457477fbe6d3435a9468b1ec1f4c`。合并后新鲜专项测试 28/28、`pnpm typecheck`、`pnpm build` 和 `git diff --check` 通过，证据 `/private/tmp/t04-account-monitor-release.BudXcd/test-evidence.json` 以 `0600` 权限绑定 tested tree `d3feb28c1dca0fb12b405ea3ddc1483599ea0cfe` 与迁移哈希 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`；`main` 已推送到服务端。生产使用既有预加载宿主蓝绿链无停机发布，记录 `/var/lib/sub2api/release-records/20260812T124618Z-production-1146130.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `blue`，API/worker 镜像均为 `ghcr.io/leesssong/xingqiao-sub2api:release-be9e124d65c7457477fbe6d3435a9468b1ec1f4c-04ebb3a9a7b228f7c2006f80e4f14b6bc9e08c02a0b40d1e5c3f4b3849cdd9f0` 且健康、重启次数为 0；PostgreSQL、Redis、Caddy 容器身份保持不变，公网 `/healthz`、`/readyz`、`/health` 均为 200。登录态 Chrome 验收确认页面标题“账号监控 - 星桥AI”、显示全站 78 个账号及原卡片，初载/reload 请求原生 `/api/v1/admin/accounts/monitor?range=24h`，点击“7 天”后仅请求 `range=7d` 并显示“7 天调用”；全程 `/xingqiao/**` 请求为 0，页面无“控制面暂时不可用”“完整性：”或替代常驻状态条。共享控制面文件未改动，无迁移、无配置变化，未使用 GitHub Actions。生产验证报告见 `docs/superpowers/reports/2026-08-12-t04-account-monitor-native-only-production.md`。候选已归档为 `/Users/gongtengxinwen/Documents/sub2api-archives/t04-account-monitor-native-only-630194346.bundle`，权限 `0600`，`git bundle verify` 通过，SHA-256 为 `ce56257a27e10405bf4109b7198d2f32a81570f6d73eb14c03b4dede5ccebf76`；确认候选已是 `main` 祖先且工作区干净后，已删除 T04 worktree 和本地分支，所有既有 stash 保留。用户随后调整队列，下一任务改为 T03-R1，不启动 T05。

**本轮小步发布流程纠偏与暂停收尾（2026-08-12）：** 状态：准备完成（纯治理文档，不属于应用部署完成态）。已确认 T01、T02 的独立 worktree、规格书、计划、复审、部署和线上验证证据继续有效，技术成果不重做、不回滚；但两包没有按用户原始要求创建“每个任务包一个用户可见的独立顶层 Codex 任务”，内部 `spawn_agent` 不能替代顶层任务隔离，因此不得宣称 T01/T02 顶层任务流程合规。T03 也是纠偏指令到达前已在途并由根任务内部代理实施、合并和发布的任务，不倒称满足新增门禁。现已把“一个任务包一个用户可见顶层任务、一个独立 worktree、完整 brainstorming、书面规格书及用户明确批准后才能 writing-plans、独立任务复审与全分支终审、`READY_FOR_ROOT_REVIEW`、根任务 `AUTHORIZE_MERGE_TO_MAIN`、一次部署和一次线上验收”的硬门禁写入全局约束和任务队列。纠偏时的暂停门禁已履行；T03 后续以组合证据完成线上验收并进入归档清理，下一步只允许创建 T04 的用户可见独立顶层任务，不得由根任务内部代理直接实施 T04，也不得并行启动 T05。

**本轮 T03 上游扣费与利润始终有值（2026-08-12）：** 状态：已完成。候选 `codex/t03-upstream-cost-profit-values@72c3645d752a5919c0020d9c8f26e6e7bfc63a9d` 基于 `main@0b6ef4ef0faec1b84de6f7133b5c99c4ae6e405d` 完成规格、TDD 实现、定向回归、`go vet`、server build、前端 confirmed-zero 契约验证与独立复审，审查结论 P0-P2 无发现；实现提交为 `fe9d04c9ccd8e40faf1e58ff137004f57964392b`，合并提交为 `main@0432b87491a313b006643212cccdcd8d49001ae4`，已推送 `origin/main`。生产无停机发布记录 `/var/lib/sub2api/release-records/20260812T084707Z-production-974305.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `green`，镜像为 `ghcr.io/leesssong/xingqiao-sub2api:release-0432b87491a313b006643212cccdcd8d49001ae4-51ca035481d9b5df24a16758660c0445bb69d346130673204bfeef3f40399ff0`，迁移哈希保持 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`；公网与全部基础服务健康。首次最近 80 条自然流水中 14 条 `confirmed` 均返回数值；收口时最新 60 条中 3 条 `confirmed` 均返回数值，样例 `usage_id=87320` 的上游实际扣费为 `0.05`、利润为 `0.0706`。其余不可用原因为 `endpoint_unsupported=34`、`record_not_found=19`、`response_unavailable=4`，均保持不可用而不伪造为 0。生产未出现 confirmed-zero 自然样本，因此按已批准规格采用“同一发布树空白值服务/Handler/前端合同测试 + 生产部署身份、明确收费路径、失败边界和健康实证”的组合证据完成验收；新鲜后端聚焦测试、`go vet`、server build、前端 33 项测试、typecheck 与 `git diff --check` 全部通过，且部署提交之后无运行时代码漂移。候选恢复归档 `/Users/gongtengxinwen/Documents/sub2api-archives/t03-upstream-cost-profit-values-72c3645d7.bundle` 已通过 `git bundle verify`，权限 `0600`，SHA-256 为 `4021d92ee877e78dd9c781bff0ee0a9bcf120cd9fb3f475f525cbe9f2d6e0e2a`；确认候选工作区干净、候选提交已是 `main` 祖先且归档可恢复后，已删除 `.worktrees/t03-upstream-cost-profit-values` 和本地分支 `codex/t03-upstream-cost-profit-values`。验证报告见 `docs/superpowers/reports/2026-08-12-t03-upstream-cost-profit-values.md`。

**本轮“合并工作区并完成发布准备”线程流程合规核查（2026-08-12）：** 状态：准备完成（只读核查）。核对结果是 T01/T02 具备独立 worktree、规格书、计划、实现与复审证据，但没有用户可见的独立顶层 Codex 任务窗口；原会话使用内部 `spawn_agent`，不能视为顶层任务隔离。该偏差已写入全局约束、任务队列和本总账，不推翻已验证技术成果。本项未实施新功能、未回滚、未清理候选，当前暂停点为创建下一个用户可见顶层任务之前。

**本轮生图分组未选账号失败归因续查（2026-08-12）：** 状态：进行中。针对近 14 天“生图”分组 `account_id IS NULL` 的 490 条失败日志，按唯一请求、错误阶段/类型/状态码、用户、API Key、路径和时间分布进行只读归因，并通过相同 `request_id/client_request_id` 关联用量、错误尝试和调度证据，确认是否能可靠追溯到具体账号；不得仅凭错误文案猜测账号，不输出凭据、完整 API Key、敏感请求体或原始上游响应，不修改生产数据、账号、分组、调度或服务。登记时根目录为 `main@74f139243`；当时唯一非 `main` worktree `.worktrees/t02-native-error-diagnostics` 保持干净且领先 5 个提交，作为在途候选不合并、不修改、不清理。该记录从 `stash@{0}` 安全保全回总账，后续继续前须遵守新的用户可见顶层任务门禁。

**本轮生图分组近 14 天账号日志表现分析（2026-08-12）：** 状态：准备完成（只读分析，不属于需部署的完成态）。范围仅限生产只读日志/用量流水与账号、分组元数据；查询窗口为 `2026-07-29 13:10` 至 `2026-08-12 13:10`（Asia/Shanghai），当前“生图”分组 ID `19` 有 9 个账号，其中 1 个可调度、8 个不可调度。共核对 1,053 条带账号成功流水、68 条带账号失败日志与 490 条未选中账号的分组级失败日志；账号比较只纳入可归属账号的自然请求，并按 request ID 去重合并成功/失败。结论：`生图-SHUAI(#135)` 样本最大（592 请求）、全口径成功率 94.76%、最近 48 小时成功率 97.96%，且 2K P50/P95 为 49.13/85.11 秒，是当前唯一可调度且综合证据最强的主力；`生图-CX(#162)` 成功率 98.33% 但仅 60 请求/2 个活跃日且当前不可调度；`生图-XN(#156)` 延时较低（2K P50/P95 51.44/67.94 秒）但成功率 92.86% 且余额不足冷却后不可调度；其余账号因成功率、延时、样本或可调度状态不足不列为优选。主动探测 4,890 条、成功 0 条，主要为 404/503，证明现有探测端点不适用于生图质量判断，未用于自然请求排名。不发起上游探测，未修改账号、分组、优先级、调度、计费、生产数据库、配置、容器或发布状态。该记录从 `stash@{0}` 安全保全回总账；后续若转为实施任务，必须遵守新的用户可见顶层任务门禁。

**本轮 T02 原生错误转译与管理员诊断 MVP（2026-08-12）：** 状态：已完成。候选 `codex/t02-native-error-diagnostics@0d95f30bb7025f14ddf35a94798602d389196830` 已合并为 `main@c600c661377928275c7e0eb2dde78a06be6c4f3f` 并推送到 `origin/main`；无迁移、无配置变更、无 GitHub Actions。交付四类稳定诊断：本站 RPM/并发限制、上游过载、通用上游失败、请求上传中断；用户仅看脱敏中文含义与建议，管理员在既有错误详情查看阶段、归属、账号选择状态、账号/分组和二次脱敏后的上游证据。已选账号后的本站账号并发/队列限制优先归为 `local_limit`，不会被 429 误判为上游过载；列表与详情使用同一最小分类证据。发布前证据 `/private/tmp/t02-native-error-release.IbsoGB/test-evidence.json`（0600）绑定 source `c600c6613`、tree `327a4258…`、迁移哈希 `f1b1f353…`，记录定向 Go 测试、`go vet`、server build、前端聚焦测试、typecheck/build 和两项蓝绿发布合同均通过。正式预检/发布返回 `downtime_required=false`；生产最终记录 `/var/lib/sub2api/release-records/20260812T061958Z-production-866650.json` 为 `succeeded`、`rolled_back=false`，活动槽 `blue`，API/worker 镜像均绑定 `c600c6613`。公网 `/healthz`、`/readyz`、`/health` 均为 200（alive/ready/ok），受保护管理更新接口未认证为预期 401。候选恢复归档 `/Users/gongtengxinwen/Documents/sub2api-archives/t02-native-error-diagnostics-0d95f30bb.bundle` 已校验，SHA-256 `5b412128ef17506fa1b3f7986666ddd02881554108a9d9949c5cb0098570c272`；交接验证报告见 `docs/superpowers/reports/2026-08-12-t02-native-error-diagnostics.md`。T03 可从本次已部署验证的 `main` 启动。

**本轮 T01 大上下文入站上传稳定性（2026-08-12）：** 状态：已完成。候选 `a3cc13a37f994360f556fc96e8e15f374a5bbcf5` 经第二轮独立复审批准，由合并提交 `129fc6003` 纳入 `main`，最终发布提交 `093daf1fbfb75b08cfcf1f1882e792442494c190` 已推送至 `origin/main`。实现：Caddy `read_body 15m` 覆盖慢速请求体读取；公共 fallback `response_header_timeout 15m` 仅覆盖完整请求写入 upstream 后的响应头等待；原生 128/128/16 MiB body limits 保持不变。生产 `15m` 策略下 301 秒持续上传、合并后短模式连续 3 次、派生 `2s` 不完整上传释放、Caddy validate/adapt、配置合同、发布控制器合同及 `git diff --check` 均通过；完整基线仍仅在既有无关 Dockerfile 1536 断言处失败，未扩大范围修改。发布器返回 `downtime_required=false`，蓝绿发布记录 `/var/lib/sub2api/release-records/20260812T015943Z-production-676675.json` 为 `succeeded`、`rolled_back=false`，活动槽为 `green`，API 与 worker 运行镜像 `ghcr.io/leesssong/xingqiao-sub2api:release-093daf1fbfb75b08cfcf1f1882e792442494c190-0844db2ebe67fc477676da890f740cc015bd2a05a06feebceca665df4b0e68a9`，迁移哈希保持 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`。新版 Caddyfile 已按运行手册先备份为 `Caddyfile.20260812T020138Z.t01.bak`，再 validate 与 reload；宿主/仓库哈希均为 `dbcb1878148edc20b46ccf75d8953cfd70d9c9d5e92085a5723aca417af5d681`，运行配置两个 server 的 `read_timeout` 均为 900 秒，Caddy 容器 ID `67ef71b6…` 未改变。公网 `/healthz`、`/readyz`、`/health` 分别返回 alive/ready/ok，API/Worker/PostgreSQL/Redis/relay-ops 健康。候选已归档到 `/Users/gongtengxinwen/Documents/sub2api-archives/t01-upload-stability-a3cc13a37.bundle`（SHA-256 `b5b3898edd98cac42bd114aa85ab7038cef9740257df023d22fd0a169af1ede3`）并删除对应 worktree/本地分支；未使用 GitHub Actions。

**余额调度 veto 审查修复（2026-08-12）：** 状态：进行中。仅在 `.worktrees/balance-scheduler-veto` 修复明确余额耗尽 veto 的 NULL fail-open、未知失败码不 veto、快照直取/容量查询一致性及余额写入后的调度同步，并补充回归测试；不合并、不部署、不删除 worktree。

**本轮余额证据调度 veto MVP（2026-08-11）：** 状态：已完成。候选 `codex/balance-scheduler-veto` 经独立复审修复 402/未知失败与 malformed 快照边界后合并到 `main@2fc0fea2e73fe9d637063db4c3c9a658386bc48e`，定向 service/repository 测试、`go vet`、构建和 diff 检查通过；通过预加载蓝绿链无停机发布，生产 `release-state` 绑定 source commit/tree 与迁移哈希 `f1b1f353…`，活动槽 `blue`，公网 `/healthz`/`/readyz` 为 alive/ready，API/worker/PostgreSQL/Redis/Caddy/relay-ops 健康。首次上传超时失败证据保留，第二次发布成功；待完成候选 bundle 归档、删除 worktree/本地分支和下一任务包启动前的总账更新。

**本轮 T01 大上下文入站上传稳定性（2026-08-11）：** 状态：进行中。余额 veto 已完成线上验证后，根线程从 `main@80eed324440bceaec8e5d583be5f24bc8edd4b49` 启动唯一下一任务包；范围仅限 Caddy 入站超时策略、既有请求体大小保护、配置合同测试及慢速/不完整上传验证，不包含错误转译、重试、调度或 CDN。T01 派生线程必须先提交规格书/计划，再在独立 worktree 实施、复审并等待根线程授权合并；若发布预检返回 `downtime_required=true`，立即暂停等待人工确认。

**本轮余额证据调度 veto MVP 发布回退（2026-08-11）：** 首次候选 `main@ccd328188` 已完成无停机发布，但线上专项验收和独立审查发现 PostgreSQL `NULL` 语义、失败码过宽、粘性直取绕过以及余额写入缓存/容量查询一致性问题。已创建回退提交 `bca98b6ed`，并于本轮以 `main@2f54e7970` 通过同一蓝绿链无停机恢复生产；公网 `/healthz`、`/readyz` 返回 alive/ready，生产 release-state 已绑定 `source_commit=2f54e7970`、活动槽 `green`，API/worker/PostgreSQL/Redis/Caddy/relay-ops 健康。候选 worktree、失败证据和审查结论保留，禁止启动下一任务包，直至余额 veto 修复版重新通过审查、合并、部署和线上验证。

**日期校正（2026-08-11）：** 下方本轮新增条目标题中的 `2026-08-12` 为先前会话的日期误记；本轮实际执行、合并与发布日期以 `2026-08-11` 的提交时间、发布记录和线上验证时间为准，后续状态更新统一使用 `2026-08-11`。

**本轮生产高级调度开启（2026-08-12）：** 状态：已完成（生产配置已写入并验证，无代码发布/部署）。通过原生 `PUT /api/v1/admin/settings` 开启 `openai_advanced_scheduler_enabled=true`，并按管理员确认写入 `openai_advanced_scheduler_weight_quota_headroom="0.5"`；Top-K、优先级/负载/排队/错误率/首包延迟沿用默认值（Top-K `7`，权重 `1/1/0.7/0.8/0.5`），未重启、未切换发布槽。运行时 `openAIWSSchedulerWeightsForRequest` 会在高级调度启用时读取该数据库覆盖并应用 `quota_headroom=0.5`；管理设置 DTO 的“有效权重”仅显示静态配置默认值，未叠加数据库覆盖，仍显示 `0`，这是已确认的展示缺口而非未写入。写入请求已记录为审计 ID `23306`（总开关）及后续 quota 覆盖审计；等待超过 5 秒缓存周期后均已读回。`/healthz`、`/readyz` 均正常；蓝绿 API/Worker 均 healthy、重启次数 `0`；release-state 仍为 source `292e3158d49511ed96fd1c5456fbb40d7853da74`、活动槽 `green`。注意：设置审计日志的 `changed` 列表会将请求体省略字段的零值列为“变化”，但服务层 `OmittedSettingKeys` 已按省略字段从写入集合剔除，当前读回与发布/健康证据未发现其他设置漂移。

**本轮余额证据调度 veto MVP（2026-08-12）：** 状态：进行中。范围严格限定为：对 OpenAI API Key 账号，将已确认的上游余额耗尽证据和上游明确返回的余额/额度不足错误接入现有候选资格链，使账号立即退出调度候选；管理员保留账号、来源、时间和原因，用户只得到泛化的可重试上游不可用提示。保持现有 403 10 分钟冷却与 3 次/180 分钟永久停用规则，不把“余额查询失败/未知”误判为余额为零，不改分组、计费、账号数据或外置控制面。当前工作区将从已盘点的 `main@ea9ab867d73b4f02c78df4fb57eeb707a9c5486e` 创建；所有代码变更先做定向 TDD、专项复审与候选验证，合并/部署须另经当前发布门禁，若预检涉及停机立即停止等待人工确认。

**本轮生产调度只读核验（2026-08-12）：** 状态：进行中（已完成根因取证，未修改生产、未部署）。生产 `/var/lib/sub2api/release-state` 与发布记录 `20260811T060627Z-production-4003239` 均绑定 `source_commit=292e3158d49511ed96fd1c5456fbb40d7853da74`、活动槽 `green`，记录为 `succeeded/promoted`，运行镜像健康；该 commit 已包含 `7811fd6cd`（缓存感知重试/failover）、`15a154ef7`（legacy 调度器过期冷却半开探测）、`572b2b4ad` 至 `e55fc3c9e`（上游故障恢复与可观测性）等调度改进。生产设置 `openai_advanced_scheduler_enabled=false`、sticky 加权关闭、quota-headroom 权重为空，因此高级加权调度未启用，实际为 legacy/default scheduler 加上运行时冷却与 failover。根因证据：`account_monitor_balance.go` 明确将余额定义为 display-only，与 scheduling state 分离；`schedulableAccountsQuery` 仅检查 active/schedulable/临时不可调度/过期/overload/rate-limit，不检查余额证据。生产账号 `#81 plus-Auv`、`#20 特惠-TK-极速` 等出现 `account_monitor_balance.status=failed,failure_code=balance_unavailable` 但仍 `schedulable=true`；#81 在 403 `insufficient balance` 后只进入 10 分钟临时冷却，403 计数达到 3 次才永久停用（180 分钟窗口），因此余额失败与调度 veto 之间存在已证实的缺口。下一步建议拆为独立 MVP：将已确认的余额失败/上游明确余额不足错误接入统一调度 veto，并保留清晰管理员原因与用户泛化错误；本轮不直接改状态或上线。

**本轮候选归档与清理（2026-08-12）：** 状态：已完成（仅本地清理，未部署）。已确认 `candidate-artifact@f2dbcbb45fe4e5f331e054fbcb42aba6ecf81111` 不是 worktree，且已完全包含在 `main@ea9ab867d73b4f02c78df4fb57eeb707a9c5486e`（`main...candidate-artifact = 24 0`）。已创建可恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/candidate-artifact-f2dbcbb45.bundle`，SHA-256 为 `59bd694a07e2fc2a0de41f8f645db465bc350070773aefda3543eec1b17cd8e8`，随后用非强制 `git branch -d` 删除本地分支；恢复说明见同目录 `.md` 文件。当前仅剩根目录 `main` worktree。

**本轮小步发布编排准备（2026-08-11）：** 状态：进行中。目标是在不启动派生线程、不实施功能、不部署生产的前提下，固化所有后续任务包必须遵守的全局约束、原生 Sub 单体边界、线程/worktree 生命周期、单候选串行合并与快速部署门禁，并把已确认需求拆成可独立验证、独立合并、独立发布的小任务包。当前工作区为根目录 `main@cea37c7f9`，与 `origin/main` 一致；根目录既有未跟踪文件 `docs/superpowers/plans/2026-08-11-release-gap-closure.md` 继续保留不动。实施前盘点确认唯一非 `main` worktree 为 `.worktrees/external-primary-production-closure`，工作区干净、分支比 `main` 领先 30 个提交且 `main` 比其领先 11 个提交；其 69 个分支差异文件和约 7,032 行新增内容属于用户已明确否决的 `external-primary` 外置主路径，因此与“只在原生 Sub 内嵌页面、增加功能”的当前产品约束冲突。用户已明确授权：记录该分支提交 SHA、制作可恢复归档，然后不合并地删除该 worktree 和本地分支；归档和清理结果已在本账及归档说明中补齐。本轮另核对 `candidate-artifact@f2dbcbb45` 仅为已成为 `main` 祖先的本地分支，不是注册 worktree，已在本轮完成 bundle 归档和本地分支清理。约束与编排文件已提交到本地 `main@ea9ab867d`，尚未推送；尚未派生线程、实施功能或部署。

**本轮逐笔计费工作区收口（2026-08-11）：** 状态：已完成。生产专项验证完成后，已逐一确认 `.worktrees/account-monitor-group-recommendation`、`.worktrees/direct-upstream-billing-fields`、`.worktrees/fix-release-version-identity`、`.worktrees/gpt-group-baseline-apply`、`.worktrees/maintenance-allowlist-fix` 均无未提交内容且 tip 已成为 `main` 祖先，随后删除五个 worktree，并通过非强制 `git branch -d` 删除对应五个本地分支；恢复依据为远端 `main@dae4afa02`、生产记录 `20260811T060627Z-production-4003239` 和可达的原提交 SHA。用户引用任务 `.worktrees/external-primary-production-closure` 当前仍有未提交实现且不属于 `main` 祖先，继续作为唯一保护工作区保留，未修改、未合并、未清理。根目录未跟踪计划 `docs/superpowers/plans/2026-08-11-release-gap-closure.md` 已完整恢复并保留。

**本轮逐笔计费原生字段直读修正完成（2026-08-11）：** 状态：已完成。除引用任务 `.worktrees/external-primary-production-closure` 外的既有非 `main` worktree 已按用户要求先合并；本功能从更新后的 `main` 创建 `.worktrees/direct-upstream-billing-fields` 实施，修复 New API 推理地址 `/api/v1` 被拼成 `/api/api/log/token` 与 `/api/api/status` 的问题，仅对原生逐笔账单调用启用 `/api` 前缀归一化，Sub2API 继续直接读取精确匹配流水的 `actual_cost`，New API 继续直接按精确匹配流水 `quota / quota_per_unit` 计算，不含估算或模糊匹配。定向 TDD、Sub2API 路径保护回归、任务复审与修复轮复审均通过；候选已由合并提交纳入并推送 `main@292e3158d49511ed96fd1c5456fbb40d7853da74`。生产通过预加载蓝绿链无停机发布，记录 `20260811T060627Z-production-4003239` 为 `succeeded/promoted`、`rolled_back=false`，活动槽为 `green`，运行镜像为 `ghcr.io/leesssong/xingqiao-sub2api:release-292e3158d49511ed96fd1c5456fbb40d7853da74-a6a23ccede2f5229d1465757beb7d7e1cb0280e7883e96b379169b734f2c0caa`，迁移哈希保持 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`。线上 `/healthz`、`/readyz` 均通过；管理员只读接口复核自然流水 `usage_id=77897` 返回 `status=confirmed`、本站实际扣费 `0.0165964`、上游实际扣费 `0.009958`、利润 `0.006638400000000001`，请求 ID 精确匹配。全程未使用 GitHub Actions，受保护工作区未修改、未合并、未清理。

**本轮逐笔计费简化与工作区收口（2026-08-11）：** 状态：进行中。用户确认将上游真实扣费收敛为直接读取原生账单字段：Sub2API 逐笔记录直接取 `actual_cost`，New API 逐笔日志直接取 `quota` 并用 `quota_per_unit` 换算，只保留 `request_id/upstream_request_id` 精确匹配；不再增加额外探测、估算或与本功能无关的兼容链。执行顺序为：先将参考任务 `019feb5a-d56d-7982-94cd-559f5e705165` 对应的 `.worktrees/external-primary-production-closure` 作为唯一保护例外，其余非 `main` worktree 全部合并到 `main` 且不做重复验证；再从更新后的 `main` 新建独立工作区完成 TDD、独立复审和本功能专项验收；随后合回 `main`，从已验证 `main` 推送并覆盖生产，只验收逐笔扣费显示链，成功后删除本轮及已收口候选工作区/本地分支。全程不使用 GitHub Actions，不触碰保护工作区的未提交内容。

**本轮遗漏修复与生产补发核验（2026-08-11）：** 状态：已完成（核验结论：无需重复合并或重复发布，但全局仍有未完成项）。`codex/maintenance-allowlist-fix@219044acb` 与已进入 `main` 的 `0b8377a` 具有相同 parent `f2dbcbb45`、相同 tree `686afce1d1e925fa4ce8c914264f55e18b21e8b9`，宿主执行器和合同测试逐文件无差异；生产提交 `6257e4be7` 已包含 `0b8377a`，因此该 worktree 是重复提交，不是漏发候选，禁止为制造新 SHA 再次合并/部署。当前 `main@05342812e` 与 `origin/main` 同步，相对生产运行提交只有总账文档差异；宿主 release-state 仍为 `6257e4be7`、活动槽 `blue`、版本 `0.1.173`，API/worker/数据库/Redis/Caddy/relay-ops 健康，公网 health/ready/home 为 200。外置主路径尚未完成：relay-ops 仍只有 `RELAY_OPS_MODE=read_only`，没有启用 `RELAY_OPS_EXTERNALIZATION_ENABLED`、核心数据库只读凭据、生产 report-set/cutover 持久卷，现有三窗口证据明确为 `isolated_local_fixture`；事务 outbox 已累积 6507 条，但这不证明消费者追平或生产双读一致。因此未执行 `external_primary` 写操作，也未用重复 Sub2API 发布伪装完成；后续需单独补齐 relay-ops 生产消费/持久化配置、真实三窗口比对和回退演练后再按逐页门禁切换。两个受保护 worktree 未修改、未合并、未清理，全程未使用 GitHub Actions。

**本轮逐笔上游扣费空值复核（2026-08-11）：** 状态：进行中。根据管理员日志详情中“上游实际扣费/利润”为 `-` 的现象，只读追踪 Sub2API / New API 上游逐请求费用能力、请求 ID 持久化、账单查询适配与精确匹配状态；本轮不修改生产数据、用户/账号并发、余额、计费、路由、账号或容器。

**本轮逐笔计费修正实现与独立审查（2026-08-11）：** 状态：进行中。实现提交 `1c8d8eea8`、`75b7bf69e`、`f2949b3b8` 已完成；New API `/api/v1` 原生账单地址归一化通过 RED/GREEN，Sub2API `/api/v1/usage` 路径保护回归通过，独立复审结论为 spec compliant / task quality approved。当前只剩合并回 `main`、推送、生产部署和本功能线上验证；旧的 Task 1 登记保留为过程快照。

**本轮逐笔计费原生字段直读修正（2026-08-11）：** 状态：进行中。用户确认简化为 Sub2API 原生流水 `actual_cost` 直读，以及 New API 原生日志 `quota` 除以 `/api/status` 的 `quota_per_unit`；只保留 `request_id/upstream_request_id` 精确匹配，不增加估算、模糊匹配、relay-ops 前置条件或无关兼容工作。实施前已把除引用任务“合并工作区并完成发布准备”之外的既有非 `main` worktree 合并到 `main@a7f57d2f6`，未做重复验证；引用任务工作区 `.worktrees/external-primary-production-closure` 作为本轮明确保护例外，不合并、不修改、不清理。当前实施工作区为 `.worktrees/direct-upstream-billing-fields`，分支 `codex/direct-upstream-billing-fields`；已定位 New API 推理地址 `/api/v1` 被错误拼成 `/api/api/log/token` 和 `/api/api/status`，本轮仅以定向 TDD 修正原生接口地址归一化。实现完成后只做本功能验收，合并回 `main`、推送并从已验证 `main` 直接覆盖生产；生产专项验证成功后才删除本候选及已收口的旧工作区/分支，全程不使用 GitHub Actions。

**本轮逐笔上游扣费空值复核（2026-08-11）：** 状态：进行中。根据管理员日志详情中“上游实际扣费/利润”为 `-` 的现象，只读追踪 Sub2API / New API 上游逐请求费用能力、请求 ID 持久化、账单查询适配与精确匹配状态；本轮不修改业务代码、生产配置、账号、计费或历史流水。当前工作区为根目录 `main@b4e72628f`；实施前已盘点全部非 `main` worktree，四个工作区均干净且 tip 已纳入 `main`，未发现完成且领先 `main` 的候选；既有保护工作区保留，不合并、不修改、不清理。

**本轮完成度核验（2026-08-11）：** 状态：已完成（核验结论：当前 Sub2API 运行代码已部署，但全局并非所有事项都完成）。`main@b4e72628f` 与 `origin/main` 同步且相对生产运行提交 `6257e4be7` 只有文档差异；生产 release-state/release-record 绑定 `6257e4be7`、迁移哈希 `f1b1f353…`，活动槽 `blue`，API/worker/数据库/Redis/Caddy/relay-ops 健康，公网 `/healthz`、`/readyz`、首页均 200，版本 `0.1.173`。逐笔成本与 New API request-ID 修复已通过自然流水 `usage_id=77897` 的 `confirmed` 精确成本验证。全部非 `main` worktree 均干净且没有保护工作区之外的已完成领先分支；但 `.worktrees/maintenance-allowlist-fix` 的 `codex/maintenance-allowlist-fix@219044acb` 仍比 `main` 多 1 个未合并提交（迁移 `fadb98d4…→f1b1f353…` 的维护 allowlist 与合同测试），尚未推送或部署。外置控制面当前生产仍为 `RELAY_OPS_MODE=read_only`；`/api/v1/xingqiao/*` 已有认证入口（未认证 401），`externalization_outbox` 当前有 6250 条事件，但没有证据证明已切换 `external_primary`，因此外置解耦/官方优先升级及相关 relay-ops 主路径不能标记为全部完成。总账中更早写成“路由 404/outbox 0”或引用已删除 `fix-official-update-stuck` worktree 的条目属于历史快照，后续应以本核验事实为准；本轮不修改业务代码、不重复全仓测试。

**本轮生产逐笔成本二次收口（2026-08-10）：** 状态：已完成。修复已合并并推送至 `main@6257e4be7e577ed99126e3f55849c77bc392ebb0`，以迁移哈希 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc` 通过预加载宿主蓝绿链无停机发布；生产记录 `20260811T022331Z-production-3838711` 为 `succeeded/promoted`，活动槽 `blue`，API 与 worker 运行镜像 `ghcr.io/leesssong/xingqiao-sub2api:release-6257e4be7e577ed99126e3f55849c77bc392ebb0-fe2b83d5a9dddb38ca2f06c58a8281bfc81fac36c8c31f12ebe6f33432970c94` 且健康。公网 `/healthz`、`/readyz`、首页均为 200，公共版本为 `0.1.173`。部署后自然 New API 流水 `usage_id=77897` 保存即时上游请求 ID `202608110226533888804978268d9d6PESdod8Z`，管理员只读成本接口精确匹配并返回 `status=confirmed`、本站实际扣费 `$0.0165964`、上游实际扣费 `$0.009958`、利润 `$0.0066384`，未放宽精确匹配或以 `account_cost` 冒充上游扣费；账号 `#42 Plus-WAWAZZ` 继续保持上游实际扣费不可用边界。候选 `.worktrees/fix-production-defects` 在确认干净且分支已并入 `main` 后删除，对应本地分支已安全删除；两个用户明确保护 worktree 未修改、未合并、未清理。全程未使用 GitHub Actions，恢复依据为远端 `main`、生产发布记录与保留的旧 `green` 镜像。

**本轮 `v0.1.173` 版本身份收口（2026-08-10）：** 状态：进行中。生产维护发布已把 `main@0b8377a971e95edff8a9a332dbfbedcc40932128`、tree `686afce1d1e925fa4ce8c914264f55e18b21e8b9` 和迁移集 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc` 成功提升到活动 `blue` 槽，发布记录 `20260810T194408Z-production-3545218` 为 `succeeded/promoted`，线上 health/ready/home 为 200、未认证管理接口为 401；但登录态验收发现版本按钮仍显示 `v0.1.172`。根因已定位为候选源码 `upstream/sub2api/backend/cmd/server/VERSION` 仍为 `0.1.172`，Dockerfile 未显式传 `VERSION` 时按设计从该文件解析，导致已发布二进制和升级状态使用旧版本身份。本轮将新增真实发布合并回归，要求目标元数据版本、候选源码 VERSION 与构建身份一致，再从最新 `main` 生成、审查、推送和重新发布候选；当前准备创建 `.worktrees/fix-release-version-identity`，两个用户明确保护 worktree 继续不修改、不合并、不清理。只有新 `main` 已推送、生产再次部署并完成登录态版本/升级状态和三项原缺陷线上验证后才能标记完成。GHCR 正式候选发布仍因缺少私有仓库凭据阻塞，本轮继续使用已审查的 preloaded 宿主蓝绿链，不使用 GitHub Actions。

**本轮生产迁移集维护门禁补全（2026-08-10）：** 状态：进行中（本地实现与验证已完成，待合并、推送、部署与线上验证）。根因已确认为生产活动迁移集 `fadb98d43e3d8e8b41178203638912cc32592a1368091e4cb44399926daead5d` 到已验证候选迁移集 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc` 尚未纳入宿主执行器的精确 allowlist，因此在启动任何候选服务前以 `reason_code=migration_set_changed` 安全退出。本轮工作区为 `.worktrees/maintenance-allowlist-fix`，分支 `codex/maintenance-allowlist-fix`；已用真实 host executor 受控 fixture 先复现 RED（精确转换被拒绝），再仅补充第 6 条已批准转换，精确回归、两个 Bash 语法检查及 `git diff --check` 均通过，独立复审无发现。本轮没有修改迁移内容、生产数据或其他发布行为，未推送、未部署，两个受保护 worktree 未修改或删除。

**本轮三项生产缺陷排查与修复（2026-08-10）：** 状态：进行中（Task 4 本地实施审查已通过，待整分支审查与后续生产收口）。
范围：一是追查 Sub2API 与 NEW 上游逐笔扣费为何未进入日志详情的“上游实际扣费/利润”，并将仅管理员可见的费用字段移入管理员信息分组；二是追查账号监控第三张及后续卡片的评分、成本悬浮层失效，统一为稳定且可访问的交互并举一反三检查同类卡片控件；三是追查官方版本 `0.1.173` 候选长期停留“检查中/准备中”、无法立即升级的状态机和宿主执行链。
当前工作区为 `.worktrees/fix-production-defects`，分支 `codex/fix-production-defects`，Tasks 1–3 基线为 `6553f6f23` 且已通过独立审查。Task 4 提交 `4ca6947bb` 已形成 `0.1.171→0.1.173` 精确冲突记录，真实 resolver 重放与人工语义合并得到同一 tree `81aa3869fb02932a782981b5686f154e1f78430a`，并修复“官方已跟踪新文件命中快照 `.gitignore` 后只留在磁盘、未进入 candidate commit”的发布链缺口；真实脚本候选提交 `2a5f5a3e3` 已导入当前工作区。
最终本地门禁已通过：后端全量 test/vet 与显式 `0.1.173` 版本注入、前端 246 文件/1746 测试及 typecheck/lint/build、updater 与 release 全套、PostgreSQL/Redis Testcontainers 迁移、候选 bundle/report 身份、normalized tree `4948ae094ce9021bfdb4020079ab034828bab894` 一致性、官方 Markdown 硬换行例外后的 diff 检查及 clean worktree。
Task 4 首轮独立审查发现 resolver 仅约束生成文件名、未约束生成内容，以及 `ent_seed_paths` 可接受路径穿越/控制字符。修复轮次 1 已按 TDD 增加逐文件 `generated_postimages` blob 合同，并在任何写入前统一拒绝不安全 Ent seed 路径；新测试先得到 2 项预期失败，修复后聚焦回归为 2 runs/24 assertions，全量 merge suite 为 28 runs/194 assertions，真实 50 冲突重放继续得到 tree `81aa3869fb02932a782981b5686f154e1f78430a`，且两份既有 resolution record 的全部生成后 blob 均与已审查快照一致。聚焦独立复审已将两项 finding 均判定为 ADDRESSED，修复 diff 无新增 Critical/Important 问题；当前待整分支最终审查。未合并 `main`、未推送、未部署、未修改生产，状态不得标记完成。
两个用户明确保护工作区继续保留，不合并、不修改、不清理；不启用 relay-ops 外置主写入，不使用 GitHub Actions。

**本轮纠偏部署登记（2026-08-10）：** 状态：已完成。已审计全部非 `main` worktree，保留用户点名的两个工作区；`main` 已推送至 `origin/main`，并从 `main@fa63c136c345914e1b60412cf8acd35edfd89ab3` 完成停机蓝绿发布。生产记录：`20260810T061914Z-production-2991041`，结果 `succeeded/promoted`，活动槽位 `green`，镜像 `ghcr.io/leesssong/xingqiao-sub2api:release-fa63c136c345914e1b60412cf8acd35edfd89ab3-64977dcd44307856faf29140f4959f25fd17483d0309351b59c204b4c06b7936`，迁移哈希 `fadb98d43e3d8e8b41178203638912cc32592a1368091e4cb44399926daead5d`。线上 `/healthz`、`/readyz`、首页均为 200，未认证管理 API 为 401；API、worker、PostgreSQL、Redis、Caddy、relay-ops 均健康。relay-ops 仍保持 `read_only`，未启用外置主写入或账务写入；不使用 GitHub Actions。

**本轮宿主门禁修复（2026-08-10）：** 首次停机预检发现生产 Caddy 容器曾被独立重建，导致 release-state 中的 Caddy 身份过期；在保持 PostgreSQL/Redis 连续性、校验当前 Caddy 镜像且用户已明确授权停机的前提下，允许同迁移集维护发布刷新共享容器身份。修复已进入 `ops/deploy-sub2api-blue-green-host.sh`，待重新推送并发布验证。

**本轮历史“待部署”清单复核（2026-08-10）：** 状态：已完成。复核基准为当前 `main@b57f4718408f45a3b8b72077c2f135f730467ad0`、`origin/main`、生产 Sub2API 发布记录 `20260810T044955Z-production-2917055` 及 relay-ops 状态文件。生产 Sub2API 当前运行 `ghcr.io/leesssong/xingqiao-sub2api:release-300754c0e…`，其源提交 `300754c0e` 是当前 `main` 的祖先，`git diff 300754c0e..main` 仅包含 `docs/` 文档，没有运行时代码或迁移差异；生产迁移哈希为 `fadb98d4…`，与当前 `main` 一致。生产 Caddyfile 哈希与仓库 `infra/Caddyfile` 一致，Caddy 运行时源提交为 `754ab2efe`；relay-ops 当前为 `52aa3023a` 且保持 `read_only`。因此历史清单中的低延迟、WebSocket `store=false` 隔离、Monitor 自动刷新、Caddy/homepage、全站账务代码、账号倍率同步、Monitor V2 精简、Monitor 状态/飞书修复、30 分钟蓝绿链和账号监控成本/余额/倍率收敛均已随已验证生产发布生效，之前标成“工程代码/配置差异待部署”是总账回填滞后，不是发布漏项。仍不能宣称完成的只有 TLS 兼容性外部客户端门禁，以及 relay-ops 外置主写入/账务授权（生产明确保持 `read_only`）；二者属于外部/运营跟进，不应通过重复停机重发伪装成已完成。

**本轮复核与清理（2026-08-10）：** 状态：已完成。已读取用户点名的“运维”和“制定上游账号测试方案”两个任务线程，并重新盘点全部 Git worktree、分支提交、脏状态与发布证据。当前仅剩 `.worktrees/account-monitor-group-recommendation`（`codex/account-monitor-group-recommendation@f71b63b1f`）和 `.worktrees/gpt-group-baseline-apply`（`codex/gpt-group-baseline-apply@03c20f370`）两个用户明确保护的工作区；二者均干净，且各自 tip 已是 `main@89aa822d9` 的祖先，没有可再合并的独有提交。本轮没有删除保护工作区，也没有发现其他非 `main` 工作区或分支可清理。此前“未上线”的主要原因已核实为发布边界分离：Sub2API 与 relay-ops 使用独立镜像/发布状态，早期只完成了 Sub2API 发布；relay-ops 控制面后来因缺少 green 槽和宿主路由先被 fail-closed 门禁拦截，完成 bootstrap、Caddy/Compose 同步和线上验证后才发布成功。账号推荐代码与 GPT 分组基线记录已合入 `main`，Sub2API 版本已部署；relay-ops 当前仍保持 `RELAY_OPS_MODE=read_only`，外置主写入/账务写入未启用。恢复证据：`git merge-base --is-ancestor`、worktree 状态输出、`main` 合并提交 `c7c0e09f8`/`ce1902e19`、relay-ops 发布证据及线上 health/ready/auth 验证记录。

**本轮任务登记（2026-08-10）：** 已复核并详细汇总本次 `cfae5d5d10e7f1a6783a28d299848109408fa4c7 → 300754c0e01bfaf04bda4fd7e9af0f032cfab9db` 生产更新，并删除除用户引用任务“运维”与“制定上游账号测试方案”关联工作区之外的非 `main` worktree；状态：已完成。当前工作区为根目录 `main@041172dc6`。实施前盘点确认三个非 `main` worktree 均干净且其分支提交均已成为 `main` 祖先：`.worktrees/fix-official-update-stuck@9ca36e0e7` 已由合并提交 `0f3ead07c` 纳入并随生产版本部署，现已通过 `git worktree remove` 删除并以 `git branch -d` 安全删除本地分支，恢复证据为 `main` 中的合并提交 `0f3ead07c` 及可达原提交 `9ca36e0e7`；`.worktrees/account-monitor-group-recommendation@f71b63b1f` 是“制定上游账号测试方案”引用任务最新实施阶段明确创建的工作区，`.worktrees/gpt-group-baseline-apply@03c20f370` 是同一引用任务较早的上游账号基线阶段，二者均作为保护例外保留。根目录 `main` 承载“运维”引用任务且不属于非 `main` 清理范围；既有未跟踪发布证据继续保留。本轮仅清理已部署且可由 `main` 恢复的本地 Git 元数据，没有业务代码或生产配置变化，无需重新部署。

**本轮发布边界复核（2026-08-10）：** 本次 `main` 合并范围与实际生产发布范围不完全相同。Sub2API API/worker 已运行 `300754c0e01bfaf04bda4fd7e9af0f032cfab9db` 镜像，宿主蓝绿执行器也已同步；但独立的 relay-ops 仍运行 `94f8b4d8e428a46e43b60bc41ad9694ded01ceba`，相对合并候选尚有 53 个 `relay-ops-service`/Caddy/Compose 文件、7251 行新增和 204 行删除未通过 relay-ops/infra 发布链上线，宿主 Caddyfile 与 Compose 哈希也不等于本次仓库版本。因此账号推荐、Sub2API 事务 Outbox/窄写接口和迁移 202 属于已部署代码；控制面持久投影、同域认证、外置余额/账单/倍率采集、管理员双读、逐页切换/回退权威等能力仅已合并到 `main`，不得宣称已在生产生效。状态：进行中（等待单独的 relay-ops 与基础设施发布授权、资格证据及线上验证）。

**本轮实施进展（2026-08-10）：** 账号监控卡片“推荐分组”已完成 Tasks 1–4 实现、审查和验证，并由合并提交 `c7c0e09f8` 纳入 `main`、推送后随 Sub2API `300754c0e` 镜像部署：新增固定滚动 7 天主动探测推荐投影，测试组显示推荐/观察/暂缓/不建议，正式组仅对正常可调度账号在明确不匹配时显示同行推荐文字与叹号 Tooltip；Codex Auth 正常默认 Pro，且没有账号改组、优先级、调度、评分权重、定时器或数据库写入。独立 whole-branch 审查发现并阻断了 Pro/专属 Pro 约束聚合、模型不可用、Codex Auth 质量门、生图/Claude 排除、前后端别名一致性和首要原因顺序稳定性问题，均已由 `663551bb8` 的 TDD 回归修复并复审。后端相关包测试与全仓 `go vet` 通过；前端 231 个测试文件、1647 项测试、类型检查和构建通过；`390px` 窄屏无横向溢出。实现报告为 `docs/superpowers/reports/2026-08-10-account-monitor-group-recommendation-implementation.md`。**状态：进行中（已合并、推送并部署，公网健康验证通过；仍缺登录态生产账号投影与卡片交互专项验收）**。

**本轮登记（2026-08-10）：** 请求 `cad40d1b-b46a-4d0b-9878-9a7636cc6780:1` 显示 0 Token、0ms、本站实际扣费 `$0` 的计费异常，以及 Codex 任务“制定上游账号测试方案”“优化更新机制”多次 429 并发限制却未出现在 Sub 原生错误监控和使用记录中的生产只读根因排查，状态：进行中。范围为对齐 Codex 任务失败时间/平台请求 ID、Sub2API handler 并发门禁、错误采集、usage log 创建和扣费写入顺序，确认零值流水与监控漏记是否同源；本轮不修改生产数据、用户/账号并发、余额、计费、路由、账号或容器，不发送额外付费探测。当前工作区为根目录 `main`；已盘点全部非 `main` worktree，`.worktrees/fix-official-update-stuck` 对应用户点名的活动任务“优化更新机制”，`.worktrees/gpt-group-baseline-apply` 对应仍在进行的上游账号任务，均作为本轮保护例外保留，不合并、不修改、不清理。

**本轮 Task 8 资格门禁收口（2026-08-10）：** 状态：进行中。Task 9 Fix Round 1 已完成本地提交与独立复核；盘点发现官方 Release 资格实现仍仅写入伪造 `ready` 状态，promote/rollback 脚本未执行真实门禁或切换。本轮范围限定为补齐候选身份/checksum/合同测试/迁移分类/数据差异/有效期和失败保持活动槽位不变的资格链，以及让提升和回退入口调用 reviewed host executor；继续不推送、不部署、不访问生产、不使用 GitHub Actions。

Task 8 本轮已将资格、提升、回退脚本改为 fail-closed：资格命令必须是绝对路径受控 host command，输出需通过 tag/commit/asset/SHA-256/adapter/contract/迁移/测试/数据差异/Passed 校验；ready 带一小时有效期，promote 与 rollback 必须调用可执行 reviewed host executor；新增脚本合同测试，`sub2api-updater` 全量 `go test ./...` 与 `go vet ./...` 通过。仍未推送、部署或线上验证，不能标记完成。

**本轮 Task 9 Fix Round 1（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck`。
范围限定为修复 Task 9 独立复审的 C1/C2/I1-I5：把逐页晋级和运行时回退权威收敛到经认证的 relay/control-plane 持久边界，强化余额时差/差额规则、三窗口同一运行和完整币种/排名/对账/版本维度，在服务端强制“监控 → 盈利 → 账务 → 对账”顺序，并用隔离本地 fixture 完成非生产演练和审计证据。本轮仅本地 TDD、演练、审查与提交；不修改 `main`/其他 worktree/远端/生产/GitHub Actions/核心业务表，未满足推送、部署和线上验证前继续为“进行中”。

Fix Round 1 实现与本地验证已完成：可信 report-set/运行时 authority、递归四页顺序、持久化幂等回退、服务端独立决策 API、前端去除 Vite/响应自证门禁、账务摘要接入及隔离 fixture 演练均有 focused RED→GREEN 证据。`ops/smoke-sub2api-release.sh --rehearsal --rollback` 已真实生成 `evidence/sub2api-rehearsal/task-9-local/` 下 4 个 report-set、12 个窗口、8 条晋级/回退审计记录，所有页面回到 `legacy_only`；独立本地复审另补强了子报告的 set/run/lineage/persistence 绑定；仍未推送、部署或线上验证，状态继续为“进行中”。

**本轮 Task 9 双读比对、逐页切换与回退门禁（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck@647948095`。
范围限定为精确比较账号/请求/账单/Token/金额/倍率/评分/排名/余额/对账结果，持久化最小、默认和最大窗口报告，并用 `legacy_only → shadow_building → dual_read_comparing → external_primary → legacy_retired` 逐页门禁与可回退标记控制账号监控、盈利、账务和对账读路径；Task 5 的保守 legacy 回退在比较证据满足前必须保留。实施前重新盘点全部非 `main` worktree：`account-monitor-group-recommendation` 领先 7 个提交但未推送、部署和线上验证，`gpt-group-baseline-apply` 领先 1 个提交且无本次生产完成证据，均继续只读保护。本地 TDD 已完成：relay-ops 全量 Go 测试、前端 232 个文件/1663 个测试、lint、typecheck、生产构建及发布 smoke/deploy 离线合同测试通过。brief 中的字面演练命令因未提供真实 rehearsal Compose、密钥文件、基线计数和 URL/版本环境而失败，未宣称回退演练成功。本轮仅本地 TDD、安全干跑、独立审查与提交，不推送、不合并、不部署、不访问生产、不使用 GitHub Actions；未具备生产三段证据前状态保持“进行中”。

**本轮 Task 5 管理员页面双读无感迁移（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck@22b21db2f`。
范围限定为在原管理菜单和 URL 中接入版本化控制面客户端、双读模式与新鲜度/完整性状态，保持现有登录、2FA、字段、筛选、排序、分页、刷新、详情和 CSV 体验；控制面失败只局部降级，不清理主站会话。实施前已复盘全部非 `main` worktree：`account-monitor-group-recommendation` 虽领先 7 个提交但证据明确未合并、未推送、未部署和未线上验证，`gpt-group-baseline-apply` 领先 1 个提交且不是生产完成候选，均保持只读且不作为“已完成并必须先合并”处理。本轮仅本地 TDD、前端验证、独立审查与提交，不推送、不合并、不部署、不访问生产、不使用 GitHub Actions；未具备生产三段证据前状态保持“进行中”。

**本轮 Task 6 Fix Round 1（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck@77c9ed585`。
范围仅修复 Task 6 独立复审的 Critical/Important 项：把有界余额采集接入控制面调度、经既有认证边界暴露账户更新命令、收紧命令身份与官方端到端幂等、失败重放、注入客户端超时及采集重试稳定事实身份；不处理已在 SDD 总账延后的未来日期事实 Minor 项。实施前已盘点全部非 `main` worktree：根工作区有未跟踪发布证据，其他候选虽领先但均无“已推送 + 已部署 + 已线上验证”完成证据，故不合并且保持只读保护。本地 RED→GREEN 已完成，relay 全量、race、vet 和官方端点聚焦测试均通过；PostgreSQL durable identity 回归因未配置 `RELAY_OPS_TEST_DATABASE_URL` 跳过。未推送、不合并、不部署、不访问生产、不增加 GitHub Actions，状态保持“进行中”。

**本轮 Task 6 外置余额、账单、倍率采集与受控写操作（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck`，基线
`370aa1b91`。范围限定为 relay-ops 的余额/账单/费率采集事实、过期语义和复用既有
控制面幂等审计命令更新；官方 Sub2API 仍是核心业务表唯一写入者。本轮仅做本地 TDD、
迁移、回归与提交，不推送、合并、部署、访问生产或使用 GitHub Actions。余额事实、
官方窄更新适配、持久幂等/冲突/过期回归及全量本地 Go 验证已完成；因尚未合并、推送、
部署和线上验证，状态继续为“进行中”。

**本轮 Task 4 fix round 3（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck`。
范围限定为收紧 relay-ops trusted-proxy 身份：仅显式配置并解析实际 Caddy
Docker peer 可转发原始浏览器 IP；默认无配置回退 RemoteAddr；补齐 TDD、
Compose/config 路由合同和本地回归；不推送、合并、部署或触碰生产。
本地实现、严格聚焦回归、全量 Go 测试、race、vet 与路由合同已通过；因本轮
明确禁止推送、部署和线上验证，状态继续为“进行中”。

**本轮 Task 4 fix round 2（2026-08-10）：** 状态：进行中。当前工作区为
`.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck`。
范围限定为修复同域管理员会话原始客户端 IP 选择、四个空读模型元数据以及
官方刷新失败与持久命令完成失败的错误组合；仅本地 TDD、回归和提交，不推送、
合并、部署或触碰生产。

**本轮官方更新“候选版本准备中”卡住修复（2026-08-09）：** 状态：进行中。用户在管理员界面从当前 `0.1.170` 升级至官方 `0.1.173` 时，弹窗持续显示“候选版本准备中”且“现在升级”不可用。证据已确认：生产 updater 进程与硬盘上均为 2026-08-07 旧版（SHA-256 `c9c5e2ab…`），2026-08-09 只安装了新 candidate preparer（SHA-256 `7c4c561d…`），形成版本断裂；且宿主并无 `xingqiao-sub2api:upstream-0.1.173` 合格镜像，所以修复更新器后还需按本地/宿主发布链生成候选。本轮同时以 TDD 补齐上游账单失败原因结构化返回和管理页提示：账号 `#42 Plus-WAWAZZ` 的 `/v1/usage/records` 返回 HTTP 404，不会用估算值填充上游实际扣费。当前工作区为 `.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck`；“制定上游账号测试方案”的 `.worktrees/gpt-group-baseline-apply` 及其未提交内容作为保护例外，不修改。

**本轮范围纠偏（2026-08-10）：** 状态：进行中。用户明确要求回到已批准的 `docs/superpowers/specs/2026-08-08-sub2api-externalized-customization-official-update-design.md`，不得把“更新器恢复 + 合格定制镜像”误报为定制解耦完成。仓库复核确认外置合同、事务 Outbox、控制面读模型、双读比较、feature flags 和后台资格状态机已经实现并进入主线；本轮继续核验生产是否已切到 `external_primary`、核心定制是否只剩薄适配器、最近三个官方候选是否达到 `ready`。在这三项均有生产证据前，外置解耦与官方优先升级保持“进行中”，`0.1.173` 仅可生成和暂存合格候选，不得宣称已完成解耦或直接覆盖生产。控制面持久投影 Task 3 与核心薄事件 Task 7 已完成本地实现并分别通过独立 SPEC/QUALITY 复审；Task 7 的真实 Testcontainers PostgreSQL 18.1/Redis 8.4 回归证明 `CreateBestEffort` 成功时 usage/outbox 同事务提交，注入 outbox 失败时两表同时回滚，请求事件在落库前包含 actual response model，健康事件查询错误会回滚。当前进入 relay 持久消费者、同域控制面 API 与隔离认证接线；所有变更仍未推送、部署或修改生产，整体状态保持进行中。

**本轮生产只读审计（2026-08-10）：** 状态：进行中，确认尚未启用外置主路径。生产 relay-ops 仍为 `RELAY_OPS_MODE=read_only`，compose/release 环境没有 `external_primary` 配置；Sub2API 的 `externalization_outbox` 表虽已由 migration 200 创建，但事件数为 0；生产数据库没有 `relay_ops` 外置读模型 schema、水位或命令表；同域 `/api/v1/xingqiao` 路由返回 404。后台 release qualification timer 未启用且 service 失败，最近 updater 记录仍为失败。当前业务容器保持 healthy，审计全程未写数据库、未重启、未部署、未切流量。

**本轮生产更新汇总与工作区清理（2026-08-09）：** 状态：已完成。已基于成功发布的生产提交 `3fb79f5291961a99a50d13b3306937a8db156b04` 形成更新汇总与可恢复清理记录；15 个已并入、干净的非 `main` worktree 已删除，9 个对应已合并本地分支已通过 `git branch -d` 安全删除，3 条已缺失的临时 worktree 注册已 prune。当前 Git 仅保留根目录 `main` 和任务“制定上游账号测试方案”的 `.worktrees/gpt-group-baseline-apply`（`codex/gpt-group-baseline-apply`）；该保护工作区的 14 个异常 GPT 账号补跑登记仍完整未提交。清理后公网 `/healthz` 与 `/readyz` 均通过。完整证据见 `docs/superpowers/reports/2026-08-09-production-update-and-worktree-cleanup.md`。

**本轮非 `main` 工作区整合与发布冻结（2026-08-09）：** 状态：准备完成，等待用户生产部署指令。全部本地分支相对 `main` 的独有提交数均为 0；已并入 GPT 分组基线、Resend SMTP 诊断、Sub 上游实际成本、监控当前配置历史边界及其交叉兼容修复。并版回归修复了 `upstream_request_id`、`account_cost` 与韧性尝试元数据在最新版 UsageLog SQL 中的列序接线，并为生产迁移集合 `9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0` → `1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98` 增加唯一精确维护 allowlist；未知、未授权或退休哈希仍在停服前失败。候选仅完成本地并版、发布资格和远程推送准备，生产尚未迁移、重启、切换或写入；后续仅按冻结证据执行发布，并保留 release identity、镜像/迁移一致性、健康检查和失败回滚最低门禁。

**本轮整合登记（2026-08-09）：** 将获批上游韧性分支 `e55fc3c9e` 合并至 `main`，合并提交 `d8cdc50d4`。Ent 已重新生成；service/handler 定向回归、编译检查、发布控制器测试通过。已 SSH 核查生产健康与日志，当前运行版本仍为 `release-abb87a0a…`。蓝绿发布已完成候选构建，但生产门禁以 `migration_set_changed` 阻止切换（新增 `200_usage_log_attempt_reconciliation.sql`，需要单独的维护授权）；未发生生产变更，状态：进行中。

**本轮任务登记（2026-08-09）：** Resend SMTP 受控复现与 deadline 诊断，状态：进行中。当前工作区为 `.worktrees/resend-smtp-timeout`（`codex/resend-smtp-timeout`）。范围严格限定为向同一现有管理员邮箱再发送一封测试邮件，同步记录 Sub2API 接口状态/耗时与 Resend 事件/配额；仅在再次复现约 20 秒超时时才按 TDD 设计 SMTP 滑动 I/O deadline 修复。本轮不改动邀请码、CAPTCHA、SMTP 配置、DNS 或 Resend 套餐，不新建用户，不打开或消费 reset token；提供商事件和生产回归未闭合前不得标记完成或开放无邀请码注册。

**本轮任务登记（2026-08-10）：** 合并全部已实施并停在发布前的可合并非 `main` worktree 到 `main`，完成合并后专项回归、构建/类型检查、迁移与发布预检并推送远程；用户随后明确授权停机部署。`main@300754c0e01bfaf04bda4fd7e9af0f032cfab9db` 已与 `origin/main` 一致，生产发布通过本地/宿主蓝绿链完成，宿主返回 `downtime_required=false`、`result=succeeded`、`active_slot=blue`、`active_upstream=sub2api-blue:8080`，运行镜像为 `ghcr.io/leesssong/xingqiao-sub2api:release-300754c0e01bfaf04bda4fd7e9af0f032cfab9db-4596d02d163929967f21a22c72ecf0a448714d77abd14a075d553aaa7921b27f`。公网 `https://api.xingqiaolab.top/healthz` 与 `/readyz` 均返回 200；生产 `release-state` 与最新记录均绑定 source commit/tree、迁移哈希 `fadb98d43e3d8e8b41178203638912cc32592a1368091e4cb44399926daead5d`，状态为 `succeeded/promoted` 且 `rolled_back=false`；PostgreSQL、Redis、Caddy 容器身份保持不变，应用与 worker 均 healthy。发布资格证据为 `release-evidence/sub2api-release-ready-300754c0e.json`，宿主发布记录为 `/var/lib/sub2api/release-records/20260810T044955Z-production-2917055.json`。根目录既有未跟踪发布证据和用户点名的“新建运营界面”“优化账号卡片”保护 worktree 均保留；已成功候选 `/private/tmp/sub2api-release-run.lEOSD1` 可清理。状态：已完成。

**本轮任务登记（2026-08-10）：** 排查账号监控卡片疑似仅每个分组前两个账号能悬浮展示评分明细与账号成本来源的问题，状态：准备完成（非终态，尚未实施修复）。真实页面已确认所有卡片均渲染评分/成本 Tooltip 且后续卡片 `mouseenter` 正常触发；共同 `HelpTooltip` 使用 `position: fixed`，却把 `window.scrollY/window.scrollX` 叠加到 `getBoundingClientRect()` 的视口坐标，滚动后将 Tooltip 推出视口。范围仅为只读复现、DOM/组件事件与定位层级分析；本轮未修改业务实现、未切换或回滚生产。当前工作区为根目录 `main`；“新建运营界面”和“优化账号卡片”继续作为保护例外，保留全部现有未提交内容。

**本轮任务登记（2026-08-10）：** 排查并修复管理员将 `GPT-Plus` 分组 RPM 更新为 `0（不限制）` 时出现“更新分组失败”，并分析中转站对上游 overloaded、平台/本站并发限制、泛化 upstream stream failure 三类错误的可控措施；状态：已完成。根因是管理页清空数字输入框后 Vue `v-model.number` 产生空字符串，`PUT /api/v1/admin/groups/6` 在 Go JSON 绑定阶段返回 400，未进入服务、数据库或缓存更新。永久修复提交 `db86c8bf9` 在创建/编辑 payload 边界把空值归一化为 `0`，含 helper 与表单级回归测试；合并后的 `main@cfae5d5d10e7f1a6783a28d299848109408fa4c7` 已推送并通过预加载镜像蓝绿发布至生产 green，宿主返回 `downtime_required=false/result=succeeded`，迁移哈希保持 `1f47135f…`。公网 `/healthz`、`/readyz` 均为 200；数据库确认 `groups.id=6/GPT-Plus/rpm_limit=0/rate_multiplier=0.2000/status=active`；已登录生产管理页实测清空 RPM 后更新成功，重新打开显示 `0`。PostgreSQL、Redis、Caddy 未因发布重建，倍率、账号绑定、调度强度、用户级限额、路由和计费保持不变；本轮专用 `.worktrees/fix-group-rpm-unlimited` 与 `.worktrees/rpm-production-release` 已在确认干净且可由 `main` 恢复后删除，对应本地分支已安全删除，三个既有活动 worktree 均未修改、未清理。

**本轮诊断收口（2026-08-10）：** 状态：准备完成（非终态，未部署修复）。生产只读证据确认：`cad40d1b-b46a-4d0b-9878-9a7636cc6780:1` 是上游已输出部分流后返回 overloaded、无 usage 的 `usage_completeness=unknown` 失败尝试审计行，本站实际扣费为 `$0`，不属于成功请求被计为 0；该请求没有对应 `ops_error_logs` 行，当前 UI 元数据仍显示为普通同步零时长记录，且 `upstream_request_id` 是合成 attempt ID。两个 Codex 线程的 429 均已写入生产 `ops_error_logs`（00:46:21、00:49:25、00:49:27、00:49:30、00:49:59，含 `/responses` 与 `/v1/chat/completions`），错误码/消息为 `GROUP_RPM_EXCEEDED / group requests-per-minute limit exceeded`，`user_id=1/api_key_id=51/group_id=6/GPT-Plus/rpm_limit=3`；它们发生在账号选择和 usage 写入之前，因此无 `account_id`、无 `usage_logs`、无上游请求。错误列表默认 `view=errors` 通过 `COALESCE(is_business_limited,false)=false` 排除这些业务限制行，只有“已排除/全部”视图及业务限制汇总能看到。当前未修改生产数据、限额、路由或代码；后续修复应集中在失败尝试元数据、真实上游 request ID 语义，以及把请求前拒绝以可检索事件呈现，而非直接放宽生产 RPM。

**本轮登记（2026-08-10）：** 请求 `cad40d1b-b46a-4d0b-9878-9a7636cc6780:1` 显示 0 Token、0ms、本站实际扣费 `$0` 的计费异常，以及 Codex 任务“制定上游账号测试方案”“优化更新机制”多次 429 并发限制却未出现在 Sub 原生错误监控和使用记录中的生产只读根因排查，状态：进行中。范围为对齐 Codex 任务失败时间/平台请求 ID、Sub2API handler 并发门禁、错误采集、usage log 创建和扣费写入顺序，确认零值流水与监控漏记是否同源；本轮不修改生产数据、用户/账号并发、余额、计费、路由、账号或容器，不发送额外付费探测。当前工作区为根目录 `main`；已盘点全部非 `main` worktree，`.worktrees/fix-official-update-stuck` 对应用户点名的活动任务“优化更新机制”，`.worktrees/gpt-group-baseline-apply` 对应仍在进行的上游账号任务，均作为本轮保护例外保留，不合并、不修改、不清理。

**本轮登记（2026-08-09）：** 账号监控卡片“推荐分组”与测试组定期打标闭环，状态：进行中（实施计划已完成，等待执行方式选择）。目标是新账号先进入 `GPT-测试分组`，由既有主动探测/账号监控链路定期评估利润、可调度状态、最新错误、样本量、成功率和延迟，生成只读的“推荐分组”标签；管理员看到后仍通过现有 Sub2API 分组编辑能力人工迁移，不自动改组。用户已确认采用实时派生方案、质量证据仅用主动探测；推荐固定使用滚动 7 天探测窗口；测试组显示推荐/继续观察/暂缓/不建议，正式组仅对正常可调度账号分析且只有明确推荐其他正式档位时显示同行叹号；Codex Auth 正常时默认公开 Pro 与专属 Pro，异常测试组账号暂缓迁入。卡片只在现有平台/当前分组/调度状态同行追加字段，不新增区块或拉长正常布局。正式设计为 `docs/superpowers/specs/2026-08-09-account-monitor-group-recommendation-design.md`，实施计划为 `docs/superpowers/plans/2026-08-10-account-monitor-group-recommendation.md`。本轮不修改生产、账号分组、调度、评分权重或数据库。当前工作区为根目录 `main`；已先整合完成且领先 `main` 的 GPT 分组补跑文档提交 `03c20f370`，保留 `.worktrees/fix-official-update-stuck` 的未提交总账改动及根目录未跟踪发布证据，不清理、不覆盖。
**本轮官方更新“候选版本准备中”卡住修复（2026-08-09）：** 状态：进行中。用户在管理员界面从当前 `0.1.170` 升级至官方 `0.1.173` 时，弹窗持续显示“候选版本准备中”且“现在升级”不可用。证据已确认：生产 updater 进程与硬盘上均为 2026-08-07 旧版（SHA-256 `c9c5e2ab…`），2026-08-09 只安装了新 candidate preparer（SHA-256 `7c4c561d…`），形成版本断裂；且宿主并无 `xingqiao-sub2api:upstream-0.1.173` 合格镜像，所以修复更新器后还需按本地/宿主发布链生成候选。本轮同时以 TDD 补齐上游账单失败原因结构化返回和管理页提示：账号 `#42 Plus-WAWAZZ` 的 `/v1/usage/records` 返回 HTTP 404，不会用估算值填充上游实际扣费。当前工作区为 `.worktrees/fix-official-update-stuck`，分支 `codex/fix-official-update-stuck`；“制定上游账号测试方案”的 `.worktrees/gpt-group-baseline-apply` 及其未提交内容作为保护例外，不修改。

**本轮生产更新汇总与工作区清理（2026-08-09）：** 状态：已完成。已基于成功发布的生产提交 `3fb79f5291961a99a50d13b3306937a8db156b04` 形成更新汇总与可恢复清理记录；15 个已并入、干净的非 `main` worktree 已删除，9 个对应已合并本地分支已通过 `git branch -d` 安全删除，3 条已缺失的临时 worktree 注册已 prune。当前 Git 仅保留根目录 `main` 和任务“制定上游账号测试方案”的 `.worktrees/gpt-group-baseline-apply`（`codex/gpt-group-baseline-apply`）；该保护工作区的 14 个异常 GPT 账号补跑登记仍完整未提交。清理后公网 `/healthz` 与 `/readyz` 均通过。完整证据见 `docs/superpowers/reports/2026-08-09-production-update-and-worktree-cleanup.md`。

**本轮非 `main` 工作区整合与发布冻结（2026-08-09）：** 状态：准备完成，等待用户生产部署指令。全部本地分支相对 `main` 的独有提交数均为 0；已并入 GPT 分组基线、Resend SMTP 诊断、Sub 上游实际成本、监控当前配置历史边界及其交叉兼容修复。并版回归修复了 `upstream_request_id`、`account_cost` 与韧性尝试元数据在最新版 UsageLog SQL 中的列序接线，并为生产迁移集合 `9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0` → `1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98` 增加唯一精确维护 allowlist；未知、未授权或退休哈希仍在停服前失败。候选仅完成本地并版、发布资格和远程推送准备，生产尚未迁移、重启、切换或写入；后续仅按冻结证据执行发布，并保留 release identity、镜像/迁移一致性、健康检查和失败回滚最低门禁。

**本轮整合登记（2026-08-09）：** 将获批上游韧性分支 `e55fc3c9e` 合并至 `main`，合并提交 `d8cdc50d4`。Ent 已重新生成；service/handler 定向回归、编译检查、发布控制器测试通过。已 SSH 核查生产健康与日志，当前运行版本仍为 `release-abb87a0a…`。蓝绿发布已完成候选构建，但生产门禁以 `migration_set_changed` 阻止切换（新增 `200_usage_log_attempt_reconciliation.sql`，需要单独的维护授权）；未发生生产变更，状态：进行中。

**本轮任务登记（2026-08-09）：** Resend SMTP 受控复现与 deadline 诊断，状态：进行中。当前工作区为 `.worktrees/resend-smtp-timeout`（`codex/resend-smtp-timeout`）。范围严格限定为向同一现有管理员邮箱再发送一封测试邮件，同步记录 Sub2API 接口状态/耗时与 Resend 事件/配额；仅在再次复现约 20 秒超时时才按 TDD 设计 SMTP 滑动 I/O deadline 修复。本轮不改动邀请码、CAPTCHA、SMTP 配置、DNS 或 Resend 套餐，不新建用户，不打开或消费 reset token；提供商事件和生产回归未闭合前不得标记完成或开放无邀请码注册。

**本轮受控复现结论（2026-08-09）：** 状态：进行中且 fail-closed。唯一授权的测试邮件 POST 在生产策略漂移于请求进行中审计时被发现后由客户端中止；确切请求开始时间戳、客户端 request ID 和错误类别均不可用，未发生重试。只读审计仍证实服务端完成：HTTP `400`、`20,191 ms`，完成时间 `2026-08-09T09:19:52Z`。Resend Emails 或 Usage 未观察到 provider acceptance（无新的 test 事件、Transactional 用量仍为月 `2/3000`、日 `1/100`）；Metrics 保持 0% 退信/投诉，Resend 发信域名状态为 `verified`。健康检查、全量 Admin settings SHA-256、受保护容器 ID 和 restart count 均未变化。`invitation_code_enabled=false` 为有意保留的用户策略，且所有 CAPTCHA 均关闭；因缺少 deadline gate 所需的客户端 timeout 错误类别，决策为 `BLOCKED_OTHER`；不得重发邮件、不得启动 Task 2/3、不得修改生产配置。证据：[`Resend SMTP 受控复现`](../superpowers/reports/2026-08-09-resend-smtp-controlled-reproduction.md)。

**本轮登记（2026-08-09）：** 管理员用量详情字段去重及 Sub 上游真实扣费自动查询，状态：进行中。用户已确认唯一账务口径为“利润 = 本站实际扣费 - 上游 Sub 实际扣费”。范围为删除管理员信息区重复的“本站请求 ID”，补齐 OpenAI 网关对上游 `x-request-id` 的持久化和 Sub 原生 `/v1/usage/records` 对 `upstream_request_id` 的返回映射，并由本站后台自动复用请求所用上游账号（当前目标为账号 `#164`）已保存的 `base_url` 与 `credentials.api_key` 查询对应上游 Sub 流水的 `actual_cost`；不新增账单授权、账号映射或五类账务数据前置条件，不以估算成本冒充上游真实扣费，不回填历史流水。当前工作区为 `.worktrees/usage-upstream-actual-cost`、分支 `codex/usage-upstream-actual-cost`；已盘点全部已登记非 `main` worktree，没有已完成且领先 `main`、必须先合并的候选，“新建运营界面”和“优化账号卡片”继续作为保护例外。尚未合并、推送、部署或完成线上验证，不能标记完成。

**本轮设计确认（2026-08-09）：** 用户否决 relay-ops 额外账单授权、账号映射和五类账务数据前置条件，明确要求本站自动使用账号已存 API Key 查询上游 Sub 原生逐笔流水。正式设计与三任务实施计划已写入 `docs/superpowers/specs/2026-08-09-sub-upstream-actual-cost-design.md` 和 `docs/superpowers/plans/2026-08-09-sub-upstream-actual-cost.md`：先补齐双端请求 ID 合同，再新增管理员专用的有界上游查询与后端利润计算，最后前端去重并仅展示两笔真实扣费及利润；上游不可达或精确匹配失败时不估算。状态保持进行中，进入 Task 1。

**本轮实施登记（2026-08-09）：** 修复生产 `/monitor` 只展示当前启用且绑定有效活动分组的渠道监控，并为监控配置身份性更新建立新的统计边界，避免旧端点、旧凭据、旧模型或旧分组历史污染更新后的 7 天可用率与时间线；状态：进行中。用户已批准快速实施、合并 `main`、推送并走无停机蓝绿生产覆盖更新。独立工作区为 `.worktrees/monitor-v2-current-config`、分支 `codex/monitor-v2-current-config`，基线为 `main@a90c07427`；全部非 `main` worktree 已盘点，没有已完成且必须先合并的领先候选，明确排除仍在进行或阻塞的调度、GPT 分组分析、SMTP 超时以及根目录发布器未提交内容。

**本轮任务登记（2026-08-09）：** Resend Free 邮件配置 Task 2 生产激活，状态：进行中。范围仅为通过官方 Sub2API Admin API 设置 `frontend_url=https://api.xingqiaolab.top`、启用邮件验证与密码重置；保持邀请制、CAPTCHA、SMTP、OAuth、计费、通知及受保护容器身份不变。当前工作区为 `.worktrees/resend-email-configuration`（`codex/resend-email-configuration`）；尚未推送、部署或完成线上邮件投递验证，不能标记完成。

**本轮 Task 2 实施与审查（2026-08-09）：** 已通过官方 Admin API 的一次三字段部分更新设置 `frontend_url=https://api.xingqiaolab.top` 并启用邮件验证和密码重置；Admin API、只读 PostgreSQL 和公共 flag 验证通过，`/healthz` 为 200，API/worker/PostgreSQL/Redis/Caddy 身份和重启计数未变，未发生重启或部署。公共 settings DTO 不暴露 `frontend_url`，该字段由 Admin API 和 PostgreSQL 证实。Task 2 独立审查已批准，Task 3 已获授权进行投递和找回密码流程线上验证；总任务状态保持进行中。

**本轮 Task 3 线上验证（2026-08-09）：** 已向首个活跃管理员邮箱发送且仅发送一次 Sub2API 测试邮件；接口因 SMTP 连接超时返回 HTTP 400，但用户明确确认该管理员邮箱实际收到测试邮件，两层证据均已保留且不互相替代。对同一邮箱的忘记密码请求返回通用 HTTP 200 反枚举响应，脱敏日志记录入队和 worker 发送；Admin API、公共 settings 和只读 PostgreSQL 复核了 `frontend_url`、邮件开关、邀请制、CAPTCHA 与白名单数量，容器身份/重启数和健康检查未变。Resend 域名仍为 verified，但活动/额度页两次超时，两个事件的直接 provider 状态、额度影响、退信/投诉均未验证；配置保持邀请制启用状态，不能批准无邀请码公开注册，任务总状态保持进行中。证据：[`Sub2API 邮件生产验证`](../superpowers/reports/2026-08-09-sub2api-email-production-verification.md)。

**本轮 Task 3 独立复核（2026-08-09）：** 审查结论：批准配置继续用于现有邀请制生产流程；阻断无邀请码公开注册及“provider 全链路已验证”结论，总任务状态保持进行中。已独立复核提交 `51cb93dc7`、Task 3 报告、正式生产验证报告及 `59a769ae2..51cb93dc7` 审查包：文档只记录同一现有管理员邮箱的一次测试邮件请求和一次找回密码请求，没有新增用户、额外收件人、验证码/token 读取、链接打开或 token 消费；三份任务报告和生产报告的定向扫描未发现完整邮箱、凭据或消息内容。审查修正了 Admin API 认证与 SMTP 认证混层口径：SMTP 连接超时不能证明 SMTP 认证成功，worker success 也只属于应用层证据，Resend 事件、额度、退信和投诉仍未验证。源码核对确认 reset URL 从已验证的 `frontend_url` 拼接、token 仅由独立 reset-password 路径消费，邀请制在创建用户前执行；CAPTCHA 仍关闭，因此批准以邀请制持续启用为前提。本轮仅修改并提交审查文档，未触碰生产；工作区盘点确认 Resend 候选原本干净且领先 `main`，调度候选仍有未提交改动且为总账记录的活动排除项，`main` 的发布器与证据改动以及“新建运营界面”“优化账号卡片”保护例外均保持不动。

**本轮 Resend whole-branch 最终审查（2026-08-09）：** 结论：`APPROVE`，仅限现有邀请制生产流程；`BLOCK` 无邀请码公开注册及“provider 全链路已验证”结论。已复核 `15cf3303d..1656be657`、计划、三份任务报告、三份正式报告、SDD/全局总账、冻结审查包的变更行集合及源码不变量。发现并修正两处文档状态/措辞漂移：Task 2 “待独立复核”已改为已复核，Task 1 “未记录 billing amount”已改为未记录支付方式/发票标识/付费购买。未发现凭据、完整邮箱或 token 泄露；三字段 Admin API 回滚载荷存在且不需直接写库。源码确认邀请码在普通注册创建用户前校验、reset URL 从已验证 `frontend_url` 拼接，CAPTCHA 仍关闭。SMTP 连接超时、Resend 事件/额度/退信/投诉仍未验证，故总账继续保持“进行中”；本轮仅修改并提交文档，未触碰生产。

**本轮登记（2026-08-09）：** 开放注册前的 Sub2API 域名邮箱可行性与配置调研，状态：进行中。范围为盘点 Sub2API 注册验证/找回密码等邮件能力、当前生产配置与域名 DNS 条件，并形成可执行的邮件服务商、发信域名、SMTP/DNS、安全和上线验证建议；本轮默认只读调研，不修改生产配置、不开放注册、不发送真实用户邮件。当前工作区为根目录 `main`，保留“新建运营界面”“优化账号卡片”保护例外，以及现有发布器修复和发布证据改动。

**本轮登记（2026-08-09）：** 客户 `client:56eb09d6-14c5-4942-986d-574f5da98e41` 经 SHUAI（账号 `#135`）调用 `gpt-image-2 /v1/images/edits` 的上游成本异常排查，后续范围扩大到“生图”分组全部上游账号，状态：进行中。范围仅限只读核对生产请求、用量、账号定价配置和成本写入链路；本轮不修改计费逻辑、不部署，当前工作区为根目录 `main`，并保留“新建运营界面”“优化账号卡片”保护例外及现有未跟踪发布证据。

**本轮排查结论（2026-08-09）：** 目标请求生产流水 `usage_logs.id=59682` 仅生成 1 张图，输入尺寸 `1024x1365` 被归类为 `2K`；因“生图”分组未绑定渠道、生产无账号统计自定义规则且 `account_stats_cost` 为空，账号成本回退为客户标准成本 `$0.134 × 2K 系数 1.5 × 账号倍率 1.0 = $0.201`，并非重复按 4 张计费。用户已确认 SHUAI 真实成本为 `$0.05/张`；账号 `#135` 的 44 条图片流水错误记录 `$8.643`，按固定价应为 `$2.20`，累计高估 `$6.443`。

**本轮全分组排查（2026-08-09）：** “生图”分组已有图片流水的 `moss/TK/SHUAI/小红豆/XN` 共 5 个账号、92 条流水，`account_stats_cost` 全部为空，记录账号成本合计 `$14.941`；尚无流水的虎云/runapi/CX 也处于同一缺口。共同根因是上游自动计费探测协议仅接受 `billing_scope=token` 并只同步 Token 倍率到通用 `accounts.rate_multiplier`，没有图片按张价格字段；图片流水却同样复制该通用倍率。固定图片成本本应由账号统计自定义按张规则写入 `account_stats_cost`，但“生图”分组渠道绑定数为 0、生产账号统计规则数为 0、分组图片单价为空，导致全部回退为本站标准图片价格。SHUAI/虎云探测分别为 404 unsupported/301 failed，其余所谓探测成功也仅证明 Token 倍率可读，不代表图片价格已同步。其他账号的真实按张价格无法由本站流水反推，需各自上游账单或明确价目作为修复输入；状态保持进行中，未修改生产配置、数据或代码。

**本轮设计确认（2026-08-09）：** 用户确认“生图”分组所有现有及未来账号从新请求开始统一按 1K `$0.06`、2K `$0.08`、4K `$0.10` 记录上游固定成本，不回补现有 92 条历史流水，也不改变客户售价。独立任务工作区为 `.worktrees/image-account-fixed-cost`、分支 `codex/image-account-fixed-cost`；后端 `go test ./...` 基线通过，正式设计规范已写入 `docs/superpowers/specs/2026-08-09-image-account-fixed-cost-design.md`，待书面复核后进入实施计划，状态保持进行中。

**本轮设计复核（2026-08-09）：** 自查发现现有报表统一按 `COALESCE(account_stats_cost, total_cost) * account_rate_multiplier` 聚合，账号配额扣减和通知也独立使用 `total_cost * account_rate_multiplier`；因此仅让图片规则写入固定 `account_stats_cost` 仍会被通用 Token 倍率二次缩放，无法保证最终上游成本固定。修复设计需显式区分“倍率前账号统计成本”和“已解析最终账号成本”，并将同一最终值贯穿用量流水、账号配额、通知及盈利/统计查询；不得通过篡改倍率快照或按倍率反向除价规避。设计待用户确认后更新规范，未进入实现，状态保持进行中。

**本轮最终成本方案确认（2026-08-09）：** 用户选择方案 A：新增可空 `usage_logs.account_cost` 作为最终账号成本快照，新图片固定档位成本直接写入且不应用账号 Token 倍率；账号配额扣减、通知、盈利统计和管理员报表统一消费该值，历史空值继续兼容旧公式且不回补。设计规范已据此更新，待书面规范复核后进入实施计划，状态保持进行中。

**本轮实施登记（2026-08-09）：** 已确认“生图”分组固定上游成本实施计划并建立候选工作区 `codex/image-account-fixed-cost`。范围为所有现有及未来账号、仅新请求生效：1K `$0.06`、2K `$0.08`、4K `$0.10`；不回补历史流水、不改变客户价格、路由或调度。当前进入 Task 1 持久化实现，状态：进行中；尚未合并、推送、部署或线上验证。
**本轮 Task 1 实现与审查（2026-08-09）：** 已在候选工作区完成 `usage_logs.account_cost NUMERIC(20,10) NULL` expand-only 迁移、UsageLog 字段、单条/批量/best-effort/无返回写入路径与查询扫描，并补充固定成本、历史 `NULL` 兼容和批量参数位次断言。实现提交 `7a125d8c2`，审查修复提交 `b44715ad5`；聚焦 unit 测试、全量 repository unit 与 `go vet ./internal/repository` 已通过。集成测试因当前环境缺少 rootless Docker 未能启动，待具备 PostgreSQL/testcontainers 环境重跑；任务进入下一阶段，尚未合并、推送、部署或线上验证。
**本轮 Task 2 实现（2026-08-09）：** 已增加结构化账号成本解析：严格按 `ClassifyImageBillingTier` 匹配 1K/2K/4K 图片区间，固定图片成本不应用账号倍率；Token、普通按次和模型文件成本保留倍率语义；未知档位继续旧回退。OpenAI 两条 usage 记录路径均写入新请求的 `account_cost` 快照。实现提交 `9d8830687`，图片计费模式守卫修复提交 `008951c23`；聚焦账号统计/记录用量测试、`go vet ./internal/service` 与 diff 检查已通过。尚未合并、推送、部署或线上验证，状态：进行中。
**本轮 Task 3 实现（2026-08-09）：** 统一扣费命令、legacy 兜底扣费和账号配额通知已改为消费解析后的最终 `account_cost`；无快照调用者继续回退 `total_cost × account_rate_multiplier`，客户余额、订阅、API Key 配额与限流继续使用 `actual_cost`。聚焦扣费测试和 `go vet ./internal/service` 已通过；数据库集成测试仍受本机 rootless Docker 缺失限制，尚未合并、推送、部署或线上验证，状态：进行中。
**本轮 Task 4 实现（2026-08-09）：** 账号统计、趋势、分组/用户聚合、仪表盘、小时聚合和账号盈利支出已统一为 `account_cost` 优先、历史空值回退旧公式；管理员 DTO/用量表/tooltip/导出新增最终账号成本，普通用户 DTO 继续隐藏全部账号成本字段。相关 backend unit 套件、20 项前端定向测试、前端类型检查和 Go vet 已通过；尚未合并、推送、部署或线上验证，状态：进行中。
**本轮 Task 5 实现（2026-08-09）：** 已增加默认只读、仅显式 `--apply` 写入的生产配置脚本；写模式使用单个 SERIALIZABLE 事务、表锁和冲突守卫，绑定唯一“生图”分组并配置 1K/2K/4K 固定成本，不写客户定价、不改账号倍率或历史流水。shell 合同测试和 Bash 语法检查通过；当前环境无 `shellcheck`，尚未执行生产数据库检查/写入，状态：进行中。

**本轮生产发布（2026-08-09）：** 状态：进行中。范围为当前 `main@e639cd494` 的 Sub2API 官方核心与星桥定制外置实现，以及“优化账号卡片”最终悬浮修复提交 `8562ca848774a28969793a9135fc9155aad3c94f`；明确排除“优化调度” worktree `/Users/gongtengxinwen/Documents/sub2api-upstream-resilience-spec` 的 8 个领先提交和当前未提交改动。发布必须先将账号卡片提交移植到 `main`，在合并后的 `main` 完成专项回归、构建、迁移/发布预检后推送并走本地/宿主蓝绿链；如发生不可用，必须在 5 分钟内完成切换或回退。只有推送、部署和线上验收全部通过后才能标记完成。

**本轮发布器修复（2026-08-09）：** 在生产发布前发现 host executor 的维护停服路径仍缺少单次操作 watchdog、部分停服失败时的恢复武装，以及回滚前旧 API readiness 的真实性校验；状态：进行中。修复必须先以 TDD 红灯复现，再由独立代理范围复审；修复只更新发布器脚本和回归测试，不重启或重建当前生产应用，不触碰“优化调度”保护 worktree。

**本轮发布器修复 Round 2（2026-08-09）：** 已按 RED→GREEN 完成每个停服后命令的剩余维护窗口 watchdog、stop 执行前恢复武装、旧 API/worker 镜像与健康证明、公共 API readiness、共享 PostgreSQL/Redis/Caddy 身份复核及 truthful rollback record。聚焦 maintenance 套件、完整 host executor matrix、Bash 语法和 diff 检查均通过；正式实现报告为 [`docs/superpowers/reports/2026-08-09-host-executor-round2-implementation.md`](../superpowers/reports/2026-08-09-host-executor-round2-implementation.md)。尚待独立范围复审、推送及仅更新生产 host executor，状态保持进行中。

**本轮账号卡片整合（2026-08-09）：** 已在 `main` 记录候选提交 `8562ca848774a28969793a9135fc9155aad3c94f` 的空 cherry-pick；原因是等价应用内 `HelpTooltip` 补丁已由祖先提交 `a0aae015a543f390d5e7bd0dd48ccb6a14421454` 整合，当前树保留其后续探测口径更新。`AccountMonitorCard` 定向测试（27 项）和前端类型检查已通过；尚未推送、部署或线上验证，发布状态保持进行中。独立审查另记录键盘无法触发该悬浮层的 P2 可访问性风险，待单独批准处理。

**本轮发布门禁修复（2026-08-09）：** 并版预检发现 Sub2API 新增 outbox 迁移会被宿主现有“迁移集合变化”门禁统一阻断。现已按 TDD 增加显式完整哈希对 allowlist：仅批准生产 `aee795202a3d...` 到本次 expand-only 集合 `5cc825b23a35...`，控制器必须同时收到 `--maintenance-authorized` 和受校验的 `RELEASE_MAINTENANCE_FROM_HASH`；未知/退休哈希仍在构建或停服前失败。宿主与控制器回归、Bash 语法和 diff 检查通过，维护不可用预算保持最多 300 秒；尚未推送、部署或线上验证，状态继续进行中。

**本轮生产推送（2026-08-09）：** 已将验证后的 `main@48c215115` 推送至 `origin/main`；推送前重新抓取远端并确认无并发领先，同时确认“优化调度”分支 9 个领先提交仍全部未合并。当前仅完成推送，尚未构建/暂存最终统一候选、部署或线上验收，状态继续进行中。

**本轮收口（2026-08-09）：** Sub2API 官方核心与星桥定制外置迁移已完成本地实现、合同测试、资格门禁和只读生产预检脚本；管理员使用路径保持同域、同账号、同 2FA、同 URL 和同数据查看语义。当前停在生产部署更新动作之前，尚未推送、部署、迁移、重启、蓝绿提升或线上验收，状态保持进行中。详见 [外置迁移手册](../runbooks/sub2api-externalization-migration.md)、[官方升级手册](../runbooks/sub2api-official-upgrade.md) 和 [生产前验证报告](../superpowers/reports/2026-08-08-externalization-production-verification.md)。

**本轮任务登记（2026-08-09）：** Sub2API 外置定制 Task 1 基线清单、管理员体验合同和运行时清单，状态：进行中（已完成本地文档与契约测试；尚未推送、部署或线上复验，不能标记完成）。实施报告：`.superpowers/sdd/2026-08-08-sub2api-externalized-customization-implementation-plan/task-1-report.md`。

**本轮推进（2026-08-09）：** Task 1 已完成任务级独立审查，Task 2 版本化集成合同与核心事务 Outbox 正在实施；当前仅进行本地实现和验证，尚未推送、部署或线上复验，不能标记完成。

**本轮推进（2026-08-09）：** Task 2 已完成本地实现、聚焦测试、`go vet` 和迁移静态审查；Task 3 控制面事件消费、幂等水位和可重建读模型进入实施。尚未推送、部署或线上复验，不能标记完成。

**本轮实施登记（2026-08-09）：** 基于当前 `main` 执行 Sub2API 官方核心与星桥定制外置迁移，状态：进行中。目标是完成本地实现、测试、候选资格、生产预检，并停在生产部署更新动作前；未满足“已推送 + 已部署 + 已验证生效”前不得标记完成。现有两个活动线程及未跟踪发布证据保留，不执行清理或覆盖。

**本轮登记（2026-08-08）：** Sub2API 官方核心与星桥定制外置长期方案，状态：进行中（用户已确认“薄核心适配器 + 外置控制面、管理员无感”方向；正式设计规范已起草，覆盖数据双读、会话隔离、官方升级、数据库迁移门禁和分阶段迁移；尚未开始运行时实施、推送、部署或生产验证，不得标记完成）。

**本轮登记（2026-08-08）：** 非 main 工作区整合与发布流程收口，状态：进行中（已保护“新建运营界面”和“优化账号卡片”两个活动线程所在的当前 main 工作区；其余非 main worktree 正在建立可恢复快照、分类并整合到独立候选，待活动线程完成后再并回 main、推送、部署和线上验证）。

**本轮进展（2026-08-08）：** 已在独立 `codex/worktree-consolidation-20260808` 候选完成全部已注册工作区盘点；当前候选已快进并入根目录 `main`，未读取或归档根目录未跟踪的发布证据。其余可读取的非 main 脏工作区均已保存二进制 diff、暂存 diff 与未跟踪文件恢复包；一条已 prunable 且目录不存在的历史 worktree 已明确记录为不可读取，待后续按 Git 元数据处理。

**本轮推送（2026-08-08）：** 当前 `main` 提交 `f9d344171` 已推送到 `origin/main`；尚未执行生产部署与线上生效验证，整合事项状态保持进行中。`优化调度` 与 `优化更新机制` 两个保护线程继续独立保留，不阻塞本次推送。

**本轮流程约束落地（2026-08-08）：** 已将“新任务先登记总账、实施前扫描领先的非 main worktree、完成项先合并 main、部署前并回 main 并版回归、从 main 推送部署、线上验证后再删除 worktree、失败候选保留并循环修复、仅用户明确点名的活动 worktree可保护”写入 `AGENTS.md`。该约束提交已随当前 `main` 推送；整合任务仍需部署和线上验证，状态保持进行中。

**保护线程状态变更（2026-08-08）：** `优化账号卡片` 线程在暂停指令到达前已通过其独立发布 worktree `8562ca848774a28969793a9135fc9155aad3c94f` 切换生产至 blue；该切换不属于本整合候选的发布，不将其视为整合任务已完成。线程已暂停后续切换、回滚和清理；应用内悬浮层尚未完成生产页面在线验收，相关 worktree 与证据必须保留，待后续明确窗口再处理。
**更新时间：** 2026-08-08

**本轮登记（2026-08-09）：** 上游故障缓存感知恢复 Task 5（客户恢复合同、管理员账号模型运行态处置、结构化观测与告警输入），工作区 `codex/upstream-resilience-implementation@c3c1b384c`；已盘点全部非 `main` worktree 与其提交/状态，`codex/image-account-fixed-cost` 和 `codex/resend-email-configuration` 保留各自既有工作区，当前任务严格基于获批的 Task 4 基线实施。修复轮次 3/5：正在补齐租户/分组隔离的恢复排除生命周期、真实 failover/billing reconciliation 事件、完整事件 schema 与 count/ratio 告警语义；尚未合并、推送、部署或线上验证。状态：进行中。

**本轮登记（2026-08-08）：** 上游故障的缓存感知调度与流式恢复，状态：进行中（用户已确认设计规格与实施计划；当前进入逐任务实现、独立审查、受控蓝绿部署和线上验证。在完成“已推送到服务端 + 已部署 + 已验证生效”前不得标记完成）。

**本轮推进（2026-08-09）：** Task 3 review fix round 5 已完成本地实现、专项验证与独立复审；`openai_advanced_scheduler_enabled=false` 时的 all-cooldown half-open fallback 已确认复用完整常规资格、freshness、DB recheck、并发槽和利润终检链，且维持单租约、无 WaitPlan、幂等完成/释放。Task 4 首轮独立审查发现 unknown usage 占用扣费幂等键、unknown 非零 token 仍可能扣费，以及 attempt/reconciliation 字段未持久化；当前处于修复轮次，正在补齐无扣费审计分支、usage-log 迁移/读写链和真实仓储集成验证。状态保持进行中；尚未推送、部署或线上验证。

**本轮登记（2026-08-08）：** 账号监控卡片统一探测口径及四项交互增强，状态：进行中（用户已确认账号卡片主指标、成功率、TTFT、总耗时、评分和证据全部以主动探测为唯一来源；同时加入成本来源警示、账号名称上游外链、评分构成悬浮展示。仅允许相关专项测试、构建、蓝绿无停机部署和线上页面/API 验证；尚未推送、部署或线上验证，不得标记完成）。

**本轮登记（2026-08-08）：** 账号运营损益页已完成本地实现并整理到提交，状态：已完成（提交 `d75eaf1de5911887bb2738c8527cb1cb28b58361` 已通过预加载镜像蓝绿部署；发布结果 `downtime_required=false`；生产 `/healthz`、`/readyz`、页面入口返回 200，运营损益 API 未认证返回 401，worker healthy，PostgreSQL/Redis/Caddy 身份与重启次数未变化）。后端接口 `/api/v1/admin/operations/account-profitability` 与前端 `/admin/operations/account-profitability` 兼容 sub2api、newapi、自购账号。收入按 `actual_cost`，中继支出按账号真实成本，采购成本按 CNY 单独展示；未配置汇率时自购账号利润/利润率保持待换算，避免混币误算。

**本轮继续登记（2026-08-08）：** 账号盈利页已完成生产部署并通过线上验证；官方升级流程正在收口为“管理员触发候选准备 → readiness 可观测 → 管理员二次确认后蓝绿切换”。生产主机当前仍缺 `/usr/local/libexec/sub2api-candidate-preparer`，不得恢复 GitHub Actions 发布链，也不得在候选未 ready 时执行生产切换；升级机制状态保持进行中。

**本轮实现登记（2026-08-08）：** 候选准备 POST、幂等状态机、失败/目标变化重试、readiness 阶段/失败原因和手动 UI 触发已完成本地实现；管理页刷新后会恢复 preparing/ready/failed 状态，updater 主程序支持注入受控的 root-owned host preparer 命令，未配置时明确返回“候选准备器未配置”，不再静默卡在准备中。后端 `go test ./...`、race、vet、前端 33 项 UI 测试与 updater 打包契约通过；生产 host preparer 实体、候选状态持久化评估、生产凭据、部署和线上验证仍未完成，状态保持进行中。

**本轮登记（2026-08-07）：** 升级弹窗在当前版本已等于目标版本时仍显示残留失败状态，状态：已完成（修复提交 `177bab334` 已推送至工作分支和 `main`，缓存版本 `20260807-2` 已部署；公网 HTML/JS、新文案、`current_version=latest_version=0.1.171`、`has_update=false`、`/health=200`、22 个模型及受保护容器身份/重启次数均已复验；管理员强制刷新后的生产截图显示 `v0.1.171`、绿色成功标记和“已是最新版本”，不再显示残留的“升级失败”状态）。

**本轮推进（2026-08-07）：** 管理页官方升级适配蓝绿生产拓扑已完成。修复提交 `ef6e12560`、`57793c0fd` 已推送；候选 `0.1.171`（提交 `20482a733af8caa40fde277c28c5df35c1ff08b4`，镜像 `sha256:bcb9a659…`）已通过完整资格矩阵、生产暂存和蓝绿切换；更新器已安装并校验，线上版本、健康接口、模型列表和共享容器身份均已复验。

**本轮登记（2026-08-07）：** 管理页官方升级适配蓝绿生产拓扑，状态：已完成（首次旧单容器执行器失败已修复；`0.1.171` 候选通过生产安装、预加载暂存和蓝绿切换；公网 `/health`、管理员 system/version、`/v1/models` 与 worker 健康均已验证，PostgreSQL、Redis、Caddy 容器 ID、启动时间和重启次数未变化）。

**本轮登记（2026-08-07）：** Sub2API `0.1.171` 生产候选登记，状态：已完成（候选源码、镜像标签、二进制版本、`linux/amd64`、迁移哈希和生产不可变暂存均已核对；蓝槽与 worker 已切换并健康运行，线上版本为 `0.1.171`）。

**本轮登记（2026-08-07）：** 升级弹窗当前进度状态可见性修复，状态：已完成（提交 `0829cf0639af13cc6913f129fcfcefe9ade86b4d` 已推送并部署；生产管理页已加载缓存版本 `20260807-1`，公网 JS/CSS 哈希分别为 `b2fb7a62…`、`200be5f4…`，健康检查 3/3 通过；PostgreSQL、Redis、Caddy 容器 ID、启动时间与重启次数均未变化）。

**本轮登记（2026-08-07）：** 账号监控分组切换误退出与成本倒挂提醒可见性复验，状态：进行中（生产审计已确认 relay-ops 经内部 Caddy 校验管理员会话时覆盖原始客户端 IP，触发 `/api/v1/auth/me` 会话绑定不匹配并撤销会话；前端会话恢复隔离已部署，内部 Caddy 客户端身份透传修复已完成实现、回归与独立审查，待推送、部署并以成本接口 200 和生产视觉检查验收）。

**本轮登记（2026-08-08）：** 账号监控「成本折合本站倍率」简化，状态：已完成（提交 `ff5d183a6` 已推送至 `origin/main` 并完成蓝绿生产部署，切换至 green 槽位且 `downtime_required=false`；无数据库迁移。聚焦后端/前端测试、类型检查、生产构建和独立复审通过；线上 `/health` 正常，账号监控 API 返回最新探测模型与 `equivalent_site_multiplier`，旧成本字段未返回，PostgreSQL/Redis/Caddy/Worker 健康且身份未变化）。

**本轮登记（2026-08-08）：** 账号监控「成本折合本站倍率」简化，状态：进行中（用户已确认移除上游原生倍率、当前分组倍率、状态、模型、样本和账单对账展示；改为按最新探测模型价格、账号有效倍率与本站标准价格计算单一折合倍率。已完成本地实现、聚焦测试、类型检查、生产构建和独立复审；尚未推送、部署或生产验证，不得标记完成）。

**本轮登记（2026-08-06）：** 账号监控评分权重入口、账号成本/预计额度与上游余额刷新修复，状态：进行中（原 Tasks 1-4 已完成本地实现和逐任务独立审查；原整体验证与审查曾通过并推送。生产完整发布链随后已同步进候选；最新官方倍率能力复核发现自定义策略和 measurement value 会造成监控与真实计费分叉，现新增 Task 5 原生倍率收敛与数据迁移，Task 6 负责重新整体验证、推送和生产门禁。部署和线上验证前不得标记完成）。

**本轮继续（2026-08-06）：** 已确认根因是候选基于 `origin/main@69caeaf8`，而生产运行独立合格发布链 `9aab62c2`，该生产链有 8 个提交未进入候选。现按用户授权进入“完整同步生产发布链、解决真实冲突、重新审查迁移集合与整分支”的阶段；禁止仅手工补 SQL、跳过生产代码或在复审前部署。

**生产同步结果（2026-08-06）：** 完整生产发布链已通过合并提交 `f64f93e4c` 纳入候选，integration 接口兼容修复 `3016c6951` 已完成独立复审。合并后的迁移集合当时为“生产完整集合 + 197”，规范哈希 `9f341792…`；该哈希已因新增 Task 5 清理迁移而失效，必须在 Task 5 完成后重新计算，当前不得执行生产门禁。

**原生倍率收敛（2026-08-06）：** 用户确认采用官方 `accounts.rate_multiplier + upstream_billing_rate_sync_enabled` 单一语义，不兼容运行时旧策略；新增一次性数据迁移把旧 `manual_override/upstream_managed` 转成官方同步布尔值并删除 `upstream_billing_rate_multiplier_policy` 与 `account_monitor_multiplier_measurement`。独立审查已确认当前候选存在策略冲突和监控/真实计费倍率分叉，现登记为 Task 5 进行中；完成实施、独立复审、整分支复审和推送前不重新执行生产门禁。

**Task 5 实施结果（2026-08-06）：** 原生倍率收敛提交 `ded650e06` 已完成；迁移 198 保留 `accounts.rate_multiplier`、转换官方 sync 开关并直接删除两个旧 JSON 键。生产只读盘点为 74 个有效账号、38 个旧策略（全部 `upstream_managed`）、16 个 measurement、29 个 OpenAI API Key 自动探测账号。后端测试/vet、前端聚焦测试/lint/typecheck/build、diff 与旧符号搜索均通过；当前保持进行中，待推送和生产门禁。

**本轮推进（2026-08-06）：** Task 3 已完成本地实现与独立复审，Task 4 成本弹窗与余额卡片进入实施；本轮仍保持进行中，尚未推送、部署或线上验证。

**本轮登记（2026-08-05）：** 六阶段生产收口独立部署执行，状态：待办（用户暂停）。Task 1 基线收口已完成；Task 2 已部署但尚待用户验收；Task 3 至 Task 9 均未启动。恢复前不得派发实施代理、生成下一任务 brief、修改生产配置或执行部署。执行入口为 [独立部署单元实施计划](../superpowers/plans/2026-08-05-six-stage-production-closure-deployment-units.md)，恢复时所有代理仍须先读取 [代理上下文合同](six-stage-production-closure-agent-context.md) 并报告固定 `CONTEXT_ACK`。

**Task 2 部署登记（2026-08-05）：** 状态：待办（已部署、待用户验收、当前暂停）。提交 `8fef0e03c80a55ec1a1cceedabd1949bf12bfe8b` 已推送到 canonical 分支和 `origin/main`，relay-ops 不可变镜像已部署，生产 accounting 唯一显式保持 `false`；镜像/源码/迁移身份、`/healthz`、`/readyz`、disabled accounting 404、未认证 reconciliation 401 及 PostgreSQL/Redis/Caddy/Sub2API 容器身份不变均已验证。恢复时第一步只能验收 Task 2；验收前不得标记完成，不得启动 Task 3。

**六阶段剩余任务待办登记（2026-08-05）：**

| 任务 | 待办状态 |
|---|---|
| Task 2 relay-ops 账务代码部署（accounting disabled） | 待办：已部署，待用户验收，暂停 |
| Task 3 激活 accounting、账本基线与日调度 | 待办：未启动 |
| Task 4 全站账单能力只读盘点 | 待办：未启动 |
| Task 5 必要平台账单适配器子计划 | 待办：条件任务，未启动 |
| Task 6 真实账单授权、映射与首个非零闭环 | 待办：未启动 |
| Task 7 独立营收页面 | 待办：未启动 |
| Task 8 Monitor 与飞书告警/恢复 | 待办：未启动 |
| Task 9 OpenAI 实际响应模型审计 | 待办：未启动 |

**当前生产交付序列：** 用户已暂停六阶段剩余任务，当前没有活动实施或部署单元。账号监控 V3 已完成；Task 2 保留已部署、accounting disabled、待验收事实并归入待办；Task 3 至 Task 9 全部归入待办。只有用户明确恢复后，协调任务才能从 Task 2 验收门继续。

**本轮登记（2026-08-02）：** 账号监控真实上游成本与对账闭环恢复核验，状态：进行中（仅在 `codex/relay-ops-public-route` 分支执行；先核对生产 Caddy 热配置、健康/认证路由、发布锁和真实只读账单输入；未满足“已推送 + 已部署 + 真实数据验证”前不得标记完成）。

**本轮登记（2026-08-03）：** 账号监控产品验收收口，状态：进行中（账号范围、聚合与首轮卡片修复已推送并部署；生产复验新增确认：监控分组查询遗漏软删除过滤，错误展示 10 个已删除历史/测试分组；账号卡片须限制桌面每行最多 2 个、收起今日调用、评分显示整数并改用通俗证据文案、弱化评分与全局优先级、补齐渠道监控式柱状图和各指标样本数；“纸面利润”统一改为“账号利润”；渠道监控成功探测统一按绿色成功展示并恢复指标样本数；“历史按日”“异常明细”及账号监控其他交互须逐项验证修复。全部重新推送、部署并在线验证前不得标记完成）。

**本轮登记（2026-08-04）：** 账号监控卡片收敛与营收页面职责拆分，状态：已完成（最终提交 `05985e62ec88b04d1e647a815eecdb1cf1155776` 已推送并部署至生产 green；账号监控继续只承接服务质量、评分、排名、成本录入、倍率与并发，营收、利润、账务和对账保持独立页面/后续事项。#118/#119 均已在线确认 `service_state=available`、可评分排名、24 小时失败数为 0，单卡刷新后检查时间同步推进）。

**本轮生产实施登记（2026-08-04）：** 账号监控卡片数据完善 V3，状态：已完成（生产 `release-state` 已更新为 `source_commit=05985e62ec88b04d1e647a815eecdb1cf1155776`、`source_tree=c37b383bf54e485d7393ff0793e30dd03f5e2328`、`migrations_hash=337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76`、`active_slot=green`，运行镜像 ID 为 `sha256:0d10260b745e2086326977303b15f6eb78e8e03de7858fe356dec046bf0e10e8`。PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 身份未变化；历史三个时间窗、7 个分组、66 个账号、迁移与并发验证事实继续有效，最终状态与刷新缺陷已完成线上复验）。

**本轮视觉纠偏登记（2026-08-04）：** 账号监控 V3 设计稿 1:1 还原，状态：已完成（不可漂移合同、独立审查、桌面/移动同视口对比和既有生产卡片验证结论全部保留；本轮状态与单卡刷新缺陷已随最终提交完成生产收口，账号监控 V3 整体交付恢复为已完成）。

**唯一总账：** 本文件是项目事项、部署状态和验证证据的唯一全局入口。

## 统计口径

本次回填审阅了 `docs/superpowers/reports/` 下截至 2026-07-30 的 60 份报告，并结合项目当前状态和 Git 历史，将同一功能的阶段性报告合并为 49 个独立事项。只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才计入“生产工程代码/配置已部署并验证”。只有仍存在运行时代码或生产配置差异，才计入“工程代码/配置差异待部署”；文档、研究、报告、离线证据、历史验证和外部验收本身不构成工程部署差异。事项按独立交付结果合并统计，不按报告文件数量统计；历史暂停或阻塞事项保留在进行中区，不制造完成结论。

## 统计摘要

| 分区 | 独立事项 | 判定 |
|---|---:|---|
| 生产工程代码/配置已部署并验证 | 20 | 已推送、已部署、已验证生效 |
| 工程代码/配置差异待部署 | 10 | 仍存在运行时代码或生产配置差异 |
| 持续实施 | 2 | 工程实现尚未完成，或仍在推进中 |
| 运维/研究跟进（非工程部署差异） | 23 | 文档、研究、历史证据或外部验收跟进 |
| **合并后的独立事项合计** | **55** | **当前总账快照** |

## 当前最重要进行中事项与本轮已完成交付

0.0. **账号监控成本、余额与评分权重修复**：恢复分组评分权重编辑入口；OpenAI API Key 账号统一使用官方 `accounts.rate_multiplier`，OpenAI 非 API Key 账号使用采购成本和预计可用额度计算等效倍率；余额跟随账号健康探测，单卡刷新同时执行官方 billing probe。**状态：已完成（最终提交 `c12f930b1` 已推送、维护迁移已部署、健康/监控 API/权重/倍率/余额已线上验证）**。
0. **OpenAI 模型映射审计**：仅覆盖 OpenAI-compatible HTTP JSON、SSE 与 Responses WebSocket；新增 `usage_logs.actual_response_model`，记录上游原始响应模型并在管理员 usage log 展示。**状态：进行中（本地实现与验证阶段，尚未推送、部署或线上验证）**。
0.1. **账号监控 V3：账号质量、评分、排名与调度信息**：2026-08-04 最终确认已覆盖旧的“账号监控同时承接经营/账务”方案；账号监控生产页只承接原生分组 Tab、七项分组汇总、真实请求时间窗、完整六段式账号卡片、卡内全局优先级、一次性采购成本、倍率与原生并发，营收、利润、账务和对账仍是独立后续事项。最终提交 `05985e62ec88b04d1e647a815eecdb1cf1155776` 已推送并部署至绿色槽位；#118/#119 均以真实请求窗口投影为可用、保留评分排名，单卡刷新请求成功且检查时间推进。**状态：已完成（已推送、已部署、已验证生效）**。

0.4. **账号卡片成本倒挂预警**：在管理员账号监控卡片接入 Sub2API/New API 通用上游原生倍率、本站标准费用折合成本倍率、逐笔账单证据和当前分组倒挂判定；至少 6 笔有效逐笔实际扣费样本才标记“确认亏损”，仅定价推算/额度测得标记“可能亏损”。本次不创建站点经营看板，不新增或迁移现有金额型投入产出区。**状态：进行中（已完成本地实现并合并远端最新代码，待推送、生产部署和线上验证）**。

1. **账号监控真实上游成本与对账闭环**：统一真实账单适配器、对账请求/异常/补登记流水、管理员刷新、日切调度、飞书日报及现有 `AccountMonitorView.vue` 可见页面已推送、部署并通过公网接口验收；本轮继续接通按显式账单账号映射和专用凭证主动采集最新逐笔账单与累计快照，日结在采集或对账未闭合时必须阻断。手工补登记重试幂等性、逐笔请求 ID 回退、迁移重复映射预检、root-only 账单 provisioning、relay-ops 不可变发布路径及蓝绿演练夹具已完成本地实现与回归。**状态：进行中（relay-ops `d3860531d`、生产 host executor、root-only provisioning wrapper 和 Caddy 公网 JSON/认证路由均已部署并验收；生产仍无全站账号的真实只读账单授权和映射，五类核心账务数据均为 0）**。

**本轮新增登记：** `relay-ops` 不可变镜像发布路径及 root-only 账单 provisioning wrapper（提交 `df4870470`）；**状态：进行中（immutable relay-ops、host executor/wrapper 与 Caddy 公网 JSON/受保护 API 路由均已部署并验证；生产的账单授权会话、上游成本快照、对账请求明细、对账执行记录和每日账务快照仍为 0，待全站所有可计费账号的合法只读账单授权和明确映射）**。

**本轮新增登记（2026-08-03）：** 账号监控入口会话隔离修复，状态：进行中（已定位为 `/relay-ops/api/reconciliation/*` 的 401 被共享 API 客户端误判为主站会话失效；正在补充请求级会话恢复隔离、回归测试、生产部署和公网登录复验，未完成部署验证前不得标记完成）。

2. **2026-08-01 main 全局低延迟配置蓝绿发布与待部署事项联合评估**：双槽 API、唯一 worker、host executor、Caddy 路由和停机 bootstrap 已完成生产部署；本轮对账应用已追加发布并验证。**状态：已完成**。
3. **2026-07-31 生产站点中断恢复**：11:56 首次故障后于 13:28 回滚恢复；14:10–14:11，“调度优化”任务误将事故回滚判断为外部覆盖，再次覆盖 Compose 并强制重建 Sub2API/relay-ops/Caddy，造成第二次中断。14:15 已再次恢复至 11:53 发布前快照；公网健康连续 5 次返回 200，首页、价格和文档入口正常，五个容器运行正常且重启计数为 0。**状态：恢复已验证；永久兼容性修复仍未推送、部署和验证**。
4. **Sub2API 低延迟/会话隔离/Monitor 自动刷新工程差异**：生产仍缺全局 OpenAI 低延迟配置、WebSocket `store=false` 会话隔离修复和 Monitor 自动刷新后端/前端/设置实现。**状态：工程代码/配置差异待部署**。
5. **relay-ops 原生 P0/P1 Bridge**：Bridge 实现仍缺通知策略兼容性和必需的只读数据库接线；relay-ops 与 Sub2API 的受控发布机制已经存在，不再作为本项缺口。**状态：持续实施**。
6. **Caddy/homepage 运行时差异与全站账务总账**：Caddy/homepage 运行时改动须独立发布；全站账务总账已进入 `origin/main`，但尚未部署、激活或生产验证。**状态：工程差异待部署**。
7. **Neko/Wawazz 门禁、D04 readiness、备份留存和后续运营**：均保留最新证据和外部验收跟进，不因文档或历史报告单独记为工程未部署。
8. **账号上游倍率自动同步**：托管/手工覆盖策略、审计、缓存同步和账号生命周期/定时触发已合并到 `main`；最终修复完成显式策略意图、提交顺序安全的 Redis 版本 fencing、按模式隔离 singleflight 和 `UpdateLastUsed` 持久化版本单调性。后端、前端及真实 PostgreSQL/Redis 集成回归通过，最终独立复审批准合并；尚未推送、部署或线上验证。**状态：工程差异待部署**。
9. **Monitor V2 卡片信息精简**：已合并移除各指标样本数量、模型折叠区及接口 `models` 字段，保留性能指标、有效调用、P95 与趋势；前后端测试已通过，但尚未推送、部署或生产验证。**状态：工程差异待部署**。
10. **Monitor 当前状态与飞书历史错误码修复**：已按“最新一次渠道探测”口径完成实现、回归并进入 `origin/main`，修正最新成功被历史失败压过，以及历史余额耗尽错误码残留导致账号正在运行却被告警为无可用账号。**状态：工程差异待部署（已推送，待生产部署和告警/恢复线上验证）**。
11. **`api.xingqiaolab.top` TLS 兼容性修复**：Caddy RSA-2048/TLS 1.2/1.3 策略已部署，但 CC Switch 刷新仍失败。2026-08-01 完成 nginx TLS 前置层受控生产切换，Compose、Caddy、nginx 及未受影响服务门禁通过；但受影响 Mac 上 TLS 1.2 仍在 ClientHello 阶段被重置，CC Switch 3.18.0 实际刷新仍显示“查询失败”，因而已回滚到 Caddy 直接接管 80/443。同一公网 IP 上仅 SNI `api.xingqiaolab.top` 的 TLS 1.2 被重置，替换 SNI、无 SNI 及服务器本机访问均成功，根因已收敛为客户端到源站之间的域名特异网络干预。**状态：进行中（nginx 代码已推送、生产试部署已回滚；待使用兼容域名或新公网/CDN 入口完成客户端验收）**。
12. **历史账号监控分组经营与评分方案（已被 V3 边界覆盖）**：该旧方案曾把账号经营数据、账务历史和对账入口放入账号监控页，并形成候选提交与发布预检证据。2026-08-04 最新确认已废止其页面职责：旧后端账务能力保留为未来“营收”页面输入，但不得恢复到 `/admin/accounts/monitor`；当前账号监控只实施账号质量、评分、排名和调度信息。**状态：历史方案停止继续实施；当前交付状态统一以上方 0.1 与 V3 不可漂移合同为准，不得把旧候选当作当前可部署结果**。
## 生产工程代码/配置已部署并验证（20 项）

1. **首版生产 Compose/Caddy/Sub2API/PostgreSQL/Redis**
   - **解决的问题：** 建立可运行的首条生产链路和基础服务边界。
   - **做了什么优化：** 从主机基线切换到完整 Compose 四服务栈，并由 Caddy 提供入口。
   - **影响范围：** 生产站点、网关、数据库、缓存和反向代理。
   - **状态：** 已完成；生产部署并通过健康与链路验证。
   - **证据：** [当前状态](current-state.md)、[relay-ops 只读生产验收](../superpowers/reports/2026-07-19-relay-ops-read-only-verification.md)

2. **relay-ops 只读部署**
   - **解决的问题：** 让运维、价格和采集信息可观察，同时避免控制面写入生产。
   - **做了什么优化：** 复用管理员会话，提供只读页面、健康检查和公开价格展示。
   - **影响范围：** relay-ops 运行容器、生产运维入口和价格页。
   - **状态：** 已完成；只读部署已验收。
   - **证据：** [relay-ops 只读生产验收](../superpowers/reports/2026-07-19-relay-ops-read-only-verification.md)

3. **飞书命令 disabled/dry_run 生产链路**
   - **解决的问题：** 在不改动路由和基础服务的前提下验证飞书回调边界。
   - **做了什么优化：** 固定命令解析、禁用态审计和 dry-run 零写路径。
   - **影响范围：** relay-ops 飞书回调、生产审计和通知链路。
   - **状态：** 已完成；disabled 和 dry_run 生产链路均已验证。
   - **证据：** [disabled 生产验证](../superpowers/reports/2026-07-20-feishu-command-disabled-production-verification.md)、[dry-run 生产验证](../superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md)

4. **生产来源、价格、用量监控**
   - **解决的问题：** 缺少来源登记、公开价格采集和用量会话的统一观察。
   - **做了什么优化：** 增加来源登记、价格 hash 去重、阶梯价格展示和只读用量会话支持。
   - **影响范围：** relay-ops 采集、管理员观察和价格页面。
   - **状态：** 已完成；生产接口、来源和采集行为已验收。
   - **证据：** [生产来源与会话监控](../superpowers/reports/2026-07-20-relay-ops-production-source-monitoring-verification.md)

5. **D04 公共注册和每日登录**
   - **解决的问题：** 将公开注册和每日登录规则落到可验证的生产链路。
   - **做了什么优化：** 完成公开注册、邀请码和每日首次登录行为的生产验证。
   - **影响范围：** D04 用户入口和注册相关行为。
   - **状态：** 已完成；该历史生产事项已验证，后续口径由 Sub2API 原生设置承接。
   - **证据：** [D04 公共注册与每日登录](../superpowers/reports/2026-07-21-d04-public-registration-daily-login-verification.md)

6. **飞书主动告警**
   - **解决的问题：** 生产异常缺少主动通知、恢复和重复抑制。
   - **做了什么优化：** 接通主动告警、恢复通知、去重和每日运营摘要。
   - **影响范围：** 原生监控、relay-ops 出站通知和运营值守。
   - **状态：** 已完成；真实群投递和零写边界已验证。
   - **证据：** [飞书主动告警生产闭环](../superpowers/reports/2026-07-21-feishu-proactive-alert-production-verification.md)

7. **飞书专业卡片**
   - **解决的问题：** 告警信息缺少结构化、可扫描的展示形式。
   - **做了什么优化：** 统一专业卡片字段、状态和恢复展示。
   - **影响范围：** 飞书告警阅读体验和运营摘要。
   - **状态：** 已完成；功能主链路生产验证通过。
   - **证据：** [飞书专业卡片生产验证](../superpowers/reports/2026-07-21-feishu-professional-card-production-verification.md)

8. **Sub2API 原生运维页只读投影**
   - **解决的问题：** 自建运维控制面与 Sub2API 原生能力重复且边界复杂。
   - **做了什么优化：** 复用原生 `/admin/ops`，将自建页面收敛为只读投影。
   - **影响范围：** 管理员运维入口、认证和监控展示。
   - **状态：** 已完成；生产页面和认证已验收，受控测试仍按门禁保留。
   - **证据：** [Sub2API 原生运维简化](../superpowers/reports/2026-07-22-sub2api-native-ops-simplification-verification.md)、[原生运维重定向生产验证](../superpowers/reports/2026-07-30-native-ops-reminder-only-production-verification.md)

9. **质量报告与飞书通知**
   - **解决的问题：** 账号和请求质量结果缺少定期汇总和通知出口。
   - **做了什么优化：** 生成只读质量报告并接入生产飞书通知。
   - **影响范围：** 质量巡检、运维摘要和通知。
   - **状态：** 已完成；生产构建、部署和只读验收通过。
   - **证据：** [质量报告飞书生产验证](../superpowers/reports/2026-07-22-quality-report-feishu-production-verification.md)

10. **账号池质量监控及 systemd**
    - **解决的问题：** 账号池健康、错误率和延迟缺少周期性生产采集。
    - **做了什么优化：** 安装周期巡检、结果落盘、单账号隔离和 systemd 调度。
    - **影响范围：** 上游账号池、容量和路由决策。
    - **状态：** 已完成；生产安装和首份结果已复核。
    - **证据：** [账号池质量监控](../superpowers/reports/2026-07-23-account-quality-monitor-verification.md)

11. **模型目录只读监控**
    - **解决的问题：** 模型发布资格和目录变化缺少自动发现与只读可见性。
    - **做了什么优化：** 增加定时发现、原子结果可见性和只读生产监控。
    - **影响范围：** 模型目录、管理员观察和发布前检查。
    - **状态：** 已完成；监控已部署；付费资格发布仍是独立门禁。
    - **证据：** [模型发布只读监控](../superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md)

12. **运维监控与飞书双域名**
    - **解决的问题：** 监控结果和飞书通知缺少稳定的双域名生产入口。
    - **做了什么优化：** 部署只读监控、双域名访问和成功的 systemd runner。
    - **影响范围：** 监控页、公开入口和飞书通知链路。
    - **状态：** 已完成；生产部署和只读验收通过。
    - **证据：** [运维监控与飞书双域名](../superpowers/reports/2026-07-23-ops-monitoring-and-feishu-dual-domain-verification.md)

13. **Xingqiao 初学者指南**
    - **解决的问题：** 新用户缺少从入口到首次使用的生产说明。
    - **做了什么优化：** 发布初学者指南、同源链接和公开路由。
    - **影响范围：** 公开文档入口和新用户上手流程。
    - **状态：** 已完成；生产文档路由和链接已验收。
    - **证据：** [初学者指南生产验证](../superpowers/reports/2026-07-25-xingqiao-beginner-guide-production-verification.md)

14. **详细指南**
    - **解决的问题：** 进阶用户缺少完整配置、排查和安装说明。
    - **做了什么优化：** 发布详细指南并验证生产路由与安装链接。
    - **影响范围：** 公开文档和用户支持成本。
    - **状态：** 已完成；生产页面和容器保留已验证。
    - **证据：** [详细指南生产验证](../superpowers/reports/2026-07-25-xingqiao-detailed-guide-production-verification.md)

15. **Feishu 用户影响告警**
    - **解决的问题：** 用户可感知的 P0/P1 影响缺少可区分的通知信号。
    - **做了什么优化：** 增加用户影响判断、合成 P0 验收和生产告警行为。
    - **影响范围：** 用户请求质量、管理员告警和响应优先级。
    - **状态：** 已完成；生产行为、健康和合成告警已验证。
    - **证据：** [用户影响告警生产验证](../superpowers/reports/2026-07-28-feishu-user-impact-alerting-production-verification.md)

16. **Monitor 可靠性与管理员可见分组**
    - **解决的问题：** 监控刷新可靠性和管理员视图分组不足。
    - **做了什么优化：** 完成合格镜像、生产提升和管理员可见分组；后续自动刷新实现另列为工程差异。
    - **影响范围：** 原生监控页面、管理员观察和运维判断。
    - **状态：** 已完成；生产 UI、健康和部署提升已验收。
    - **证据：** [Monitor 可靠性与管理员可见性](../superpowers/reports/2026-07-30-monitor-reliability-admin-visibility-production-verification.md)

17. **Monitor V2 7/30 天窗口**
    - **解决的问题：** 监控窗口过短，无法比较短期和长期趋势。
    - **做了什么优化：** 增加 7 天、30 天窗口和桶化结果，并完成生产提升。
    - **影响范围：** 监控统计、管理员诊断和容量判断。
    - **状态：** 已完成；生产提升和结果验证通过。
    - **证据：** [Monitor V2 长窗口](../superpowers/reports/2026-07-30-monitor-v2-long-window-buckets-production-verification.md)

18. **原生运维重定向与飞书纯提醒**
    - **解决的问题：** 旧运维入口和飞书入站控制能力造成边界混淆。
    - **做了什么优化：** 旧 `/ops` 路由重定向到原生 `/admin/ops`，飞书收敛为出站提醒、恢复和日报。
    - **影响范围：** 公网入口、管理员运维和飞书权限边界。
    - **状态：** 已完成；部署、公网审计和生产健康均已通过。
    - **证据：** [原生运维重定向与纯提醒生产验证](../superpowers/reports/2026-07-30-native-ops-reminder-only-production-verification.md)

19. **账号监控 V3 生产闭环**
    - **解决的问题：** 账号监控页未按最终职责边界完整呈现真实分组、评分、排名、成本与并发，且采购成本迁移需要受控维护发布。
    - **做了什么优化：** 部署最终 V3 卡片、三个真实请求窗口、原生分组汇总、采购成本/倍率模式、稳定排名和实时并发；以绿色槽位完成采购成本附加迁移。
    - **影响范围：** `/admin/accounts/monitor`、账号监控 API、账户采购成本迁移与蓝绿 API/Worker 切换。
    - **状态：** 已完成；最终提交 `05985e62ec88b04d1e647a815eecdb1cf1155776` 已推送、生产部署且状态/单卡刷新缺陷线上验证生效。
    - **证据：** [V3 生产闭环证据](../../.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/production-verification.md)。

20. **账号监控成本折合本站倍率简化**
    - **解决的问题：** 成本区域展示了重复的上游倍率、分组倍率、样本和对账字段，且折合成本未严格使用最新探测模型。
    - **做了什么优化：** 仅保留成本折合本站倍率；按最新探测模型价格、账号有效倍率和本站模型价格计算，缺少有效价格显示 `--`。
    - **影响范围：** `/admin/accounts/monitor` 账号卡片成本区域及账号监控 API。
    - **状态：** 已完成；`ff5d183a6` 已推送、蓝绿部署并通过线上健康、接口和容器身份验收，无数据库迁移、无停机。
    - **证据：** [本轮发布证据](/Users/gongtengxinwen/Documents/sub2api搭建/release-evidence/sub2api-account-monitor-equivalent-cost-ff5d183a.json)。

## 运维/研究跟进（23 项）

以下事项是文档、研究、离线证据、历史验证或外部验收跟进，不因缺少一次工程部署而分类为“工程代码/配置差异待部署”。

1. **离线部署基线**：解决基础设施契约和本地启动验证问题；优化 Compose/Caddy 基线；影响基础设施准备；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-l1-2-offline-baseline-verification.md)。
2. **人工账本模拟**：解决充值、用量和对账规则缺少可重复样例；优化链式事件和模拟校验；影响账务设计；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-manual-ledger-verification.md)。
3. **运营与止损基线**：解决日常检查和止损动作缺少固定节奏；优化 report-only 评估器；影响运营流程；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-operations-baseline-verification.md)。
4. **支付控制模拟**：解决支付状态机和退款幂等规则缺少离线验证；优化禁用态 provider 选择和状态机；影响支付边界；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-payment-control-simulation-verification.md)。
5. **MVP 定价模拟**：解决首版价格、倍率和毛利规则缺少可回归样例；优化定价计算器和配置校验；影响价格策略；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-pricing-simulation-verification.md)。
6. **路由韧性离线基线**：解决重试、熔断和上游评分规则缺少安全基线；优化失败分类和容量建议；影响路由策略；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-routing-resilience-verification.md)。
7. **订阅账号采购比较**：解决账号来源、筛选和验收缺少非敏感比较材料；优化硬淘汰与评分清单；影响采购决策；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-subscription-account-procurement-verification.md)。
8. **上游接入资料包**：解决上游接入所需资料和验收项不完整；优化候选模板、接入表和安全边界；影响上游接入准备；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-upstream-intake-verification.md)。
9. **D03 GPT-5.4 Mini 定价**：解决指定模型的价格映射缺少明确基线；优化价格区间和计费映射；影响模型定价；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-17-d03-pricing-verification.md)。
10. **Neko 候选短测**：解决候选上游性能和价格缺少短样本比较；优化短测指标与保守结论；影响上游筛选；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-18-neko-upstream-short-verification.md)。
11. **上游渠道比较/扩容研究**：解决渠道、容量和扩容策略缺少统一研究；优化候选比较与容量建议；影响上游规划；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-18-upstream-channel-comparison.md)、[扩容实践](../superpowers/reports/2026-07-19-gateway-scaling-practices.md)。
12. **Benchmark V2/协议适配器**：解决协议适配和基准测试缺少可复用实现；优化 benchmark v2 与适配器契约；影响上游测试工具；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-21-upstream-benchmark-protocol-adapters-verification.md)。
13. **XM 目录发现与非功能基线**：解决 XM 目录发现和 SSE/拓扑非功能信息不足；优化只读发现与非功能记录；影响目录研究；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-21-xm-upstream-discovery.md)、[非功能基线](../superpowers/reports/2026-07-21-upstream-sse-capacity-and-topology-nonfunctional-verification.md)。
14. **GPT Codex 缓存节省本地实现**：解决缓存节省和计价收益缺少本地计算基线；优化缓存字段、节省估算和验证样例；影响成本分析；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md)。

15. **Neko/Wawazz 生产分流门禁**：解决候选上游分流缺少同快照质量门禁；优化账号发现、余额新鲜度和自然质量判定；影响生产路由；**状态：运维跟进，外部质量证据和配置复核未完成**；[证据](../superpowers/reports/2026-07-20-neko-wawazz-production-split-verification.md)、[v3 readiness](../superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md)。
16. **D04 受控开放/轻量门禁/v3 readiness**：解决开放注册与活动上游准入条件不清；优化由原生账号列表动态发现并保留只读门禁；**状态：运维跟进，外部阻塞/历史保留；没有新鲜 GO 不得放行**；[证据](../superpowers/reports/2026-07-21-d04-controlled-launch-v2-verification.md)、[轻量门禁](../superpowers/reports/2026-07-22-d04-lightweight-launch-gate-verification.md)、[v3 readiness](../superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md)。
17. **生产备份异地副本与 7 天留存**：解决备份异地性、持续性和恢复演练缺口；优化 `pg_dump`、restic 和 R2 验收方案；影响数据库恢复和灾备；**状态：运维跟进，外部阻塞；真实连续 7 天备份与独立恢复待完成**；[证据](../superpowers/reports/2026-07-22-production-backup-restore-verification.md)。
18. **GPT Codex 缓存定价 24 小时门禁**：解决缓存收益实现缺少生产价格和持续观察门槛；优化 24 小时观察与价格变更门禁；影响成本和定价；**状态：运维跟进，待价格观察与验证**；[证据](../superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md)。
19. **模型发布资格监控**：解决模型目录发现与真正发布资格混淆；优化只读发现和资格前置检查；影响模型发布；**状态：运维跟进，历史阻塞保留；资格测试、余额和覆盖证据不足**；[证据](../superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md)、[滚动模型策略](../superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md)。
20. **账号倍率监控 SSH 复核**：解决生产账号倍率与监控结果可能漂移；优化只读倍率检查和 SSH 复核清单；影响路由与价格；**状态：运维跟进，待新鲜生产复核**；[证据](../superpowers/reports/2026-07-26-account-monitor-multiplier-verification.md)。
21. **Sub2API 0.1.166 管理员切换**：解决合格镜像已交付但运行版本仍需管理员确认切换；优化官方版本合并、合格镜像和回滚边界；影响网关运行时；**状态：运维跟进，外部阻塞/待管理员切换；不单独计为工程部署差异**；[证据](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。
22. **Sub2API 受控本地发布流程**：GitHub Actions 发布 workflow 已按管理员要求退役；受控 worktree 的本地候选链已完成 `discover -> merge -> qualify -> stage -> advance`，并通过生产蓝绿切换和线上验证。**状态：已完成；已推送、已部署、已验证生效**。
23. **Feishu 通知合并生产部署**：解决多通知路径重复和边界不统一；优化通知合并、纯出站契约和本地回归；**状态：运维跟进，待 48 小时生产观察和公网验证；运行时实现已在生产基础上，不能仅因观察未完成称为工程未部署**；[证据](../superpowers/reports/2026-07-29-feishu-notification-consolidation-local-verification.md)。

## 工程代码/配置差异待部署（11 项）

1. **Sub2API 全局 OpenAI 低延迟配置**：生产尚未应用 `cded84a7e` 对应的全局 relay latency 配置；**状态：工程代码/配置差异待部署**。
2. **Sub2API WebSocket `store=false` 会话隔离修复**：生产尚未应用 `c49c0a6ec`；**状态：工程代码/配置差异待部署**。
3. **Monitor 页面自动刷新后端/前端/设置**：生产尚未应用自动刷新实现及其设置暴露；既有 Monitor 可靠性和长窗口能力不等于该实现已部署；**状态：工程代码/配置差异待部署**；[证据](../superpowers/reports/2026-07-30-monitor-reliability-admin-visibility-production-verification.md)、[长窗口](../superpowers/reports/2026-07-30-monitor-v2-long-window-buckets-production-verification.md)。
4. **Caddy/homepage 运行时改动**：生产 Caddy 镜像仍为 `79b0f0a724bb412cfe94d0e2ffb35a4796e4ba7e`，首页运行时改动须独立于 Sub2API 发布；**状态：工程代码/配置差异待部署**；[当前状态](current-state.md)、[0.1.166 合格镜像证据](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。
5. **全站账务总账**：代码、迁移、受保护账务页面/API、00:10 日调度、清账脚本、本地端到端和隔离 PostgreSQL 验证已进入 `origin/main`；尚未部署、清账激活或生产验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-whole-site-accounting-design.md)、[计划](../superpowers/plans/2026-07-31-whole-site-accounting-ledger-implementation-plan.md)、[本地验证](../superpowers/reports/2026-07-31-whole-site-accounting-ledger-task7-local-verification.md)。
6. **账号上游倍率自动同步**：上游倍率解析后的托管账号回写、审计和缓存同步已合并到 `main`，本地测试已通过；尚未推送、部署或线上验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-account-rate-multiplier-sync-design.md)、[计划](../superpowers/plans/2026-07-31-account-rate-multiplier-sync-implementation-plan.md)。
7. **Monitor V2 卡片信息精简**：已合并删除样本数量、模型列表和接口 `models` 字段，保留核心性能指标；本地前后端测试已通过，尚未推送、部署或生产验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-monitor-v2-card-simplification-design.md)、[计划](../superpowers/plans/2026-07-31-monitor-v2-card-simplification-implementation-plan.md)。
8. **Monitor 当前状态与飞书历史错误码修复**：最新有效渠道探测成功时 Monitor 显示正常，最新失败但近期有成功时显示降级；飞书容量改用账号最新探测状态，最新成功不再继承历史 `balance_exhausted`。实现与回归已进入 `origin/main`，尚未部署或线上验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-monitor-current-status-and-feishu-stale-error-design.md)、[计划](../superpowers/plans/2026-07-31-monitor-status-feishu-alert-fix-implementation-plan.md)。
9. **30 分钟指令驱动 Sub2API 蓝绿发布**：API/worker 角色、双槽 Compose、restart-stable Caddy 路由、停机门禁、恢复/回滚、候选等待、运行态证明和 host deadline 已完成本地实现与 focused 验证；2026-08-01 已补足 controller 向 host executor 传递绝对 deadline 的契约。生产预检确认无 host executor / `release-state`、Caddy 仍是遗留单实例上游，首次切换双槽需停机 bootstrap。**状态：工程差异待部署**；[运行手册](../runbooks/sub2api-blue-green-production-deployment.md)。
10. **`api.xingqiaolab.top` TLS 兼容性修复**：nginx TLS 前置实现已以 `95a81dc37` 推送到 `main`；2026-08-01 生产切换后服务端 TLS 1.2/1.3 均正常，但受影响客户端和 CC Switch 验收仍失败，已完成受控回滚。后续需改用不被干预的兼容域名或更换公网/CDN 入口，nginx 配置当前不在生产生效。**状态：工程差异待部署（前置层方案未通过客户端门禁）**。
11. **账号监控成本、余额、评分权重与官方倍率收敛**：已恢复分组评分权重入口，OpenAI API Key 统一使用官方 `accounts.rate_multiplier` 与 `extra.upstream_billing_rate_sync_enabled`，非 API Key 使用人民币采购成本和预计可用 USD 额度；支持上游余额、单卡强制刷新，并通过迁移 198 删除旧策略/measurement 数据。自动倍率展示现已读取官方 probe snapshot 的 `status`、`received_at`、`fresh_until`，过期/失败会明确标记。最终提交 `c12f930b1` 已推送并部署；生产迁移哈希已为 `0204f39423f3218ffa0c8d4e3d665f7113c4990610e0dd22e9f5910c4d578c6d`，健康接口、监控 API、分组权重、native 倍率和余额快照均已线上验证。**状态：已完成；已推送、已部署、已验证生效**；[设计](../superpowers/specs/2026-08-06-account-monitor-cost-balance-and-score-weights-design.md)、[计划](../superpowers/plans/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan.md)。

## 持续实施（2 项）

1. **原生 P0/P1 Feishu Bridge 独立扩展**：`ops_alert_events` 出站桥接实现尚缺通知策略兼容性和必需的只读数据库接线；**状态：持续实施**；[设计](../superpowers/specs/2026-07-30-native-p0-p1-feishu-bridge-design.md)。
2. **用量明细/支持卡片/上游账号状态等后续运营功能**：仍待设计与实现；**状态：持续实施**；[证据](current-state.md)。

## 总账维护规则

以后每次任务都必须在实施前先登记到本文件，并将初始状态写为“进行中”。任务收尾时，只有完成服务端推送、生产部署和生效验证，才可转入“生产工程代码/配置已部署并验证”；仍存在运行时代码或生产配置差异的事项进入“工程代码/配置差异待部署”，尚未完成实现的进入“持续实施”，文档、研究、离线证据和外部验收进入“运维/研究跟进”，不得仅因这些材料缺失而声称工程未部署。每次收尾同时更新统计摘要和 current-state 入口。

**本轮维护门禁登记（2026-08-04）：** 采购成本附加迁移维护门禁，状态：已完成（最终提交 `82095b80770236eac24adb0bdb1b80cd639675cb` 已推送并完成维护发布；生产迁移哈希已为 `337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76`。PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 与 `release-state` 一致且未重建；维护路径仅切换 API/Worker，生产验证通过）。
**本轮非 main 工作区整合与生产发布（2026-08-09）：** 状态：已完成。所有候选已并入并推送至 `main`，生产已从干净候选 `3fb79f5291961a99a50d13b3306937a8db156b04`（tree `380600ff2b5718343d64f4729f984f4d9e3ea2ac`）完成维护发布。发布记录 `20260809T154902Z-production-2353963` 为 `succeeded/promoted`、`rolled_back=false`，活动槽为 `blue`，迁移哈希为 `1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98`。公网 `/healthz` 与 `/readyz` 均通过，首页返回 200，受保护管理 API 未认证返回 401；API 与 worker 均 healthy 且重启计数为 0，PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 保持既有容器身份。停机前遇到的 SSH 瞬断均未改变生产，一次误判参数修改也已在未部署前撤销。

**本轮新增登记（2026-08-09）：** GPT 文本上游账号分组配置基线分析；范围为排除生图、Claude 账号和自测分组目标，基于生产现有 7/30 天真实请求、主动探测、成本倍率、分组权重与调度优先级证据，形成 GPT-Pro、【专属】GPT-Pro、GPT-Plus、GPT-特惠的账号归属和主力/次级/备用基线，并评估现有评分体系是否足以持续检查分组与优先级合理性。当前工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/gpt-group-baseline-analysis`，分支 `codex/gpt-group-baseline-analysis`。**状态：进行中（已完成全部非 main worktree 盘点并确认无已完成且必须先合并的领先候选；仅使用现有生产数据，未修改生产分组、路由、优先级或账号状态）**。

**本轮 Task 1 证据采集（2026-08-09）：** GPT 文本上游账号分组配置基线分析的只读生产证据快照，状态：进行中。范围仅限既有生产 PostgreSQL 聚合与现有管理员监控投影，收集 7/30 天真实请求和主动探测、账号/分组配置、评分权重、调度设置；不执行上游 API 探测，不修改生产数据库、服务、容器、文件、分组、路由、优先级或账号状态。当前工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/gpt-group-baseline-analysis`，分支 `codex/gpt-group-baseline-analysis`。保护例外：总账已登记的“新建运营界面”“优化账号卡片”，以及含未提交改动的 `codex/upstream-resilience-implementation` 工作区均保持不动。

**本轮 Task 1 证据采集完成（2026-08-09）：** 只读生产快照已捕获，UTC `2026-08-09T10:15:42.482Z`；JSON SHA-256 `766fb926165614744480695b8080b1d7b281ec5107b7f995cfb470a9944ddfc3`，报告为 `docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-evidence.md`。schema、聚合、管理员监控投影、结构校验和定向敏感词扫描已完成；扫描唯一命中为非秘密账户类型字面值 `apikey`，已在报告中标记手工复核。当前仍未推送、部署或线上验证，整体状态保持进行中。

**本轮 GPT 分组配置研究基线收口（2026-08-09）：** 已基于同一只读快照完成全部 67 个 GPT 文本账号的逐项分析，形成 `GPT-Pro 5`、`GPT-Plus 7`、`GPT-特惠 5`、`暂不进入公开组 38`、`人工处理后补跑 12` 的配置基线；公开与专属 Pro 统一为同一账号池和 `1/2/3` 优先级层级。报告为 `docs/superpowers/reports/2026-08-09-gpt-group-configuration-baseline.md`。同时确认现有评分公式可保留为组内排序基础，但真实请求尚未进入最终质量分、Plus 与特惠质量权重相同、OpenAI 调度仍读取全局 `accounts.priority`，因此不足以独立完成持续归组和组内优先级审计。**状态：准备完成，不能标记已完成；本轮无生产配置变更，后续若采纳账号迁移或评分/调度改造，必须另行审批、合并到 main、部署并线上验证。**

**本轮 GPT 分组基线生产应用登记（2026-08-09）：** 用户已明确批准通过 Sub2API 管理接口应用已有归组结论，并新增利润下限：公开 Pro/Plus 每组至少保留 `0.05x`，特惠和专属 Pro 至少保留 `0.02x`。因此账号成本上限分别为公开 Pro `0.25`、Plus `0.15`、特惠 `0.08`、专属 Pro `0.23`；由于公开/专属 Pro 必须镜像同池，Pro 实际统一上限为 `0.23`。当前工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/gpt-group-baseline-apply`，分支 `codex/gpt-group-baseline-apply`。已盘点全部非 main worktree：领先分支均为仍在实施、正在整合或未完成生产闭环的活动候选，未发现必须先合并的已完成候选；主工作区存在其他任务的未解决冲突并保持不动。**状态：进行中；计划仅调整账号分组，先保存回滚快照，再调用管理员接口，随后只读复核分组、利润门槛、Pro 镜像和生产健康。**

**本轮 GPT 分组基线生产应用结果（2026-08-09）：** 已通过 Sub2API 管理接口完成 69 个 GPT 文本账号归位，最终为公开/专属 Pro 同池 4、Plus 4、特惠 4、测试隔离 57；最低利润分别为 `0.10/0.05/0.05/0.02`，API、数据库、活动 green API、worker 和 `/health` 验证通过。写入后发现管理员 JWT 会话对新账号 172/173 执行竞争性回绑；审计确认后已按利润政策重新隔离并连续读回稳定。生产前后快照和完整报告见 `docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-application.md`。**状态：进行中（生产配置已应用并验证；因主工作区存在其他任务未解决冲突，本分支尚未合并、推送到 main，按总账规则暂不标记已完成）。**

**本轮非 main 工作区整合与发布前准备（2026-08-10）：** 已审查并合并 `codex/account-monitor-group-recommendation`、`codex/fix-official-update-stuck`、`codex/gpt-group-baseline-apply` 三个干净候选到 `main`；三者相对 `main` 均为 0 个独有提交，保护例外 worktree 与根目录未跟踪发布证据均保留。合并后 Sub2API 后端 `go test ./...`、`go vet ./...`、后端/前端生产构建、前端全量 Vitest/lint/typecheck，以及当前蓝绿拓扑、host executor、更新器、发布资格/冒烟/备份/relay-ops 合同均通过。`make -C upstream/sub2api/backend test` 仅因环境缺少 `golangci-lint` 在 lint 阶段退出；旧版 `validate-official-sub2api-release.sh`、`validate-sub2api-rehearsal.sh` 和 `validate-baseline.sh` 与现行蓝绿拓扑/3072MB 构建参数不匹配，已改用现行蓝绿拓扑与基线合同覆盖并记录该差异。当前迁移集合哈希为 `738ad63324d900283383a523ce82e821fe5d8bb19d56de3834a6c817fb6611a5`。**状态：进行中（已合并、发布前检查完成，待推送；不执行部署，等待用户部署指令）。**

**本轮非 main 工作区整合与发布状态更新（2026-08-10）：** `main` 已推送至 `origin/main`，远程指针为 `becb50bdb`；候选 worktree 均无相对 `main` 的独有提交，部署前证据与未跟踪文件仍保留。**状态：进行中（已推送、发布前检查完成，等待用户明确部署指令；本轮未执行部署或线上写入）。**

**本轮停机维护部署门禁（2026-08-10）：** 用户已明确授权停机部署。部署执行前复核发现当前迁移集合哈希 `738ad63324d900283383a523ce82e821fe5d8bb19d56de3834a6c817fb6611a5` 相对生产 `1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98` 的差异包含对已部署迁移 `200_externalization_outbox.sql` 的历史改写（追加 `claim_token`），会触发已应用迁移 checksum 不一致，不能仅扩展宿主维护 allowlist。当前在 `main` 上按 TDD 恢复迁移 200 的生产原文、将该列拆入新的 expand-only 迁移并更新精确维护转换；完成聚焦回归、推送后将直接执行已获授权的维护发布和线上验证。**状态：进行中；生产尚未因本次授权发生修改。**

**本轮迁移门禁修复完成（2026-08-10）：** `200_externalization_outbox.sql` 已恢复为生产提交 `3fb79f5291961a99a50d13b3306937a8db156b04` 的字节原文（SHA-256 `e4e2b329f9c0a1cedfd1a87fb1d945082da9cc2248afbcb4ebe4872ff03cd9d2`），`claim_token` 改由新 `202_externalization_outbox_claim_token.sql` 以幂等 expand-only 方式增加；最终迁移集合哈希为 `fadb98d43e3d8e8b41178203638912cc32592a1368091e4cb44399926daead5d`。宿主仅新增 `1f47135… -> fadb98d4…` 精确维护转换，错误候选 `738ad633…` 和未知哈希均在停服前拒绝。迁移回归、host executor 全量合同、release controller 合同、Bash 语法和 `git diff --check` 均通过。**状态：进行中；修复尚未提交、推送、部署或线上验证。**

**本轮 14 个异常 GPT 账号补跑复核（2026-08-09）：** 用户已人工处理账号 `49,50,80,105,106,163,164,165,168,169,170,171,172,173`，授权基于最新生产真实请求、主动探测、成本倍率与可调度状态定向复核；仅在现有证据不足时通过 Sub2API 原生账号测试每账号最多补测一次。仍异常、不可调度、成本未知或利润不达标的账号直接跳过并保留测试隔离；已恢复且证据充分的账号按最高适配档和利润门槛调用 Sub2API 管理接口归组。当前工作区与分支继续使用 `codex/gpt-group-baseline-apply`。**状态：进行中。**

**本轮 14 个异常 GPT 账号补跑复核结果（2026-08-09）：** 已通过 Sub2API 管理接口将 `49,164,165` 归入 Plus，`50` 归入特惠，`172,173` 归入公开/专属 Pro 镜像池；`80,105,106,163,168,169,170,171` 因成本、删除、不可调度、持续探测失败或质量不匹配继续跳过。并发新建且直接进入公开池的 `174–177` 因仅 0–1 条样本（其中 177 还违反 Plus 利润门槛）已隔离到测试组。最终为 Pro/专属 Pro 各 6、Plus 7、特惠 5，最低利润 `0.10/0.05/0.05/0.02`，连续三次 API 读回、数据库利润/镜像校验和活动 blue API/worker/relay-ops 健康均通过。证据见 `docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-application.md`。**状态：进行中（生产配置已应用并验证；本分支仍未合并、推送到 main，按总账规则不标记已完成）。**

**本轮遗漏 relay-ops 补齐部署（2026-08-10）：** 之前的发布只切换了 Sub2API 镜像，relay-ops 仍停留在旧提交 `94f8b4d8e428a46e43b60bc41ad9694ded01ceba`，且宿主 Caddyfile 未包含 `/api/v1/xingqiao/*` 控制面路由，因此已合并到 `main` 的持久化投影、同域控制面、隔离管理员认证、余额/账单/倍率采集和受控写命令没有线上入口。本轮从干净候选 `main@52aa3023a014216190154e6f809c904ba4a97de0`（tree `b059b368156b42ed9bc21dadd2b1971f6dc1ef86`）重新执行 relay-ops 全量测试、race、vet、路由/发布控制器/宿主执行器合同和 `git diff --check`，生成绑定迁移哈希 `888670e352809881e055a0922a2702f645e555e372ffd85043f49af5fd299ea4` 的资格证据。首次切换在停机前按 fail-closed 门禁发现宿主缺少 `sub2api-green`，未改 relay；随后按已授权停机窗口完成 green 槽 bootstrap、校验并同步仓库 Caddyfile、重建 Caddy，再通过受控 `release-relay-ops.sh` 完成 relay-ops 发布。生产 release-state 为 `succeeded`，当前镜像 `ghcr.io/leesssong/xingqiao-relay-ops:release-52aa3023a014216190154e6f809c904ba4a97de0`，镜像 ID `sha256:023c5c94e03e23c5746600444c1964b08e8bd64478aaa59148de316d3900b830`；PostgreSQL、Redis、Caddy、blue/green API、worker 身份均保持稳定。线上 `/healthz`、`/readyz` 返回 `200`，控制面未认证返回 `401`，退役旧 relay 页面返回 `404`，relay `/healthz` 和 `/readyz` 均为 `alive/ready`，relay_ops 数据库读模型与命令表已存在。**状态：已完成；已推送、已部署、已验证生效。**

**本轮 Task 1：New API 原生账单地址归一化（2026-08-11）：** 范围为修复推理地址 `/api/v1` 生成 `/api/api/...` 的窄问题；当前工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/direct-upstream-billing-fields`，分支 `codex/direct-upstream-billing-fields`。仅修改原生账单聚焦测试、地址归一化实现及本条进度证据，不触碰受保护工作区、`main`、部署或无关测试。RED：`go test ./internal/service -run 'TestSubUpstreamCostServiceConfirmsNewAPIQuotaCostForAPIInferenceBase|TestSubUpstreamCostServiceConfirmsMatchedActualCost' -count=1` 如预期失败，新 `/api/v1` 用例返回 `unavailable` 而非 `confirmed`，因请求被构造为 `/api/api/...` 并由仅接受原生端点的服务器返回 404。GREEN：同一命令通过；`go test ./internal/service -run 'TestSubUpstreamCost|NewAPI' -count=1` 通过；`git diff --check` 通过。修复轮 RED：`go test ./internal/service -run 'TestRefreshBalanceSelectsExactlyOneSourceFromDeclaration' -count=1` 先因 Sub2API `/api` 基地址被错误归一化为 `/v1/usage` 而返回 404；随后将 `/api` 去除逻辑隔离到 New API 原生 helper，并让原生 New API 账单/余额路径使用该 helper，修复轮 GREEN 与上述全部聚焦验证通过。**状态：进行中（本地实现与聚焦验证完成，尚未合并、推送、部署或线上验证）。**

**本轮三个既有顶层任务总统筹（2026-08-12）：** 用户授权当前根任务替代人工盯盘，严格串行协调既有顶层任务：先完成“更新官方版本”的合并、推送、生产部署与线上验收；随后先把该生产内容回合并进当前 T03 候选，再按“快速迭代”队列从 T03、T04 起逐包推进；最后执行“调度优化（2）”。用户进一步授权：各任务停在规格、计划、实现、合并或部署审查门禁时，由总统筹根任务代为执行简单审查；审查通过后可直接放行并继续下一步，无需重复请求人工批准，目标保持为全部任务按顺序部署生效。当前总统筹任务工作区为 `/Users/gongtengxinwen/Documents/sub2api搭建`，分支为 `main`。启动盘点确认：`codex/official-v0175-fast-merge` 领先 `main`；`codex/t03-r1-upstream-cost-persistence` 与 `main` 分叉；`codex/upstream-resilience-hardening` 与 `main` 分叉且存在未提交规格/队列/总账变更，必须原样保留。总统筹已建立持续目标、串行计划和低令牌等待机制；只依据任务工作区、正式交接文档、验证证据与审查结果放行，不凭任务标题猜测状态。**状态：进行中。**

**本轮官方 `v0.1.175` 总统筹独立审查（2026-08-12）：** 根任务已通过 GitHub Releases API 重新确认当前最新稳定版为 `v0.1.175`，并在临时官方仓库核对 `v0.1.173 -> v0.1.175` 共 114 个官方变更路径，候选全部覆盖且唯一额外源码路径为 `XINGQIAO_UPSTREAM.md`，官方迁移目录无变化。候选复跑的 handler/service 测试、Go vet、39 项用量页 Vitest、前端 typecheck/build 和 diff 检查已执行，但独立审查发现候选提交 `29cc455caf69ab2f0b6f5dd25f3b738d926fe241` 的成功响应夹具仍是空 `response.completed`，并缺少 Messages 401/403 与工具副作用 no-replay 对称回归；审查过程中形成的未提交草稿同时修改了 handler 与测试，导致验证结果与提交 SHA 不一致。根任务已拒绝合并，要求在同一候选按 TDD 重新验证、最小修复、提交新 HEAD，并再次独立复审。**状态：进行中（审查拒绝，候选保留，尚未合并、推送或部署）。**

**本轮官方 `v0.1.175` 总统筹第二次独立审查（2026-08-12）：** 修复轮 1 已提交 `3cde9e99f081e25eb9074080f0993f27ccffd7b9`，证据文档收敛到干净候选 `6690b282878e026ad45b34c74d057e36045f53a9`；根任务在该 HEAD 上重新执行 handler/service 测试、Go vet、39 项用量页 Vitest、前端 typecheck/生产构建、diff、版本身份与冲突标记检查，均通过。第二位独立审查员仍拒绝放行：原始 Anthropic Messages 请求中的 `messages[].content[].type="tool_result"` 在没有顶层 `tools` 时未被 `openAIRequestHasSideEffects` 识别，随后转换层会将其变为 `function_call_output`，因此配置化 401/403 仍可能错误重试或切换账号；现有 Messages 回归仅覆盖顶层 `tools`，Responses 也缺少 `function_call_output` 的 handler 级专项回归。根任务要求修复轮 2 仅补齐这两类续轮结果的副作用检测和行为回归，并将任务报告明确绑定最终候选 HEAD。**状态：进行中（第二次审查拒绝，候选保留，尚未合并、推送或部署）。**

**本轮官方 `v0.1.175` 总统筹第四次独立终审（2026-08-12）：** 修复轮 3 按 TDD 证明安全非池或未配置认证状态的 401/403 在旧逻辑下只调用 `[9910]`，随后将“语义可重放”与“同账号池重试资格”分离；最终候选 `9569229676f586ae5233329a8a8d086ea5ce3dc6` 的 Responses/Messages 对称矩阵验证安全 next-account failover 为 `[9910,9911]`、配置化池为 `[9910,9910,9911]`，tools、`function_call_output`、`tool_result` 和输出后请求保持 no-replay。第四位独立审查员在该干净 HEAD 新鲜复跑完整 handler、Go vet、39 项前端专项测试、typecheck、生产构建、gofmt、diff、114 条官方路径、零 migration delta 与版本身份检查，结论为 `APPROVE`。根任务授权候选进入“合并到 main → 并版回归 → 推送 → 宿主蓝绿发布 → 线上验收”门禁；尚未完成推送、部署或线上验证。**状态：进行中。**

**本轮官方 `v0.1.175` 生产发布验收（2026-08-12）：** 已将候选合并到 `main@350e050575377d8e31ed153624bb19da3591517f`，并版后的后端 handler/service、Go vet、前端 Usage 39 项专项、typecheck、生产构建和 `git diff --check` 均新鲜通过；测试证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-12-main-350e050575377d8e31ed153624bb19da3591517f-v1.json`，权限 `0600`，绑定 tree `47e98861f921bebb6d62e41e8a44c142d4d7fe4f` 与迁移哈希 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`。`main` 已推送到远端；宿主蓝绿记录 `20260812T212526Z-production-1519752.json` 返回 `succeeded`，`downtime_required=false`，活动槽为 `green`，源提交/源码树与本地证据一致，候选镜像 ID 为 `sha256:2f73be08b1d62bbdb40c25cd3f049bdcdd3501eeaea8fe71982ba1eb06adc566`，PostgreSQL/Redis/Caddy 身份保持不变。线上 `/healthz`、`/readyz`、`/health` 均 `200`，管理员版本接口返回 `0.1.175`，Usage stats/list 接口均 `200` 且有数据，未认证管理员接口返回 `401`；blue/green API、worker、PostgreSQL、Redis 均 running/healthy/restart `0`。**状态：已完成；已推送、已部署、已验证生效。**

**T03-R1 Task 2 登记（2026-08-12）：** 已完成 Task 1 的独立复审并放行；当前在 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`、分支 `codex/t03-r1-upstream-cost-persistence` 上完成四个独立 Ent evidence/review/daily-value/setting schema、expand-only 迁移 222、迁移/schema 守门测试和仓库 `make generate`。迁移专项、Task 1 遗留字段守门、Ent schema 测试、integration 测试编译、生成确定性、`UsageLog` 哈希不变和 `git diff --check` 已通过；integration 运行时因当前环境没有可用 Docker daemon 未执行。Task 2 未修改 `usage_logs`、官方流水插入、后续登记服务、管理员 API/UI、生产或其他 worktree。**状态：进行中（本地实现与验证完成，等待独立复审；尚未合并、推送、部署或线上验证）。**

**T03-R1 Task 2 复审与 Task 3 启动（2026-08-12）：** Task 2 初审发现 evidence/review 的 `usage_log_id` 唯一性未进入 Ent 生成迁移元数据；修复提交 `125bdf814d70808ef7a7912f3c0469e651aaa456` 已补充两个显式唯一索引，并通过 `make generate`、迁移/Ent 专项、PostgreSQL `TestMigrationsSchema` 集成测试、生成幂等性和 `git diff --check`。scoped re-review 已确认阻断消除且无新回归。现启动 Task 3：仅在官方流水成功插入后的既有响应后任务中执行一次 Sub/New 精确终态证据登记；登记失败不得影响官方流水，禁止重试、补查、扫描、回填、OAuth 上游查询、读时联网和 `usage_logs` 写入。Task 3 round-1 复审修复已恢复官方 `CreateBestEffort` 的批处理和新 context 同步 fallback，只从其实际插入结果观察 ID 后登记；增加持久化 `account_financial_settings.enabled_at` fail-closed 边界（nil/nil enabled_at/启用前均零 HTTP/证据）、稳定 reason 词表、stream/nonstream response-after 合同、非 Sub/New 排除及有界 Sub/New 请求测试。原 GREEN 矩阵、server 编译、`git diff --check` 已通过；仍待复审、根任务授权合并、推送、部署及线上验证。**状态：进行中（尚未合并、推送、部署或线上验证）。**

**T03-R1 Task 3 复审与 Task 4 启动（2026-08-13）：** Task 3 提交 `b8c5a7f8c0342a7176a5ad232170480c3af6c77b`、修复 `f41f4682c575231f72a82ec3c98fa44a7a12b661` 与 `70a6d89703bcbf664db3737aef40e8f67d9b9619` 已完成两轮 scoped re-review。最终独立复审确认真实 `GatewayHandler.ChatCompletions` 非流式与 `OpenAIGatewayHandler.Responses` 流式成功路径均经过官方 `RecordUsage -> CreateBestEffortWithResult -> RegisterOnce`，且证据登记只接受持久化正向 Sub/New 原生账本身份；官方直连 OpenAI/Anthropic/Gemini/Grok API-key、unsupported-only、unknown metadata、OAuth/Vertex/Live 均零账单 HTTP、零 evidence。规格符合性与代码质量均为通过，open findings 为 0；保留一个非阻断测试缺口：尚无 fresh PostgreSQL 并发 `CreateOnce` race 集成测试。现按批准计划启动 Task 4：仅实现本地快照财务汇总、异常核对、OAuth 每日成本、今日覆盖与既有审计日志；不得联网读取上游、修改 `usage_logs`、扩展普通用户 DTO 或触碰 Task 5+。**状态：进行中（Task 3 本地完成并复审通过；Task 4 启动；尚未合并、推送、部署或线上验证）。**
**T03-R1 Task 7（2026-08-13）：** 状态：准备完成（本地实现、验证和独立 scoped re-review 均通过；尚未合并、推送、部署或线上验证）。当前工作区 `/Users/gongtengxinwen/.codex/worktrees/7292/sub2api搭建`，分支 `codex/t03-r1-upstream-cost-persistence`。提交 `32708e05c`、`9d6e21bd8`、`aa5c1ed2b` 与证据提交 `8db318626` 完成管理员 UsageView 异常流水 Tab、route `tab/range/account/evidence/review` 恢复、筛选/分页/同筛选导出、逐笔/选中项/当前筛选核对及只读本地 evidence 详情。初审的 NewAPI unavailable 空 quota 来源误判已改为列表、CSV 和管理员详情统一使用持久化 `source`；`task-7-source-rereview.md` 结论为 finding ADDRESSED、Spec APPROVE、Quality APPROVE、open findings 0。定向 Vitest 55/55、typecheck、生产 build 与 diff-check 通过；无后端、schema、migration、main、生产、部署或 GitHub Actions 变更。现进入 Task 8 集成矩阵与全分支终审。

**T03-R1 Task 8 集成验证与全分支终审（2026-08-13）：** 状态：READY_FOR_ROOT_REVIEW（尚未合并、推送、部署或线上验证）。已执行负面门禁、冻结旧候选 `ce5691527` 的只读 RED、后端/前端集成矩阵、迁移/hash/config/diff 检查与全分支终审。迁移 222 SHA-256 为 `47f786d6b2b020d0211a17d4ccd2bc6bb3774a315f483fdc0ac45657c9ee738e`，相对 merge base `19492c57da24270eb2b3e9b5d9727c2865aebb9e` 仅新增该 SQL 和测试，无既有迁移修改、配置或 GitHub Actions delta。后端 migration/repository/service/handler 测试、vet、build，前端 53 项 Vitest、typecheck、production build 与 diff-check 均通过。终审确认原 `SubUpstreamCostService.GetByUsageID` nil-service fallback 违反管理员本地读取/零上游 HTTP 合同；已最小修复为只读 `AccountFinancialService.GetUsageEvidence`，本地服务缺失时 fail-closed 返回 500，并以真实测试上游证明 HTTP 调用为 0。最终 Spec APPROVE、Quality APPROVE、open findings 0；`downtime_required=unverified until root preflight`；`stash@{0}` 保留。禁止自行合并 main、推送、部署或启动 T05。

**T03-R1 根任务合并、推送与无停机预检（2026-08-13）：** 状态：进行中（已合并、已推送、生产发布被停机门禁阻断，尚未部署或线上验收）。候选 `2e28308a04495482fbe16af05344111bb4a0b0a7` 经精确根授权合并为 `main@dbe1231e0243d7190996f21cd8b8e5ac17b8420a`；唯一冲突为总账，已同时保留监控 P95 热修与 T03-R1 记录。当前已推送发布基线为 `main@f0746826c10a83688ad4363ac860a74f0f041e32`、tree `73dd8ed30451a5e018871f5f52816e5883585c17`。合并后重新通过后端迁移/repository/service/admin-handler/handler 测试、vet、server build，前端 6 文件 53/53 Vitest、typecheck、production build、负向守门与 diff-check；`0600` 证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-13-main-f0746826c10a83688ad4363ac860a74f0f041e32-t03-r1-v1.json`，迁移集合 hash 为 `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`。受控宿主蓝绿预检返回 `downtime_required=true`、`reason_code=migration_set_changed`、预计不可用 300 秒；在启动候选、流量切换、停服务、迁移或重启前停止，生产未变更。回滚保持当前活动槽；如需继续，必须由用户明确授权“允许停机部署”，之后才可走受审的维护发布路径。T05 继续禁止启动。

**T03-R1 停机授权后的维护发布契约核验（2026-08-14）：** 用户已明确授权“允许停机部署”。根任务核验时 `main@fbd85f608fb6519a52e87b6a8d71011efcfa90a2` 与 `origin/main` 一致、工作树干净；相对已验证 `f0746826` 仅总账/队列文档变化。当前活动迁移集为 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`，T03-R1 候选集为 `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`。受审宿主执行器只列出至 `MAINTENANCE_6`（`fadb… -> f1b1…`），没有 `f1b1… -> 6a0e…` 的精确 allowlist；维护标志配合正确旧 hash 仍会在任何停服、迁移、重启或切换前以 `migration_set_changed` fail-closed。已运行本地 `tests/operations/deploy_sub2api_blue_green_host_test.sh`，通过 fail-closed 与预加载运输契约；未调用生产发布链，生产未变更。下一步必须以独立用户可见顶层维护任务补充并复审唯一的精确迁移转换，再从更新后的 `main` 重新验证、生成绑定证据并使用已授权维护路径发布；不得伪造或绕过 allowlist，T05 继续禁止启动。**状态：进行中（停机授权已具备，维护发布契约缺口待独立任务闭合）。**
