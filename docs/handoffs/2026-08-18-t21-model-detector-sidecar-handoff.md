# T21 生产模型检测 Sidecar 接入与离线状态纠正交接

## 候选信息

- 任务包：T21
- 基线：`main@769c5807cc1543d0149179d7954469db80872b6b`
- 候选分支：`codex/t21-model-detector-sidecar`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t21-model-detector-sidecar`
- 状态：`READY_FOR_ROOT_REVIEW`（sidecar 补强候选，提交本文件后）

## 变更范围

- 后端将模型检测器状态显式区分为 `ready`、`unconfigured`、`unavailable`，并通过现有 admin API/projection 暴露 `detector_state`。
- 检测服务未配置或不可用时，模型选项和排队入口使用稳定的服务状态语义，不再把全部模型误报为“检测器暂不支持”。
- 前端原生账号模型检测对话框和账号卡片显示“检测服务未接入”或“检测服务暂不可用”；只有 `ready` catalog 中确实未收录的模型显示“检测器暂不支持”。原生连接测试模型保持可用。
- `infra/compose.yaml` 通过现有共享环境锚点向 blue、green、worker 透传 `SUB2API_MODEL_DETECTOR_URL` 与 `SUB2API_MODEL_DETECTOR_TOKEN`，不暴露 detector 端口。
- `upstream/sub2api/backend/cmd/model-detector` 是独立标准库 HTTP 二进制，按 T15 合同提供 `/v1/catalog` 与 `/v1/detect`；它以同一不可变 Sub 镜像的独立 Compose 服务运行，不嵌入 Sub API 进程，也未复制 `tools/gpt56_api_detector-git`。
- 生产近 7 天模型事实 `gpt-5.6-terra`、`gpt-5.6-sol`、`gpt-5.4` 已纳入默认 catalog；`MODEL_DETECTOR_MODELS` 可由宿主覆盖。
- `ops/deploy-sub2api-blue-green-host.sh` 对旧版已渲染 JSON Compose 在全部只读预检通过后原子补齐 detector 服务、私网 URL/token、健康启动与候选镜像；失败会恢复旧 Compose 与 secret env。
- 新增 Compose 合同测试 `tests/operations/model_detector_compose_contract_test.sh`。

不包含数据库迁移、历史回填、生产业务数据写入、计费/盈利/调度/评分变化、原生连接测试变化或 GitHub Actions。未复制 `tools/gpt56_api_detector-git` 的核心、基线或报告。

## 验证

- 后端 service/repository/routes 直接相关测试通过。
- 后端受影响包 compile-only 通过，`go build ./cmd/server` 通过；sidecar `go test ./cmd/model-detector -count=1`、`go build ./cmd/model-detector` 通过。
- 前端 `AccountMonitorCard.spec.ts` 与 `AccountMonitorView.spec.ts`：`96/96` 通过。
- `pnpm exec vue-tsc --noEmit` 通过。
- `pnpm build` 通过。
- `bash tests/operations/model_detector_compose_contract_test.sh` 通过。
- `bash tests/operations/release_sub2api_blue_green_test.sh` 通过。
- `bash tests/operations/deploy_sub2api_blue_green_host_test.sh` 通过（包含候选、回滚、停机和旧拓扑 fail-closed 门禁）。
- 变更 Go 文件 `gofmt` clean，`git diff --check` 通过。

## 发布与验收门禁

- 预期 `downtime_required=false`，根总控仍须在合并后的 `main` 运行既有发布预检，以其结果为准。
- 当前生产宿主旧 Compose 尚未声明 detector 服务；候选发布时宿主执行器会在只读预检后生成共享 token、原子升级 Compose 并以候选镜像启动私网 detector。
- 线上必须确认：`model-detector` healthy；登录态 admin API 返回 `detector_state=ready` 且 catalog 至少含 `gpt-5.6-terra`/`gpt-5.6-sol`/`gpt-5.4` 中一个；选择该模型执行一次检测并进入 `normal`、`abnormal` 或 `insufficient` 终态；API Key/base URL/token 不出现在日志或响应。
- 若真实上游模型目录不可达，允许记录 `insufficient/upstream_unavailable` 作为真实终态，但不得把 ready catalog 误报为 `detector_unsupported`。

## 回滚

- 回滚到上一已验证 Sub 镜像/`main` 提交，保留 T15 数据与迁移不变。
- 失败时执行器自动恢复旧 Compose/secret env 并回滚 API/worker；手工回滚到上一已验证 Sub 镜像时同时移除 `model-detector` 服务和 `SUB2API_MODEL_DETECTOR_*` 环境，应用回到 `unconfigured` 语义。

## 根总控后续动作

1. 审查候选并合并到根 `main`。
2. 在合并后的 `main` 运行直接相关门禁、推送和既有蓝绿发布链。
3. 完成 detector 健康检查、`ready` catalog 读取及至少一次真实检测终态验收，并确认凭据不出现在日志或响应中。
