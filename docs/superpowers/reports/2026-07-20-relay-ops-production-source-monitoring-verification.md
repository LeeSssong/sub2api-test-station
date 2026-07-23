# Relay-Ops 生产来源与会话监控验证（2026-07-20）

## 范围

本轮完成生产来源登记、公开价格采集、管理员录入页、服务器端用量会话支持和只读部署。没有修改 Sub2API 路由、价格、余额、Key 或 PostgreSQL。

## 生产来源

| 来源 | 公开分组 | Base URL | 公开价格/证据页 | 用量页 | 性能页 |
|---|---|---|---|---|---|
| Neko | `GPT-Pro` | `https://api.999555999.com` | `https://www.999555999.com/pricing` | `/usage` | `/monitor` |
| Wawazz | `GPT-Plus` | `https://wawazz.xyz` | `/home`（无公开模型价格页） | `/usage` | `/monitor` |

Neko 和 Wawazz 的公开页当前都没有可机器验证的采购倍率字段；Wawazz 的主页也不能证明逐模型价格或 `0.05x` 倍率。`0.10x/0.05x` 仍来自 Sub2API 配置与既有人工计费证据，真实变化需登录会话或账单辅助证据。

## 实现

- 生产来源只保存 HTTPS 地址、公开分组关联和可选原生 monitor ID，不保存第二份生产 API Key。
- 公开页面内容 hash 未变化时不重复解析或通知；生产来源不触发候选付费探测。
- 候选采集周期为 6 小时；付费同步/SSE 仍只在显式 `probe` 模式运行。
- 管理员 `/ops` 支持配置 Cookie/Bearer 用量读取会话。秘密文件只能挂载在 `/run/secrets/upstream-sessions/`，数据库只保存引用、指纹和末四位。
- 用量读取每小时运行；401 或登录页在重试一次后标记“上游用量读取会话失效”，通过状态机去重并附登录链接；恢复后发送恢复事件。账单证据只辅助显示，不自动改变价格或路由。

## 验证

- 服务器容器：`go test ./...`、`go vet ./...` 通过；账单会话、HTTP、调度器、应用和存储 focused tests 通过。
- 镜像：`sub2api-relay-ops:ops-pricing-parser-20260720-v7`，AMD64 image ID `sha256:d6a42dcd9076acaeb1facf8a352bf349da3f695d4c8196179f5e7ff6de9ea6ae`。
- 生产 `/healthz`、`/readyz`、`/pricing` 返回 200；新 `relay-ops` healthy。未认证调用合成验收接口返回 `401`。
- 仅 `relay-ops` 容器重建；Sub2API、PostgreSQL、Redis、Caddy 容器 ID 保持不变；候选 `probe_runs` 仍为 0。
- `/run/secrets/upstream-sessions` 当前为空；Feishu Webhook、Agent API Key 和真实上游会话未安装，因此未伪造真实倍率/账单告警结果。

## 零成本合成告警验收实现

- 新增 `POST /relay-ops/api/acceptance/synthetic`，仅管理员、严格 Origin 和 JSON `{}` 请求可调用。
- 固定事件写入现有 incident 状态机，再复用现有 Agent/Feishu 链路；同一事件重复调用不会重复分析或通知。
- 单元测试覆盖无 Agent/Feishu、通知失败降级、重复调用去重和消息脱敏；合成服务没有上游采集器、账单读取器或 Sub2API 写接口依赖。
- 首次管理员触发在事件写入后因未配置 Agent 被装配为 typed-nil 接口而 panic；服务健康和调度未受影响。新增装配回归测试并以真正 nil 接口修复，同时让重复未配置验收稳定返回 `fallback/not_configured`。
- v5 重试同一固定事件成功，页面显示“上游：未访问 · Agent：确定性回退 · 飞书：未配置”。relay-ops 审计库显示事件 `confirmed`、样本数 `2`，Agent 分析 `0`、通知投递 `0`；没有重复分析、外部调用或投递尝试。
- v6 把 `agent configured` / `notifier configured` 两个布尔状态纳入证据哈希，不包含 Key、Webhook 或其它秘密。自动化测试证明从未配置切换到已配置时会重新分析和投递一次，再次调用继续去重；因此后续真实凭据验收不需要删除现有 incident。

## 管理员验收控件与响应式验证

- `/ops` 新增“发送测试告警”按钮和 `aria-live` 结果区；客户端固定提交 JSON `{}`，使用同源凭据和现有 Bearer helper，不接受事件文本、不记录 Token，运行期间按钮禁用。
- 首次生产移动复验发现整页宽度 `898 > 468`；根因是 CSS Grid section 保留宽表最小内容宽度，已以 `.ops-main section { min-width: 0 }` 修复。
- 资源版本化后仍复用旧页的根因是管理员动态 HTML `/relay-ops/api/ops-view` 可被缓存；服务端现返回 `Cache-Control: no-store`，引导脚本也用 `cache: 'no-store'`，自身通过版本参数更新。
- 最终 Chrome 验收：移动端 `document.scrollWidth=clientWidth=468`，所有 section 计算 `min-width=0px`，两个表格容器各为 `434px` 可视宽度、`880px` 内部滚动宽度；按钮可见且可键盘聚焦，状态区为 `aria-live=polite`，页面无“内测”字样。清除移动模拟后桌面端无整体横向溢出。
- 最终验证：固定 Go 1.24 容器 `go test ./...`、`go vet ./...`，两份 JavaScript `node --check`、relay-ops Compose/Caddy 契约及 `git diff --check` 均通过。生产未认证合成接口返回 `401`；v7 容器健康且重启数为 0；仅 relay-ops 容器变化，Sub2API、PostgreSQL、Redis、Caddy ID 保持不变。

## HTML 倍率误匹配修复

- 完成性审计发现旧解析器对整个 HTML 文本搜索任意 `数字 + x/倍`。Neko 页面终端装饰 `bash - 80x24` 被误记为 `80x`；Wawazz 页面压缩/编码片段被误记为 `0x`。两者都不是上游采购倍率。
- v7 只采信带“倍率 / multiplier / rate”明确上下文、显式 data 属性或结构化 JSON 字段的倍率，并拒绝 `0x`。证据新增 `pricing-evidence-v2`；解析器版本变化会在内容 hash 未变化时重新读取一次，保持旧快照不删除。
- 生产自然 5 分钟调度已追加 Neko/Wawazz 两条 `unparseable` 最新快照，任务状态 `passed`。这表示公开页面没有可靠倍率字段，而非页面或上游不可用。
- 修复后 `probe_runs=0`、Agent 分析 `0`、通知投递 `0`；未读取账单会话、未访问候选 API、未修改 Sub2API 路由/价格/余额/Key。仅 relay-ops 容器重建，四个基础容器 ID 保持不变。

## 外部门禁

下一步只需在服务器秘密目录安装经管理员确认的上游会话文件、Feishu Webhook 和 Agent API Key，然后通过 `/ops` 登记对应文件引用并做一次低风险读取验收。任何真实凭据不得进入 Git、聊天、普通文档或日志。
