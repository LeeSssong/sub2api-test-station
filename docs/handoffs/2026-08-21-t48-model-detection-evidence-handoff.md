# T48 模型检测证据交接

- 任务包：T48 模型映射/替换双证据检测与上游返回值展示
- 状态：`READY_FOR_ROOT_REVIEW`
- 基线 `main`：`525b35d3a`
- 实现候选提交：`b4a080e5c`（包含 UI/i18n 收口；此前实现提交 `d83a71fbf`、`7db89718f`、`16adb6e9f`）
- 当前分支：`codex/t48-model-detection-evidence`
- 当前 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t48-model-detection-evidence`

## 交付内容

1. 后端 sidecar 继续复用 `juice_summary`，新增有界 `model-detection-evidence-v1`：
   - 请求/申报模型
   - `/models` 目录状态、数量和最多 10 个模型摘要
   - 主动 `/v1/responses` 顶层 `model`（仅此字段，不把目录候选冒充响应模型）
   - 指纹状态、候选和相似度
   - `verified`、`suspected_mapping`、`suspected_replacement`、`high_risk_inconsistent`、`insufficient` 结论
2. 主动探针使用低 Token、非流式请求，响应缺少顶层 `model` 时明确记录 `missing`。
3. 管理员账号模型检测弹窗新增证据区，模型/指纹不匹配时明确显示请求模型、上游实际响应模型、目录摘要、指纹候选与相似度；旧结果无证据包时保留兼容显示且不伪造上游响应模型。
4. 中英文 locale 同步补齐证据和结论文案。
5. 现有 sidecar bounded summary 脱敏合同测试覆盖凭据、Base URL、Authorization、prompt/output/request/response 递归移除。

## 验证证据

- `go test ./cmd/model-detector -count=1`：通过
- `go test ./internal/service -run 'TestHTTPAccountModelDetectionSidecar' -count=1`：通过
- `go test ./internal/repository -run 'TestAccountModelDetection' -count=1`：通过
- `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`：103/103 通过
- `pnpm typecheck`：通过
- `pnpm build`：通过（仅既有 dynamic-import/Browserslist/deprecation 警告）
- `gofmt`、`git diff --check`：通过
- 作用域扫描：未修改 `.github/workflows`、数据库 migrations；未发现新增敏感数据持久化

## 发布边界

- 未合并根 `main`、未推送、未部署、未修改生产数据。
- T47-R2 当前占用唯一发布单车道；T48 等待根总控授权后再整合。
- 无数据库迁移、无配置 schema 变化、无 GitHub Actions。
- 预期 `downtime_required=false`，以合入最新 `main` 后根发布预检为准。
- 回滚：恢复上一已验证镜像；无数据回滚需要。

## 已知限制

当前 `native-1` 只提供模型目录和主动响应模型证据，不伪造行为指纹基线。真实指纹候选仍必须来自符合 T15 合同和许可门禁的独立 detector；未接入时页面显示“未检测行为指纹”。
