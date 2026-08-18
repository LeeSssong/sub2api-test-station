# T25 自建渠道监控最终视觉与主动探测重试实施计划

**目标：** 在现有 Monitor V2 合同上完成最终页面收口，并使每模型主动探测最多执行 6 次、成功即停。

**架构：** 前端仅修改 Monitor V2 组件与双语文案；后端在 `runCheckForModel` 外增加可测试的单模型重试函数，由现有并发调度调用，历史持久化仍只接收最终结果。

## 1. 前端 RED

- 修改 `MonitorV2View.spec.ts`，断言真实请求文案、P50/P95 保留、圈选项消失、倍率强化标记存在。
- 修改 `MonitorV2Timeline.spec.ts`，断言成功、失败、空桶全部为同一颜色和高度。
- 运行两个 focused tests，确认因当前实现而失败。

## 2. 前端 GREEN

- 修改 `MonitorV2GroupCard.vue`、`MonitorV2Timeline.vue`、`MonitorV2View.vue`。
- 修改中英文 `dashboard.ts` 的调用证据文案。
- 运行前端 focused tests。

## 3. 后端 RED/GREEN

- 新增 `channel_monitor_retry_test.go`，以可控执行函数覆盖首次成功、第六次成功、六次全失败和取消停止。
- 运行测试确认 RED。
- 在 `channel_monitor_service.go` 增加最小重试函数并接入 `runChecksConcurrent`。
- 在 `channel_monitor_const.go` 定义重试次数；在 runner 中同步扩展超时预算。
- 运行 focused Go tests并 gofmt。

## 4. 直接验证与视觉验收

- 运行 Monitor V2 前端测试、typecheck/build。
- 运行 channel monitor 直接相关测试、service compile-only、server build、diff-check。
- 启动候选 Vite 页面，以真实组件 fixture 截取桌面和移动端；检查同高同色、倍率、删项和溢出。

## 5. 根整合与发布

- 提交候选并形成 handoff；根审查范围后合入 `main`。
- 在合并后的 `main` 重跑最小门禁和发布预检。
- `downtime_required=false` 时推送并使用既有本地/宿主蓝绿链发布；线上确认自建模式、页面合同、探测记录与公网健康。
- 更新总账和队列为 `DONE`，记录发布源、宿主记录、证据和回滚依据。
