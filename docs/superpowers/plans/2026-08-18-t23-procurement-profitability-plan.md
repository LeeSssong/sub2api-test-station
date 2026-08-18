# T23 实施计划

- [x] 新增 226 expand-only 采购版本/结算台账迁移与迁移守门测试。
- [x] 新增 service 纯公式、版本解析、结算幂等和 SQL 聚合测试（先 RED 再 GREEN）。
- [x] 在管理员账号更新路径追加版本台账与审计；新增失效结算 handler/route。
- [x] 新增独立自购盈利查询 service/handler/route，保持旧渠道 USD API 不变。
- [x] 扩展前端 API、经营页自购视图、移动布局及 Vitest。
- [x] 运行直接相关 Go/Vitest、typecheck、build、diff-check；形成 handoff，状态 READY_FOR_ROOT_REVIEW。

验收命令：
`go test ./internal/service ./internal/handler/admin ./internal/repository -run 'Procurement|SelfPurchased|AccountProfitability' -count=1`
`pnpm --dir frontend vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
`pnpm --dir frontend run typecheck && pnpm --dir frontend run build`
