# T03 上游扣费与利润始终有值实施计划

> 基线：`main@0b6ef4ef0faec1b84de6f7133b5c99c4ae6e405d`
>
> 工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t03-upstream-cost-profit-values`
>
> 分支：`codex/t03-upstream-cost-profit-values`

## Goal

让管理员原生流水在精确命中 Sub/New 上游账单后始终得到数值上游实际扣费和利润，其中上游原生扣费空白按 `0`。

## Context

- 服务：`upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- 服务测试：`upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
- 管理员处理器测试：`upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go`
- 前端已验证 confirmed zero：`upstream/sub2api/frontend/src/components/usage/usageDetail.ts`

## Constraints

- 只保留精确请求 ID 匹配；不得新增模糊匹配或估算。
- 只有精确命中记录后的原生扣费空白归零；查询或命中失败不归零。
- 普通用户继续不可见。
- 无迁移、无配置、无 relay-ops、无历史回填、无生产动作。

## Tasks

### Task 1: RED 覆盖 Sub/New 空白原生扣费

- [ ] 在 `sub_upstream_cost_test.go` 增加 Sub `null`、缺失、空字符串定向用例。
- [ ] 增加 New `quota` 为 `null`、缺失、空字符串定向用例。
- [ ] 保留非空非法数值失败用例，证明没有把坏数据静默归零。
- [ ] 运行聚焦命令并记录预期失败原因。

### Task 2: GREEN 实现最小账单数值归一化

- [ ] 在 `sub_upstream_cost.go` 增加受限的可空 JSON 数值解析器。
- [ ] Sub 精确记录命中后从归一化数值得到实际扣费。
- [ ] New 精确记录命中后以归一化 quota 换算成本；空 quota 为零。
- [ ] 非空非法值继续返回 `response_unavailable`。
- [ ] 运行 Task 1 聚焦测试并确认通过。

### Task 3: 管理员接口与权限回归

- [ ] 在 `usage_handler_detail_test.go` 证明管理员接口可返回 confirmed zero 与数值利润。
- [ ] 运行管理员 handler 聚焦测试。
- [ ] 运行现有前端 confirmed-zero helper/dialog 聚焦测试，确认无需 UI 改动。

### Task 4: 验证与独立复审

- [ ] `go test ./internal/service -run 'TestSubUpstreamCost' -count=1`
- [ ] `go test ./internal/handler/admin -run 'TestAdminUsageGetUpstreamCost|TestAdminUsageGetByID' -count=1`
- [ ] `go vet ./internal/service ./internal/handler/admin`
- [ ] `go build ./cmd/server`
- [ ] 前端 `usageDetail.spec.ts` 与 `UsageDetailDialog.spec.ts` 聚焦 Vitest。
- [ ] `pnpm typecheck`（若没有前端实现变化，仍作为 DTO/调用契约验证）。
- [ ] `git diff --check` 与范围自查。
- [ ] 独立审查规格、实现、测试和权限边界；修复发现后重新验证。
- [ ] 更新总账与验证报告，提交候选并汇报 `READY_FOR_ROOT_REVIEW`。

## Done when

- Sub/New 明确收费与空白扣费均按规格返回数值。
- 查询失败、未命中与非法非空数据不伪装成零扣费。
- 普通用户不可见边界不变。
- 无迁移、配置、relay-ops、生产操作或超范围变更。
- 独立复审通过，候选工作区干净且仅等待根线程授权。
