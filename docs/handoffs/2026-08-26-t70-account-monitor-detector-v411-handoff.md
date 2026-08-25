# T70 账号检测分层监测与记录面板交接

## 状态

`READY_FOR_ROOT_REVIEW`（候选实现与直接相关验证完成，等待根发布总控刷新、合并、发布和线上验收）。

## 基线与提交

- 基线：`main@7e33e50d2`
- 候选分支：`codex/t70-account-monitor-detector-v411`
- 候选 worktree：`.worktrees/t70-account-monitor-detector-v411`
- 本次候选提交：见当前分支最新提交

## 交付范围

- detector 记录新增 `profile`、`mode`、`trigger_reason`、计划/有效样本、证据状态和指纹状态；旧行读取为历史/unknown，不回填。
- 新增迁移 `228_account_model_detection_evidence.sql` 和稳定 `(created_at,id)` 游标分页，handler 支持 `limit/cursor/status/profile/mode`。
- 日常周期检测使用 medium/monitor；手动检测使用 low/manual；首次结果、连续异常/证据不足和模型冲突可排队 high/escalation，账号级冷却和活动任务去重保留。
- sidecar v4.1.1 字段合同在有界响应内解析，拒绝超范围样本，凭据/URL/提示词/输出仍不落库或前端渲染。
- 账号卡片的模型检测状态入口改为账号级完整记录面板；桌面为右侧表格抽屉，窄屏为全屏时间线，详情只渲染受限的双证据字段。

## 直接相关验证

- `go test ./internal/service ./internal/repository ./internal/handler/admin ./migrations -run 'Detection|AccountMonitor|ModelDetection' -count=1`
- `go build ./cmd/server`
- `pnpm vitest run src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`：103 tests passed
- `pnpm typecheck`
- `pnpm build`
- `gofmt` 与 `git diff --check`

## 迁移与配置

- 新增一份向前兼容 schema 迁移，不回填已有检测行。
- 无新增配置、无生产业务数据写入、无 GitHub Actions。
- 预期 `downtime_required=false`，以根发布预检实际输出为准。

## 发布与回滚

- 必须从刷新后的根 `main` 运行现有本地/宿主蓝绿发布链，不得从候选 worktree 直接发布。
- 回滚恢复上一已验证镜像；迁移由既有原子切换和迁移保护处理，不删除历史检测记录。

## 未验证项与风险

- 尚未做生产登录态线上专项验收，需确认管理员历史接口字段、桌面抽屉/窄屏时间线无横向溢出，以及 detector 未配置时仍显示真实未接入语义。
- 需要根总控在最新 `main` 上复跑直接相关检查、发布预检和线上验收后才能标记完成。
