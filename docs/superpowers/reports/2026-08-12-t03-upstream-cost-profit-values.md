# T03 上游扣费与利润始终有值验证报告

## 状态

- 任务包：T03 上游扣费与利润始终有值
- 基线：`main@0b6ef4ef0faec1b84de6f7133b5c99c4ae6e405d`
- 候选：`codex/t03-upstream-cost-profit-values@fe9d04c9ccd8e40faf1e58ff137004f57964392b`
- 交接状态：`READY_FOR_ROOT_REVIEW`
- 尚未合并、推送、部署或线上验证

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

## 发布属性与回滚

- 数据库迁移：无
- 配置变化：无
- 前端实现变化：无
- GitHub Actions：无
- 预期 `downtime_required=false`；最终值仍须由根线程在合并后的 `main` 发布预检确认
- 合并前回滚：丢弃候选分支/工作区
- 合并后回滚：revert 候选提交；无数据或配置回滚

## 未验证项与风险

- 候选线程未访问生产，未执行生产预检、部署或线上验证。
- 上游账单响应可能存在未见过的非标准数值语义；实现仅接受有限数值，并将非空非法值保持为不可用。
- `quota_per_unit` 的 `null`、空字符串、非数字和 `0` 可继续补充枚举测试；现有解析与正数检查已拒绝这些值，此项为非阻断覆盖增强。
