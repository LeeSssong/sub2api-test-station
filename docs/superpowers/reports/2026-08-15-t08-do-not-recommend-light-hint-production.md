# T08“暂不建议入组”轻提示生产验收

日期：2026-08-15

## 发布身份

- 根 `main`：`1bebe479257e39c9433782836788238399e76b0e`
- source/tested tree：`6b9eb0a7f79d65f47e82e944f5d467d1f83323b9`
- 本地测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-15-main-1bebe4792-t08-light-hint-v1.json`
- 生产记录：`/var/lib/sub2api/release-records/20260815T085054Z-production-4053846.json`
- 结果：`succeeded/promoted`，`rolled_back=false`，`downtime_required=false`
- 活动槽：green；blue 保留上一版可回退镜像
- 迁移、配置、依赖和 GitHub Actions：无变化

## 生产健康

- green API 与 worker 使用 T08 镜像且为 `healthy`。
- PostgreSQL、Redis、Caddy 保持健康。
- 公网 `/healthz`、`/readyz`、`/health` 分别返回 200，内容为 `alive`、`ready`、`ok`。

## 管理员页面验收

通过已登录管理员会话访问 `/admin/accounts/monitor`：

- 页面标题为“账号监控”，字段和操作文案为中文。
- 真实账号监控接口返回 200；页面加载 82 张账号卡片、7 个分组标签和全局评分规则区域。
- 现有账号信息入口可打开原生账号信息弹窗；弹窗不展示凭据原文。按 `Escape` 关闭后焦点回收到“查看账号信息”按钮。
- 390x844 移动视口：`document.documentElement.scrollWidth === clientWidth`（390），账号名、元信息和四个账号操作按钮无几何重叠且均可见。
- 生产前端资源 `AccountMonitorView-BUFsws1A.js` 已包含 `暂不建议入组`、`hover-click`、`recommendation-reason-trigger`、两行截断和视口宽度约束；`HelpTooltip` 资源包含 `Escape`、焦点/外部点击关闭逻辑。

## 样本边界

本次生产返回的真实推荐对象中只有 2 个 `recommended`，没有 `not_recommended` 自然样本（其余账号无推荐对象）。因此没有篡改生产数据或伪造线上业务样本；`not_recommended` 的标签、原因映射、悬浮/点击/键盘、移动端关闭、未知原因兜底和其他推荐状态回归由同一发布树的 64/64 定向测试覆盖。线上页面只做真实数据、资源身份、响应式布局和现有操作回归验收。

## 回滚

T08 无数据库或配置变更。若需回滚，使用既有蓝绿回滚链切回上一版 blue 镜像并重建前端；不需要数据回滚。
