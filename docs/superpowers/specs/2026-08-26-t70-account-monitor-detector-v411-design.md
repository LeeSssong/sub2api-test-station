# T70 账号检测分层监测与记录面板设计规格

状态：DESIGNING，待用户审阅。

## 1. 问题证据与基线

已核对 `chen-006/gpt56_api_detector` 最新 `main@0e323cf4923e1f757223927083bda267f5da4052`，版本为 v4.1.1。该版本将检测分为低/中/高覆盖档，独立计算 Juice 与行为指纹，拆分申报模型和实际请求模型，并明确“匹配度不是路由概率”“检测不通过不等于中转主动混用”。其官方档位计划为低 19 条、中 49 条、高 158 条；中档适合作为常规检测，高档覆盖多种请求格式和长上下文对照。

当前 Sub 已具备原生账号监控、异步模型检测队列、`account_model_detection_runs` 历史表、`/v1/catalog` 与 `/v1/detect` 私网 sidecar 合同、账号卡片和模型检测弹窗。当前缺口是：自动任务没有低/中/高策略和升级原因；检测结果字段无法表达计划请求数、有效样本数和证据状态；卡片只展示最近一条结果，历史接口固定返回 25 条且前端将证据压成 JSON 文本，运维无法判断异常是否连续或高档复核是否完成。

## 2. 目标

1. 以 Sub 原生账号监控为唯一调度和运营事实源，把 detector 作为执行型证据引擎。
2. 采用中档日常监测、低档手动快速复核、高档首次入组/异常升级的分层策略，控制请求量并保留高档复核能力。
3. 记录每次检测的档位、模式、触发原因、计划请求数、有效样本数、双证据状态和 detector 版本。
4. 将“点击查看最近 xxx”改为账号级完整检测记录面板：桌面右侧抽屉表格 + 展开详情，窄屏全屏时间线 + 展开详情。
5. 让管理员能区分通过、明确异常、证据不足、检测器不可用和历史参考，不把匹配度当成路由概率。
6. 对旧历史记录保持兼容；不改变账号调度资格、错误隔离、计费、用量或原生质量评分。

## 3. 非目标

- 不复制或打包上游 PolyForm Noncommercial 检测器的核心实现、可信基线、报告逻辑或完整源码进入当前商业生产镜像。
- 不在 Sub 内建设第二套 detector 持久化、连续线程、控制面或调度器。
- 不把 detector 结果直接转换为账号暂停、调度降权或计费事实；这些仍由现有 Sub 原生规则负责。
- 不保存 API Key、Base URL、Authorization、完整提示词、完整输出、原始响应体或无限模型目录。
- 不回填历史检测记录，不删除旧检测数据，不改变已有 T48 双证据语义。

## 4. 影响用户与边界条件

受影响用户为管理员和运维人员，主要场景是账号入组、周期巡检、异常复核和争议排查。普通终端用户不看到 detector 原始字段。非 API Key 账号、未配置模型、sidecar 未配置或 sidecar 不可用时，页面显示真实的“不支持/检测器未接入/检测器不可用”状态，不伪造通过或异常。

一次 detector 检测可能产生上游微量请求和费用，但这些请求不进入 Sub 用户请求链、不写入正常用量账单、不参与经营统计。高档升级有冷却和账号级活动任务去重，sidecar 失败不会触发无限重试。

## 5. 方案比较与选择

### 方案 A：完整搬运 detector 持续逻辑

把 v4.1.1 的概率抽样、滚动窗口、请求格式格和持久化全部复制到 Sub。覆盖最全，但形成第二套调度/事实源，并触发上游 PolyForm Noncommercial 许可风险。不选。

### 方案 B：原生监控编排 + detector 证据引擎（选定）

Sub 复用现有 runner、队列、历史表和账号投影，只向私网 sidecar 传递档位、模式和触发原因。中档按原生时隙执行，低档手动，高档由明确升级规则排队。证据结构化落到现有检测记录和投影，前端使用一个响应式记录面板。该方案满足监测场景且不建立平行控制面。

### 方案 C：只改展示

只把当前 JSON 改成三块证据，没有档位策略和自动升级。无法解决持续监测请求量与异常复核问题。不选。

## 6. 端到端数据与控制流

