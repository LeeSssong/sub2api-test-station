# relay-ops 不可变发布路径

该路径只发布 `relay-ops`，不会停止、重建或重新加载 PostgreSQL、Redis、Caddy、`sub2api-blue`、`sub2api-green` 或 `sub2api-worker`。

## 发布前

在干净、真实物理路径的 Git worktree 中运行测试，并用 `ops/write-relay-ops-test-evidence.sh` 生成 0600 evidence：

```sh
bash ops/write-relay-ops-test-evidence.sh \
  --output /var/lib/sub2api/release-evidence/relay-ops.json \
  --command 'bash tests/operations/release_relay_ops_test.sh' \
  --command 'bash tests/operations/deploy_relay_ops_host_test.sh'
```

设置显式 `RELAY_OPS_IMAGE_REPOSITORY`、`RELEASE_SSH_TARGET`、`RELEASE_SSH_KEY`、`RELEASE_SSH_KNOWN_HOSTS` 和 `RELEASE_SSH_PORT`。SSH key、known-hosts 和 evidence 必须是 canonical、非符号链接且 0600。

仅在获准的生产窗口运行 controller；本地 controller 构建并 push `linux/amd64` 镜像，解析不可变 digest，再通过严格 host-key SSH 调用 host executor：

```sh
bash ops/release-relay-ops.sh --mode production \
  --evidence /var/lib/sub2api/release-evidence/relay-ops.json
```

## 宿主机门禁与回滚

host executor 先做只读门禁：Linux、本机 Docker context、Compose 配置、root-owned 路径、现有服务 container ID、现有 relay-ops immutable digest。门禁通过后才执行 `docker compose pull relay-ops` 和 `docker compose up -d --no-deps --force-recreate relay-ops`。

它随后核对请求 digest、container health、通过未重建的 Caddy 访问 `/healthz` 和 `/readyz`（readiness 只有完成 relay-ops 数据库迁移后才可用），以及 PostgreSQL、Redis、Caddy、两个 API 槽和 worker 的 container ID 未变化。

宿主机在变更前以 root 0600 原子写入最小 release state。任何 post-change 门禁失败都会 pull/up 立即上一 immutable relay-ops digest，并重新验证相同的 shared-service ID 不变量；rollback 不能证明时直接失败并保持人工处理状态。rollback 不删除迁移、数据库记录或秘密。

生产路径要求真实 root、默认 Docker context、无 `DOCKER_HOST`，并且不会接受 rehearsal 模式。
