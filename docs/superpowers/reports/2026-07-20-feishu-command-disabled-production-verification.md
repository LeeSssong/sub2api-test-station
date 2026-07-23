# Verification: 飞书确定性命令生产 disabled 部署

## Scope

在不修改 Sub2API 路由的前提下，部署包含五条固定群命令的 relay-ops，安装最小飞书回调配置，并在真实群聊验证 `disabled` 模式的回调、解析、回复和审计边界。

## Commands / Steps

- 在生产主机的受限构建目录构建 AMD64 镜像；构建上下文不含 `.env`、Key、Cookie、密码或浏览器状态。
- 从生产当前 `compose.yaml` 和 `Caddyfile` 制作最小补丁，使用真实 `.env` 仅做静默 Compose 解析，不输出变量值。
- 使用固定 Caddy 镜像验证暂存配置，备份现有配置后原子替换。
- 只重建 `relay-ops`；Caddy 使用管理 API 热加载新配置，没有重建基础容器。
- 检查容器模式、空文件变量、只读挂载、健康状态、公开页面、活跃 Caddy 路由和基础容器 ID。
- 配置 `im.message.receive_v1` 事件订阅，通过 challenge，并在 `飞书新用户体验群` 发送带结构化 bot mention 的精确命令。
- 真实事件暴露 `mentioned_type="bot"`；解析器只在恰好一个 `app`/`bot` mention 且位于文本前缀时剥离，并保留多 mention 拒绝边界。

## Results

- PASS：镜像 `sub2api-relay-ops:feishu-command-bot-mention-20260720-v1` 为 `linux/amd64`，image ID `sha256:5ea34afe054842c8ff32a34358f3315e2c7cc08e822839a372efe36c4852a9f2`。
- PASS：relay-ops 健康，重启计数 `0`，运行模式仍为 `read_only`，命令模式为 `disabled`。
- PASS：五个飞书文件均为只读挂载；命令处理器和 worker 在 `disabled` 模式下运行，只产生拒绝结果，不创建路由操作。
- PASS：内部 `/healthz`、`/readyz` 正常；公开 `/pricing`、`/ops` 均返回 HTTP 200。
- PASS：Caddy 活跃配置包含唯一精确 `POST /relay-ops/api/feishu/events` 反向代理；无凭据禁用态的 POST、GET 和相邻 POST 均不可用。
- PASS：Sub2API、PostgreSQL、Redis 和 Caddy 容器 ID 与部署前一致；没有重建四个基础服务。
- PASS：生产 Compose 与 Caddy 备份分别保存在 `/opt/sub2api/production/compose.yaml.bak-feishu-disabled-20260720-v1` 和 `/opt/sub2api/production/Caddyfile.bak-feishu-disabled-20260720-v1`。
- PASS：challenge 通过；真实群两次 `查询当前分组状态` 均被精确解析，回复包含 `命令功能未启用`、`rejected` 和短审计号。
- PASS：两条 PostgreSQL 记录均为 `command_text=查询当前分组状态`、`status=rejected`、`error_code=command_disabled`、`reply_attempts=1`、`reply_delivered=true`。
- PASS：原生 Admin API 复读确认公开分组、四个路由账号的状态、可调度性、并发、倍率、运行阻断和模型目录未变；没有 Sub2API 路由写入。

## Evidence

- 生产构建上下文已重新验证：Go 1.24.13 Bookworm/AMD64 下 24 个包 `go test ./... -race -count=1` 全部通过；固定 Go 1.24.13 Alpine 构建镜像下 `go vet ./...` 通过。固定 Alpine 镜像设置 `CGO_ENABLED=0` 且不含 GCC，因此只用于静态构建和 vet，race 验证必须使用同版本 Bookworm 工具链。
- 本地隔离 PostgreSQL store 测试、镜像构建、Compose/Caddy 和部署契约此前均已通过。
- bot mention 修复后重跑 Go 1.24.13 Bookworm/AMD64 `go test ./... -race -count=1`、Alpine `go vet ./...` 和 `tests/relay_ops/validate_relay_ops_contract.sh`，均通过。
- 原生 Admin API 复读确认：`GPT-Pro(2)` / `GPT-Plus(6)` 仍为 active + `1.0x`；Neko `7` 并发 `2` 且可调度；Wawazz `8` 并发 `1` 且可调度；Aliu `2` 和 Neko 复制账号 `9` 均不可调度且覆盖六个必需模型。
- 生产 Compose 暂存文件 SHA-256：`433d07db8c5c42800c780807b9204a90939bc13f47fe0d2508f4d92d451f17c1`。
- 生产 Caddy 暂存文件 SHA-256：`2c98dfe53a3c17d9ac1a6890704c46f2973b47f28629b13fb5fcf8e67228da86`。

## Not Verified

- 未进入 `dry_run`，未验证真实主/灾备账号 ID 和预计切换状态。
- 未进入 `enabled`，未执行任何 Sub2API 路由写入。

## Follow-ups

1. 收到当次精确生产变更授权后，只把 `RELAY_OPS_FEISHU_COMMAND_MODE` 改为 `dry_run`，只重建 relay-ops。
2. 在真实群聊逐条验证五条命令，并用规范化前/后快照证明 Sub2API 零写入。
3. `enabled` 必须等待用户查看 dry-run 证据后另行明确批准。
