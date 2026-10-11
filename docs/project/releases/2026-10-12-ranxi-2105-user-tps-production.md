# ranxi v2.10.5 与用户 TPS 主站发布（2026-10-12）

- 目标与结果：用户授权主站 `64.83.10.67` 不停机升级。最新正式版 v2.10.5，官方 SHA `d977e7a5e24e5536db6ce8b8478743e0c4b11f77`；不包含该 tag 之后 production 的数据库迁移。主站发布成功、未回滚；测试站未查询、未同步。
- 改动：从官方 v2.10.4 基线导入 97 文件增量，保留本地计费/配额、Fast 调度、监控及用户壳。普通用户使用记录延迟列显示 TPS，采用输出 token / 总耗时；内部耗时详情仍只在管理员开启。同步上游 BPS 凭据恢复、诊断、请求时区等功能；新增上游一致性测试发现本地公开设置长上下文徽标漏映射，已补齐。
- 来源：`codex/ranxi-2105-user-tps` 快进合入根 main，推送并 fetch 后确认干净且非 detached；部署 commit `8081c05faec84bdf37c1324bfbf33d2d5a366666`、tree `09b37615d4304bb2fe879c4cc5995ae8f6a93d95` 与 origin/main 一致。本记录为部署后文档提交，不改变已部署代码。
- 制品：`ghcr.io/leesssong/xingqiao-sub2api:release-8081c05faec84bdf37c1324bfbf33d2d5a366666-7c1451ddd03ee5706b13e91ed42d4b0a2b0c59071a2f5c8569e6d728985c878e`；宿主 image/manifest digest `sha256:7c1451ddd03ee5706b13e91ed42d4b0a2b0c59071a2f5c8569e6d728985c878e`，OCI config digest `sha256:f28b0220c2093052497f01564d1c031a80143851fe3a4aa550c442ccde259134`。二进制 SHA256 `57db32894858dae83819d84e589b880bc317916246673b6bbc887a56dd071301`。从已推送 main 的 Git archive 构建；核实 Dockerfile、Go/前端依赖输入未变后复用现网受核验 runtime，重新构建内嵌前端与 Go 二进制，镜像仅构建一次。
- 数据库：全部迁移 SQL 字节不变；hash `e5d79ffa00d518804211a877ede6c7cb54277ada8c76e975526dca622445a6de` 与现网一致；未使用历史在线迁移例外或停机路径。
- 验证：TPS 回归 RED→GREEN，62 项通过；升级相关前端/用户页/i18n 107 项通过、11 项原有跳过；类型检查通过。Go `-tags unit` 下 BPS/Excel/时区/公开设置/Fast 相关 service/admin/DTO/repository 测试通过，公开设置修复后重跑通过，basispoints 包通过。首次未加 unit 标签导致测试 helper 缺失，已纠正命令；未声称全量测试通过。独立只读审查无重要问题。
- 线上：API、singleton worker、独立 reauth worker 健康；版本 2.10.5、来源标签、实际镜像与状态一致。主站 `/health`、`/readyz`、发布链认证版本与模型列表检查通过。北京时间约 02:22—02:42 的 437 次健康采样全部 HTTP 200；采样不是对所有请求零错误的证明。PostgreSQL、Redis、Caddy、detector 的容器 ID/镜像/启动时间/重启数前后完全相同；Caddy 仅 reload。codex 域名为独立 codex2api 路由，健康通过，额外尝试其未提供的 /readyz 返回 404，不属于本次 Sub2API 就绪检查。
- 页面：线上真实前端资源配合隔离普通用户/API fixtures，1440px 与 390px 显示 `50.0 t/s`，中文提示正确、无管理员耗时按钮；390px fresh-load sidebar=76px、页面无横向溢出。响应式切换后立即截图曾命中过渡帧，重新加载后验证正常。未登录真实用户读取其记录、未触发真实模型/BPS/支付业务。入口资源 `/assets/index-Bsoqhp1y.js` 与本地制品逐字节一致，SHA256 `8a9d095d5794b141ebcd165ae4af91653aa7a5c0b37c3fe160868da6c9fac0d0`。
- 阶段耗时：前端 47s，Go 编译 114s，镜像装配约 14s；宿主约 02:26:15 开始，02:26:44 平滑切流，旧 green 自然排空 90s（forced=false），约 02:28:16 完成。上传/预检未单独计时。singleton worker 先安全排空后更新；独立 reauth 由既有发布链暂停/恢复，镜像未变。
- 回滚入口：`/usr/local/libexec/deploy-sub2api-blue-green-host.sh --rollback --record /var/lib/sub2api/release-records/20261011T182615Z-production-324911.json`，使用发布记录对应宿主环境。旧 dcbf1d5c9 镜像与配置保留；未回滚。回退至旧版会失去 v2.10.5 的 BPS RT 并发保护语义。
- 证据：受保护本地 `~/.config/sub2api/release-evidence/ranxi-2105-20261012/`，含源码快照、构建来源/校验和、测试与发布日志、健康采样、宿主记录和示例截图。已合入功能分支标记为历史证据，不再重复合并。
