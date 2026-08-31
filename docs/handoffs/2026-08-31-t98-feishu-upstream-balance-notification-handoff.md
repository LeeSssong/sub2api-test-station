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

## Recovery Delta (2026-08-31)

- 候选已刷新到当前根 `main@43ffa2353ed96da668d0846753f472fea922d07d`，刷新合并提交为 `34aed9938f4d4cc7c375c56057e8bc143b82db5b`，候选 tree `8f7da9accdee563ea61ad4f229ac9d8c6a0764ea`；候选相对根 `main` ahead 6、behind 0，工作树干净。
- 新增修复提交 `d0a4c9a1b`：`Resolve` 使用严格 `last_observed_at` 单调 CAS；活动 claim 只接受更新观测或同时间同状态同数值续租；stale `RETURNING` 空行提交事务并返回未 claim；无活动行时以最新 resolved 非空观测水位阻止旧观测重开事件。`b3b4be899` 的冲突 BaseURL scope 隔离保持不变。
- 刷新后直接验证：service、notify、repository（`-vet=off`）、migration、converter 目标测试通过；`go build ./cmd/server` 通过；relay-ops `go test ./...`、三个二进制构建、退休/配置/secret 合同测试通过；`git diff --check` 通过。
- 本候选未执行且不得自行执行：根合并授权、推送、部署、停机、迁移 231、旧表清理、secret/开关生产写入、验收/主站真实消息。根总控必须先按发布来源门禁和主站明确授权路径处理；“推送部署”不能授权功能 worktree 直接发布。

## Production Result (2026-08-31)

- 维护发布已成功生效于主站：生产 release record 绑定 source commit `c651bcb7078b085905384a7782c29c2d23404858`、source tree `79144c1c56676a1e975237371a0c826c21f6275e`、migration hash `0bda54bbf75076c03bbd780603ccdca20c5b09e46ca7e2b4d2a1717c90e5dc57`，活动槽 `green`；`/healthz`、`/readyz`、`/health` 均 200。
- 验收站 API/worker/detector 当前运行同一 `c651bcb7` 镜像且 healthy，`/admin/lab/health` 与登录页 200；同 commit/tree 对账以容器镜像标签和主站 release record 为证据。未发送真实飞书消息。
- migration 231 的非敏感 schema/rule 已随该迁移集合生效；旧通知历史清理/退役仍是已执行历史，不得重放。

## Notification Secret Permission Blocker

- 主站 worker 环境开关为启用态，但启动日志记录 `secret_unavailable`，sender 按规格 fail-closed。
- `/opt/sub2api/production/secrets/upstream-balance` 目录及五个文件均存在，目录 `0700 root:root`、文件 `0600 root:root`；worker 实际降权 UID `1000:1000` 对五个文件均不可读，root 可读。内容未被读取或写入聊天/仓库。
- 要激活新通知，需单独明确授权：将该目录所有权改为 `1000:1000` 并保持 `0700`，将五个文件所有权改为 `1000:1000` 并保持 `0600`，只重启 `sub2api-worker`，随后做脱敏日志/健康核对。此动作不改文件内容、不改 API/数据库、不发送测试消息；未获授权前保持当前 fail-closed。
