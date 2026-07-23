# Neko 生产定价与网关验证

**日期：** 2026-07-19  
**范围：** D03 生产模型定价、升级兼容性恢复和受控计费冒烟

## 结果

- Sub2API 运行版本为 `v0.1.161`。升级后曾因 `gateway.text_max_body_size` 默认值超过现有网关上限短暂返回 502；固定 `GATEWAY_TEXT_MAX_BODY_SIZE=16777216` 后仅重建 Sub2API，HTTPS `/health` 恢复 200，PostgreSQL、Redis、Caddy 未重建。
- 生产渠道 `GPT` 已创建并关联 GPT 用户组 `0.15x`。
- 渠道使用 `billing_model_source=requested`、`restrict_models=true`、账号统计采用模型标准价，图片生成桥接关闭。
- 已配置六个文本模型：`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini`。未配置图片模型和内部探针模型。

## 受控请求

使用临时低额度、7 天有效 Key，完成六次同步请求和一次 `gpt-5.6-sol` SSE 请求；测试后 Key 与用户均已删除。

| 检查 | 结果 |
|---|---|
| 六个模型同步 | 6/6 HTTP 200 |
| `gpt-5.6-sol` SSE | HTTP 200，收到内容和 `[DONE]` |
| 未知模型 | HTTP 404，未进入定价路由 |
| 每次 usage | 输入 8、输出 5 Token |
| 用户扣费 | 7 条合计约 `$0.000125`，符合标准价乘 `0.15x` |
| Neko 成本显示 | 约 `0.07x` 标准价，符合既有统一验收证据 |

## 未验证

- Neko 24 至 72 小时稳定性、首尔到大陆用户链路、长流错误率、供应商硬 RPM/TPM 上限和商业再分发授权。容量短测已单独记录在 `2026-07-19-neko-capacity-verification.md`。
- 本次冒烟不是耐久性、SLA 或商业授权证明。

## 后续

保持 Neko 为唯一生产上游，Aliu 调度关闭并保留为暂停回滚候选；继续邀请制和支付关闭，采集稳定性、网络和商业条款证据。
