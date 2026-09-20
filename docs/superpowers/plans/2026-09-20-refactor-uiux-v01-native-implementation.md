# refactorUIUX v0.1 sub 原生实施计划

日期：2026-09-20
分支：`codex/refactor-uiux-v01`
基线：`2c163fce`

## 目标

将冻结原型的用户端信息架构和星桥品牌视觉移植到 sub2api 原生 Vue 前端，同时保留现有认证、API 密钥、用量、支付、订单与安全设置业务逻辑。普通用户主入口保持 `/dashboard`，页面语义改为“AI 工具”；其他稳定 URL 不变。

## 事实源优先级

1. `docs/prototype/2026-09-12-user-end-prototype-structure.md`
2. `DESIGN.md`
3. 当前 `prototype/pages.mjs`、`dialogs.mjs`
4. `prototype/styles.css` 的最终层叠结果
5. QA 截图只用于布局参照

## 实施批次

### 批次 1：用户壳层与令牌

- 新建用户端专用主题样式，提供 DESIGN.md 颜色、边框、间距和响应式令牌。
- 调整 `AppLayout`：242/200/76px 侧栏合同、40/24px 内容边距、去除用户端装饰渐变。
- 调整 `AppSidebar`：品牌固定为“星桥 AI Link”，普通用户主导航只保留 AI 工具、使用记录、我的密钥；充值、订单、资料仍由底部入口/账户菜单进入。
- 保持管理员壳层不变。

验证：`AppSidebar.spec.ts`、`UserShellComposition.spec.ts`、typecheck。

### 批次 2：AI 工具页

- 用真实 `/groups/available`、`/groups/rates`、`/monitor-v4?window=1h` 与 `/keys` 数据替换旧 Dashboard 统计页。
- 工具卡展示 Codex、Claude Code、Grok、DeepSeek 的可用线路、最佳线路和已关联密钥数。
- “我的 AI 线路”展示线路身份、近一小时成功率、最近观测和关联密钥；刷新使用真实接口。
- 创建/关联密钥跳转到原生 `/keys`，不引入演示创建逻辑。
- 各 API 独立失败，保留局部错误与重试。

验证：新增 Dashboard 页面测试、typecheck、定向 Vitest。

### 批次 3：现有五页视觉结构

- 使用记录：保留现有请求、筛选、统计、图表、错误请求与 CSV；增加冻结原型页面头和视觉容器。
- 我的密钥：保留 CRUD、线路切换、CCSwitch、分页与列设置；改造工具栏、端点、表格和弹窗视觉。
- 充值/兑换：保留 `/purchase` 与 `/redeem` 两条原生业务路由以及 `paymentFlow`；统一页面头、分段导航、左右结算结构。
- 订单：保留完整原生状态、取消、退款、分页；增加原型页面结构和返回充值入口。
- 个人资料：复用原生资料、密码、余额通知、TOTP、Passkey；统一设置页层级与表面。

验证：各页面已有定向测试 + 新增必要的结构测试、typecheck。

### 批次 4：视觉与交互回归

- 在 1730×1000、1280×900、900×1000、390×844、1395×675 验证布局。
- 验证侧栏响应式、表格横向滚动、弹层焦点、加载/空/错状态。
- 运行 typecheck、定向 Vitest、生产 build。

### 批次 5：集成与测试站

- 代码审查并合并 `main`。
- 确认根目录干净、`main == origin/main` 后推送。
- 仅通过 `ops/release-sub2api-test-station.sh` 部署独立测试站。
- 验证 `/health`、`/readyz`、六服务状态和浏览器实际新版页面。
- 不部署主站，不删除 named volumes。
