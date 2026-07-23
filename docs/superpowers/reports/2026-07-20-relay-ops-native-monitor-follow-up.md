# relay-ops 原生监控复核（2026-07-20）

## 结论

`relay-ops` 的生产数据链路已经复用 Sub2API 原生能力：每 5 分钟同步公开分组、Ops/Usage 窗口和 Channel Monitor 引用；候选上游仍由独立调度器每 6 小时运行。代码不复制生产 Key、不直接写 Sub2API PostgreSQL，也不会在 `read_only` 模式发送候选 API 请求。

线上原生渠道监控已完成 Neko Pro 的首条有效样本；Wawazz 的历史成功结论被本次现网复核覆盖：原有 Key 已失效，替换 Key 可鉴权但上游余额不足。relay-ops 已读取两个公开分组和原生监控引用，未成功的上游不会被伪造成健康。

## 当前证据

- 生产公开分组：`GPT-Pro`（Neko，账号成本 `0.10x`）和 `GPT-Plus`（Wawazz，账号成本 `0.05x`），站内均 `1.0x`。
- Wawazz 正式 Plus 同步/SSE 历史验收仍保留；当前原生监控以新 Key 检测返回 `INSUFFICIENT_BALANCE`，状态为错误。旧 `relay-monitor` Key 曾返回 `INVALID_API_KEY`，已删除。
- Sub2API 原生 `/admin/channels/monitor` 已建立 `GPT-Pro / Neko` 和 `GPT-Plus / Wawazz` 两项监控，均为 `3600` 秒间隔、`120` 秒抖动、启用状态打开。
- Neko 手动检测：`gpt-5.6-sol` 正常，7 天可用率 `100%`，延迟 `1396 ms`。该结果是当前样本，不等同于长期 SLA。
- Wawazz 替换 Key 手动检测：上游返回 `INSUFFICIENT_BALANCE`，HTTP 往返约 `233 ms`；原生用户 `/monitor` 总体显示 `DEGRADED`，等待上游补余额后自动复测。
- GPT-Pro 生产复验：同步和 SSE 均 HTTP 200，均 `10` 输入 / `5` 输出 Token；SSE 收到 `[DONE]`。Sub2API 两条记录各扣 `$0.000200`，Neko 对应各实际 `$0.000020`，标准 `$0.000200`，实测成本倍率 `0.10x`；首字约 `2.84s` / `5.15s`。
- `relay-ops` `/healthz`、`/readyz` 正常，模式为 `read_only`；5 分钟生产同步后 `/ops` 已显示 `GPT-Plus`、`GPT-Pro` 两个公开分组，不会因无监控条目伪造样本。
- 候选 `probe_runs=0`，未执行任何真实候选同步或 SSE。

## 验证

- `ruby ops/upstream-benchmark.rb validate`：`valid: 3 channels, profile mvp-text-v1, 20 runs, 8 decisions`。
- `bash tests/infra/validate-baseline.sh`：PASS。
- `docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet`：PASS。
- Go 容器回归：`go test ./...` PASS；`go vet ./...` PASS。

## 下一步门禁

1. Wawazz 上游补余额后复核一次 Plus 监控和正式请求；在此之前不重试、不切换路由、不自动充值。
2. Neko Pro 监控继续按 3600 秒低频运行，出现状态变化或延迟趋势再进入事件状态机。
3. 任何候选上游真实 probe 仍需先报告每候选最多 2 个请求、费用上限 `$0.002`，获得明确批准后再切换 `RELAY_OPS_MODE=probe`。

本轮未读取、输出或持久化任何 Key；未修改生产余额、路由、价格或数据库。

## 现网复核补充（2026-07-20）

本次操作使用真实生产域名 `https://api.43-133-75-82.sslip.io`（Sub2API `v0.1.161`）。本机 `localhost:8080` 是旧的 `v0.1.153` 开发实例，仅产生了一个隔离测试用户，已删除，不纳入生产证据。

- 生产下游隔离用户和 `$0.01` GPT-Pro Key 仅存活于本次复验，完成两条请求后已删除。
- Neko 监控 Key 为独立低额 Key，仅通过浏览器内存转移；Wawazz 旧失效 Key 已删除，替换 Key 保留用于低频复测。
- relay-ops `/ops` 已显示 `GPT-Pro` 与 `GPT-Plus`，候选 `probe_runs` 仍为 `0`，服务保持 `read_only`。
