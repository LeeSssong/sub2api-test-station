# 独立测试站接管手册

## 运行身份

- 服务器 SSH alias：`sub2api-test-station`，当前地址 `49.51.203.200`，用户 `ubuntu`。
- 部署根目录：`/opt/sub2api-test-station/`；活动 release 必须从 `release-state.json` 与 `test-station-api` 容器的 Compose `config_files` 标签实时核对，不在手册中固定 SHA。
- Compose project：`sub2api-test-station`。
- 最近一次只读核对到的应用 tag：`sub2api-test-station-runtime:645ce06834698cebcd6707836a234d7096e0a081`。该值只用于交接追溯，不能作为活动指针；实际运行身份必须从容器标签与 `release-state.json` 实时核对。
- homepage/Caddy 镜像：`xingqiao-caddy:homepage-20260902-v17-fingerprinted-lightbox`，image ID `sha256:314761379d810462bbeef03c28e88edd7da5775f3f958756a87f601f70c3eedb`。
- 最近一次只读核对到的 source commit/tree：`645ce06834698cebcd6707836a234d7096e0a081` / `d16f0f3f13d2f837587b73455d22aa3ac9eef540`；同样必须实时核对。

## 凭据读取

禁止把密码、Token、私钥或 API key 粘贴到 GitHub、文档或聊天。另一台本机 Codex 只读取以下 0600 文件：

- GitHub CLI 登录：系统 keyring，通过 `gh auth status` 验证，不读取明文 token。
- 测试站 SSH key：`/Users/awen/.ssh/tencent_lighthouse_seoul_sub2api`。
- SSH known_hosts：`/Users/awen/.config/sub2api/known_hosts`。
- 测试站环境变量：服务器 `/opt/sub2api-test-station/.env`；本机验收环境模板 `/Users/awen/.config/sub2api/acceptance-20260827.env` 不得用于覆盖本测试站环境。
- 接管索引：`/Users/awen/.config/sub2api/test-station-credentials-index.md`，权限 0600；该文件只列路径、变量名、用途和验证命令，不保存秘密值。

## 日常核对

活动 Compose 以 API 容器标签为准，不能从历史 release 路径猜测：

```bash
ssh -T sub2api-test-station 'sudo -n bash -s' <<'REMOTE'
set -euo pipefail
config=$(docker ps \
  --filter label=com.docker.compose.project=sub2api-test-station \
  --filter label=com.docker.compose.service=test-station-api \
  --format '{{.Label "com.docker.compose.project.config_files"}}' | head -n 1)
release=${config%/compose.yaml}
docker compose --project-name sub2api-test-station \
  --env-file "$release/.env" -f "$config" ps
REMOTE
curl --fail http://49.51.203.200/
curl --fail http://49.51.203.200/health
```

`/readyz` 必须同时检查状态码、JSON Content-Type 和 `status`，不能只使用 `curl --fail`。当前运行版本仍返回 SPA HTML，因此它是已知发布阻断项。

发布与回退脚本禁止执行 `docker compose down`、`down -v` 或删除 `sub2api-test-station-*` volumes。人工停服属于单独运维事件，不是普通发布步骤。

## 发布与回滚

正式发布入口仍为根目录干净 `main` 上的 `ops/release-sub2api-test-station.sh`。候选补强版增加以下门禁，但在合入并推送根 `main` 前不得使用：

1. 本地校验 `main == origin/main`，构建 Linux/amd64 镜像，记录 tar 归档 SHA-256、Docker image ID 和迁移集合 SHA-256。
2. 宿主从活动 API Compose 标签与 `release-state.json` 双重解析上一 release；两者不一致即停止。
3. 候选启动前调用 `ops/backup-sub2api-test-station-host.sh`，在 `/opt/sub2api-test-station/backups/<UTC timestamp>/` 生成 `postgres.dump`、`redis-dump.rdb`、`app-data.tar.gz`、`SHA256SUMS` 和 `metadata.json`。备份失败时不启动候选。
4. 候选加载后必须验证 tag 的 Docker image ID；然后对同一 project 执行 `up -d --remove-orphans`，保留 PostgreSQL、Redis 和 app-data named volumes。
5. API、worker、detector、PostgreSQL、Redis、Caddy 达到健康状态后，连续三次验证 `/health` 为 JSON `status=ok`、`/readyz` 为 JSON `status=ready`。
6. 候选启动后的任一失败会使用上一 release 的 Compose/Caddy/`.env` 对同一 project 自动执行应用回退，并重新验证服务与探针；不恢复数据库、Redis 或 app-data 备份。

成功状态使用 `release-state.json` schema v2，记录 source commit/tree、迁移集合 SHA、镜像归档 SHA、image ID/tag、当前/上一 release、备份目录、project、结果和时间，不写秘密。失败时保持上一成功状态不变，在候选 release 内写入权限 0600 的 `failure.json`，只记录受控失败阶段和回退结果。

常规应用回退保留当前 named volumes，以避免撤销发布后的充值、订单、兑换、密钥和资料写入。备份只用于另行批准的数据灾难恢复，不由发布控制器自动回放。

本补强候选目前位于 `codex/refactor-uiux-release-controller`，尚未合并、推送或部署。当前运行 `/readyz` 返回 HTML，在后端 readiness 修复合入前，补强控制器会按设计 fail closed，不能用于真实发布。

## 已知限制

当前宿主 Docker IPv6 端口层拒绝 `[::]:80`，因此 Caddy 仅绑定 `0.0.0.0:80`；IPv4 `49.51.203.200` 已验证。当前 `/readyz` 返回 HTTP 200 SPA HTML，不符合发布控制器的 JSON readiness 合同；必须先完成后端 `/readyz` 修复及嵌入前端绕过，再考虑合并后的测试站发布。

## 另一终端 Codex 启动提示

```text
你接管的是独立测试站，不是主站 /admin/lab。先完整阅读仓库 AGENTS.md、docs/project/native-sub-incremental-delivery-constraints.md、docs/project/acceptance-station-global-constraints.md，以及 docs/operations/independent-test-station-handoff.md。再读取 /Users/awen/.config/sub2api/test-station-credentials-index.md（只读路径和权限，不打印秘密）。使用 SSH alias sub2api-test-station 和 GitHub CLI keyring 登录态。先做只读版本、容器、网络、卷、健康和主站不变性核对；任何部署只作用于 Compose project sub2api-test-station，禁止触碰主站 project、主站数据库、主站 Redis、生产 secrets 或全局 docker compose down。
```
