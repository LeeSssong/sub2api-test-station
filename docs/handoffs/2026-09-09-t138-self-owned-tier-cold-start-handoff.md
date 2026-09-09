# T138 自购账号首选层与 API Key 冷启动调度交接

## 状态

`READY_FOR_ROOT_REVIEW`

本候选只完成代码与直接相关本地验证，未合并根 `main`、未推送根 `main`、未部署验收站或主站。

## 基线与提交

- 基线：`main@c1e486c00e462582604adc19e5c6dc3d59225c0a`
- 计划提交：`a23806087`
- 实现提交：`7d0952f7d`
- 候选分支：`codex/t138-self-owned-tier-cold-start`
- 候选 worktree：`.worktrees/t138-self-owned-tier-cold-start`

## 实现内容

- 在统一普通文本 OpenAI HTTP 质量调度中复用 `Account.IsOpenAIOAuthLike()` 将 OAuth/SetupToken 识别为自购层，API Key 进入 API Key 层。
- 所有资源层仍在现有资格、余额/利润、运行时阻断、能力、请求排除和槽位逻辑之后处理。
- 自购层内按原生 priority、负载、等待数和账号 ID 排序，不写入或伪造质量分。
- API Key 仅在 `OpenAIQualityBreakdown.Confidence < 0.75` 时使用 priority 冷启动先验；priority=50 信号为 0，信号范围限制为 `[-15, 15]`，成熟后为 0。
- 调度投影入口复用同一资源层和冷启动信号，避免展示排序与实际选择不一致。
- 未新增迁移、配置、账号归属字段、质量事实源或日志系统。

## 变更文件

- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go`
- `docs/superpowers/plans/2026-09-09-t138-self-owned-tier-api-key-cold-start-scheduling.md`

## 验证

通过：

- `go build ./internal/service`
- `go build ./cmd/server`
- `gofmt`
- `git diff --check`

未通过/环境阻断：

- `go test ./internal/service -run 'TestOpenAIUnifiedQuality(ResourceTier|SelfOwnedTier|ColdStart)' -count=1`
- 原因是当前基线测试包存在与 T138 无关的缺失符号：`resolveCompositeModelOwnership`、`decodeCodexManifestModels`；测试在编译阶段失败，未进入 T138 测试执行。

## 迁移、配置与发布

- migration：无
- 配置：无
- 生产业务数据：未触碰
- 凭据/敏感数据：未触碰
- `downtime_required`：未执行发布预检，不适用
- 主站发布：未执行
- 验收站发布：未执行

## 回滚

候选未进入任何环境。根审合并前直接不合并即可；若后续已合并，回滚为提交级 revert，不涉及数据迁移或数据恢复。

## 剩余风险

- 测试包基线缺失符号需要根总控在合并后的统一验证中另行处理或记录。
- 本实现将自购资源层优先于 API Key，但仍服从既有利润分区和所有硬门禁；高成本/不可用自购账号不会绕过这些门禁。
- 自购层没有使用质量排序是本任务的明确产品策略；自购账号发生真实运行故障后仍依靠现有冷却、隔离和资格链路退出候选。
