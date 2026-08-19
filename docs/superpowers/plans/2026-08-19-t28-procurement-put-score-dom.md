# T28 实施计划

1. 建立后端 RED 测试：采购 PUT 不先提交通用更新；采购 service 覆盖 `cost_pending` NULL、幂等 replay/冲突、事务错误和 rollback。
2. 调整 `AccountHandler.Update`：采购字段先走原生台账事务；采购-only 成功后 `GetAccount` 返回，混合请求才调用通用账号更新且不携带采购字段。
3. 加固 `UpdateProcurementConfig` 的 nullable 扫描、重放语义与错误上下文，确保清空/重复提交无重复版本。
4. 在 `AccountMonitorView` 以一次成本弹窗会话和 payload 管理幂等键，API 仅显式透传；保存和清空保持 PUT+reload，区分服务错误与 reload 错误。
5. 保留单卡内部布局；为 `AccountMonitorView` 增加乱序输入的页面级 DOM 排序测试，并断言桌面/390px 使用同一普通 Grid DOM 顺序且无 reverse/order。
6. 运行 Go/Vitest focused tests、typecheck/build、gofmt、diff-check；形成 handoff，状态 READY_FOR_ROOT_REVIEW，不合并、不推送、不部署、不清理。
