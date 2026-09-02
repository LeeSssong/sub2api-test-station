# T120 账号监控加载性能与错误恢复交接

- 状态：`READY_FOR_ROOT_REVIEW`，停在发布前。
- 基线：`main@bced1a52b032e2a6e4b4f87c2cfceb057e2123fe`。
- 候选：`codex/t120-account-monitor-performance`。
- 后端：模型检测投影由逐账号串行改为固定 8 worker；移除每账号一次重复设置读取；单账号检测失败降级，不阻断整页。账号详情仅在显式请求时计算真实原生调度评分。
- 前端：移除卡片内部解释文案；新增可见“编辑账号”按钮并复用原生编辑弹窗；触发原因中文化；柱状 hover 即时只显示 TTFT P95；颜色阈值为 ≤10000ms 绿色、>10000ms 黄色、失败红色、无数据灰色。
- 错误恢复：保留 timeout/canceled/network 传输分类，账号监控页分别显示中文提示并保留重试入口。
- 验证：前端 5 个测试文件 80/80；`pnpm typecheck`；`pnpm build`；Go service/admin 定向测试；`go build ./cmd/server`；`git diff --check`。
- 迁移/配置/数据：无迁移、无配置 schema 变化、无生产数据写入。
- 未验证：未在生产测量发布后的接口 P95；未执行部署或线上验收。
- 回滚：回滚候选提交即可，无数据恢复动作。
