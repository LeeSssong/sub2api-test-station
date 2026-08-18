# T22 Channel Monitor Ops View 交接

状态：`READY_FOR_ROOT_REVIEW`  
基线：`main@9d5f658d039ae6f076e558c9d60f01d8de7993f7`  
分支：`codex/t22-channel-monitor-ops-implementation`  
工作区：`/Users/gongtengxinwen/.codex/worktrees/1181/sub2api搭建`

## 交付内容

官方 Channel Monitor V2 已调整为默认 24h 的简洁运营视图。首屏展示整体状态、成功率、首 Token、缓存率、分组矩阵和趋势；模型明细、错误分类、用户排行改为可访问的详细分析并按需加载。无流量和低样本分别显示“已就绪·暂无流量”和“待观察”，不进入健康评分；真实评分和黄/红异常保留。

## 变更文件

- `upstream/sub2api/frontend/src/features/channel-monitor-v2/monitorFormat.ts`
- `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts`
- `upstream/sub2api/frontend/src/features/channel-monitor-v2/RelayPulseMatrix.vue`
- `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts`
- `upstream/sub2api/frontend/src/views/user/ChannelStatusV2View.vue`
- `upstream/sub2api/frontend/src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/channelMonitorV2.ts`
- T22 规格、计划和实现自审记录

## 验证

- `pnpm vitest run src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts src/features/channel-monitor-v2/__tests__/MetricCell.spec.ts src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts src/views/user/__tests__/ChannelStatusView.mode.spec.ts`
- `pnpm typecheck`
- `pnpm build`
- `git diff --check`
- Playwright 本地 fixture：1440x900、390x844；两者页面宽度无溢出，24h 激活，详细分析可展开。

## 发布与回滚

- 后端/API/迁移/配置 schema：无变更。
- 预期 `downtime_required=false`；最终以根总控发布预检为准。
- 回滚优先使用 `channel_monitor_mode=v1`；代码级回滚使用上一活动槽/不可变镜像。
- 本 worktree 未修改根 `main`、全局队列/总账，未推送、部署或访问生产。

## 根总控后续

审查候选提交后，在合并后的 `main` 上运行既有发布预检、构建/专项回归、部署和线上登录态验收；重点确认默认 24h、四个时间窗、详细分析请求时机、真实低样本文案以及桌面/390px 无整页溢出。候选提交 SHA 以本分支最终 `git rev-parse HEAD` 为准。
