# Native OpenAI Four-Switch and Logical Monitoring Verification

日期：2026-09-01
候选 worktree：`codex/t87-logical-request-error-lifecycle`
状态：`READY_FOR_ROOT_REVIEW`

## 已实现

- 普通 OpenAI HTTP 文本路径不再通过 `extra_retry_count` 替换运行时重试预算。
- 保留 `extra_retry_count` 的设置、解析和 API 兼容性。
- OpenAI 原生请求级切号上限由 2 提升为 4；对应允许初始 attempt 加最多 4 次跨账号切换。
- 保留账号级 `pool_mode_retry_count`、错误分类、余额/429 冷却、流安全和客户端断开保护。
- Monitor V4 主查询和分组真实请求查询均按 `group_id + logical request key` 选择最终事件；跨账号通过精确 `request_id -> logical_request_id` 映射关联，禁止单独使用 `client_request_id`；账号管理查询仍保留账号级物理 attempt 粒度。

## 验证结果

- 通过：`go test ./internal/repository -run 'TestAccountMonitorRepository(GroupRealRequestProjectionDeduplicatesAcrossAccountsByFinalEvent|ProjectMonitorV4)' -count=1`
- 通过：`go test ./internal/config -run TestGatewayOpenAISharedHealthDefaultsAndHardLimits -count=1`
- 通过：`go test ./internal/service -run 'TestOpenAIUnifiedQuality' -count=1`
- 通过：T87 主查询与分组查询的逻辑请求去重回归，包含跨账号恢复场景。
- `git diff --check` 通过。
- handler 包定向测试暂不能执行：候选基线已有编译错误，涉及 `ProvideHandlers` 参数数量和缺失的 `openAIAccountScheduleModel`，与本次改动无关。

## 范围与发布门禁

- 未修改根目录 `main`、全局任务队列、生产配置、生产数据或发布链。
- 未部署、未推送，不能据此宣称线上已生效。
- 根发布总控需在干净且与 `origin/main` 一致的 `main` 上完成审查、合并、直接相关回归和发布预检。
