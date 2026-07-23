# 飞书确定性分组切换运维手册

## 1. 边界

本功能只接受以下五条群聊文本命令，必须逐字一致：

```text
切换 GPT-Pro 到灾备
切换 GPT-Plus 到灾备
恢复 GPT-Pro 主分组
恢复 GPT-Plus 主分组
查询当前分组状态
```

命令不会调用 shell，不接受动态参数，不修改用户、Key、分组名称、价格或模型范围。切换只调整配置中四个账号对象的 `group_ids`，并在目标账号不可调度时将其 `schedulable` 设为 `true`。

生产启用顺序固定为：

```text
disabled -> dry_run -> enabled
```

没有真实群聊 dry-run 证据和用户当次明确批准，不得设置为 `enabled`。

## 2. 飞书应用配置

在飞书开发者后台创建企业自建应用，启用机器人，并配置最小权限：

- `im:message:send_as_bot`
- 开发者后台为“接收群聊中提及机器人消息”显示的最小接收权限；以后台当前权限标识为准，不额外申请通讯录、私聊或文件权限

订阅事件：

```text
im.message.receive_v1
```

回调 URL：

```text
https://<正式域名>/relay-ops/api/feishu/events
```

只允许该 URL 的 `POST` 绕过本站管理员会话。`GET`、相邻路径和其他 `/relay-ops/api/*` 继续走管理员边界。

在开发者后台生成 verification token 和 Encrypt Key，发布应用版本，并把机器人加入需要执行命令的群。私聊、bot、app、system 和非文本消息都会被忽略。

## 3. 安装秘密文件

所有值从密码管理器直接安装到服务器，不粘贴到聊天、不写入 Git、不进入 shell 历史。以下命令中的来源路径只是占位符：

```bash
sudo install -d -m 0750 -o root -g root /opt/sub2api/secrets
sudo install -m 0640 -o root -g 10002 /path/from-password-manager/feishu-app-id /opt/sub2api/secrets/feishu-app-id
sudo install -m 0640 -o root -g 10002 /path/from-password-manager/feishu-app-secret /opt/sub2api/secrets/feishu-app-secret
sudo install -m 0640 -o root -g 10002 /path/from-password-manager/feishu-verification-token /opt/sub2api/secrets/feishu-verification-token
sudo install -m 0640 -o root -g 10002 /path/from-password-manager/feishu-encrypt-key /opt/sub2api/secrets/feishu-encrypt-key
sudo install -m 0640 -o root -g 10002 /path/from-password-manager/feishu-routing.json /opt/sub2api/secrets/feishu-routing.json
```

Compose 的宿主机路径变量分别为：

```text
RELAY_OPS_FEISHU_APP_ID_HOST_FILE
RELAY_OPS_FEISHU_APP_SECRET_HOST_FILE
RELAY_OPS_FEISHU_VERIFICATION_TOKEN_HOST_FILE
RELAY_OPS_FEISHU_ENCRYPT_KEY_HOST_FILE
RELAY_OPS_FEISHU_ROUTING_HOST_FILE
```

安装完成后还必须把下列容器内路径写入生产 `.env`；默认留空时即使模式为 `disabled` 也不会加载命令子系统：

```text
RELAY_OPS_FEISHU_APP_ID_FILE=/run/secrets/feishu-app-id
RELAY_OPS_FEISHU_APP_SECRET_FILE=/run/secrets/feishu-app-secret
RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE=/run/secrets/feishu-verification-token
RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE=/run/secrets/feishu-encrypt-key
RELAY_OPS_FEISHU_ROUTING_FILE=/run/secrets/feishu-routing.json
```

没有安装飞书凭据时，五个宿主机路径和五个容器路径都保持为空；Compose 会把占位目标只读绑定到 `/dev/null`，relay-ops 可继续以 `disabled` 启动且不会创建回调 worker。

文件必须是普通文件，权限只能是 `0600` 或 `0640`。应用错误和日志不得包含值、完整 `chat_id`、完整 `open_id`、服务器路径或原始飞书响应。

## 4. 只读发现路由 ID

路由文件固定包含 `GPT-Pro` 和 `GPT-Plus` 两项。先通过 Sub2API `v0.1.161` 管理端或受限只读 Admin API 核对：

- 两个公开分组的 ID、名称、平台和倍率
- 每组主账号与灾备账号 ID
- 所有被引用账号的状态、`schedulable`、现有完整 `group_ids`
- 目标账号具备的必要模型

