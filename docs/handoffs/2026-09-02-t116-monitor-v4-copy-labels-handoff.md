# T116 Monitor V4 成功率文案收敛交接

## 状态

`READY_FOR_ROOT_REVIEW`

## 基线与候选

- 基线：`main@6fe774df5a345f6bdc024e7ac5540de8ffa84fab`
- 候选分支：`codex/t116-monitor-v4-copy-labels`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t116-monitor-v4-copy-labels`
- 候选提交：`36521b64c8a1e1c4a0ca3887df70a31df4c50180`

## 变更

- Monitor V4 卡片圆环标签从“体验成功率”改为“成功率”。
- footer 只显示“基于 N 次调用”，N 读取现有 `request_count`。
- 移除用户卡片中的“综合成功”“真实请求成功”“探测补足”“空桶”文案及旧 DOM 选择器。
- 保留 API、DTO、统计公式、真实请求/探测内部字段、颜色阈值、P95、缓存命中率和布局。

变更文件：

- `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`
- `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`
- `docs/superpowers/specs/2026-09-02-t116-monitor-v4-copy-labels-design.md`
- `docs/superpowers/plans/2026-09-02-t116-monitor-v4-copy-labels.md`

## 验证

- TDD RED：旧组件在新文案断言下失败，缺少 `data-test="sample-count"`。
- TDD GREEN：`HybridPerformanceGroupCard.spec.ts` 9/9 通过。
- Monitor V4 直接回归：3 个测试文件、18/18 通过。
- `pnpm typecheck`：通过，`vue-tsc --noEmit` 返回 0。
- `git diff --check`：通过。
- 用户可见文案扫描（排除测试断言）：未发现四个禁用短语。

## 发布与风险

- 无迁移、配置、依赖或生产数据写入。
- 预期 `downtime_required=false`，最终以根 `main` 发布预检为准。
- 未推送、未合并、未部署，根目录 `main` 未被 T116 功能代码修改。
- 回滚：恢复上一已验证槽位/镜像，或在根 `main` 上形成明确 revert 后按发布链执行。

## 根总控动作

候选等待根总控审阅和 `AUTHORIZE_MERGE_TO_MAIN`。合并前需重新确认根 `main` 干净、目标 SHA 未漂移，并在根 `main` 上运行项目要求的直接回归和发布预检。
