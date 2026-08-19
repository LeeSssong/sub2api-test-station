# T28 采购保存链路与评分顺序生产验收

## 发布身份

- Source commit: `5be1681c58ae9e66001193e400eac25d47fb24f4`
- Source/tested tree: `40e33c4cc043a271b0d85e1ac7964769967c74f4`
- Migrations hash: `18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`
- Evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-5be1681c5-t28.json` (`0600`)
- Host record: `/var/lib/sub2api/release-records/20260819T095405Z-production-4169960.json`

## 发布结果

既有本地/宿主蓝绿控制器返回 `downtime_required=false`、`result=succeeded`，活动槽为 `green`，活动上游为 `sub2api-green:8080`，`rolled_back=false`。release-state 中 source commit、source tree、tested tree 与 evidence 一致；迁移哈希保持不变。

活动 API、worker 与 model-detector 均使用镜像 ID `sha256:359c1018f9bc4cf841d5659c68c5d34728526c8a5965a2642e52fd6454e11ad0`，状态为 healthy，重启计数为 0。PostgreSQL、Redis 与 Caddy 保持运行，重启计数为 0。

公网验证：

- `/healthz`: HTTP 200，`{"status":"alive"}`
- `/readyz`: HTTP 200，`{"status":"ready"}`
- `/health`: HTTP 200，`{"status":"ok"}`

## 功能闭合

根合并后的同一 tested tree 已通过：

- 采购 handler/service 专项 Go 测试；
- Go server build；
- 账号 API、账号卡片与 AccountMonitorView 共 101 个 Vitest；
- 前端 typecheck 与 production build；
- gofmt、`git diff --check`、零迁移与零 GitHub Actions 范围检查。

这些用例覆盖采购-only PUT 在台账事务后直接读取账号、混合 PUT 的既有双事务边界、保存/清空/未知结果重试的会话级幂等键、成功后 reload，以及乱序账号输入按 `group_rank` 强到弱进入最终 DOM。生产验收保持只读，没有为测试幂等或事务行为修改真实采购成本、版本台账或账号配置。

## 回滚

上一活动蓝槽继续保留 T27 不可变镜像 `sha256:38ec11531b345cffeeb4e48da52638f18be6f9070c6c7d59a23bc973cd58b715`。本次没有迁移或数据回滚要求；需要回退时使用既有蓝绿控制器恢复上一活动槽和 release-state。