不得查询或输出账号凭据字段。不得直接修改 Sub2API PostgreSQL。使用 [示例路由文件](../../config/operations/feishu-routing.example.json) 建立真实的 `0600/0640` 文件；真实 ID 不写入仓库。

两个主账号必须唯一，且主账号不能兼任任何灾备账号。同一个灾备账号可以被两个分组复用；此时切换命令会按账号串行执行。`GPT-Pro` 与 `GPT-Plus` 的分组 ID 必须不同，每组主账号和灾备账号必须不同，所需模型列表不能为空。

## 5. disabled 验收

保持：

```text
RELAY_OPS_FEISHU_COMMAND_MODE=disabled
```

2026-07-20 已完成无凭据禁用态部署：生产镜像为 `sub2api-relay-ops:feishu-command-disabled-20260720-v1`，五个容器文件变量为空，五个占位挂载均为只读 `/dev/null`。该状态不会创建回调处理器或命令 worker；Caddy 已热加载精确 POST 路由，Sub2API、PostgreSQL、Redis 和 Caddy 容器均未重建。

先验证配置和服务：

```bash
cd /opt/sub2api/production
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --no-deps --force-recreate relay-ops
docker compose --env-file .env -f compose.yaml ps relay-ops
```

未安装五个真实文件时，外部回调路径应保持不可用，内部 `/healthz` 和 `/readyz` 必须正常。安装完整文件并保持模式为 `disabled` 后，在飞书后台完成 challenge；随后在群聊发送五条命令中的任意一条，预期收到“命令功能未启用”，Sub2API 的账号绑定和 `schedulable` 均无变化。未知命令只返回固定帮助，不执行路由操作。

## 6. dry-run 验收

将模式改为 `dry_run`，仅重建 `relay-ops`：

```bash
docker compose --env-file infra/.env -f infra/compose.yaml up -d --no-deps --force-recreate relay-ops
```

逐一发送五条命令，核对：

- 查询命令返回两个分组的当前主/灾备角色
- 四条切换命令返回预计前后角色
- Sub2API 账号的 `group_ids` 和 `schedulable` 没有写入
- 重复投递同一 `event_id` 不会生成第二次执行
- 私聊、机器人消息、额外参数和相邻文本不会执行
- 审计中有稳定状态、错误码、耗时和脱敏状态快照

只有全部通过并保存证据后，才能请求用户单独批准 `enabled`。

## 7. enabled 验收

获得当次批准后，先只对一个分组执行：灾备切换、状态查询、主分组恢复。每一步核对：

1. 目标账号先加入公开分组并复读确认。
2. 源账号随后移出公开分组并复读确认。
3. 用户 API Key、分组倍率和模型范围不变。
4. 网关同步请求成功。
5. SSE 正常结束并收到 `[DONE]`。
6. 飞书回复与 PostgreSQL 审计一致。

禁止发送 `confirm_mixed_channel_risk`，禁止自动回滚不确定写入，禁止自动禁用源账号全局调度。

## 8. 审计查询

只查询 relay-ops 自有数据库：

```sql
SELECT event_id, command_text, group_name, target_role, status, error_code,
       duration_ms, reply_attempts, reply_delivered, received_at, completed_at
FROM relay_ops.feishu_command_events
ORDER BY received_at DESC
LIMIT 50;
```

需要事故复核时再查询 `before_state` 和 `after_state`。快照只允许公开分组名、角色、绑定/可调度布尔值、分组 ID 和用户倍率；出现其他字段应视为安全缺陷并立即退回 `disabled`。

## 9. partial 与回滚

`partial` 表示至少一个写入结果无法确定。系统不会自动回滚。处理顺序：

1. 立即把命令模式改回 `disabled`。
2. 通过只读 Admin API 重读公开分组、主账号和灾备账号。
3. 确认当前是 `primary`、`backup`、`mixed` 还是 `none`。
4. 若为 `mixed` 或 `none`，在 Sub2API 管理端人工恢复单一目标账号绑定。
5. 用同步请求和 SSE 验证用户路径。
6. 记录人工操作、前后状态和证据，再恢复到 `dry_run`。

应用回滚只需恢复上一版 relay-ops 镜像并保持 `RELAY_OPS_FEISHU_COMMAND_MODE=disabled`。不要删除命令审计表，不要清理 PostgreSQL 记录，不要修改 Sub2API 数据库。