1. `AccountMonitorRunner` 触发现有 detector schedule slot；服务检查账号类型、模型登记、sidecar catalog、活动任务和冷却窗口。
2. 服务按策略创建 `medium/monitor`、`low/manual` 或 `high/escalation` 任务，写入现有 `account_model_detection_runs`，slot key 保证幂等。
3. 执行器 claim 任务，向 sidecar 发送脱敏之外仅在内存中使用的 API Key/Base URL、申报模型、实际请求模型和任务策略。
4. sidecar 返回有限的 Juice、行为指纹、模型响应和样本摘要；Sub 客户端继续执行 body、字段、字符串和敏感 key 限制。
5. repository 完成任务并保存结构化摘要；`AccountModelDetectionService` 计算当前结果、历史最终参考和是否需要升级。
6. 对异常/证据不足结果按升级策略最多排队一个高档任务；高档结果只形成运营建议，不改变调度事实。
7. 账号卡片展示一行当前摘要；点击“查看检测记录”调用历史接口，打开响应式记录面板。

## 7. 监测策略与结论语义

### 7.1 默认运行

- 自动检测使用官方 `medium` 档和 `monitor` 模式，沿用现有时隙去重，不在每个账号内启动长期 detector session。
- `low` 仅用于管理员手动快速复核。
- `high` 用于首次入组、连续异常、明确模型冲突或管理员手动复核。
- 每次记录 `planned_requests`、`valid_samples` 和 `trigger_reason`，便于运维评估检测成本和覆盖面。

### 7.2 升级规则

- 没有有效历史结果的账号：完成普通检测后安排一次 `high/escalation` 基线。
- `medium` 连续两次 `abnormal`：安排一次高档复核。
- Juice 明确与申报模型不一致，或行为指纹强烈指向其他型号：立即安排高档复核。
- 单次 `insufficient`：继续下一次普通周期；连续两次证据不足才升级。
- 同一账号高档任务设置冷却窗口；已有 queued/running 任务时不重复排队。

### 7.3 管理员可见结论

- `通过`：Juice 和行为指纹均取得足够证据且无硬冲突。
- `异常`：Juice 明确冲突、输出完整性硬异常，或行为指纹强烈指向与申报模型不同。
- `证据不足`：样本不足、无模型越过强指向线、多个模型同时越线、检测器失败或网络失败。
- `检测器未接入/不可用`：sidecar 合同未配置或健康检查失败，属于服务状态而非账号异常。
- `历史参考`：只表示当前没有新鲜最终证据时可供查看的最近最终结果，不覆盖当前状态、不改变调度资格。

## 8. 接口与字段契约

### 8.1 Sidecar 请求

`POST /v1/detect` 在现有字段基础上增加：

```json
{
  "run_id": "uuid",
  "claimed_model": "gpt-5.6-sol",
  "request_model": "实际请求别名",
  "api_key": "仅内存传输",
  "base_url": "仅内存传输",
  "profile": "low|medium|high",
  "mode": "monitor|escalation|manual",
  "trigger_reason": "scheduled|first_run|consecutive_abnormal|insufficient|model_conflict|manual"
}
```

`api_key` 和 `base_url` 不进入 Sub 日志、数据库、响应或前端。sidecar 可以选择独立合法实现或获得授权的 v4.1.1 制品；Sub 不假定执行器内部实现。

### 8.2 Sidecar 响应

```json
{
  "status": "normal|abnormal|insufficient",
  "profile": "medium",
  "planned_requests": 49,
  "valid_samples": 46,
  "evidence_state": "complete|insufficient|unavailable",
  "juice_status": "pass|mismatch|insufficient|non_gpt",
  "fingerprint_status": "strong_match|unclear|unavailable",
  "fingerprint_candidate": "gpt-5.6-luna",
  "fingerprint_similarity": {"gpt-5.6-luna": 0.98},
  "detector_version": "4.1.1",
  "error_code": "",
  "error_message": ""
}
```

所有新增字符串、数字、数组和嵌套摘要沿用现有 bounded summary 限制；指纹相似度不是路由比例。

### 8.3 Sub 持久化与历史接口

现有 `account_model_detection_runs` 增加必要字段：`profile`、`mode`、`trigger_reason`、`planned_requests`、`valid_samples`、`evidence_state`、`juice_status` 的结构化状态和 `fingerprint_status`。旧行读取为 `profile=unknown`、`mode=historical`、`evidence_state=historical`，不回填。

历史接口继续使用现有管理员路径，增加 `limit`（默认 25，最大 100）、`cursor`、`status`、`profile` 和 `mode` 查询参数，返回 `{items,next_cursor}`。游标按 `(created_at,id)` 倒序稳定分页；无下一页时 `next_cursor` 为空。

### 8.4 前端面板契约

账号卡片只显示当前结论、最近档位、最近完成时间和“查看检测记录”入口。面板顶部显示当前有效结论及历史参考标记；列表行显示时间、档位、模式、触发原因、计划/有效样本、Juice、行为指纹和综合结论；展开区显示申报模型、实际请求模型、detector 版本、错误分类和双证据摘要。原始 JSON 只作为受限技术详情，不作为首屏结论。

