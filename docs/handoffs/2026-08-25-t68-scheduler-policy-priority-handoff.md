# T68 调度策略优先级交接

- 状态：`READY_FOR_ROOT_REVIEW`
- 原始基线：`main@c70f11193`
- 候选 worktree：`.worktrees/t68-scheduler-policy-priority`
- 候选分支：`codex/t68-scheduler-policy-priority`
- 当前提交：以候选 worktree `HEAD` 为准（最终 SHA 见根交接消息）
- 未合并、未推送、未部署，等待根总控发送 `AUTHORIZE_MERGE_TO_MAIN`
- `downtime_required`：待根发布预检确认；本候选无迁移、无停机动作

## 交付范围

- 服务端新增业务策略类型、严格校验、推荐默认值和 legacy `weighted_override` / `fair` / `preset` 兼容读取。
- 服务端将利润、首字速度、完整耗时及三个运营控件编译为现有 native scheduler 权重、公平、探索和 sticky 输入；客户端 `compiled_snapshot` 不具备覆盖权。
- native scheduler 继续沿用资格、容量、冷却、故障域、sticky、原子并发抢槽、fresh/DB recheck 等安全边界。
- SettingsView 改为数字优先级、三个固定三段控件、固定“服务不中断”护栏、摘要、场景预览和按组草稿/reset；legacy DOM 仅隐藏保留以兼容旧客户端/测试。
- 中英文 locale 与 API 类型同步更新。

## 变更文件

- `upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- `upstream/sub2api/backend/internal/service/setting_parse.go`
- `upstream/sub2api/backend/internal/service/settings_view.go`
- `upstream/sub2api/frontend/src/api/admin/settings.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`
- `docs/handoffs/2026-08-25-t68-scheduler-policy-priority-handoff.md`
- `.superpowers/sdd/2026-08-25-t68-scheduler-policy-priority/task-report.md`

## 验证

通过：

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin -run 'Test(OpenAIScheduler|NormalizeOpenAIScheduler|ParseOpenAIScheduler|Setting.*Scheduler|Admin.*Setting)' -count=1
go build ./cmd/server

cd ../frontend
pnpm typecheck
pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts
pnpm build

git diff --check
```

Vitest 结果为 44/44；构建成功。测试输出中的既有 `router-link` resolve warning、jsdom AggregateError 和 Vite chunk warning 未导致失败，未扩大为无关修复。

## 发布与回滚

- 无数据库迁移、无生产数据写入、无新增配置事实源、无 GitHub Actions。
- 根总控须先在最新 `main` 上合并并重新执行直接门禁、发布预检、推送、蓝绿部署和线上设置页/调度专项验收。
- 若合并后失败，保留本候选继续修复；回滚恢复到合并前 `main` 或本候选上一稳定提交。

## 剩余风险

- 具体业务优先级到 native 系数的映射由服务端固定，需根发布后结合真实账号负载做只读专项观察。
- 旧 legacy 控件仍存在于隐藏兼容层，运营主界面不可见。
