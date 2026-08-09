# Sub2API unified production deployment verification

验证日期：2026-08-09（Asia/Shanghai）

## 发布范围

- 部署候选：`main@abb87a0a8ba4d57cfcf8e38065c5459825062346`
- 明确排除：`优化调度` worktree `/Users/gongtengxinwen/Documents/sub2api-upstream-resilience-spec`
- 传输：预加载 Linux AMD64 镜像；未使用 GitHub Actions
- 目标：`ubuntu@43.133.75.82:2222`（`VM-0-17-ubuntu`）
- 迁移门禁：生产 `5cc825b23a35f64ecb2b2def9ae73170c7c512015f112d0773ba232e5ab85703` → 候选 `9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0`

## 发布结果

- 宿主结果：`result=succeeded`
- 活动槽位：`green`，上游 `sub2api-green:8080`
- `downtime_required=false`
- 镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-abb87a0a8ba4d57cfcf8e38065c5459825062346-181f767d76af3d280339b851d557ef95f3ddda0f57580700e4a5d18e3dd52520`
- 镜像 ID：`sha256:181f767d76af3d280339b851d557ef95f3ddda0f57580700e4a5d18e3dd52520`
- 镜像标签已证明 source commit/tree、tested tree、migration hash 与候选一致，且 `qualified=true`
- PostgreSQL、Redis、Caddy 容器 ID 保持不变；API/worker 均 healthy、重启次数为 0

## 线上验收

- `GET /healthz`：HTTP 200，`{"status":"alive"}`
- `GET /readyz`：HTTP 200，`{"status":"ready"}`
- 带生产 Gateway Key 的 `GET /v1/models`：HTTP 200
- 带管理员 Key 的 `GET /api/v1/admin/accounts/monitor`：HTTP 200；返回 78 个账号，`evidence_source` 为 `monitor_probe=69`、`stale=9`，响应包含 `score_breakdown`、`homepage_url` 和 `multiplier.source`
- 生产数据库确认 `usage_logs.account_cost` 列存在，`201_usage_log_account_cost.sql` 已登记

## 生图固定成本配置

部署后先执行默认只读检查，再执行显式 `--apply`，最后再次只读复核。生产配置结果为：

- 唯一渠道：`生图固定上游成本`
- 唯一分组绑定：`生图`
- 唯一规则：`生图分组图片固定成本`
- `openai` / `gpt-image-*` / `billing_mode=image`
- 1K `$0.06`、2K `$0.08`、4K `$0.10`
- `apply_pricing_to_account_stats=false`
- 客户模型定价行数：0

历史 `usage_logs.account_cost IS NULL` 流水未回补；未制造付费图片请求，首条自然 1K/2K/4K 流水的最终成本仍需后续自然流量观察。

## 结论

本轮候选已完成推送、生产部署和线上验收；共享数据服务未重建，管理员仍使用原域名、账号、2FA、URL 和数据查看路径。`优化调度` 未合并、未部署；其余本轮已纳入候选的运行时代码已生效。
