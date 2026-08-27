# 组合部署包交接：T80 + T81

日期：2026-08-27

## 候选身份

- 根分支：`main`
- 候选提交：`6d82f68d0cd0f550c7850ae85921091a1bd89b7a`
- 候选树：`707157892cf49978ffcf2a9605e9c2e0dec617c7`
- 工作区：`/Users/gongtengxinwen/Documents/sub2api搭建`
- 工作区状态：合并后干净；未推送、未部署

## 包含内容

1. T80 OpenAI 长请求调度准入韧性：账号级跨模型/跨分组首输出前 admission lease、slow-session guard、首语义输出释放、失败/取消幂等清理、共享写入 context 隔离和脱敏可观测性。
2. T81 管理员仅赠送额度充值：充值模式允许现金金额或赠送额度任一大于零；双零仍拒绝；退款仍要求现金金额大于零；复用原生 `quota-ledger`。

## 直接验证

- T80：后端 config/repository/service/handler 定向测试、`go build ./cmd/server`、gofmt 检查、`git diff --check`。
- T81：`pnpm exec vitest run src/components/admin/user/UserBalanceModal.spec.ts`（4/4）、`pnpm typecheck`、`pnpm build`、`git diff --check`。
- 组合候选：无迁移文件变化；无生产配置和业务数据写入。

## 明确排除

- 测试站邮箱模板一致性核对：无代码变更，不纳入包。
- 生图用户报障：只读根因分析；退款候选 usage log `166983`、`167016` 未执行。
- 服务质量运维：只读日志统计，无代码变更。
- T79 验收站仍是独立 `VERIFYING` 事项，不能伪装成 T80/T81 的功能验收。

## 发布门禁

- 本包尚未推送 `origin/main`，也未部署验收站或主站。
- 发布契约修订后，验收站发布交付契约测试已通过；宿主蓝绿测试已执行 fail-closed、预加载归档传输和停机门禁场景，完整宿主测试在本地长耗时场景下未在 180 秒窗口内结束，未据此宣称通过。
- 常规路径必须先完成验收站真实功能验收，并取得用户明确“测试站验收通过，部署主站”。
- 紧急路径只有用户明确“快速部署到主站”才可启用；仍需主站发布链、健康检查、回滚保护，并在主站成功后立即用同一 commit 同步/核对验收站。
- 若发布预检返回 `downtime_required=true`，必须在任何停机、迁移、重启或切换前再次取得停机授权。

## 回滚

使用宿主发布链保留的上一已验证蓝绿槽/镜像回滚；本包无迁移，不需要数据回滚。未执行任何生产退款、余额修改或批量数据写入。
