# T139 质量快照刷新与短窗口调度时效优化交接

## 状态

`READY_FOR_ROOT_REVIEW`

## 基线与候选

- 基线 `main`：`1ee82471f`
- 候选分支：`codex/t139-quality-freshness`
- 候选 worktree：`.worktrees/t139-quality-freshness`
- 候选提交：`8b35ffbe5`

## 本次改动

- 质量统计查询范围从 7 天改为最近 1 小时。
- 将查询结果拆分为互斥窗口：`W5=[now-5m, now)`、`W55=[now-1h, now-5m)`。
- 窗口评分权重改为 W5 40%、W55 60%，质量评分版本更新为 `t139-v1`。
- 质量快照 TTL 与刷新节流均固定为 5 分钟；调度只读已发布内存快照，过期后由后续调度触发一次刷新。
- 完整真实请求的 usage 日志成功写入后触发合并式异步刷新；图片、视频、Web Search、部分或未知 usage 不触发。
- 保留 T138 自购账号首选层、API Key 冷启动 priority 先验、`priority=50` 中性以及原生不可用/不可调度硬门禁语义。
- 页面和排名仍按账号维度，不新增模型级排名，不删除冷却账号展示。

## 文件范围

- `upstream/sub2api/backend/internal/repository/usage_log_quality.go`
- `upstream/sub2api/backend/internal/repository/usage_log_quality_test.go`
- `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- `upstream/sub2api/backend/internal/service/openai_account_quality.go`
- `upstream/sub2api/backend/internal/service/openai_account_quality_test.go`
- `upstream/sub2api/backend/internal/service/openai_gateway_service.go`
- `upstream/sub2api/backend/internal/service/openai_gateway_usage.go`
- `upstream/sub2api/backend/internal/service/openai_quality_score.go`
- `upstream/sub2api/backend/internal/service/openai_quality_score_test.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_score_test.go`
- `docs/superpowers/plans/2026-09-09-t139-quality-freshness-implementation-plan.md`

## 验证

通过：

- `go test ./internal/repository -run 'TestUsageLogRepositoryListOpenAIAccountQuality|TestOpenAIAccountQualityQuery' -count=1`
- `go build ./internal/service`
- `go build ./cmd/server`
- `gofmt`
- `git diff --check`

未通过/未能运行：

- 服务包定向测试无法编译，主线已有无关缺失符号：
  - `resolveCompositeModelOwnership`
  - `decodeCodexManifestModels`
- 该阻断在 T139 基线同样存在，未修改无关主线代码。

## 数据、迁移与配置

- 无数据库 migration。
- 无配置文件或运行时配置项变更。
- 不写入生产数据，不读取或携带凭据。
- 未部署验收站或主站。

## 回滚

根审合并前可直接放弃候选分支；合并后使用恢复到合并前已验证 `main` 提交的前向 revert。T139 不改变数据结构，因此无需数据库回滚。

## 剩余风险

- 服务包完整测试仍受基线缺失符号阻断，需根线程在合并前按现有项目策略决定是否补齐基线缺失或保留该未验证项。
- 本候选未进行部署和线上专项验收。
