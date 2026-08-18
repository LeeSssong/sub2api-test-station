# T21 生产模型检测 Sidecar 接入与离线状态纠正交接

## 候选信息

- 任务包：T21
- 基线：`main@74aa0d0126e7097cecb4d6d6df33b767da65a494`
- 候选分支：`codex/t21-model-detector-sidecar`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t21-model-detector-sidecar`
- 状态：`READY_FOR_ROOT_REVIEW`（提交本文件后）

## 变更范围

- 后端将模型检测器状态显式区分为 `ready`、`unconfigured`、`unavailable`，并通过现有 admin API/projection 暴露 `detector_state`。
- 检测服务未配置或不可用时，模型选项和排队入口使用稳定的服务状态语义，不再把全部模型误报为“检测器暂不支持”。
- 前端原生账号模型检测对话框和账号卡片显示“检测服务未接入”或“检测服务暂不可用”；只有 `ready` catalog 中确实未收录的模型显示“检测器暂不支持”。原生连接测试模型保持可用。
- `infra/compose.yaml` 通过现有共享环境锚点向 blue、green、worker 透传 `SUB2API_MODEL_DETECTOR_URL` 与 `SUB2API_MODEL_DETECTOR_TOKEN`，不暴露 detector 端口。
- 新增 Compose 合同测试 `tests/operations/model_detector_compose_contract_test.sh`。

不包含数据库迁移、历史回填、生产业务数据写入、计费/盈利/调度/评分变化、原生连接测试变化或 GitHub Actions。未复制 `tools/gpt56_api_detector-git` 的核心、基线或报告。

## 验证

- 后端 service/repository/routes 直接相关测试通过。
- 后端受影响包 compile-only 通过，`go build ./cmd/server` 通过。
- 前端 `AccountMonitorCard.spec.ts` 与 `AccountMonitorView.spec.ts`：`96/96` 通过。
- `pnpm exec vue-tsc --noEmit` 通过。
- `pnpm build` 通过。
- `bash tests/operations/model_detector_compose_contract_test.sh` 通过。
- 变更 Go 文件 `gofmt` clean，`git diff --check` 通过。

## 发布与验收门禁

- 预期 `downtime_required=false`，根总控仍须在合并后的 `main` 运行既有发布预检，以其结果为准。
- 当前生产宿主未配置 `SUB2API_MODEL_DETECTOR_URL`、`SUB2API_MODEL_DETECTOR_TOKEN`，也未提供 detector 容器或 sidecar 制品。因此本候选上线后可验收：页面显示“检测服务未接入”、不显示“检测器暂不支持”、离线时检测入口禁用且原生连接测试正常。
- “至少一个受支持模型可选择并完成检测”只有在提供符合 T15 合同和许可门禁的 sidecar URL/token、catalog 返回实际模型并通过一次终态检测后才能宣称完成；候选本身不伪造该证据。
- sidecar 制品必须是外部、可替换且满足既有 T15 许可边界的合同实现；不得把现有 PolyForm Noncommercial 工具源码、基线或报告复制进生产镜像。

## 回滚

- 回滚到上一已验证 Sub 镜像/`main` 提交，保留 T15 数据与迁移不变。
- 如仅需撤销接入，删除宿主 detector URL/token 配置并重启既有蓝绿链；应用会回到 `unconfigured` 语义。

## 根总控后续动作

1. 审查候选并合并到根 `main`。
2. 在合并后的 `main` 运行直接相关门禁、推送和既有蓝绿发布链。
3. 完成健康检查及上述离线语义线上验收；具备合规 sidecar 后再补做真实 catalog/检测验收。
