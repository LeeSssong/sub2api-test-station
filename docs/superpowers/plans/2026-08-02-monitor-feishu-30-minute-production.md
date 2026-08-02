# Monitor 与飞书联动 30 分钟生产实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 30 分钟内部署 Monitor 最新状态和飞书历史错误码修复，并验证一次告警和一次恢复都链接 `/admin/accounts/monitor`。

**Architecture:** Sub2API 按最新探测结果产生当前状态；无 UI 的 relay-ops 复用现有出站通知器发送告警、恢复和日报。不新增 P0/P1 Bridge，不恢复飞书入站控制。

**Tech Stack:** Go、Sub2API Monitor V2、relay-ops accounthealth/dailyreport、Feishu App Bot、现有两个 production executor。

## Global Constraints

- 前置条件：全站账单数据闭环任务已结束并交付明确状态。
- 先读 `docs/project/active-delivery-contract.md` 和三个来源任务完整历史。
- 所有管理链接指向 `/admin/accounts/monitor`。
- 最新成功清除历史错误码影响；关闭分组只提醒一次。
- 不扩建原生 P0/P1 Bridge，不恢复任何入站命令或写操作。
- 不为验收制造可能影响用户路由的真实账号故障；使用现有受控合成验收入口或可恢复测试夹具。
- 总时间预算 1800 秒；总账仅由协调任务更新。

---

### Task 1: 部署并验证当前状态、告警和恢复

**Files:**
- Read: `docs/superpowers/specs/2026-07-31-monitor-current-status-and-feishu-stale-error-design.md`
- Read: `docs/superpowers/plans/2026-07-31-monitor-status-feishu-alert-fix-implementation-plan.md`
- Verify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Verify: `relay-ops-service/internal/accounthealth/`
- Verify: `relay-ops-service/internal/dailyreport/health.go`
- Report: `docs/superpowers/reports/2026-08-02-monitor-feishu-production.md`

**Interfaces:**
- Produces: 最新探测驱动的 Monitor 当前状态。
- Produces: 飞书告警、恢复、日报；卡片链接 `/admin/accounts/monitor`。

- [ ] **Step 1（0-4 分钟）：确认代码已在生产候选范围**

Run:

```bash
git merge-base --is-ancestor a1a10cdff origin/main
git status --short
ssh -o BatchMode=yes -o ConnectTimeout=10 sub2api-prod sudo -n true
```

Expected: 修复已在唯一基线；worktree clean；生产连接正常。检查账号评分发布后的 Sub2API 镜像是否已经包含 `a1a10cdff` 的 Sub2API 部分，只部署仍缺失的 relay-ops 部分。

- [ ] **Step 2（4-10 分钟）：运行目标回归**

Run:

```bash
(cd upstream/sub2api/backend && go test ./internal/service -run 'Test.*MonitorV2' -count=1)
(cd relay-ops-service && go test ./internal/accounthealth ./internal/dailyreport -count=1)
(cd relay-ops-service && go vet ./...)
git diff --check
```

Expected: 旧失败后最新成功显示正常；最新失败但近期成功显示降级；最新成功不继承历史余额耗尽错误码。

- [ ] **Step 3（10-20 分钟）：只部署缺失组件**

先比较生产 Sub2API/relay-ops 镜像标签与目标 tree。已由账号评分任务部署的 Sub2API 不重复发布；只通过 relay-ops immutable executor 重建仍缺修复的 relay-ops。共享容器身份必须保持不变，失败时恢复上一 relay-ops 镜像。

- [ ] **Step 4（20-27 分钟）：告警和恢复验收**

使用现有受控合成入口验证一个账号/分组状态序列：

```text
当前失败 -> 发送一条中文告警；
重复同证据 -> 不重复发送；
最新成功 -> Monitor 显示恢复并发送一条中文恢复；
历史 balance_exhausted -> 不再影响最新成功；
关闭分组 -> 只提醒一次；
所有卡片链接 -> /admin/accounts/monitor。
```

验收前后核对账号调度、余额、倍率、路由和用户数据均未被修改。

- [ ] **Step 5（27-30 分钟）：交付证据**

报告推送 SHA、实际运行的 Sub2API/relay-ops 镜像、告警和恢复投递记录、去重结果、卡片链接、零写边界和回滚点。缺少告警或恢复任一真实投递证据时保持“进行中”。
