# T54-R1 分组调度三步流程与全局命名预设交接

状态：`READY_FOR_ROOT_REVIEW`（本地直接验证完成；未合并、未推送、未部署、未进行线上验收）

## 候选与基线

- 任务声明基线：`main@cacf5804d5eaa35cf5aa1f05cf06255f104b2e67`
- 候选分支：`codex/t54-r1-group-scheduler-preset-workflow`
- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t54-r1-group-scheduler-preset-workflow`
- 候选实现提交（本 handoff 提交前）：`373244637b3f67068ba54a869c805a0e4e8c3e10`
- 候选实现 tree：`6a488d93945a582712235bc907765de1de4b298b`
- 验证时根 `main`：`1af258ba776a9ff6e72248f36cb685d1e5a4a4a3`

候选从任务声明基线之后包含 Task 1–3 实现。验证命令按 brief 原样在当前 worktree 执行，其中 `git diff ... main...HEAD` 以验证时的 `main@1af258b…` 为目标。根 `main` 已在候选创建后前进，候选相对当前 `main` 处于 `REFRESH_REQUIRED`：根总控合并前必须将候选刷新到最新 `main`，解决仅本任务范围冲突（如有），并重跑本 handoff 中的直接门禁。

## 实现范围

本候选仅覆盖 T54-R1 已批准范围：

- 原生 OpenAI 调度分组与默认订阅分组分离；调度分组不再套用 `subscription_type=subscription` 过滤。
- SettingsView 按“选择分组 -> 选择策略模式 -> 配置参数/选择预设”渐进展示。
- 策略模式仅保留“自定义参数”和“预设模式”；预设模式最终参数禁用并以服务端快照序列化。
- 自定义参数可保存为管理员命名预设，管理员预设可重命名；被引用的管理员预设不能删除。
- 内置特惠/均衡/Pro 预设保持既有数值且不可变；旧 `weighted_override/fair` 数据继续兼容。

## 变更文件

相对验证时 `main@1af258b…` 的候选变更路径（无 migration 或 `.github/workflows`）：

- `.superpowers/sdd/2026-08-23-t54-r1-group-scheduler-preset-workflow/task-2-report.md`
- `upstream/sub2api/backend/internal/handler/admin/setting_handler.go`
- `upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`
- `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- `upstream/sub2api/backend/internal/handler/dto/settings.go`
- `upstream/sub2api/backend/internal/service/domain_constants.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- `upstream/sub2api/backend/internal/service/setting_parse.go`
- `upstream/sub2api/backend/internal/service/setting_update.go`
- `upstream/sub2api/backend/internal/service/settings_view.go`
- `upstream/sub2api/frontend/src/api/admin/settings.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`

The only file added by Task 4 is this handoff.

## Direct validation evidence

All commands below exited `0` on the candidate implementation:

1. `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler/admin -run 'Test(OpenAIScheduler|.*Scheduler.*Setting|.*Setting.*Scheduler)' -count=1`
   - `internal/service`: PASS
   - `internal/handler/admin`: PASS
2. `cd upstream/sub2api/backend && go build ./cmd/server`
   - PASS
3. `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts`
   - `1` file, `43/43` tests PASS.
4. `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`
   - typecheck PASS; production Vite build PASS (`1060` modules transformed).
5. `git diff --check main...HEAD`
   - PASS, no whitespace errors.
6. Scope checks:
   - `git diff --name-only main...HEAD` contains no migration path and no `.github/workflows` path.
   - No changed dependency manifests/lockfiles, deployment/production files, or configuration schema files were found.

The Vitest run emits existing non-blocking `router-link` resolution warnings, one existing jsdom XHR `AggregateError` warning, pnpm `pnpm.overrides` configuration warning, Browserslist staleness notice, and Node/Vite deprecation notices. They did not fail assertions or change exit status.

## Release properties

- Database migrations: none.
- Dependency changes: none.
- Production-data writes: none performed or introduced.
- GitHub Actions: none.
- Configuration/schema changes: none.
- `downtime_required`: expected `false` for this code-only settings/UI change; the root release preflight remains authoritative. If preflight returns `true`, stop before any stop/restart/switch and obtain the required authorization.

## Unverified items and risks

- Candidate has not been refreshed onto current root `main@1af258b…`; this is a mandatory `REFRESH_REQUIRED` action before integration.
- Root merge, push, release preflight, blue-green deployment, runtime readiness, and logged-in production SettingsView verification remain unverified.
- No production settings were modified during this task, so persisted preset behavior in production remains for post-deploy online verification.
- Existing test-harness warnings listed above remain non-blocking but should be retained as context when reviewing the root run.

## Rollback

Before integration, discard this candidate and retain the current verified root `main`. After integration/deployment, roll back by promoting the previous verified blue-green application slot or reverting the T54-R1 merge on `main`; no database rollback or data cleanup is required because this candidate adds no migration or production-data write.
