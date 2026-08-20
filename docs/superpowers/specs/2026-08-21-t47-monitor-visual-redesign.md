# T47 性能监测页面竞品视觉重塑规格

## Goal
将 custom/performance-monitor 的展示层重塑为用户提供的竞品参考风格，并为“性能监测”侧边栏菜单增加专属监测图标；保持 Monitor V2 的原生数据、刷新、窗口切换、乐观展示和无障碍契约不变。

## Scope
- 修改 Monitor V2 页面标题区、分组卡片和时间线的视觉层。
- 时间线改为紧凑竖条，保留 24/28/30 桶和 tooltip。
- 保留当前 UP/DOWN/NO DATA 语义与颜色映射。
- 为虚拟 performance-monitor 菜单项提供内置 SVG 图标。
- 补充或更新直接相关 Vitest。

## Non-goals
- 不增加 API、数据库、迁移、配置或新的监控事实源。
- 不改变 Monitor V2 数据字段、刷新间隔、窗口值、乐观展示逻辑。
- 不改变旧 /monitor 路由行为。
- 不修改管理端渠道监控入口。

## Acceptance
- custom/performance-monitor 首屏在桌面与窄屏呈现竞品式紧凑服务线结构。
- 24/28/30 桶、状态、tooltip、键盘 focus 和 aria-label 测试通过。
- 侧边栏包含性能监测专属图标，菜单仍指向 custom/performance-monitor。
- 直接相关测试、typecheck、build、diff-check 通过。
