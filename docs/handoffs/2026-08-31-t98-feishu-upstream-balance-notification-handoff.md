# T98 飞书上游余额通知重构交接

日期：2026-08-31
状态：`READY_FOR_ROOT_REVIEW`（仅本地候选；未合并、未推送、未部署、未清生产表、未发送真实消息）

## Delivered

- Sub2API worker 原生读取已有余额快照，按规范化 BaseURL 形成唯一当前 USD 余额；仅纳入 active/openai/api_key 账号。
- P2 低余额每 30 分钟、P1 零余额每 5 分钟；状态切换立即发送，healthy 只 resolve 不发恢复。
- 卡片复用红/橙等级主题，P1 只 `@` 接收人；显示 BaseURL、一次余额、登记簿账号/明文密码及全部关联账号的 `scheduler_rank`。
- 原生事件账本支持 BaseURL scope、generation、lease、CAS、失败退避和并发去重，不保存卡片或凭据。
- 受保护 XLSX 转换器和 worker-only secret 目录合同已交付；真实工作簿曾在临时 0700 目录验证为 35 个唯一 BaseURL，临时 JSON 已删除。
- relay-ops 旧通知内容、规则、writer、retry、escalation、scheduler、策略文件和业务包已移除；非通知能力构建和测试通过。
- 九张授权旧表的独立清理命令已交付，默认 count-only、执行双门禁；未执行清理。

## Candidate

- 分支：`codex/t98-feishu-upstream-balance-notification`
- 基线：`main@06695141ff6459c08f316a97e6049ba2a42034bd`
- 已刷新到：`main@a928c671d3133fc33d59cd6f56c351674af0406e`
- 刷新合并提交：`11ee7f9f6 Merge branch 'main' into codex/t98-feishu-upstream-balance-notification`
- 实现提交链：
  - `1b1245117 feat: add baseurl balance notification evaluation`
  - `80667fbe5 feat: add protected upstream balance secret loader`
  - `20f672d30 feat: add upstream balance feishu cards`
  - `51c9ca2fd feat: add baseurl scoped native alert ledger`
  - `57fa4ebb8 feat: wire native upstream balance notification worker`
  - `c28168886 feat: add protected upstream login registry conversion`
  - `f997da054 refactor: retire legacy feishu notification runtime`
- 最终文档提交只增加验证报告、交接和计划勾选；根审以分支最终 `HEAD` 为候选 SHA。

## Verification

- 合入 `main@a928c671d` 后重跑：service 15/15、notify 7/7、repository 8/8、migration 1/1、converter 4/4 通过。
- `go build ./cmd/server` 通过。
- relay-ops 34 个包 0 失败；常驻服务、provision、独立清理命令均构建通过。
- relay-ops 依赖退休合同、Compose 合同、worker secret 合同、gofmt、diff check 和敏感信息扫描通过。
- 精确命令、已知仓库基线失败和跳过项见 `docs/superpowers/reports/2026-08-31-t98-feishu-upstream-balance-notification-verification.md`。

## Migration And Configuration

- Sub2API：新增 migration 231，为 `ops_alert_events` 增加非敏感投递状态并注册系统规则。
- relay-ops：普通 runtime migration 不再创建旧通知表；`015` 仅是受控删除清单，不会自动执行。
- production worker：新增只读 `/run/secrets/upstream-balance` 挂载，`SUB2API_UPSTREAM_BALANCE_NOTIFICATION_ENABLED=false` 默认关闭。
- relay-ops：删除飞书、通知策略、agent 环境变量和挂载；`.env.example` 保留五个旧宿主路径占位供凭据迁移。

## Root Review And Release Order

1. 根线程盘点领先 worktree并确认 T98 可进入唯一 `INTEGRATING` 车道；若 `main` 仍为 `a928c671d` 可直接根审，若已前进则先重新刷新并复跑专项验证。
2. 在合并后的 `main` 重跑本报告的直接相关测试、server/relay-ops 构建和发布预检，记录 `downtime_required`。
3. 固定兼容镜像和回滚目标，部署已移除旧 writer 的 relay-ops/runtime；确认旧 sender、retry 和 scheduler 全部停止。
4. 先运行独立清理命令 count-only 核对九表，只在已批准的受控发布阶段同时提供双门禁执行无备份删除。
5. 部署 migration 231 和 worker secret 目录；验收站使用独立虚构/验收凭据完成真实通知验收后，才考虑生产启用开关。
6. 当前“继续”不属于主站两种明确授权语义；主站只能在用户明确说“测试站验收通过，部署主站”或“快速部署到主站”后执行。

## Rollback

- 清表前：关闭新 sender、回退 T98 代码并走既有发布链；migration 231 的附加列可保留，不影响旧业务。
- 清表后：按批准规格不恢复旧通知历史或旧 sender；回滚只关闭/修复新 sender，并保留非敏感原生事件账本供后续处理。
- 任何通知失败不得回滚余额刷新、调度、计费或网关业务路径。
