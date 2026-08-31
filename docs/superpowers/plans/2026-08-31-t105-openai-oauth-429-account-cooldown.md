# T105 OpenAI OAuth 429 账号级原生限流恢复实施计划

1. 为 OpenAI OAuth 429 增加账号级持久化辅助方法，封装 reset 解析、官方 5 秒 fallback、`SetRateLimitedIfLater` 和 runtime blocker 同步。
2. 在公共 OpenAI failover 转换点接入该方法，覆盖 Responses、Messages、Chat Completions、WS、Images、Alpha Search、Embeddings 等路径；不改非目标分支。
3. 增加 service/handler 直接测试，验证 retry window、耗尽转换、reset 优先级、手动恢复和非 Responses 覆盖。
4. 运行定向 Go 测试、格式与 diff 检查，记录 handoff 为 `READY_FOR_ROOT_REVIEW`，等待根总控授权合并和发布。
5. 在分组选号耗尽时增加一次有界恢复轮次：刷新账号投影，清除 T105 短 cooldown 与请求级排除，保留原生 7 天 quota 和长期账号状态；仅清理本次明确观测到的 T105 fallback 账号。
