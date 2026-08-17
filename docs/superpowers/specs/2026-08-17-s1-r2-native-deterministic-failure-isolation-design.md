# S1-R2 确定性故障原生隔离编排设计

**日期：** 2026-08-17
**状态：** 已批准，可进入实施计划
**基线：** `main@a00fdb186b9598c0ab0ca747d9dff1a5cea04ae2`
**任务边界：** 候选最多到 `READY_FOR_ROOT_REVIEW`；不合并、不推送、不预检、不部署、不访问生产；不启动 S2/S3。

## 1. 问题证据与当前原生能力

最新主线已经具备以下原生能力，S1-R2 直接复用：

- `Account.IsSchedulable()` 先检查 `status/schedulable`、过期、overload、429、`temp_unschedulable_until` 与 quota；scheduler snapshot/outbox 已传播账号行变化。
- `model_rate_limits` 已按 `(account_id, mapped/canonical model)` 过滤，明确 404/model-not-found 与 Codex plan-gated 模型已能写账号模型冷却。
- API Key 401 已走 `SetError(status=error, schedulable=false)`；OAuth 401 先失效缓存并进入刷新窗口，缺少 refresh token 或刷新最终失败时走原生 error 状态。
- OpenAI 账号模型已有进程内 transient failure streak、10/45 秒 cooldown、half-open、sticky 逃逸与 failover；hard failure 不进入这套 transient 计数。
- Responses SSE 已检测缺少 `response.completed/response.done/response.failed` 的终态，保留已输出不可重放、客户端断连后短时 drain、计费幂等与 proxy-ID circuit。
- 管理员已有 `recover-state`、账号测试、清除 error/临时状态/模型限制的原生入口。

缺口是：余额不足在不同入口可能被永久 error、10 分钟冷却或泛化 403 处理；模型不支持目前固定 30 分钟后自动穿透；HTTP SSE 在终态前断开主要记录 proxy circuit，没有一致进入账号 + canonical model transient cooldown；现有 reason payload 不能稳定解释确定性分类与所有权。

## 2. 目标与非目标

目标：

1. 将明确余额不足统一写原生账号级 `temp_unschedulable`，默认 90 分钟，配置仅接受 60–120 分钟。
2. 将确认凭据失效保持在原生 `status=error/schedulable=false`；OAuth 首次 401 仍先走刷新，只有缺少刷新能力或刷新最终失败才确认。
3. 将明确模型不支持写原生 `model_rate_limits[canonical_model]`，并以 `probe_required` 标记为不自动到期；只有受控成功探测或管理员恢复清理。
4. 将 SSE 在成功终态前断开记录为账号 + canonical model 的 transient failure：未输出且安全可重放时可故障转移；已输出时禁止重放，但仍累计 cooldown。
5. 在原生 reason/payload 中记录有界 episode 元数据，只解释分类、来源、所有权与恢复，不参与 scheduler eligibility。

非目标：不新增表或第二套 scheduler veto；不改变普通 5xx/429、sticky、half-open、proxy circuit、流式 drain、计费幂等；不把泛化 403、网络失败、空/截断/不完整模型清单硬隔离；不实现 S2/S3；不新增管理页面。

## 3. 方案比较

### 方案 A（推荐）：原生状态 payload 最小扩展

分类器只决定故障类别和作用域，执行仍调用原生 `SetTempUnschedulable`、`SetError`、`SetModelRateLimit` 与账号模型 transient API。episode 元数据嵌入已有 reason/model-rate-limit payload，管理员读模型可从原生账号状态投影。

优点：没有新调度事实源、无迁移、变更面最小；旧代码忽略新增 JSON 字段仍安全。缺点：本包只保留当前活动 episode 与既有审计操作，不提供独立历史查询表。

### 方案 B：新增 isolation episode 表，但 scheduler 仍读原生状态

审计更完整，但需要迁移 226、事务编排、CAS 和管理查询，显著扩大本包；T15 未合并迁移 225 还会增加未来刷新冲突面。

### 方案 C：独立 isolation 表直接参与调度

可集中表达状态，但形成第二套 veto，与强制原生优先合同冲突，不采用。

选择 A。若未来确有历史审计查询需求，应单独立项，不在本包提前建设。

## 4. 分类与原生执行合同

| 分类 | 确定证据 | 作用域 | 原生载体 | 恢复 |
|---|---|---|---|---|
| `balance_exhausted` | 稳定机器码或允许列表中的明确余额/额度耗尽消息；有效余额数值明确不足 | account | `temp_unschedulable_until/reason` | 90 分钟后普通原生到期；后续新鲜失败可再次延长；管理员可恢复 |
| `credential_invalid` | API Key 明确 401；OAuth 无 refresh token；OAuth 刷新最终失败或刷新后同凭据再次 401 | account | `status=error/schedulable=false` | 成功受控探测/刷新或管理员恢复 |
| `model_unsupported` | 明确 `model_not_found`、`unsupported_model` 或 plan/model access 机器码，且 canonical model 已知 | account_model | `model_rate_limits[canonical_model]` + `probe_required` | 成功模型探测、完整模型目录重新出现或管理员恢复 |
| `stream_incomplete` | 上游 SSE 已开始但在成功终态前 EOF/读取错误，排除客户端取消/deadline | account_model transient | 现有 `RecordOpenAIAccountModelFailure` | 10/45 秒 cooldown 与 half-open；未输出才允许当前请求 failover |

