# T47-R2 Monitor V2 紧凑布局实施计划

## Goal
在不改变 Monitor V2 数据和交互契约的前提下，实施紧凑服务行视觉：放大文字、统一基线、移除 Tooltip 常驻占位、贴近时间标签。

## Steps
- [ ] 为 GroupCard/Timeline 添加布局回归断言并先观察 RED。
- [ ] 调整 GroupCard 的字号、网格、卡片间距和垂直对齐。
- [ ] 调整 Timeline 柱体高度、轨道间距、时间标签和 Tooltip 悬浮层。
- [ ] 运行直接 Vitest、typecheck、build、diff-check，并检查状态契约未变。
- [ ] 生成 handoff，等待根任务合并、推送、蓝绿发布和线上专项视觉验收。

## Done when
直接相关测试通过，前端 typecheck/build/diff-check 通过，且变更仅限 Monitor V2 卡片/时间线、测试和本任务文档。
