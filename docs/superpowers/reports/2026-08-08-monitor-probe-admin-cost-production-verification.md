# 账号纯探测与管理员成本详情生产验收

**日期：** 2026-08-08
**候选提交：** `2af5cb245d55cd55a0f68cc69ba7de016ae325ee`
**候选树：** `7c4298c4c798c673c0f6eecb672bbcdd9d7432bb`

## 发布结果

- relay-ops 已运行镜像 `ghcr.io/leesssong/xingqiao-relay-ops:release-2af5cb245d55cd55a0f68cc69ba7de016ae325ee`。
- Sub2API 按用户明确要求采用直接停机覆盖更新，blue、green 和 worker 均运行镜像 `xingqiao-sub2api:release-2af5cb245d55cd55a0f68cc69ba7de016ae325ee-40ed21d1e6d472658bb1f4d4a6b5fcd9d6ae3ba63a8ebfd56c2cb5968d865967`。
- 生产 `release-state` 已绑定 source commit、source tree、镜像 ID 和迁移哈希 `aee795202a3dd14c191c5e395add6beb58942950bf530d9961ae80a359998429`。
- 回滚快照保存在 `/var/lib/sub2api/release-backups/20260808T131500Z-direct-2af5cb245`；发布记录为 `/var/lib/sub2api/release-records/20260808T131500Z-direct-2af5cb245.json`。

## 在线验证

- `/health`、`/healthz`、`/readyz` 均返回成功状态。
- PostgreSQL、Redis、Caddy 和 relay-ops 容器身份与发布前一致；只替换 Sub2API blue、green 和 worker。
- `schema_migrations` 已记录 `199_add_usage_log_upstream_request_id.sql`，`usage_logs.upstream_request_id` 列存在。
- 账号监控返回 schema version 4、85 个账号、8 个分组，且所有账号具备纯探测核心字段。
- 零探测样本账号均未标记 normal、未评分、未排名；eligible 账号均为 normal 且有成功探测样本。
- 延迟字段只出现在存在成功探测样本的账号上。
- 管理员 usage 详情同时返回本站 `request_id` 与可空的 `upstream_request_id`；现有历史样本尚无上游请求 ID，符合迁移后的历史 NULL 语义。
- 生产前端懒加载资源包含“探测成功率”“探测样本”“上游请求 ID”“成本依据”“待对账”“已确认”文案。

## 真实数据边界

生产当前没有逐笔成本证据，因此账号投影不返回 cost guard 证据，流水详情按设计保持待对账。relay-ops 成本接口使用独立管理员会话鉴权，Sub2API 管理员机器密钥请求返回 401；未降低该鉴权边界，也未伪造确认成本或毛利。
