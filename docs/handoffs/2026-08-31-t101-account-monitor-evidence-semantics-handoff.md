# T101 账号监控证据语义修复交接

日期：2026-08-31
状态：`READY_FOR_ROOT_REVIEW`（候选已刷新；未合并、未推送、未部署）

## Delivered

- Repository 为每个请求账号固定返回 24 个按时间顺序排列的真实请求桶，空桶保留为 0/null；SQL 非空聚合按桶索引覆盖。
- 全站账号卡利润率显示“按分组查看”；具体分组继续显示已确认或估算利润率，其他既有状态语义保持不变。
- 图表标题改为“真实性能 · 真实请求”；按钮改为“刷新探测状态”，可访问名称明确主动探测不生成真实请求样本。
- 没有新增页面、API、状态存储、依赖、迁移、配置或平行事实源。

## Candidate

- 分支：`codex/t101-account-monitor-evidence-semantics`
- 初始基线：`main@3c5b9710a807904b8449708c71e3931b7f838490`
- 已无冲突刷新到：`main@879787096a7bc4b3ff4ab4820d4a5f3c3a63a29a`
- 规格：`b3922e3d1`；计划：`2d98f8346`
- 后端实现：`ff0ff3670`；前端实现：`00af1e05a`
- 刷新合并：`2e4884087`，tree `7b2217bf7f85abea65fcaab537d540920326973a`
- 最终候选为包含本交接文档提交的分支 `HEAD`。

## Verification

- Go 1.27.0 repository 定向测试与完整 `cmd/server` 编译通过。
- 前端 2 files、56/56 tests、`vue-tsc --noEmit` 和 1098 modules production build 通过。
- 桌面全站/具体分组和 390x844 移动截图通过；24 根柱等宽，移动端无整页横向溢出。
- `git diff --check` 通过；无 workflow、migration、lockfile、Go 依赖或配置变化。
- 完整命令、截图路径和剩余风险见 `docs/superpowers/reports/2026-08-31-t101-account-monitor-evidence-semantics-verification.md`。

## Root Integration Boundary

- 当前 T99 为 `INTEGRATING` 且等待独立主站发布授权，唯一整合/部署车道尚未释放；T101 只能保持 `READY_FOR_ROOT_REVIEW`。
- 车道释放后，根发布总控应确认 `main` 未漂移；若漂移则先在本候选刷新并重跑直接相关验证，再发出精确 `AUTHORIZE_MERGE_TO_MAIN`。
- 合并后只需重跑本任务 repository、card/view、typecheck/build、范围检查与受控发布预检。
- 无 migration 或配置回滚步骤。若发布后撤回，在根 `main` 反转 `ff0ff3670` 与 `00af1e05a` 的运行代码变化并走既有发布链。

## Deployment Gate

- `downtime_required=unverified until root preflight`。
- 可以在车道释放后部署验收站并使用真实管理员登录态做专项验收。
- 主站只接受用户明确说“测试站验收通过，部署主站”或“快速部署到主站”；当前指令不满足该门禁。
