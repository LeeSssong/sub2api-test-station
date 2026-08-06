# 全站账务运行时 30 分钟生产实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 30 分钟内把全站账务、分组归属和运营聚合代码部署到内部 relay-ops，并让 `/admin/accounts/monitor` 可读取受保护账务数据。

**Architecture:** relay-ops 继续作为无 UI 的后台任务服务；唯一管理员页面是 Sub2API `/admin/accounts/monitor`。本任务只发布运行时代码和受保护接口，不伪造账单授权或非零数据。

**Tech Stack:** Go、PostgreSQL、Docker、Caddy、relay-ops immutable/preloaded host executor。

## Global Constraints

- 前置条件：账号评分任务已生产部署并验证。
- 先读 `docs/project/active-delivery-contract.md` 和三个来源任务完整历史。
- 账务范围是全站所有可计费账号，不按供应商名称限定。
- 不暴露 relay-ops UI，不恢复 `/relay-ops/api/ops-view`。
- 本任务不执行账单凭据 provisioning；真实数据闭环由下一任务完成。
- 目标在 30 分钟内完成；完成更新、生产部署和线上验证优先，不得仅因超过目标时间停止。总账仅由协调任务更新。

---

### Task 1: 构建并部署全站账务运行时

**Files:**
- Read: `docs/runbooks/relay-ops-release-path.md`
- Read: `docs/runbooks/whole-site-accounting-ledger.md`
- Verify: `relay-ops-service/internal/reconciliation/collector.go`
- Verify: `relay-ops-service/internal/reconciliation/runtime.go`
- Verify: `relay-ops-service/internal/store/postgres.go`
- Report: `docs/superpowers/reports/2026-08-02-accounting-runtime-production.md`

**Interfaces:**
- Produces: 全站 Sweep、管理员 Refresh、DailyClose。
- Produces: 受保护的运营汇总、历史和对账接口。
- Preserves: `/relay-ops/api/ops-view` 为 404；管理员接口未认证为 401。

- [ ] **Step 1（0-4 分钟）：确认基线和前置生产状态**

Run:

```bash
git status --short
git rev-parse HEAD
ssh -o BatchMode=yes -o ConnectTimeout=10 sub2api-prod sudo -n true
ssh -o BatchMode=yes -o ConnectTimeout=10 sub2api-prod "sudo -n docker ps --format '{{.Names}} {{.Image}} {{.Status}}'"
```

Expected: worktree clean；HEAD 是账号评分任务完成后的唯一基线；生产评分已验收；relay-ops、Caddy、Sub2API 双槽、worker、PostgreSQL、Redis 健康。

- [ ] **Step 2（4-10 分钟）：运行最小账务回归**

Run:

```bash
(cd relay-ops-service && go test ./internal/reconciliation ./internal/store ./internal/http ./internal/app -count=1)
(cd relay-ops-service && go vet ./...)
bash tests/relay_ops/validate_relay_ops_contract.sh
git diff --check
```

Expected: 全部通过；Collector 遍历所有合法来源，Sweep/DailyClose 使用全量账号范围。

- [ ] **Step 3（10-13 分钟）：确认迁移、镜像与共享容器回滚基线**

读取当前 relay-ops image ID、迁移 hash、release state 和 PostgreSQL/Redis/Caddy/Sub2API 容器 ID。所有值只写入 0600 证据文件，不打印 secret。若存在 release lock、partial record 或共享容器漂移，立即停止。

- [ ] **Step 4（13-22 分钟）：执行 relay-ops 不可变发布**

按照 `docs/runbooks/relay-ops-release-path.md` 使用现有 controller/host executor；优先复用已构建且标签与当前 source tree、tested tree、迁移 hash 完全一致的 linux/amd64 镜像。只重建 relay-ops，脚本保留阶段超时，超过 30 分钟目标后仍在原范围内继续到生产验收完成。

Expected: 新 relay-ops healthy；内部 `/healthz` 为 `alive`、`/readyz` 为 `ready`；迁移启动成功；所有共享容器 ID 不变；失败时恢复上一 immutable 镜像并复验健康。

- [ ] **Step 5（22-28 分钟）：线上接口验收**

从生产机回环验证：

```text
/healthz 与 /readyz 返回 JSON 200；
/relay-ops/api/ops-view 返回 404；
未认证账务/对账/运营接口返回 401，而不是前端 HTML；
合法管理员会话能从 /admin/accounts/monitor 读取全站、分组和账号账务范围；
中文页面没有直接展示数据库英文表名或字段名。
```

- [ ] **Step 6（28-30 分钟）：交付证据**

报告推送 SHA、relay-ops 运行镜像、迁移、共享容器不变性、页面/API 结果和回滚点。明确写出真实账单数据仍由下一任务验收，不把空数据称为账务完成。
