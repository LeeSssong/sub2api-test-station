# 全站账单数据闭环 30 分钟生产实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 30 分钟内为生产所有可计费账号建立合法账单授权与映射，执行真实采集、对账和日结，并在 `/admin/accounts/monitor` 验证非零数据。

**Architecture:** 使用已部署的 root-only provisioning wrapper 写入专用 `billing_read` 会话；Collector 自动遍历全部合法来源。凭据不进入 Git、聊天、日志或页面。

**Tech Stack:** relay-ops provisioning、PostgreSQL、上游账单 API、Sub2API Admin API、`/admin/accounts/monitor`。

## Global Constraints

- 前置条件：账务运行时代码已生产部署并验证。
- 先读 `docs/project/active-delivery-contract.md` 和三个来源任务完整历史。
- 不写死 Wawazz、Neko、账号 ID 或供应商名称；从当前 Sub2API 启用且可计费账号动态发现范围。
- 只使用已有合法专用只读账单凭据；不得从浏览器存储、用户凭据或加密账号秘密中提取替代品。
- 所有面向人的数据名称使用中文。
- 若某个账号缺合法只读凭据或适配器，本任务必须列出中文缺口并保持全站闭环“进行中”。
- 目标在 30 分钟内完成；真实数据闭环和线上验证优先，不得仅因超过目标时间停止。总账仅由协调任务更新。

---

### Task 1: Provision、采集和真实数据验收

**Files:**
- Read: `docs/project/active-delivery-contract.md`
- Read: `docs/runbooks/whole-site-accounting-ledger.md`
- Verify: `ops/provision-billing-source-host.sh`
- Report: `docs/superpowers/reports/2026-08-02-accounting-data-closure-production.md`

**Interfaces:**
- Consumes: 当前 Sub2API 启用且具备账单能力的账号集合。
- Produces: 账单授权会话、上游成本快照、对账请求明细、对账执行记录、每日账务快照。

- [ ] **Step 1（0-5 分钟）：动态盘点全站覆盖范围**

通过受保护 Admin API 或生产数据库只读查询生成账号覆盖矩阵：账号 ID、账号类型、是否启用、是否可调度、账单适配器、是否已有专用只读授权、是否有账单账号映射。输出不得包含供应商 secret、Cookie、Token 或完整 Base URL 查询参数。

Expected: 每个应计费账号明确为“可采集”或“缺授权/缺映射/缺适配器”；不得把历史上游登记当成当前账号范围。

- [ ] **Step 2（5-10 分钟）：安全写入全部已具备条件的来源**

使用生产已安装的 root-only provisioning wrapper，逐个提交声明文件；wrapper 必须验证 root ownership、0600、immutable secret ref、`billing_read` scope、active bearer 和正数 billing account mapping。写入后只查询非敏感映射与数量。

- [ ] **Step 3（10-17 分钟）：执行全量 Sweep 和管理员刷新**

触发全量 Sweep，随后通过合法管理员会话调用 Refresh。每账号独立超时和故障隔离；一个账号失败不能抹掉其他账号的成功数据。不得制造模型流量或伪造上游账单。

- [ ] **Step 4（17-23 分钟）：执行对账与日结**

按当前上海业务日执行 reconciliation 和 DailyClose。请求匹配先使用上游请求 ID，未命中再使用本站请求 ID；采集失败、未配置来源、未决/冲突成本或上游交易无本站请求时，日结必须阻断并给出中文原因。

- [ ] **Step 5（23-28 分钟）：验证页面和五类真实数据**

在 `/admin/accounts/monitor` 验证：

```text
账单授权会话 > 0；
上游成本快照 > 0；
对账请求明细 > 0；
对账执行记录 > 0；
每日账务快照在闭合业务日 > 0；
全站、分组、账号汇总不会因跨分组账号重复；
用户实际计费、上游真实扣费、纸面利润、利润率、覆盖率和待对账数量可解释。
```

若当天没有任何真实业务请求，允许对账请求明细保持 0，但不得把它标记为真实账务闭环完成；必须等待自然请求或单独获批的最小真实请求。

- [ ] **Step 6（28-30 分钟）：交付覆盖矩阵和证据**

报告只含中文业务名称、账号 ID、覆盖状态、非敏感数量、失败原因、生产运行镜像和回滚点。只有所有应计费账号覆盖且存在可解释真实数据时才报告完成。
