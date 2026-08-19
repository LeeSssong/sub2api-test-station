# T30 OAuth 自购闭环交接

- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`main@6bf76430c65105232e87fd3cd1cb9eec4b05d010`
- 候选分支：`codex/t30-oauth-self-purchased-closure`
- 范围：采购保存 internal error/半成功契约、全量未删除 OAuth CNY 报表、逐行共享采购成本入口。

## 实现

- CNY 报表由 `accounts.deleted_at IS NULL AND accounts.type='oauth'` 驱动；无采购版本/投影账号生成 `cost_pending`，0 流水仍显示。
- 修复真实长幂等键写入 `audit_logs.request_id VARCHAR(64)` 可能导致事务失败的问题：采购台账保留完整键，审计副本使用 `LEFT($3,64)`。
- 写入输入/不存在/幂等冲突保留 4xx/409；真正内部错误返回中文 message、reason、account_id/request_id。
- 台账提交后 `GetAccount` 失败返回可识别 HTTP 202 `procurement_saved_readback_failed` 与采购投影；重复同键只 replay，不重复写版本。前端 API 归一 interceptor rejected object 为判别结果，页面显示“已保存但刷新失败”。
- CNY 每行提供录入/编辑成本，复用 `AccountMonitorCostDialog` 与 `adminAPI.accounts.updateProcurementCost`；默认额度 60 USD，支持保存/清空，刷新失败保留弹窗与明确提示。
- 共享 Dialog 改为最小成本账号结构，不伪造监控状态；OAuth 跨平台统一进入采购模式。

## 变更文件

见 `git diff --name-only 6bf76430c...HEAD`；仅后端采购服务/handler、前端共享成本表单/CNY 页/API、直接测试、规格计划与本交接。

## 验证

- Go：采购、自购报表、handler partial-success/幂等定向测试。
- Vitest：API interceptor、共享 Dialog、CNY 页录入/编辑/默认60/保存/清空/reload 成败。
- Typecheck、production build、`git diff --check`。

## 未验证

- 未执行生产写入、部署、线上验证或认证浏览器截图；按最新范围不要求额外生产写入专项。

## 迁移/配置/发布

- 无迁移，无配置，无 GitHub Actions。
- `downtime_required=false`（预期；根合并后预检为准）。
- 未合并、推送、部署、修改全局队列/进度或生产。

## 回滚与风险

- 回滚：根总控恢复上一已验证 main/镜像；无 schema 回滚。
- 剩余风险：HTTP 202 partial-success 依赖现有 interceptor rejected-object 合同，已由 API 单测覆盖；生产数据库真实错误需根发布后日志/线上专项确认。
