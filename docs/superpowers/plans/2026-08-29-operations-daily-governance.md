# 运营日报、错误生命周期、模型准入与调度质量治理实施计划

> 四个任务包各自使用独立顶层 Codex 任务、`codex/<task>` 分支和 worktree。根总控是唯一合并、推送、部署和线上验收执行者。

## Loop Brief

- **Goal：** 消除管理员消耗被误记为收入，把错误从最终 HTTP 状态升级为逻辑请求生命周期，让分组模型列表阻断 Luna 请求，并使 T82 后质量变化可按真实日志复算。
- **Context：** 规格为 `docs/superpowers/specs/2026-08-29-operations-daily-governance-design.md`。事实源只限 `usage_logs`、`ops_error_logs`、原生分组/账号模型配置和 T82 调度日志。
- **Constraints：** 不改扣费、余额、价格和历史流水；不新增平行事实源；不自动降级模型；不制造付费探测；不用 GitHub Actions；主站仅在用户明确授权后发布。
- **Done when：** 各任务完成直接相关实现/测试，根总控再按单车道完成合并、验收站、获授权的主站发布和线上验证。

## 0. 共同执行规则

- [ ] 新任务建立前重新读取全局约束、队列、总账和验收站约束，从当时最新干净 `main` 建立独立 worktree。
- [ ] T85 当前占用一个实现槽位；T84 已合入，保留 worktree 干净且不领先主线。所有根目录未跟踪文档保持原样，不暂存、移动或删除。
- [ ] 单一候选才能处于合并、部署或线上验收；每个任务维护自己的规格、计划、测试和交接，根总控维护队列/总账。

## 1. T86 分组模型请求准入与 Luna 映射清理（先行）

**目的：** 对启用非空 `groups.models_list_config` 的分组，在账号选择前拒绝列表外模型；当前 Luna 返回 `400/model_not_supported/retryable=false`，不产生账号 attempt、failover、上游调用或计费。

**候选文件：** `internal/handler/no_account_error.go`、新的或现有模型准入 helper、Chat/Responses/Messages/OpenAI-compatible/image/embedding/count-token handlers、对应测试；独立的受控原生配置脚本/运行手册。

- [ ] 先写失败合同测试：目录外模型不调用账号选择、上游转发或 usage 计费。
- [ ] 实现统一 `ValidateRequestedModelForGroup`（最终名称遵循代码约定）：仅在列表启用且非空时执行；空列表/未启用保持原语义；配置读故障不能误报永久不支持。
- [ ] 在每个入口、任何映射与调度之前调用 helper。未开始的流返回 HTTP 400；已开始的流只发送一次协议终止事件。
- [ ] 错误对象固定为 `invalid_request_error`、`model_not_supported`、`retryable=false`、`resume_supported=false`，文案不泄漏账号、分组或上游。
- [ ] 保留“模型被允许但暂时无可用账号”的 `503 local_capacity_exhausted`，不改上游 502/503。
- [ ] 准备受控映射清理：先只读核对，再仅移除账号 278、279、280、281、282、289、290、291 的 `model_mapping.gpt-5.6-luna`，事务后核对其他映射哈希不变。实现/测试阶段不得执行生产变更。
- [ ] 验证 Chat、Responses、Messages、兼容入口、图片/embedding/count-token 的流/非流合同；验证 Sol/Terra、空目录、禁用目录、临时耗尽和诊断故障回退；运行受影响 Go 测试、`go build ./cmd/server`、`gofmt`、`git diff --check`。

## 2. T87 逻辑请求错误生命周期投影

**目的：** 优先按 `logical_request_id`、缺失时 `request_id` 聚合全部 attempt，区分自动恢复、单次用户可见失败、重试耗尽、不可安全重放停止。

**候选文件：** `internal/service/ops_errors.go`、ops-error repository/测试、`ops_error_logger.go`、ops 管理 API DTO/handler/页面。

