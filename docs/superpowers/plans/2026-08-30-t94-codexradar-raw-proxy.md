# T94 CodexRadar 外部 API 宽容透传实施计划

- [x] 移除后端对第三方社区响应的固定 schema/点位业务拒绝。
- [x] 无共同模型/effort 时返回 fresh 空综合 tab。
- [x] 放宽前端对 fresh 空 tab、零样本点和重复档位的本地拒绝。
- [ ] 运行 CodexRadar 后端与前端直接相关测试。
- [ ] 审查差异并形成候选提交与交接。

验证命令：

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestCodexRadarCommunity' -count=1

cd ../frontend
pnpm vitest run src/features/monitor-v2/__tests__/codexRadarCommunity.spec.ts \
  src/features/monitor-v2/__tests__/CodexRadarCommunityMatrix.spec.ts

git diff --check
```
