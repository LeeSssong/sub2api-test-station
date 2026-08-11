# T01 大上下文入站上传稳定性实施计划

## Goal / Context / Constraints

- Goal: 让持续上传超过 300 秒的大上下文请求不被 Caddy 固定窗口误杀，并保留原生请求体大小保护。
- Context: `infra/Caddyfile` 当前公共 Sub2API fallback 使用 `response_header_timeout 300s`，应用侧已配置 128 MiB 全局/网关与 16 MiB 文本限制。
- Constraints: 仅改 T01 范围；不碰错误转译、重试、调度、CDN、生产、`main` 或其他任务包；不新增迁移。

## Steps

- [x] 登记总账并写入本规格与计划。
- [x] RED：新增/更新 Caddy 合同测试，使旧 300 秒策略失败，并覆盖现有 body-limit 合同。
- [x] GREEN：在 `infra/Caddyfile` 设置 `servers.timeouts.read_body 15m`，将公共 fallback `response_header_timeout` 调整为 `15m`。
- [x] 增加受控慢速/不完整上传验证工具，默认使用短的合同模式，支持显式 `T01_SLOW_UPLOAD_DURATION_SECONDS=301` 的长验收；不自动触碰生产。
- [x] 运行合同测试、Caddy validate/adapt（Docker）、上传验证、后端 ingress/body-limit 定向测试、构建/`go vet`（必要范围）和 `git diff --check`。
- [x] 自审 diff、确认无迁移/生产变更，更新总账为 `READY_FOR_ROOT_REVIEW`，提交候选并向根线程汇报。

## Done when

合同测试和受控上传验证通过，Caddy 配置可适配，现有 body-limit 与健康路由无回归，候选提交只包含 T01 文件，且已报告基线 SHA、提交 SHA、测试、停机属性、回滚和剩余风险。
