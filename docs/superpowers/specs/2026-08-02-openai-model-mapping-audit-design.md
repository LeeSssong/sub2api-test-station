# OpenAI 模型映射审计设计

## 目标

仅在 OpenAI-compatible 网关记录客户端请求模型与上游原始响应模型，帮助发现模型映射不一致；不改变路由、调度、计费、客户端响应内容或保存请求/响应正文。

## 方案

在 usage log 增加 nullable `actual_response_model VARCHAR(100)`。OpenAI HTTP JSON、SSE 和 Responses WebSocket 处理链路复用一个轻量模型提取器：JSON 优先读取 `response.model`、其次顶层 `model`；SSE 逐事件解析完成相关事件；WebSocket 逐消息解析 JSON。提取成功后通过 request ID 更新该 usage log，仅写入模型字符串；提取失败或缺失时保持 NULL，不影响转发。

管理后台 usage log DTO 和表格增加实际返回模型列。所有响应转发继续使用原始字节/消息，不重编码、不注入字段。

## 错误处理与隐私

模型字段为空、JSON 无效、非目标事件或更新失败均只记录调试级别/忽略，不阻断请求。提取器不持久化完整请求体、完整响应体、SSE 原文或用户内容。

## 验证

覆盖模型不一致、缺少 model、HTTP/SSE/WebSocket 提取及客户端响应字节不变；迁移和管理员列表回归测试通过。
