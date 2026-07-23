# GPT-Pro / GPT-Plus 生产分组切换验证（2026-07-20）

## 结论

生产分组已经完成双池拆分：

- `GPT-Pro`：公开、站内倍率 `1.0x`，绑定 `neko-production-primary`，账号成本倍率 `0.10x`，并发 `3`。
- `GPT-Plus`：公开、站内倍率 `1.0x`，绑定 `wawazz-production-primary`，账号成本倍率 `0.05x`，并发 `1`。
- 两组沿用同一套六个已定价 GPT 文本模型，未开放图片、音频或内部探测模型。
- 临时 `wawazz-test` 分组已删除，Aliu 仍暂停且不接收流量。

## Wawazz 证据

生产 URL allowlist 追加 `wawazz.xyz` 后，仅重建 Sub2API，PostgreSQL、Redis、Caddy 未重建。隔离账号管理连接测试成功；隔离同步与 SSE 均返回 HTTP 200，SSE 收到 `[DONE]`。

两条隔离用量记录各显示用户扣费约 `$0.002413`、上游实际费用约 `$0.000121`，实际成本约为标准价的 `0.05x`；首字延时约 `1.10s`。Wawazz 用量页同时出现对应请求，模型与分组均为 `gpt-5.4` / `gpt-plus`。

正式切换后，`GPT-Plus` 同步与 SSE 均返回 HTTP 200，SSE 正常结束。

## Neko 证据与阻塞

Neko 账号和 Pro 分组配置已保存，但管理端连接测试、正式 GPT-Pro 同步和 SSE 均返回：

```text
INSUFFICIENT_BALANCE
```

该结果指向 Neko 上游余额/额度不足，不是本地分组、倍率或 URL allowlist 错误。在余额恢复前，GPT-Pro 不能标记为“验收通过”，也不能对外承诺 Pro 可用性。

## 清理与回滚边界

- 本轮创建的三个用户 API Key、隔离用户和临时分组均已删除。
- 未记录或持久化任何 Key 原文。
- 生产回滚只需将 Neko 账号成本倍率恢复为原值并重新绑定原分组；不删除 PostgreSQL/Redis 卷，不修改 Aliu 暂停状态。

## 下一步

用户恢复或确认 Neko 上游余额后，只重新执行 GPT-Pro 一次同步、一次 SSE，并核对用户扣费与上游实际成本。通过后，再把 Neko/Wawazz 的公开价格、性能和用量页面录入 `relay-ops`，保持 `read_only`；飞书和 Agent 仍不自动修改路由、价格、余额或 Key。
