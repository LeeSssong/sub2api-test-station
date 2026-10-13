# 智商监测轻量图片判定（候选实现，未部署）

## 范围

用户确认：只给现有画作预览加截图式右上角斜标识，不做画廊。每幅新作品只截一张图、最多发一次视觉判题请求；只判断人物/动作/场景和明显结构错误，不评审美、不认脸、不检查动画和细微肢体。沿用原有生成状态、时间轴、分组状态和统计；图片判定独立保存在结果 JSON。旧记录不补判，页面刷新/放大不触发截图或付费调用。

## 实现

- 智商绘图任务保存作品和 pending 判定后，由原后台任务领取判图。数据库 CAS 将 pending 改为 judging，重复领取失败；最终写入仅更新 judgment，不改 HTML、生成状态、计费聚合或其他元数据。无 SQL schema 迁移。
- 不创建无界 goroutine 或独立队列。沿用原计划租约/并发。崩溃或写入失败后不自动重复付费请求；两分钟期限后只读 DTO 投影为判定异常。
- Chromium 隔离渲染单帧，1024×768 截图缩小为 768×576 PNG；最多 1 MiB 编码前图片，20 秒截图超时，同进程串行截图。临时文件自动清理，不存储截图/base64。去掉脚本、外部资源入口、页面跳转，前置 CSP，清除应用环境凭据，保持浏览器沙箱。
- 原生 Responses、Chat Completions、Anthropic 协议携带一张图；固定短提示词，low 推理（模型支持时），JSON 三态＋短原因。只调用一个已领取并发槽的账号；请求失败/无效回复不切账号重试。Agent Identity 的通常恢复重试在图片判题请求中禁用。
- 配置在原画图规则中：可选独立判题分组/模型，否则沿用糖果题判题分组/模型，但不沿用糖果题的数值比对提示词。默认糖果题的内置校验不提供视觉模型；没有配置则明确显示判定异常。目前支持 OpenAI 与 Anthropic 分组，模型必须支持图片输入。
- 角标：通过、未通过、待判定、判定异常、未判定。失败作品和详情仍可预览；刷新同一记录只更新角标，不重新取图。详情原状态标注“生成成功”，视觉判定原因独立展示。

## 本地验证

- 前端相关组件/规则测试 16 项通过；vue-tsc --noEmit 通过；Vite 生产构建通过。已有 Browserslist/大 chunk 警告，不属于此次改动。
- Go 智商监测、糖果题判题、绘图任务与新增判图相关测试全部通过（最终运行含真实 Chromium 截图）。原生 Responses / Chat Completions / Anthropic 请求验证各携带一张图片。最终结果见 `output/iq-drawing/backend-tests.log`。
- 原生 macOS Google Chrome 真实截图测试通过，校验 PNG 尺寸及中心像素，非 mock。发现并处理 Chrome 在成功写入截图后 teardown watchdog 非零退出的情况：仅在未超时且 PNG 完整、尺寸/大小合格时接受结果；截断 PNG 仍拒绝，有单独回归测试。
- PostgreSQL 集成测试 `TestIntelligenceDrawingClaimAndCompletion` 通过，校验只领取一次、只完成一次、原作品/生成状态/元数据不变。测试使用一次性本地 PostgreSQL/Redis。仓库原有 account_ops_batch/once/openai_excel_oauth 测试构造参数缺失，通过 `output/iq-drawing/integration-overlay.json` 临时补 nil 参数，未修改或纳入这些无关源码。
- 真实 Vue 组件浏览器检查：1280×900、375×812、放大详情；手机页面 scrollWidth=375，无横向溢出，角标白字。截图/fixture 位于 `output/iq-drawing/`，是本地示意数据，不是实际模型评测。
- 只读代码审查发现视觉 verdict 改变原有统计的范围回归，已修正为独立 judgment，并增加生成状态保持的回归测试。

## 运行限制与后续验证

- Dockerfile 已加入 Chromium 与中文字体；基于现有固定 runtime 的本地依赖镜像构建成功。
- 本机 ARM 环境启动 amd64 Chromium 容器时报 `clone: Invalid argument` / `ptrace: Function not implemented`，因此 Linux 容器实际截图尚未验证通过。未以 `--no-sandbox` 绕过。后续部署前必须在目标原生架构、非 root 用户且支持 Chromium 沙箱的环境完成截图冒烟；可通过 INTELLIGENCE_CHROMIUM_PATH 指定浏览器路径。依赖安装成功不等于运行验证成功。
- 未调用真实付费判题模型，真实模型准确率、耗时及 token 成本未实测。协议与流程测试不替代真实上游验收。
- 未合并、推送或部署；主站/测试站均未访问，线上配置未修改。保留独立 worktree 与分支 codex/intelligence-drawing-judge。
