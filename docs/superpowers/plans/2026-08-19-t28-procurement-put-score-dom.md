# T28 实施计划

1. 建立后端 RED 测试：采购 PUT 不先提交通用更新；采购 service 覆盖 `cost_pending` NULL、幂等 replay/冲突、事务错误和 rollback。
2. 调整 `AccountHandler.Update`：采购字段先走原生台账事务，通用账号更新保持既有入口但不再携带采购字段；保留非采购字段行为和错误映射。
3. 加固 `UpdateProcurementConfig` 的 nullable 扫描、重放语义与错误上下文，确保清空/重复提交无重复版本。
4. 前端 API 为采购编辑会话生成/复用幂等键；保存和清空保持 PUT+reload，区分服务错误与 reload 错误。
5. 调整 `AccountMonitorCard` 三列 DOM 顺序为评分、优先级、排名，补桌面/390px 稳定布局断言。
6. 运行 Go/Vitest focused tests、typecheck/build、gofmt、diff-check；形成 handoff，状态 READY_FOR_ROOT_REVIEW，不合并、不推送、不部署、不清理。
