# T93 Monitor V4 最终用户可见结果实施计划

- [x] 复用 T85 的桶级真实/探测选择和逻辑请求去重。
- [x] 将 partial usage 从成功候选降为最终失败候选，避免失败 attempt 因 `actual_cost>0` 被误算成功。
- [x] 保持生图专用账号不进入文本探测池，并保持余额不足遵循原生不可调度。
- [ ] 增加逻辑请求最终结果回归测试并运行 Monitor V4 后端/前端直接相关测试。
- [ ] 审查差异并形成候选提交与交接。

验证命令：

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestAccountMonitorRepositoryProjectMonitorV4' -count=1
go test ./internal/service -run 'TestMonitorV4|TestAccountMonitor.*(Pool|Run|Probe)' -count=1
go build ./cmd/server

cd ../frontend
pnpm vitest run src/features/monitor-v4
pnpm typecheck
```