明确排除：泛化 403、5xx/网络失败的确定性硬隔离、客户端取消、请求错误、内容策略、上下文过长、空模型目录、未完成分页、截断或无法证明完整性的目录。

## 5. 数据与接口契约

新增的有界 JSON 元数据只写已有原生字段：

```json
{
  "source": "deterministic_failure_isolation",
  "failure_class": "balance_exhausted",
  "scope": "account",
  "canonical_model": "",
  "evidence_code": "insufficient_balance",
  "episode_id": "<random opaque id>",
  "owner_revision": "<account updated_at snapshot or opaque revision>",
  "recovery_policy": "expires|probe_required",
  "error_message": "<bounded sanitized message>"
}
```

`model_rate_limits` 继续保留 `rate_limited_at`、`rate_limit_reset_at` 和 `reason`。`probe_required=true` 时 `isRateLimitActiveForKey` 不因 `rate_limit_reset_at` 经过而自动放行；普通 rate-limit payload 没有该字段，维持原到期语义。确定性模型限制使用有限的下一探测时间记录，不使用不可解析的无限日期。

分类器输出固定枚举：`classified/failure_class/scope/canonical_model/evidence_code/recovery_policy`。全文只用于允许列表匹配；保存前脱敏并限长，不保存凭据、API key、请求体或完整上游响应。

## 6. 控制流与失败语义

1. 推理错误先走模型不支持/凭据/余额确定性分类；命中后写对应原生状态，并排除当前账号。
2. 未命中确定性分类时继续原有 403/429/5xx、自定义规则和 transient 路径。
3. SSE 缺成功终态时，同时保留 proxy-ID circuit，并记录账号模型 transient。若没有向客户端输出、请求安全可重放且无副作用，沿用现有 failover；已输出则只冷却，不重放。
4. 原生状态写失败时仍排除当前请求，但不扩大为账号级封禁；记录受限运维日志，后续调度保持原行为。
5. episode 元数据绝不被 scheduler 单独读取；资格判断只来自 `IsSchedulable/IsSchedulableForModel` 与既有 transient 状态。

## 7. 兼容与恢复

- 无数据库迁移、无配置文件新增；新增设置读取使用现有 setting service，默认 90，越界值按拒绝/默认安全处理并有测试。
- 普通 `model_rate_limits` 继续按时间到期；仅带本包 `probe_required=true` 的 payload 保持阻断。
- 管理员 `recover-state` 继续是宽恢复入口；成功原生账号测试/模型探测只清理由本包匹配作用域拥有的状态，不清除无关模型、429、overload 或人工暂停。
- 关闭或回滚二进制不会删除已写原生状态；管理员可经现有恢复入口清理。没有破坏性 down migration。

## 8. 场景验收矩阵

- 明确余额不足：账号级 90 分钟临时不可调度；配置 60/120 有效，59/121 拒绝或回退默认。
- 泛化 403、余额网络错误：不触发确定性隔离。
- API Key 401：原生 error/不可调度；OAuth 首次 401 有 refresh token 时不直接永久禁用。
- 明确模型不支持：只阻断 canonical model；同账号其他模型可调度；时间经过不自动穿透。
- 空/截断/分页未完成模型目录：不写模型限制。
- SSE 未收到成功终态且未输出：记录账号模型 transient 并允许既有安全 failover。
- SSE 已输出后断开：记录 transient/cooldown，但不得重放。
- 正常 `response.completed`：清理相应 transient 成功状态，不记录 incomplete failure。
- scheduler snapshot/outbox、sticky、计费幂等、proxy circuit 保持现有合同。

## 9. 测试、发布与回滚

仅运行直接相关测试：分类器表驱动测试、rate-limit 原生投影测试、model `probe_required` 调度测试、SSE missing-terminal failover/cooldown 测试、必要相关包 compile/typecheck/build、迁移集合不变检查和 `git diff --check`。不运行全仓、压力、mutation、soak、重复浏览器矩阵或形式化额外 reviewer。

本任务不执行发布预检，因此 `downtime_required=unverified`。未来根总控刷新/整合后才运行既有本地/宿主链；不得使用 GitHub Actions。

回滚为切回上一二进制并由管理员按原生状态恢复；不批量删除未知状态，不修改生产数据。

## 10. 自审与批准记录

- 占位符扫描：无 TBD/TODO/implement-later。
- 一致性：余额 90 分钟、60–120 配置范围；凭据原生 error；模型账号+canonical model；SSE incomplete 使用 transient，均与委派合同一致。
- 范围：不新增迁移，不占用 225/226；不修改 S2/S3；不访问生产。
- 歧义处理：`stream_incomplete` 明确属于 transient 冷却/故障转移，不冒充确定性永久硬隔离；`probe_required` 仅适用于明确模型不支持。

**批准记录：** 根总控在本顶层任务委派中提供了完整且无待决产品问题的批准合同，并明确允许在现有合同足以安全决策时完成书面代审。2026-08-17 本规格按该授权批准；若实现发现需新增迁移、历史回填、凭据安全变化、不可逆数据操作或生产访问，批准立即失效并停止请求根总控决策。
