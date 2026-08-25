# T62 深色主题与呼吸动效候选交接

## 范围

仅优化第四套 `/custom/performance-monitor` 顶部“分组状态”卡片的深色主题与圆环呼吸动效：

- 深色卡片从纯白视觉改为冷蓝灰分层表面，圆环内部使用独立深色表面。
- 深色文字分为标题、指标、辅助信息三档，保持可读对比度并降低刺眼白色。
- 圆环呼吸周期调整为 2.8 秒，峰值外圈辉光扩大到 48px，并增加内圈辉光；不旋转、不位移百分比。
- 保留绿/黄/红状态语义、静止可用性百分比、P95、统一样本数、页面结构和下方推荐内容。

## 变更

- `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`
- `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`

无 API、数据库、迁移、配置 schema 或生产数据变更。

## 验证

- `pnpm vitest run src/features/monitor-v4 src/features/monitor-v2`: 12 个测试文件、55 个测试通过。
- `pnpm typecheck`: 通过。
- `pnpm build`: 通过，1070 个模块构建完成。
- `git diff --check`: 通过。

## 发布状态

候选分支：`codex/t62-monitor-dark-breathe`。

本候选尚未合并到 `main`、尚未推送生产；生产部署需后续明确授权。预期 `downtime_required=false`。
