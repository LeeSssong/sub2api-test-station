# 独立测试站接管手册

## 运行身份

- 服务器 SSH alias：`sub2api-test-station`，当前地址 `49.51.203.200`，用户 `ubuntu`。
- 部署目录：`/opt/sub2api-test-station/releases/e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b/`。
- Compose project：`sub2api-test-station`。
- 应用镜像：主站活动镜像 `ghcr.io/leesssong/xingqiao-sub2api:release-e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b-1010e4b54f92f43a473e5f81e0a05b892733e4ae334207a54a609946ab794fd1`，image ID `sha256:1010e4b54f92f43a473e5f81e0a05b892733e4ae334207a54a609946ab794fd1`。
- homepage/Caddy 镜像：`xingqiao-caddy:homepage-20260902-v17-fingerprinted-lightbox`，image ID `sha256:314761379d810462bbeef03c28e88edd7da5775f3f958756a87f601f70c3eedb`。
- 主站 source commit/tree：`e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b` / `ee671c292487d5e005beb696e02593e766146248`。

## 凭据读取

禁止把密码、Token、私钥或 API key 粘贴到 GitHub、文档或聊天。另一台本机 Codex 只读取以下 0600 文件：

- GitHub CLI 登录：系统 keyring，通过 `gh auth status` 验证，不读取明文 token。
- 测试站 SSH key：`/Users/gongtengxinwen/.ssh/tencent_lighthouse_seoul_sub2api`。
- SSH known_hosts：`/Users/gongtengxinwen/.config/sub2api/known_hosts`。
- 测试站环境变量：服务器 `/opt/sub2api-test-station/.env`；本机验收环境模板 `/Users/gongtengxinwen/.config/sub2api/acceptance-20260827.env` 不得用于覆盖本测试站环境。
- 接管索引：`/Users/gongtengxinwen/.config/sub2api/test-station-credentials-index.md`，权限 0600；该文件只列路径、变量名、用途和验证命令，不保存秘密值。

## 日常核对

```bash
ssh -T sub2api-test-station 'sudo -n docker compose --project-name sub2api-test-station --env-file /opt/sub2api-test-station/releases/e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b/.env -f /opt/sub2api-test-station/releases/e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b/infra/independent-test-station/compose.yaml ps'
curl --fail http://49.51.203.200/
curl --fail http://49.51.203.200/health
curl --fail http://49.51.203.200/readyz
```

只停止独立 project：

```bash
ssh -T sub2api-test-station 'sudo -n docker compose --project-name sub2api-test-station --env-file /opt/sub2api-test-station/releases/e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b/.env -f /opt/sub2api-test-station/releases/e00c37e0e5ac076aaddc043fccf82af6bc5a1d1b/infra/independent-test-station/compose.yaml down'
```

不要使用不带 `--project-name` 的 `docker compose down`，不要删除 `sub2api-test-station-*` volumes。

## 发布与回滚

发布前先从根目录干净 `main` 生成并校验主站活动制品；本手册中的 release 目录只作为当前已验证制品记录。测试站配置变更必须先在候选 worktree 验证，再合入并推送根 `main`。切换失败时保留新 release 目录，使用 `/opt/sub2api-test-station/backups/20260902T183440Z/` 中的 Compose、PostgreSQL dump、Redis/app-data tar 和 `SHA256SUMS` 恢复旧栈。

## 已知限制

当前宿主 Docker IPv6 端口层拒绝 `[::]:80`，因此 Caddy 仅绑定 `0.0.0.0:80`；IPv4 `49.51.203.200` 已验证。`/readyz` 返回 HTTP 200，但当前镜像/配置返回应用 HTML，若需要 JSON readiness，应先在候选中复现并单独修正路由后再部署。

## 另一终端 Codex 启动提示

```text
你接管的是独立测试站，不是主站 /admin/lab。先完整阅读仓库 AGENTS.md、docs/project/native-sub-incremental-delivery-constraints.md、docs/project/acceptance-station-global-constraints.md，以及 docs/operations/independent-test-station-handoff.md。再读取 /Users/gongtengxinwen/.config/sub2api/test-station-credentials-index.md（只读路径和权限，不打印秘密）。使用 SSH alias sub2api-test-station 和 GitHub CLI keyring 登录态。先做只读版本、容器、网络、卷、健康和主站不变性核对；任何部署只作用于 Compose project sub2api-test-station，禁止触碰主站 project、主站数据库、主站 Redis、生产 secrets 或全局 docker compose down。
```
