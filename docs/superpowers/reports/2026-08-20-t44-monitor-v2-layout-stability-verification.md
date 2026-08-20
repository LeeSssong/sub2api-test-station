# T44 Monitor V2 时间线布局稳定性与卡片防抖验证报告

## 候选身份

- 基线：`main@2d7c38de2c932478ed82b415e201855ef75839e4`
- 分支：`codex/t44-monitor-v2-layout-stability`
- 实现提交：`7942e8d496c8d6ee5a4a40bd4d5623d634f8a984`
- 当前 HEAD/tree（文档提交前）：`7942e8d496c8d6ee5a4a40bd4d5623d634f8a984` / `01fc18b8fc906f894672dc47632c4635d908e118`
- 状态：候选实现完成，文档提交后进入 `READY_FOR_ROOT_REVIEW`

## 实现范围

1. `MonitorV2Timeline.vue` 去除桌面固定 `620px` 轨道；通过 `--timeline-count` 与 CSS Grid 让 24/28/30 桶均匀填充父列。
2. 小屏在 `@media (max-width: 639px)` 下切换为 `min-width: 620px` 的内部 flex 轨道，横向滚动继续限定在 `data-timeline-scroll`。
3. 时间柱移除 translate/scale 几何变换，仅保留颜色/阴影反馈；tooltip 结构、状态映射和无数据表达不变。
4. `MonitorV2GroupCard.vue` 移除 `transition-all` 与 `hover:-translate-y-0.5`，改为只过渡背景、边框和阴影，保持恒定 1px border 与卡片高度。
5. 更新时间线/页面直接相关测试，锁定 24/28/30 count、移动端内滚动与无几何位移合同。

## 新鲜验证

在最终实现 HEAD（`7942e8d`）上执行：

- `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`：2 files / 18 tests passed。
- `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/features/monitor-v2/__tests__/CodexRadarRecommendations.spec.ts src/features/monitor-v2/__tests__/CodexRadarCommunityMatrix.spec.ts src/features/monitor-v2/__tests__/api.spec.ts`：6 files / 35 tests passed。
- `pnpm typecheck`：exit 0。
- `pnpm build`：exit 0（仅已有 Vite 动态导入/Browserslist 提示）。
- `git diff --check 2d7c38de2c932478ed82b415e201855ef75839e4...HEAD --`：exit 0。
- 变更范围仅 4 个前端组件/测试文件；无 backend、migrations、config、`.github/workflows` 或生产数据文件。

## 未验证项

- 未执行真实设备/登录态浏览器截图；由根发布后及用户真机验收完成。
- 未执行合并后的 root-main 回归、发布预检、推送、部署或线上专项验收。

## 发布属性与回滚

- 迁移：无。
- 配置：无。
- 生产数据写入：无。
- 候选 `downtime_required=false`，以根预检为最终事实。
- 回滚：恢复 T44 提交或切回上一不可变镜像，无数据回滚。
- 剩余风险：浏览器字体/缩放会造成柱宽视觉差异；小屏固定 620px 仍需真机确认，但页面级横溢出由内部滚动容器隔离。

## 结论

T44 直接功能实现与相关前端验证完成，候选停在 `READY_FOR_ROOT_REVIEW`；未合并、未推送、未部署、未修改根 `main` 或全局总账。
