# 账号监控卡片 Task 2 Fix Round 2

日期：2026-08-14

## 修复范围

- 在等待账号详情前立即读取“更多”按钮的 `DOMRect`，避免异步边界后 `MouseEvent.currentTarget` 变为 `null`，菜单现在使用真实触发点定位。
- 将详情请求本身与入口 UI 副作用分离：请求 Map 只负责同账号网络去重；当前入口代际独立控制 loading、错误提示和打开目标。旧账号请求后续失败不会覆盖新账号成功界面或延长当前 loading。
- 新增旧请求失败/新请求成功的竞态回归，以及菜单位置不走右下角兜底的断言。
- 未修改账号管理页、监控 DTO、后端、迁移、配置、全局总账或任务队列。

## 验证

- 账号监控卡片、监控视图、信息壳：3 files，71 tests passed。
- 原生 `AccountActionMenu` 与 `AccountsView` Spark 回归：2 files，19 tests passed。
- `pnpm typecheck`：PASS。
- `pnpm build`：PASS；仅仓库既有构建警告。
- `git diff --check`：PASS。

## 状态

等待独立 scoped re-review；未合并、未推送、未部署或触碰生产。
