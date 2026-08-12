# T01 大上下文入站上传稳定性规格

## Goal

修复原生 Sub2API 公共 Caddy 对慢速、大上下文请求的固定 300 秒代理窗口误伤，同时保留现有应用侧请求体大小与安全边界。

## Scope

- 在 `infra/Caddyfile` 为客户端请求体读取建立显式、有限的长窗口策略。
- 将公共 Sub2API upstream 的固定 `response_header_timeout 300s` 调整为与长请求策略一致的受控窗口。
- 不改变 `infra/compose.yaml` 与 Sub2API 的现有全局、网关和文本请求体大小限制。
- 增加 Caddy 配置合同测试，验证长窗口、无意外短超时和现有大小保护兼容。
- 增加受控慢速/不完整上传验证：持续上传超过 300 秒仍可完成；完全中断或超过受控窗口时连接可释放并留下可诊断结果。

## Non-goals

不修改错误中文转译、上游重试、账号调度、CDN、账务、生产配置或发布脚本；不新增平行控制面。

## Design

公共 Caddy 全局 `servers.timeouts.read_body` 设为 `15m`，这是覆盖慢速上传、避免请求体尚未读完就被终止的直接策略，同时仍为未完成连接设置有限资源释放边界。Sub2API fallback upstream 的 `response_header_timeout` 设为 `15m`，移除固定 `300s` 窗口；它只在 Caddy 已把完整请求（包括请求体）写给 upstream 后开始计时，用于允许长推理等待响应头，不作为慢上传修复的因果证据。

请求体大小保护继续由当前 Compose 环境和 Sub2API 原生 middleware 承担：全局 128 MiB、网关 128 MiB、纯文本 16 MiB。Caddy 不新增更小的 `request_body` 限制，避免大上下文在边缘被提前截断。

## Acceptance

1. 配置适配/校验成功，公共 fallback 不再包含 `response_header_timeout 300s`，且 read-body 与 response-header 均为 15 分钟。
2. 合同测试确认三项应用侧 body limit 未变化。
3. 受控慢上传持续超过 300 秒后收到 upstream 成功响应；不再因 Caddy 300 秒窗口返回 502。
4. 配置合同/Caddy adapt 证明生产 `read_body` 为有限的 `15m`；从同一生产 Caddyfile 派生的短覆盖配置仅把该值改为 `2s`，快速证明不完整上传到期后连接会释放且不会到达 upstream。健康检查与普通请求不回归。

## Deployment / rollback

这是 Caddy 配置级更新，不涉及数据库迁移。蓝绿发布链可先验证并 reload Caddy；预期 `downtime_required=false`，但根线程必须以发布预检输出为准。回滚为恢复上一版 `infra/Caddyfile`/镜像并执行同一 Caddy validate + reload。
