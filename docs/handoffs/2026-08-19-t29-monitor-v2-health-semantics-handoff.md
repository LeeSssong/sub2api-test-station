# T29 Monitor V2 二态健康展示与统一指标口径交接

状态：`READY_FOR_ROOT_REVIEW`

任务：T29

原始基线：`main@3fd0e86bf9f24541a10c2f189e17d3c36c01272f`

刷新主线：`main@25eb6a63c99e3bb87d4e62acc8fce69f0089186e`

刷新合并提交（候选功能 tip）：`b09c88d4b2baaeacc54b9612698123815963ed7e`

刷新后候选功能 tree：`13dc9cf5bd2d06c27707a8e727a93fb4c1da0ac3`

分支：`codex/t29-monitor-v2-health-semantics`

工作区：`/Users/gongtengxinwen/.codex/worktrees/t29-monitor-v2-health-semantics`

## 交付内容

- Monitor V2 API 合同升级为 v6，分组、整体状态与时间线只使用 `operational / unavailable`，中文只显示“运行中 / 服务不可用”。
- 页面移除服务可用率、真实请求成功率、有效调用占比、缓存命中率及所有百分比；这些值也不进入时间线提示或无障碍文本。
- 卡片保留真实的 TTFT P50、总延迟 P50、输出 TPS 和倍率；样本数只用于后端最小样本门槛，不在页面显示。
- 单个 `eligible` CTE 按同一时间窗、分组、主模型、正实际成本、文本 Token 语义及完整性能证据计算 TTFT P50/P95、总延迟 P50/P95 和 TPS。
- Pro/旗舰分组稳定置顶并显示“旗舰”徽标；不复制 Plus 数值，不修改真实统计结果。
- 运行中时间线使用薄荷绿，服务不可用使用红色；桌面与 390px 均采用紧凑横向卡片且无整页横向溢出。

## 变更范围

- 后端 Monitor V2 repository、service、handler、Wire 及直接测试。
- 前端 Monitor V2 types、API 校验、卡片、时间线、页面、i18n 及直接测试。
- T29 规格、实施计划与本交接。
- 未修改根 `main`、全局队列/总账、发布脚本或 GitHub Actions；候选仅将根 `main@25eb6a63c99e3bb87d4e62acc8fce69f0089186e` 合入当前 worktree 用于刷新，未在根 `main` 上写入。

## 验证

- `go test ./internal/repository ./internal/service ./internal/handler -run 'TestMonitorV2' -count=1`
- `go build ./cmd/server`
- Monitor V2 前端 8 个测试文件，共 34 个测试。
- `vue-tsc --noEmit`
- `vue-tsc -b` 与 Vite production build（刷新后均退出码 0）。
- `git diff --check`、gofmt、迁移/配置/GitHub Actions 范围检查（刷新后均通过）。
- 刷新后 `git diff --name-only main..HEAD` 仅包含本交接、规格、计划和 T29 Monitor V2 后端/前端直接相关文件；无 T28 文件、迁移、配置或发布链文件。
- 本地视觉：桌面 `/Users/gongtengxinwen/.codex/worktrees/t29-monitor-v2-health-semantics/output/playwright/t29-monitor-desktop.png`；390px `/Users/gongtengxinwen/.codex/worktrees/t29-monitor-v2-health-semantics/output/playwright/t29-monitor-mobile-390.png`。桌面 `clientWidth=1432 / scrollWidth=1432`，390px 文档 `clientWidth=382 / scrollWidth=382`；未发现重叠或横向溢出。

## 发布属性与回滚

- 数据库迁移：无。
- 配置或配置 schema：无。
- 生产数据写入：无。
- `downtime_required=false`；最终仍以根总控在合并后执行的发布预检为准。
- API 合同从 v5 升到 v6；前端与后端必须随同一不可变镜像发布。
- 回滚使用上一活动蓝绿槽/不可变镜像，恢复 v5 前后端组合；不涉及数据回滚。

## 剩余风险与根总控后续

- T29 已在 T28 生产验收后的最新干净 `main@25eb6a63c99e3bb87d4e62acc8fce69f0089186e` 上完成刷新；刷新合并无冲突，`git diff main..HEAD` 仅包含 T29 文件。根总控应以刷新后的候选功能 tip/tree 进入后续合并门禁，并一并携带本次 T29-only handoff 元数据提交。
- 统一主模型统计依赖主动监控记录中的最新非空 `PrimaryModel`；缺少该模型或少于 5 个合格样本时页面显示破折号，不猜测数值。
- 生产合并后重点验收：无任何百分比/成功率/缓存率，所有状态严格二态，Pro 第一且带旗舰徽标，TTFT/TPS/总延迟/倍率仍为真实值，桌面与 390px 无溢出。

候选功能提交 SHA/tree：`b09c88d4b2baaeacc54b9612698123815963ed7e` / `13dc9cf5bd2d06c27707a8e727a93fb4c1da0ac3`。本次 handoff 更新作为 T29-only 提交落地，最终 tip/tree 以当前 worktree 的 `git rev-parse HEAD` 与 `git rev-parse HEAD^{tree}` 为准；本任务不推送、不部署、不清理 worktree。根总控合并前仍需确认 `main` 未漂移。
