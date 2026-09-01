# Native OpenAI Four-Switch and Logical Monitoring Handoff

候选分支：`codex/t87-logical-request-error-lifecycle`
候选状态：`READY_FOR_ROOT_REVIEW`

## 交接内容

本候选包含两部分：恢复普通 OpenAI HTTP 请求的 Sub 原生重试/切号边界并提高到 4 次跨账号切换；完成 T87 Monitor V4 逻辑请求终态投影，使一次跨账号逻辑请求只计一次最终结果。T87 原规格未修改，`extra_retry_count` 仍保留为兼容字段但不参与运行时控制。

## 审查重点

1. 合并前确认普通 OpenAI 文本入口的 `retryBudget.unified` 在生产路径不可达，且质量选号仍只负责排序。
2. 确认跨账号恢复成功的请求在分组中只出现一个成功样本，最终 503/unsafe stop 只出现一个失败样本；关联只能使用精确 `request_id` 和 `logical_request_id`，不能使用单独的 `client_request_id`。
3. 确认账号管理页面仍使用账号级查询，管理员可继续看到物理 attempt 和坏账号诊断。
4. 处理候选基线已有的 handler 编译错误后，再运行 handler 直接相关测试和服务构建。

## 发布限制

本候选未推送、未部署、未修改生产数据。只有发布总控可合并到根 `main`、推送、执行验收站/主站发布和线上验证。
