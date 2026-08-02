# 账号评分与监控入口 30 分钟生产实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 30 分钟内把已实现的分组评分、服务健康和经营视图部署到 `/admin/accounts/monitor`，并完成线上保存/恢复默认验收。

**Architecture:** 复用现有 Sub2API 管理端、迁移 188 和 account monitor 服务，不新增页面或评分系统。使用已授权的 additive-migration maintenance release；PostgreSQL、Redis 和 Caddy 不停止或重建。

**Tech Stack:** Go、PostgreSQL、Vue 3、TypeScript、Docker、现有 Sub2API maintenance host executor。

## Global Constraints

- 先读 `docs/project/active-delivery-contract.md` 和三个来源任务完整历史。
- 唯一页面是 `/admin/accounts/monitor`；不恢复 relay-ops UI。
- 默认权重固定为 `15/45/20/20`，只影响监控排名。
- 用户已经确认接受本次迁移所需的受控维护停机；不可用窗口上限 300 秒。
- 只发布账号评分与监控入口，不修复无关全量测试或改写账务/飞书实现。
- 总时间预算 1800 秒；第 22 分钟仍未开始生产变更则停止，不临时扩大范围。
- 总账由协调任务更新；本任务不得修改 `docs/project/project-progress.md`。

---

### Task 1: 绑定候选、验证和生产发布

**Files:**
- Read: `docs/project/active-delivery-contract.md`
- Read: `docs/runbooks/sub2api-blue-green-production-deployment.md`
- Verify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Verify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Verify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Report: `docs/superpowers/reports/2026-08-02-account-monitor-scoring-production.md`

**Interfaces:**
- Produces: `GET/PUT/DELETE /api/v1/admin/account-monitors/groups/:group_id/score-weights`。
- Produces: `/admin/accounts/monitor` 分组 Tab、服务健康、经营汇总、评分和卡片优先级编辑。

- [ ] **Step 1（0-3 分钟）：确认唯一基线和生产锁**

Run:

```bash
git status --short
git rev-parse HEAD
git diff --quiet a0f343588..f560400f2 -- upstream/sub2api
ssh -o BatchMode=yes -o ConnectTimeout=10 sub2api-prod sudo -n true
```

Expected: worktree clean；HEAD 是协调任务指定基线；Sub2API 源码自合格候选 `a0f343588` 后无差异；SSH 和免密 sudo 成功。任一不符立即停止。

- [ ] **Step 2（3-8 分钟）：运行目标回归**

Run:

```bash
(cd upstream/sub2api/backend && go test ./internal/repository ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor' -count=1)
(cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts)
git diff --check
```

Expected: 全部通过且 worktree 仍干净。不得因既有无关测试失败进入修复支线。

- [ ] **Step 3（8-12 分钟）：确认迁移和回滚身份**

Run:

```bash
ssh -o BatchMode=yes -o ConnectTimeout=10 sub2api-prod "sudo -n docker image inspect 5540e46c1b20 --format '{{json .Config.Labels}}'"
ssh -o BatchMode=yes -o ConnectTimeout=10 sub2api-prod "sudo -n test -x /usr/local/libexec/deploy-sub2api-blue-green-host.sh"
```

Expected: 活动迁移 hash 为 `176e6659b45bffbf11f5e1fce7dfbaf60906fe974553d7156fdc516231f4f5d0`；只允许迁移到 `c618fc284897bb24c662297ba6cb263064a1e04a024e5432f50f082ac7317408`；上一镜像与 release state 可回滚。

- [ ] **Step 4（12-22 分钟）：执行已授权维护发布**

使用现有 0600 测试证据和发布环境，不打印 secret：

```bash
bash ops/release-sub2api-blue-green.sh \
  --mode production \
  --evidence "$SUB2API_TEST_EVIDENCE" \
  --maintenance-authorized
```

Expected: 总预算由 controller 限制为 1800 秒；维护不可用窗口不超过 300 秒；只停止 API/worker；PostgreSQL、Redis、Caddy 身份不变。失败时证明恢复旧 API/worker 镜像和 Caddy 上游。

- [ ] **Step 5（22-28 分钟）：线上验收**

必须使用合法管理员会话验证：

```text
/admin/accounts/monitor 可以打开；分组 Tab 按倍率降序；
默认评分显示 15/45/20/20；四项合计不是 100 时不能保存；
保存一组权重后只影响该组；恢复默认返回 15/45/20/20；
账号卡修改原生优先级后立即刷新；关闭分组不显示持续故障。
```

同时验证评分 API 不再是 404，生产运行镜像标签绑定本次提交，基础容器身份不变。

- [ ] **Step 6（28-30 分钟）：交付证据**

在报告中仅记录：推送 SHA、运行镜像/提交、迁移结果、页面/API 验收、基础容器身份、回滚点和未验证项。将最终结果发送给协调任务；没有完整三项证据时报告“进行中”。
