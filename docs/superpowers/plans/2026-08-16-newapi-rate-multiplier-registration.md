# T13 NewAPI 上游倍率自动登记实施计划

> **For agentic workers:** 使用 `superpowers:subagent-driven-development` 逐任务执行；每个实现任务先用 `superpowers:test-driven-development`，完成后由独立 reviewer 只读复审。

**Goal:** 在真实成功 usage 落库后，针对合格的 NewAPI API-key 账号，使用精确匹配 NewAPI 日志 `other.group_ratio` 自动登记原生 `accounts.rate_multiplier`，并提供并发安全、每日北京日期刷新和管理员“已登记”状态。

**Baseline:** `45de05dffa560f8d2f92695258d4928e6d18ac34`（已合入根 `main@4f31ec3dd`）；候选分支 `codex/newapi-rate-multiplier-registration`。

**Scope:** 只复用现有 usage 落库后 registrar、NewAPI 精确日志查询、账号 repository/outbox 和倍率 UI；不新增表/列/迁移/配置/依赖/cron/API，不改用户扣费、路由、调度公式或历史数据。

## 门禁

- 正式规格 `f2fc807d4fbca5f5917a00b3fadb890061cf3522` 已获用户批准；本计划由根总控代审后进入实现。
- 候选只维护自身规格、计划、实现、测试、复审和 handoff；不得修改根全局队列/总账、合并、推送、部署或访问生产。
- 每个任务完成后 fresh 独立 task review，修复 finding 后 scoped re-review；全部任务后 fresh whole-branch review，全部批准后才 `READY_FOR_ROOT_REVIEW`。
- 最小验证只覆盖相关 Go service/repository/handler/UI 测试、必要 compile/typecheck、race 或 SQL contract（若现有包需要）、diff/范围/禁区扫描；不跑全仓、压力、历史回填或生产写入。

## Task 1: NewAPI 日志字段解析与资格判断

**Files:** 现有 NewAPI usage/cost evidence service 与其直接测试文件（按仓库实际路径）；不得修改无关上游解析器。

1. 先写 RED 表驱动测试，覆盖 `other` 为 JSON 字符串和对象、有效 `group_ratio`（含 0 与小数）、缺失/null/字符串/布尔/负数/>100/非有限值拒绝，以及只读取顶层 `group_ratio`。
2. 扩展 `newAPIUpstreamUsageRecord` 的最小结构化字段，不持久化完整 `other`、凭据、token 或请求体；明确不使用 `model_ratio`、`completion_ratio`、`user_group_ratio`、`quota` 或反推结果。
3. 复用已有 request_id/upstream_request_id 精确匹配结果；拒绝模糊匹配、退款/冲正记录和无法证明 NewAPI 的行。
4. 固定资格：API-key、NewAPI 身份证据、无成功原生 Sub billing 声明、usage 已成功落库、非空 ID 且精确匹配。原生声明优先并阻止 T13 登记。

## Task 2: CAS 领取、完成、释放与 post-usage 接线

**Files:** 现有 accounts repository、usage registrar/gateway 接线及直接 Go 测试。

1. 先为三个窄 repository 合同写 RED：`ClaimNewAPIRateRefresh`、`CompleteNewAPIRateRefresh`、`ReleaseNewAPIRateRefresh`（具体命名可遵循现有风格，但语义不变）。
2. 领取在同一账号行上 CAS 校验资格、原生声明缺失、`last_refresh_date` 与北京日期、未过期 claim；租约固定 5 分钟，未获权请求不得发专用查询。
3. 完成事务锁定账号行并再次校验 token/资格，原子写入数据库精度的倍率、`accounts.extra.newapi_rate_registration` 成功快照、清除 claim，并按既有语义追加 scheduler outbox；任何失败整体回滚。
4. 释放只清除相同 token 的未完成 claim，不触碰其他请求或最后成功快照；失败后当天后续成功请求可重试。
5. 在统一 `writeUsageLogBestEffortWithRegistrar` 挂点接线，不在 Gateway 复制逻辑；登记失败不得影响已成功请求、usage 或扣费。
6. 覆盖首次登记、同日跳过、跨日刷新、并发单赢家、日志延迟/查询失败释放、完成失败回滚、原生声明后来出现和身份变化清除标记。

## Task 3: 管理员倍率状态展示

**Files:** 现有 `UpstreamBillingRateCell`、账号 extra DTO 与直接前端测试。

1. 复用已有账号倍率 API/DTO，读取 `newapi_rate_registration.status=registered` 时显示紧凑“已登记”和既有倍率。
2. tooltip/详情只显示登记来源、首次登记时间和最近成功更新时间；不显示 claim token、usage_log_id、原始 `other`、token 或凭据。
3. 保留管理员手动编辑，并展示“下一次每日成功刷新可能覆盖手动值”的既定提示；不新增开关、按钮或批量管理 API。
4. 账号 type/base URL/key/token 身份变化清除登记快照和 claim，普通备注/并发/分组绑定变化不清除；为该边界添加直接 UI/service 测试。

## Task 4: 定向验证、报告与提交

1. 运行相关 service/repository/handler focused tests、必要 `go test ./... -run '^$'` 或包级 compile check、前端直接测试/typecheck（若触及 UI）、必要 race/SQL contract；不执行全仓无关验证。
2. 运行 `gofmt`、`git diff --check`、允许路径 guard、禁区 guard，确认无迁移/配置/依赖/Actions/生产数据文件变化；扫描任务文档邮箱和凭据。
3. 创建 `docs/superpowers/reports/2026-08-16-newapi-rate-multiplier-registration-implementation.md`，记录基线/提交/tree、测试、未验证项、失败语义、`downtime_required=false`、回滚方式和剩余风险。
4. 提交后依次完成任务复审、finding 修复与 scoped re-review、whole-branch review；生成最终 handoff，停在 `READY_FOR_ROOT_REVIEW`。

## 根总控后续

根总控只在候选通过两级复审、基线未漂移且为唯一可部署候选后授权合入最新 `main`。合并后执行最小主线门禁、创建 0600 发布证据、预检、推送、蓝绿部署和管理员倍率“已登记”即时验收；预期无迁移且 `downtime_required=false`。部署/验收失败保留原候选、失败证据和回滚依据，在同一候选修复；回滚为上一已验证提交，不回滚或删除历史倍率数据。

