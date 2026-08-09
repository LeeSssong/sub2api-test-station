# GPT 分组配置基线：生产只读证据清单

## 采集结果

- UTC 采集时间：`2026-08-09T10:15:42.482Z`（JSON `captured_at`）。
- 范围：生产 PostgreSQL 只读聚合，以及生产蓝槽 Sub2API 管理员监控接口的脱敏投影；未执行上游或网关 API 探测。
- 生产边界检查：`sub2api-prod` SSH、Linux、`sudo -n` 和 7 个运行容器检查均通过；未读取 `.env` 或容器环境内容。
- 数据库 schema 检查：仅读取 `information_schema.columns` 中 `accounts`、`groups`、`account_groups`、`usage_logs`、`ops_error_logs`、`account_monitor_results`、`account_monitor_group_score_weights`、`settings` 的字段名/类型。

## 精确计数

| 集合 | 数量 | 口径 |
|---|---:|---|
| 非删除分组 | 7 | IDs `2, 6, 15, 17, 19, 20, 21` |
| 非删除账号 | 78 | OpenAI 76、Anthropic 2；类型 `apikey` 73、`oauth` 5 |
| 真实请求 7d 账号 | 46 | `usage_logs` 与非 token-counting `ops_error_logs` 聚合 |
| 真实请求 7d 分组 | 6 | 同上，含 `group_id` 的聚合行 |
| 真实请求 30d 账号 | 66 | 同上 |
| 真实请求 30d 分组 | 9 | 同上 |
| 主动探测 7d 账号 | 102 | `account_monitor_results` 窗口聚合，含历史/已删除账号 ID |
| 主动探测 30d 账号 | 102 | 同上 |
| 最新探测行 | 112 | 每个账号 ID 最新一行，含历史/已删除账号 ID |
| 调度设置键 | 14 | 仅 `openai_advanced_scheduler_*` 允许列表 |

管理员监控投影在 `7d`、`30d` 均返回 78 个账号和 7 个分组，作为数据库账号/分组总数的独立只读交叉证据。采集时 API 健康摘要分别为：7d `monitoring=65, available=46, unavailable=19, paused=13`；30d `monitoring=65, available=46, unavailable=19, paused=13`。

## 运行命令（脱敏）

1. `ssh -o BatchMode=yes -o ConnectTimeout=15 sub2api-prod 'set -eu; test "$(uname -s)" = Linux; sudo -n true; sudo docker ps --format "{{.Names}} {{.Status}}" | sed -n "1,20p"'`
2. `sudo docker exec -i sub2api-postgres-1 ... psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"`，SQL 仅为目标表 `information_schema.columns` 查询。
3. 通过同一容器执行 `SELECT`/CTE 聚合：分组/账号配置、7d/30d 真实请求、7d/30d 探测、最新探测和允许列表调度设置；错误关联按 `account_id/request_id`、`is_count_tokens=false`、`status_code>=400`。
4. 生产主机内将受保护 key 读入短生命周期 shell 变量（值未打印、未保存、未写入报告），调用蓝槽 `GET /api/v1/admin/accounts/monitor?range=7d|30d`，立即用 `jq` 仅保留分组计数、健康摘要和账号状态计数；随后 `unset`。
5. `jq -e` 字段/类型校验、`sha256sum` 和敏感词扫描；完整响应体未落盘。

## 产物与哈希

- JSON：`.superpowers/sdd/2026-08-09-gpt-group-configuration-baseline-analysis/production-evidence.json`
- SHA-256：`766fb926165614744480695b8080b1d7b281ec5107b7f995cfb470a9944ddfc3`
- 本清单：`docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-evidence.md`

## 验证结果

- `jq -e 'has("captured_at") ... has("scheduler")'`：通过。
- 所有账号聚合行的 `account_id` 类型为 JSON number：通过。
- 结构计数与上述精确计数一致：通过。
- 敏感词扫描未发现授权 header、密钥值、Cookie、密码、TOTP、prompt、request/response body；唯一命中为账号类型字面值 `apikey`（非凭据值），标记为手工复核项。

## 关注事项

- `account_monitor_results` 的 102 个窗口账号和 112 个最新账号行包含历史/已删除账号；后续 Task 2 必须与当前 78 个非删除账号按数字 ID 连接，不能把历史行当作现役池。
- `settings` 中 14 个高级调度键均为显式 `false` 或空字符串；运行时默认权重由应用配置推导，不能仅凭空覆盖值断言最终运行时权重。后续分析应保留该不确定性并引用代码默认值作为补充依据。
- 7d/30d 监控 API 投影是只读交叉证据，不替代数据库聚合；API 与数据库采集时间存在秒级漂移。
- 未执行任何生产写入、服务重启、容器重建、迁移、上游调用或调度变更。
