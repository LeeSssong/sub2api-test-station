# T138 自购账号首选层与 API Key 冷启动调度实施计划

> 本计划只覆盖 T138 已批准规格，不实施相邻 24 小时质量窗口任务。

## 目标

在现有统一普通文本 OpenAI HTTP 调度路径中，保留全部原生硬门禁，并实现：

1. OAuth/SetupToken 自购账号形成首选资源层；只要该层有可用账号，就不进入 API Key 层。
2. 自购层内不伪造质量分，按现有账号 priority、实时负载和稳定账号 ID 选择。
3. API Key 账号在质量证据不足时使用有界 priority 冷启动信号；`priority=50` 中性，较小值提升、较大值降低。
4. API Key 质量成熟后，排序恢复为真实质量主导，priority 不再永久覆盖质量。
5. 账号 priority 读取复用现有调度快照/分组 priority 逻辑，不新增字段、迁移或配置。

## 变更文件与设计

### 1. `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`

- 扩展统一质量候选的内部字段：资源层、质量冷启动状态、质量置信度、priority 冷启动信号和用于层内比较的负载信息。
- 增加基于现有 `Account.IsOpenAIOAuthLike()` 的资源层判定；未知类型不判为自购。
- 增加有界 priority 归一化函数：默认 50 返回 0，结果固定范围内，使用现有 priority 读取函数并支持分组 priority。
- 增加质量成熟阈值常量，使用既有 `OpenAIQualityBreakdown.Confidence`；成熟后冷启动信号为 0。
- 将统一质量候选排序改为：自购层优先；自购层内 priority、实时负载、等待数、账号 ID；API Key 层内冷启动时使用 priority 修正质量排序，成熟时保持原有质量排序。
- 在统一选择器完成所有现有硬门禁后分层；若存在自购候选，仅对自购候选排序和获取槽位，否则保留 API Key 候选。
- 不改变利润分区、槽位获取、fresh recheck、sticky、安全重放和非普通文本路径。

### 2. `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`

先新增失败测试，再实现：

- OAuth/SetupToken 被识别为自购，API Key 不被识别为自购。
- 健康自购候选存在时，API Key 不进入最终候选顺序。
- 自购候选按 priority、负载、等待数和账号 ID 排序。
- 自购全部被硬门禁排除时，API Key 正常回退。
- API Key 冷启动 priority 信号：50 中性、1 提升、100 降低，且信号有界。
- 冷启动信号随 quality confidence 增加而衰减；成熟质量不受 priority 永久覆盖。
- API Key priority=1 的无样本账号在统一选择器中不会因中性空质量被直接置底。
- 普通质量排序在全部 priority=50 时保持既有结果。
- 生图路径继续绕过统一质量选择器。

### 3. 可能的直接辅助测试

若编译或行为验证显示 projection/候选池存在独立排序入口，则仅在
`openai_account_scheduler_projection.go` 增加同一资源层/冷启动排序的最小适配测试；不引入第二套规则。若当前统一选择器已完成候选排序且 projection 不影响目标路径，则不修改该文件。

## TDD 顺序

1. 运行现有统一质量测试，记录基线。
2. 添加上述 RED 测试，运行聚焦测试，确认失败原因对应缺失行为而非环境问题。
3. 实现最小排序和分层逻辑。
4. 运行 GREEN 聚焦测试及受影响的统一质量/调度测试。
5. 运行 `gofmt`、`go test` 相关包、`go build ./cmd/server` 和 `git diff --check`。
6. 检查差异，确认无 migration、配置、前端、凭据或生产数据改动。

## 验收与边界

- 自购层仍受余额、模型能力、利润门、冷却、运行时阻断、并发槽和请求级排除限制。
- priority 不抢占已获得槽位或运行中请求。
- quality snapshot 缺失时只产生中性质量和有界冷启动信号，不把缺失证据当作成功质量。
- 非统一普通文本入口保持原有调度行为。
- 日志和决策结构不新增敏感数据；若现有决策字段不足，本任务只保留内部行为测试，不新增平行日志系统。

## 提交步骤

1. 计划提交到 `codex/t138-self-owned-tier-cold-start`。
2. 功能实现和直接相关测试通过后，补充 T138 handoff/verification 文档。
3. 提交实现与文档，候选状态保持 `READY_FOR_ROOT_REVIEW`。
4. 不合并根 `main`、不推送根 `main`、不部署验收站或主站。

