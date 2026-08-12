# T03 上游扣费与利润始终有值验证报告

## 状态

- 任务包：T03 上游扣费与利润始终有值
- 基线：`main@0b6ef4ef0faec1b84de6f7133b5c99c4ae6e405d`
- 实现提交：`fe9d04c9ccd8e40faf1e58ff137004f57964392b`
- 候选：`codex/t03-upstream-cost-profit-values@72c3645d752a5919c0020d9c8f26e6e7bfc63a9d`
- 合并提交：`main@0432b87491a313b006643212cccdcd8d49001ae4`
- 状态：已推送、已部署并完成组合证据线上验收

## 交付范围

- Sub `actual_cost` 与 New `quota` 在原生账单记录被既有精确请求 ID 规则命中后，`null`、缺失或空字符串归一为 `0`。
- JSON number 和有效数值字符串继续使用原值；利润仍由后端按 `本站实际扣费 - 上游实际扣费` 计算。
- 非空非法数值返回 `response_unavailable`；未精确命中返回 `record_not_found`；查询、鉴权、端点、网络、分页或响应失败不伪装为零扣费。
- New `quota_per_unit` 仍是必需的换算元数据，缺失、非法或非正数时保持不可用。
- 管理员接口可返回 confirmed zero 与数值利润；普通用户接口和 DTO 未增加成本或利润字段。

## TDD 与最终验证

RED 阶段的聚焦测试按预期失败：Sub 空字符串及 New 的 `null`、缺失、空字符串均错误返回 `unavailable`。最小实现完成后，同一测试转绿。

最终验证：

```text
go test ./internal/service -run 'TestSubUpstreamCost' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service

go test ./internal/handler/admin -run 'TestAdminUsageGetUpstreamCost|TestAdminUsageGetByID' -count=1
ok github.com/Wei-Shaw/sub2api/internal/handler/admin

go vet ./internal/service ./internal/handler/admin
go build ./cmd/server
# exit 0

frontend vitest: 2 files, 33 tests passed
pnpm typecheck
# exit 0

git diff --check
# exit 0
```

独立审查额外将服务聚焦测试以 `-count=20` 重复运行并通过，结论为 P0-P2 无发现。审查确认：空白归零只发生在精确命中之后；非法值、未命中和 `quota_per_unit` 失败边界不变；管理员专属权限未变化。

## 发布、生产与线上验收

- 数据库迁移：无
- 配置变化：无
- 前端实现变化：无
- GitHub Actions：无
- 发布预检与正式发布均返回 `downtime_required=false`。
- 生产记录：`/var/lib/sub2api/release-records/20260812T084707Z-production-974305.json`，结果为 `succeeded/promoted`、`rolled_back=false`。
- 活动槽：`green`。
- 生产镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-0432b87491a313b006643212cccdcd8d49001ae4-51ca035481d9b5df24a16758660c0445bb69d346130673204bfeef3f40399ff0`。
- 迁移哈希保持 `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`。
- 公网 `/healthz`、`/readyz`、`/health` 均返回 200；API、worker、blue/green、PostgreSQL、Redis 和 Caddy 健康。

2026-08-12 线上自然流水验收分两次完成：

1. 首次最近 80 条样本中 14 条为 `confirmed`，均返回数值上游扣费和利润；例如 `usage_id=87220` 的本站实际扣费为 `0.01415275`、上游实际扣费为 `0.011322`、利润为 `0.00283075`。
2. 收口时复核部署后的最新 60 条成功流水：3 条 `confirmed` 均返回数值，57 条保持不可用；不可用原因为 `endpoint_unsupported=34`、`record_not_found=19`、`response_unavailable=4`。确认样例包括 `usage_id=87320`，上游实际扣费 `0.05`、利润 `0.0706`。

生产自然流量仍未出现“精确命中原生账单且扣费字段为空”的 confirmed-zero 样本。根据已批准规格，T03 只在精确命中后把空白、缺失或 `null` 归一为 `0`；端点 404、未命中和响应非法不属于归零范围，不能伪造为零成本。空白分支由与生产运行代码一致的提交树上的服务测试、管理员 Handler confirmed-zero 测试和前端 confirmed-zero 展示测试证明；线上则证明同一部署身份、明确收费主路径、不可用失败语义和系统健康。因此采用“同一发布树合同测试 + 生产身份/收费路径/失败边界实证”的组合证据完成验收。

2026-08-12 收口前新鲜回归：

```text
go test ./internal/service -run 'TestSubUpstreamCost' -count=1
go test ./internal/handler/admin -run 'TestAdminUsageGetUpstreamCost|TestAdminUsageGetByID' -count=1
go vet ./internal/service ./internal/handler/admin
go build ./cmd/server
frontend vitest: 2 files, 33 tests passed
pnpm typecheck
git diff --check
# 全部 exit 0
```

从生产部署提交 `0432b8749` 到收口时 `main` 的差异仅有三份治理文档，没有运行时代码漂移。

## 恢复与归档

- 应用回滚：revert `fe9d04c9ccd8e40faf1e58ff137004f57964392b` 后重新走受控蓝绿发布；无数据或配置回滚。
- 候选恢复归档：`/Users/gongtengxinwen/Documents/sub2api-archives/t03-upstream-cost-profit-values-72c3645d7.bundle`。
- 归档 SHA-256：`4021d92ee877e78dd9c781bff0ee0a9bcf120cd9fb3f475f525cbe9f2d6e0e2a`。
- 归档权限：`0600`；`git bundle verify` 已确认完整历史与候选 ref。

## 边界与剩余风险

- 上游账单响应可能存在未见过的非标准数值语义；实现仅接受有限数值，并将非空非法值保持为不可用。
- `endpoint_unsupported`、`record_not_found` 和 `response_unavailable` 仍会明确显示不可用而非伪造数值；这是规格内安全边界，不是 T03 未完成项。
- `quota_per_unit` 的 `null`、空字符串、非数字和 `0` 可继续补充枚举测试；现有解析与正数检查已拒绝这些值，此项为非阻断覆盖增强。