桌面端面板为约 640px 右侧抽屉，列表为表格列，单行展开双证据详情；窄屏切换为全屏面板，列表变为时间线行，详情保持双证据结构。筛选为全部、异常、证据不足、高档复核；加载更多使用 `next_cursor`。

## 9. 失败、安全与兼容语义

- sidecar 超时、限流、无效 JSON、上游网络错误统一为 `evidence_state=unavailable` 或 `insufficient`，不得伪造替换结论。
- sidecar 客户端保留 64 KiB body limit、8 KiB summary limit、递归敏感 key 删除和字符串长度限制。
- API Key、Base URL、Authorization、prompt、output、response、token、secret 和 credentials 永不落库或入日志。
- 旧历史结果只显示其实际字段；缺少档位、样本数或 v4.1.1 证据时显示“历史记录”，不猜测为新格式。
- detector 结果不写 `usage_logs`，不改变余额、利润、账号成本、调度资格、熔断状态或错误码转换。
- 未配置合法 sidecar 时保留现有 `service_unconfigured` 语义；连接探测和其他账号监控继续工作。

## 10. 兼容、迁移与配置

需要一份向前兼容迁移，为检测运行表增加字段和必要索引；迁移只改变 schema，不回填历史行。现有 `trigger_kind` 的 `manual/scheduled` 语义保留，新增模式独立存储。sidecar URL/token 配置名不变；`MODEL_DETECTOR_VERSION` 只用于版本展示和能力核对，不在 Sub 内硬编码可信基线。

上游 PolyForm Noncommercial 许可、Required Notice 和公开检测站非商业限制必须保留在 sidecar 制品和部署文档中。当前任务只修改 Sub 合同、调度、存储和 UI；没有授权的情况下不得把上游源码、基线或报告复制进 Sub 镜像。

## 11. 场景化验收矩阵

| 场景 | 预期行为 |
|---|---|
| 普通周期检测 | 创建 medium/monitor，记录 49 条计划和实际有效样本 |
| 首次入组 | 普通结果完成后只排队一个 high/escalation 基线 |
| 连续两次异常 | 自动排队 high，显示触发原因为连续异常 |
| 单次证据不足 | 继续普通周期，不立即放大请求量 |
| 连续两次证据不足 | 排队 high，显示证据不足升级 |
| Juice 与申报模型冲突 | 结论异常，自动高档复核，但不直接暂停账号 |
| 指纹强烈指向其他模型 | 显示申报/实际请求/候选三者，结论异常并高档复核 |
| detector 未配置 | 显示检测器未接入，连接监控仍可用 |
| 历史旧结果 | 面板正常打开，标记历史记录，不伪造档位或样本 |
| 桌面面板 | 右侧抽屉表格，单行展开双证据详情，支持筛选和加载更多 |
| 窄屏面板 | 全屏时间线，无横向溢出，单行展开详情 |
| 敏感信息扫描 | 数据库、日志、API 响应和前端无 API Key/Base URL/Authorization/prompt/output |

## 12. 测试与发布策略

后端定向测试覆盖策略升级、连续异常计数、冷却/幂等、sidecar 请求响应、历史分页、迁移兼容、旧记录回退和敏感字段清理。前端 Vitest 覆盖面板列表、筛选、游标加载、双证据展开、五类结论、响应式窄屏和旧历史结果。必要门禁为相关 Go/Vitest、`go build ./cmd/server`、`pnpm typecheck`、`pnpm build`、`gofmt`、`git diff --check`。

任务候选必须从最新 `main` 独立 worktree 实施；不得修改其他任务候选、根发布源或生产配置。合并后的 `main` 只执行本任务直接相关回归、构建和发布预检。预计 `downtime_required=false`；若预检返回 true，任何停服/迁移/重启前暂停。部署沿用既有本地/宿主蓝绿链，不使用 GitHub Actions。失败时保留候选、失败证据和修复，不清理 worktree。

回滚为恢复上一已验证镜像；若迁移已执行，使用既有原子切换和迁移保护回滚，不删除历史检测记录。

## 13. 用户批准记录

- 用户确认日常策略采用“中档日常、低档快速复核、高档异常升级”。
- 用户确认增加账号级完整检测记录面板，替换卡片上的“点击查看最近 xxx”。
- 用户确认响应式混合形态：桌面右侧抽屉表格 + 详情，窄屏全屏时间线 + 详情。
- 用户确认监测策略、升级规则、结论语义、接口字段、安全边界和验收范围。

