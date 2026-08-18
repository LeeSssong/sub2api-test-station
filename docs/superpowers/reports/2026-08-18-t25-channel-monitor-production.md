# T25 自建渠道监控生产发布验收

- 发布源：`main@20c563345fe802b9662faf9189ca8cc7ecb3d3aa`
- source tree：`e62afb7f22a9519ec985416a4994fb2fa216d4da`
- 0600 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-18-main-20c563345-t25-channel-monitor-v1.json`
- 宿主记录：`/var/lib/sub2api/release-records/20260818T181312Z-production-3451615.json`
- 发布结果：`succeeded/promoted`，`rolled_back=false`，`downtime_required=false`，活动槽 `blue`
- 镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-20c563345fe802b9662faf9189ca8cc7ecb3d3aa-fbffebf4a5ab24815ce15dfceeff77a9ce61dfe1678c4ddc90021b640c76aa71`
- 迁移哈希：`18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`，与生产基线一致，无迁移变化。

## 线上验收

- 公网 `https://api.xingqiaolab.top/healthz`、`/readyz`、`/health` 均返回 HTTP 200。
- 原生管理员设置已保存：`channel_monitor_enabled=true`、`channel_monitor_mode=v1`。
- 登录态 `/monitor` 已渲染自建 Monitor V2，而非官方 V2 页面；页面为中文，保留旧版卡片结构、TTFT P50、输出 TPS、总延迟 P50、缓存命中率、样本数、基础倍率和青绿色趋势柱体。
- 页面 DOM 核对：3 张分组卡片；存在 `TTFT P50`；存在“有效调用”“基于 N 次真实请求。”和“基础倍率”；不存在 `P95`、`TTFT P95` 或 `总延迟 P95`。

## 验证矩阵

- Monitor V2 focused tests：4 个测试文件、27/27 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `go test -tags=unit ./internal/service -run 'ChannelMonitor|RunCheckForModel|MonitorRunner' -count=1`：通过。
- `go test ./internal/service -run '^$' -count=1`：通过。
- `go build ./cmd/server`：通过。
- T25 后端相关文件 `gofmt -d`：无输出；`git diff --check 0aac82dbc..HEAD`：通过。

首次本地调用因复用旧维护环境变量而在生产变更前被脚本门禁拦截；清除该旧变量后同一源重新执行，未发生生产变更，最终发布成功。
