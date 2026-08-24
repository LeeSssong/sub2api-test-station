# T54-R2 调度预设语义、中文参数与有界校验交接

## 状态

`READY_FOR_ROOT_REVIEW`

候选分支：`codex/t54-r2-scheduler-labels`
候选提交：`adbf0b2c0`
预计发布属性：无迁移、无生产业务数据写入、`downtime_required=false`

## 已实现

- 内置预设 ID 保持 `builtin:special_offer`、`builtin:balanced`、`builtin:pro`，管理员展示名改为“体验优先 / 体验均衡 / 利润优先”。
- 前端调度参数使用中文短标签、有限区间和简短说明；内部 API key 未改名。
- TopK、权值、公平参数在前端控件和后端写入校验中均有界；运行时读取旧值采用同一边界语义，冷落阈值保留 `0` 兼容含义。
- 预设模式的参数仍只读，自定义模式仍可编辑；三步分组流程、原生分组来源、管理员命名预设逻辑保持不变。
- 未修改调度算法选择顺序、S1/S2 硬预算、自动重试、sticky、故障域、Monitor V2、迁移或生产配置。

## 验证证据

- `go test ./internal/service -run 'Test(OpenAIScheduler|NormalizeOpenAIScheduler|ParseOpenAIScheduler)' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts`：44/44 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `git diff --check`：通过。

## 根总控动作

1. 在当前根 `main` 盘点 T56 及其他领先 worktree，确认不把独立任务混入本次发布。
2. 将候选提交合并到根 `main`，在合并后的 `main` 重跑必要门禁和发布预检。
3. 仅从已验证的 `main` 按既有本地/宿主蓝绿链推送、部署和线上验收；不使用 GitHub Actions。
4. 线上验收设置页中文预设、中文参数区间/说明、预设禁用和旧配置读取；公网 `/healthz`、`/readyz`、`/health` 均应为 200。

## 回滚

优先切回上一蓝绿槽或上一发布镜像；本任务无数据库迁移，不需要数据库回滚。若线上发现旧客户端兼容性问题，恢复上一槽后保留候选 worktree和证据继续修复。
