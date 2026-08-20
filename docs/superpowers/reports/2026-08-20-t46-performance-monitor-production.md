# T46 性能监测自定义页面生产验收

## 发布身份

- 发布源：`main@9ed4e020675a0a9afd2d194d3d7c52a619df4743`
- 源码树 / 测试树：`56d8a5cb95182a6acbfb754a983e14fef3c5333c`
- 迁移集合：`18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`
- 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-20-main-9ed4e02067-t46-performance-monitor.json`（0600）
- 宿主记录：`/var/lib/sub2api/release-records/20260820T173243Z-production-1470220.json`

## 发布结果

- 控制器：`downtime_required=false`、`result=succeeded`
- 宿主：`state=promoted`、`rolled_back=false`
- 活动槽：`green`
- API、worker、model-detector 使用同一不可变 T46 镜像并保持 healthy；上一 `blue` 槽保留回滚。
- 公网 `/healthz`、`/readyz`、`/health` 均返回 HTTP 200。

## 登录态专项验收

- 导航显示“性能监测”，链接为 `/custom/performance-monitor`。
- 用户个人导航中 `/monitor` 固定链接数量为 0；未增加旧链重定向。
- 页面标题为“性能监测 - 星桥AI Link”，主区渲染原生 Monitor V2。
- 线上 7 天视图显示 GPT-Pro、GPT-Plus、GPT-特惠分组及每组 28 个时间桶，继续使用原生探测数据和既有乐观展示语义。
- 桌面客户区 `clientWidth=1432`、`scrollWidth=1432`；窄屏客户区 `clientWidth=383`、`scrollWidth=383`，均无整页横向溢出。

## 本地门禁

- `pnpm vitest run src/router/__tests__/title.spec.ts src/router/__tests__/performance-monitor-route.spec.ts src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts src/features/monitor-v2/__tests__`：11 files / 49 tests passed。
- `pnpm typecheck`：passed。
- `pnpm build`：passed。
- `git diff --check`：passed。

## 回滚

恢复上一已验证 `blue` 槽或回退 T46 应用提交；无数据库回滚、历史数据修复或配置迁移。

## 清理与恢复

- 已移除候选分支 `codex/t46-performance-monitor`、候选 worktree 和四个 T46 临时发布 worktree。
- 恢复 bundle：`/Users/gongtengxinwen/Documents/sub2api-archives/t46-performance-monitor-9ed4e020/t46-refs.bundle`
- SHA-256：`bd34822cb7fd3c96ee355e5c0ed613c8ddf6010ded75f6f9e683d77e059bbbb4`
- `git bundle verify`：passed。
- 用户指定的 `/private/tmp/sub2api-monitor-v3-preview` 继续只读保护，未清理其未提交内容。
