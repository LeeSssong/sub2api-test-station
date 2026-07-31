# 项目全局进度总账 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立项目级任务登记和部署生效总账，让并行任务始终可见，并回填项目从建立至今的完成与进行中事项。

**Architecture:** 在根目录协作规则中加入统一生命周期约束，在 `docs/project/project-progress.md` 保存唯一总账。历史状态以生产验证报告为证据，按事项而不是文件数量统计；生产部署生效才算完成。

**Tech Stack:** Markdown、Git worktree、现有 `docs/superpowers/reports` 验证记录和 Git 历史。

## Global Constraints

- 实施前必须先登记到 `docs/project/project-progress.md`，初始状态为“进行中”。
- 任务结束必须同时满足：已推送到服务端、已部署、已验证生效。
- 仅本地/离线测试、镜像生成、代码合并或报告完成不得标记为“已完成”。
- 每项只写解决的问题、做的优化、影响范围、状态和证据，不写敏感凭据。
- 保留主工作区既有未提交改动，不使用 `git add -A`，只提交本任务文件。

## File Structure

| File | Responsibility |
|---|---|
| `AGENTS.md` | 项目级强制工作流，要求实施前登记和部署后收尾 |
| `CLAUDE.md` | 兼容现有项目执行入口，链接并重复关键结束标准 |
| `docs/project/project-progress.md` | 唯一任务总账、当前统计、历史事项和进行中事项 |
| `docs/project/current-state.md` | 增加总账入口，保持当前状态文档可发现 |
| `docs/superpowers/specs/2026-07-31-project-progress-ledger-design.md` | 记录已确认的设计和历史回填口径 |

### Task 1: Add the project-wide task lifecycle rule

**Files:**
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Produces: a rule visible to future sessions before implementation starts.

- [ ] **Step 1: Add the rule to `AGENTS.md`.** Add a short mandatory section requiring pre-registration, status updates, and the three-part completion gate.
- [ ] **Step 2: Add the same completion gate to `CLAUDE.md`.** Link the global ledger without replacing existing deployment/worktree rules.
- [ ] **Step 3: Verify the rule is discoverable.**

Run:

```bash
rg -n "project-progress|实施前|推送到服务端|部署.*验证|已完成" AGENTS.md CLAUDE.md
```

Expected: both files contain the ledger path and the deployment-effective completion rule.

- [ ] **Step 4: Commit only the rule files.**

```bash
git add AGENTS.md CLAUDE.md
git commit -m "docs: require project progress ledger lifecycle"
```

### Task 2: Create and backfill the global progress ledger

**Files:**
- Create: `docs/project/project-progress.md`
- Modify: `docs/project/current-state.md`

**Interfaces:**
- Consumes: `docs/superpowers/reports/*.md`, current-state documentation, and recent Git history.
- Produces: one concise ledger with current counts, completed production items, preparation-only items, and active items.

- [ ] **Step 1: Record the counting rule and current snapshot.** State the report inventory and explicitly distinguish completed production work from preparation-only work.
- [ ] **Step 2: Add the completed production items.** Summarize each item using problem, optimization, impact, status, and evidence; link to its verification report.
- [ ] **Step 3: Add preparation-only and in-progress items.** Include unattended release activation, qualified image administrator update, Feishu consolidation deployment, whole-site accounting implementation, and any other item whose evidence says pending or blocked.
- [ ] **Step 4: Add the ledger link to `docs/project/current-state.md`.**
- [ ] **Step 5: Verify every ledger row has the required summary fields and no sensitive values.**

Run:

```bash
rg -n "解决的问题|做了什么优化|影响范围|状态|证据" docs/project/project-progress.md
rg -n -i "(api[_ -]?key|token|cookie|password|secret|oauth)" docs/project/project-progress.md
```

Expected: required fields are present; the second command returns no credential values or sensitive data.

- [ ] **Step 6: Commit only the ledger and index link.**

```bash
git add docs/project/project-progress.md docs/project/current-state.md
git commit -m "docs: add project-wide progress ledger"
```

### Task 3: Perform an independent whole-branch consistency review

**Files:**
- Review: `AGENTS.md`
- Review: `CLAUDE.md`
- Review: `docs/project/project-progress.md`
- Review: `docs/project/current-state.md`

**Interfaces:**
- Consumes: Task 1 and Task 2 commits plus source reports.
- Produces: verified counts and a clean worktree with no unrelated staged files.

- [ ] **Step 1: Recount source reports and compare to the ledger snapshot.**
- [ ] **Step 2: Check that every “已完成” item has deployment evidence, and every pending item remains “进行中” or “准备完成”.**
- [ ] **Step 3: Run Markdown/link and sensitive-value checks.**
- [ ] **Step 4: Run `git status --short` and confirm only this branch’s intended changes exist.**
- [ ] **Step 5: Commit any review-only correction separately, or report that no correction is needed.**
