# T61 实施计划

1. 先写页面与卡片 RED 回归，锁定推荐组件、卡片布局、倍率、闭环与动效约束。
2. 将 `HybridPerformanceView` 恢复到 Monitor V2 页面骨架，并在顶部网格挂载第四套卡片。
3. 调整 `HybridPerformanceGroupCard` 为原型 A 结构：顶部倍率、状态点、大圆环、P95 双列和居中底部信息。
4. 补齐中英文最近探测文案，运行 Monitor V2/V4 测试、类型检查、构建和 diff 检查。
5. 候选合入根 `main` 后推送，执行无停机蓝绿发布和登录态桌面/窄屏线上验收。