- [ ] 写 fixture：中间 upstream 错误而最终 200 只产出一条 `auto_retry_recovered`。
- [ ] 建立固定优先级：`stopped_unsafe_to_replay` > `auto_retry_recovered` > `retry_exhausted_user_visible` > `single_attempt_user_visible`。
- [ ] 投影 `attempt_count`、`failover_count`、`upstream_error_count`、`final_status`、`terminal_reason`、`unsafe_to_replay`、`switch_allowed`、`switch_reason`、`user_visible`。
- [ ] 历史缺失逻辑 ID 的记录保守视为单 attempt，不能猜测成功或永久错误；用户接口不暴露诊断。
- [ ] 用 2026-08-27 的自动恢复、带/不带 failover 最终失败、`unsafe_to_replay=true` 场景固定回归，确认分母不重复扩大。

## 3. T88 运营日报账务口径与逐日对账

**目的：** 原生业务总览分开普通用户收入、管理员内部消耗、普通用户/管理员/全站有效成本和对外经营贡献。

**候选文件：** `internal/service/business_overview.go`、测试、管理员 business-overview handler/DTO/路由、经营总览前端组件/API 类型/i18n/测试。

- [ ] 从普通用户与管理员共存的 SQL fixture 开始，继续排除 `usage_completeness='unknown'`。
- [ ] 使用既有有效成本公式，新增 `external_revenue`、`admin_internal_consumption`、`ordinary_effective_cost`、`admin_effective_cost`、`all_effective_cost`、`operating_contribution`。
- [ ] 使用原生用户角色关联，不按邮箱、名称、API key 前缀推断管理员。
- [ ] 实现不变量：普通+管理员 `actual_cost=全站 actual_cost`，普通+管理员有效成本=全站有效成本；失败则 `reconciliation_status=failed`，前端不得显示已确认利润。
- [ ] 以 2026-08-27 固定基线验证：外部收入 $111.824727、管理员消耗 $115.067963、全站成本 $140.388174、经营贡献 -$28.563447；覆盖空窗、仅管理员、仅普通用户、成本 fallback、unknown attempt。

## 4. T89 T82 调度性能与路由 503 原因投影

**目的：** 拆分本站 routing 503 与上游 502/503，记录 admission、slow-session、安全重放和分段耗时；只在相同资格集合下比较 T82 前后。

**候选文件：** `openai_shared_health.go`、`openai_account_scheduler*.go`、调度投影类型/测试、`ops_error_logger.go`、ops-error repository/service/handler 和监控页面。

- [ ] 建立封闭 routing 原因码：`no_candidate`、`transient_account_block`、`admission_lease_rejected`、`slow_session_guard`、`model_policy_rejected`、`no_available_channel`、`diagnosis_failure`。
- [ ] 记录 `admission_wait_ms`、`account_selection_ms`、`upstream_connect_ms`，复用已有 auth/routing/TTFT/响应耗时；未知值保持空值而不是伪造 0。
- [ ] 投影 `admission_result`、`admission_reject_reason`、`slow_session_guard_hit`、`safe_replay_decision`、`switch_allowed`、`switch_reason`。
- [ ] T86 合入后模型策略拒绝仅作为 400，不得记为 routing 503。
- [ ] 固定北京时间窗口、group/model/stream/request type/final status/有效文本资格，逻辑请求作为错误密度分母；验证 T83 空桶探测门禁不回归。

## 5. 顺序、发布和回滚

1. T86 优先，先停止 Luna 重试和上游消耗。
2. T87、T88 相互独立，但受 T85 与最多两个功能 worktree 限制；均从届时最新 `main` 开始。
3. T89 等待 T86/T87 的错误契约稳定，避免为旧 503 语义重复建设观测。
4. T86 的应用发布与生产映射清理分两步：先验证入口拒绝，后执行精确配置清理；任一步失败都保留现场、停止后续发布。
5. 回滚只恢复上一已验证应用版本；不删除、不回填历史 usage/ops 日志。

## 6. 交付证据

每个任务的 handoff 必须记录基线 commit、文件范围、测试结果、未覆盖真实场景、验收站结果、主站授权文本、发布 record、线上只读验证及回滚版本。
