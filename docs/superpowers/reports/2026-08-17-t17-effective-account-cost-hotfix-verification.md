# T17 用量详情有效账号成本口径热修验证报告

## 范围

- 任务包：T17
- 候选实现基线：`main@120562f95`（候选随后包含 `e887bfe44` 刷新提交）
- 实现 tip：`e6fde59ba`
- 目标：管理员用量详情的“上游扣费/利润”统一使用 Sub 原生有效账号成本；严格 upstream evidence 只作辅助核验状态。
- 分支：`codex/t17-effective-account-cost-hotfix`

## 已验证命令

前端（工作目录 `upstream/sub2api/frontend`）：

- `pnpm exec vitest run src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/UsageDetailDialog.compat.spec.ts`：通过，3 个文件 / 38 个测试。
- `pnpm typecheck`：通过。
- `pnpm build`：通过，Vite 完成 1046 个模块转换并生成生产资源。

后端（工作目录 `upstream/sub2api/backend`）：

- `go test ./internal/handler/admin -run 'TestAdminUsageGetByID|TestAdminUsageGetUpstreamCost' -count=1`：通过。
- `go test ./internal/handler/dto -run 'Test.*Usage.*' -count=1`：通过。
- `go test ./internal/server -run '^$' -count=1`：通过（编译门禁，无测试文件）。
- `go build ./cmd/server`：通过。
- 受影响 Go 测试文件执行 `gofmt -w` 后无工作区差异。

范围检查：

- `git diff --check`：通过。
- 数据库迁移改动：0。
- `.github/workflows` 改动：0。
- 后端生产代码改动：0；现有 DTO/API 字段契约已由 focused tests 覆盖。

## 行为矩阵

主金额公式固定为：

`COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`

- 显式 `account_cost`（包括 0）直接作为有效账号成本，不再次乘倍率。
- `account_cost` 为空时，使用 `account_stats_cost`，再乘 `account_rate_multiplier`。
- `account_stats_cost` 也为空时，使用 `total_cost`，再乘倍率。
- 所有成本源为空或非有限数时，主金额保持 `-`。
- 利润始终为 `actual_cost - effective_account_cost`。
- evidence `confirmed`、`unavailable` 或 evidence 请求失败均不替换、不隐藏主金额和利润。
- PascalCase/snake_case evidence 兼容层继续保留，strict evidence 的状态、原因和来源仍可辅助展示。

## 边界与未验证项

- 未修改价格、倍率、成本写入、经营页聚合、账务幂等、数据库、生产数据或历史 evidence 表。
- 未运行全仓测试、压力/soak/mutation、race 或无关浏览器矩阵；这些不属于 T17 直接相关门禁。
- 根发布链以普通预加载蓝绿模式完成，迁移哈希保持 `aaebed88f7fb712e1f518e73cc89bd44eb214f365f3b49f003598c93883a4604`，等效 `downtime_required=false`；未使用维护授权。
- 功能回滚方式：恢复上一版已验证的不可变蓝绿镜像；本改动无数据库迁移、无历史回填、无运行时开关。

## 生产发布与验收

- 发布源：已推送的根 `main@892db8cefb37bcab14b0aded8082811ac3935f48`，tree `ff44ca32ccbb79c64e1dfecfa9e1484ad9ff24b8`。
- 0600 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-892db8cef-t17-effective-account-cost-v1.json`。
- 不可变镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-892db8cefb37bcab14b0aded8082811ac3935f48-3e150376a8e786004b8df26ab10fa6ecac2fa9f991ada91eda31be38e4bfe28a`。
- 宿主 final record：`/var/lib/sub2api/release-records/20260817T102828Z-production-2034943.json`，`result=succeeded`、`state=promoted`、`rolled_back=false`。
- 活动槽：`blue`；API 与 worker 使用同一不可变镜像，均为 `running/healthy/restart 0`；上一 `green` 槽保持 healthy 作为回滚依据。
- 公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。
- 控制器在宿主成功写入 final record 后遇到 SSH 连接关闭，因未收到最终 stdout JSON 本地返回失败；只读核对确认 release-state、final record、容器和镜像标签均已一致完成，且没有残留 `.partial`/`.failed` 文件。该故障属于成功落盘后的传输层假阴性，不改变生产发布结果。
- 生产 API 对 `usage_log_id=125512` 的列表与详情均返回 `account_cost=0.00600594`、`actual_cost=0.0100099`；严格 evidence 为 `unavailable/endpoint_unsupported`。
- 登录态页面验收：使用记录首行显示账号成本 `$0.012441`；对应详情显示“上游实际扣费 `$0.012441`”与“利润 `$0.012949`”，同时保留严格账单不可用提示，未再显示 `-`。
- 恢复 bundle：`/Users/gongtengxinwen/Documents/sub2api-archives/t17-effective-account-cost-hotfix-9ffbdbc2.bundle`，`git bundle verify` 通过，SHA-256 `c8aa71b345f74486e97cafdd2a6078afe22b8fa6da62c7c35386646c767c3879`。
- 在确认候选干净、已为根 `main` 祖先、远端和生产闭环后，T17 功能 worktree/分支及本次临时 release worktree 已安全清理；另一个既有用户可见 T17 worktree `codex/t17-usage-detail-effective-cost` 未修改、未清理。

## 结论

T17 已完成实现、直接相关验证、根合并、推送、无停机蓝绿发布和线上验收，可标记为 `DONE`。
