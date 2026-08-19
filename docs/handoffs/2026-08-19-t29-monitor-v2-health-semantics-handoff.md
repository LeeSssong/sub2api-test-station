# T29 Monitor V2 二态健康展示与统一指标口径交接

状态：`READY_FOR_ROOT_REVIEW`

任务：T29

基线：`main@3fd0e86bf9f24541a10c2f189e17d3c36c01272f`

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
- 未修改根 `main`、全局队列/总账、发布脚本或 GitHub Actions。

## 验证

- `go test ./internal/repository ./internal/service ./internal/handler -run 'TestMonitorV2' -count=1`
- `go build ./cmd/server`
- Monitor V2 前端 8 个测试文件，共 34 个测试。
- `vue-tsc --noEmit`
- `vue-tsc -b` 与 Vite production build。
- `git diff --check`、gofmt、迁移/配置/GitHub Actions 范围检查。
- 本地视觉：桌面 `/Users/gongtengxinwen/.codex/worktrees/t29-monitor-v2-health-semantics/output/playwright/t29-monitor-desktop.png`；390px `/Users/gongtengxinwen/.codex/worktrees/t29-monitor-v2-health-semantics/output/playwright/t29-monitor-mobile-390.png`。桌面 `clientWidth=1432 / scrollWidth=1432`，390px 文档 `clientWidth=382 / scrollWidth=382`；未发现重叠或横向溢出。

## 发布属性与回滚

- 数据库迁移：无。
- 配置或配置 schema：无。
- 生产数据写入：无。
- `downtime_required=false`；最终仍以根总控在合并后执行的发布预检为准。
- API 合同从 v5 升到 v6；前端与后端必须随同一不可变镜像发布。
- 回滚使用上一活动蓝绿槽/不可变镜像，恢复 v5 前后端组合；不涉及数据回滚。

## 剩余风险与根总控后续

- T29 基线早于正在发布验收的 T28。T28 线上验收完成后，根总控应先要求 T29 在本 worktree 刷新到届时最新干净 `main`，仅解决本任务范围冲突并重跑上述直接门禁，再授权合并。
- 统一主模型统计依赖主动监控记录中的最新非空 `PrimaryModel`；缺少该模型或少于 5 个合格样本时页面显示破折号，不猜测数值。
- 生产合并后重点验收：无任何百分比/成功率/缓存率，所有状态严格二态，Pro 第一且带旗舰徽标，TTFT/TPS/总延迟/倍率仍为真实值，桌面与 390px 无溢出。

候选提交 SHA 以本分支最终 `git rev-parse HEAD` 为准；本任务不合并、不推送、不部署、不清理 worktree。
