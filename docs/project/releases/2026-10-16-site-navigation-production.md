# 首页导航精简与页面禁用（2026-10-16）

- 目标：主站 64.83.10.67，用户授权合并 main、推送远端并不停机部署；另确认同时禁用渠道定价页和监控页。
- 改动：首页桌面/移动导航移除模型、状态，文档改为新手教程，Hero 同步文案；移除渠道定价入口及路由、/monitor 和性能监控目标路由，保留 /custom/performance-monitor 的 NotFound 别名防止通用自定义页面重新匹配。后台渠道监控、账号监控及 API 未改。
- 合入分支：codex/simplify-site-navigation；应用来源 main commit 0076f1b80dd9b2972c79b2d989adf7e2382eac0d，tree 84ed72a78f729de311e255f6c2fd479cc5d76f58。推送并 fetch 后来源门禁通过，根 main 干净、非 detached，commit/tree 与 origin/main 一致。本记录为发布后文档提交；功能分支保留为已合入历史证据。
- 验证：首页14项、路由/侧栏23项、i18n 3项通过，两项目类型检查通过；四个旧路径及尾斜杠解析为 NotFound。Vue 前端构建19.65秒，首页构建及 Linux amd64 Go embed 构建成功。pnpm 包装入口因共享依赖目录保护退出，随后按原脚本顺序直接执行同样的检查和构建，未修改依赖。
- 制品：应用 manifest sha256:e43241e77fa096e189ec35c1c33a6f41b331c771171c0d2bc3252c4fbcd4caa9，config sha256:3cfd71696967d1e9cd47a6ddb9b4b46141056dd268d70bbae23ab93e3f98c4c7；首页 image sha256:1fb17857e2bddea0ddf12b22ac7b5a60ce9512d9059e5280c9f0e5703e56207a。应用运行依赖复用前版可追溯镜像，应用二进制来自已推送 main 本地构建，无数据库变化。
- 发布：应用蓝绿 green→blue，候选健康后平滑切流，旧 API 159秒自然排空、forced=false；worker、reauth、detector、PostgreSQL、Redis 未替换。首页独立候选启动/校验/平滑切流9.47秒，旧首页镜像与容器保留。构建和上传未全程精确计时。
- 线上：health=ok、readyz=ready；真实新浏览器普通根地址和桌面/移动菜单均为首页/控制台/新手教程；教程 HTTP 200。旧浏览器根页面存在旧缓存，强制刷新或新会话可见新版本；未登录旧路径进入既有登录守卫，登录态路由行为由本地测试验证，未使用生产账号登录验收。Playwright request API 的教程探测超时，curl 复核 HTTP200。
- 回滚：应用 /usr/local/libexec/deploy-sub2api-blue-green-host.sh --rollback --record /var/lib/sub2api/release-records/20261015T174120Z-production-873379.json（需原发布环境）；首页 /opt/sub2api/production/homepage-releases/0076f1b80dd9b2972c79b2d989adf7e2382eac0d/previous-Caddyfile。两部分均未回滚，旧制品保留。
- 证据：本地 ~/.config/sub2api/release-evidence/site-navigation-20261016/；宿主 /var/lib/sub2api/release-staging/site-navigation-20261016/。测试站未查询、未同步。
