# T53 管理员隔离测试站候选交接

## 状态

`READY_FOR_ROOT_REVIEW`（仅候选交接；未合并 `main`、未推送、未部署、未线上验收）。

## 基线与候选

- 基线 `main`：`ca92211e9341e5ae2baba49666604bcd049c6754`
- 候选分支：`codex/t53-admin-lab`
- 候选 HEAD：`032fac2ec85012cb4216836a1a9923dc67dfacf0`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t53-admin-lab`

## 交付范围

- 独立 Compose project `sub2api-admin-lab`：API、worker、前端、PostgreSQL、Redis、mock upstream、mock payment、lab gateway。
- `/admin/lab/` 路由挂载；生产 `/admin/`、`/api/`、SSE/WS fallback 保持原生生产 upstream。
- 独立 lab JWT/CSRF secret、Cookie 名称、前端 localStorage namespace、数据库、Redis、卷和网络。
- mock 支付、mock 上游、lab notification outbox 和 fail-closed egress guard。
- `LAB_ONLY` 种子 manifest、幂等 dry-run seed、显式 lab reset guard。
- 未实现额度账本、充值包、经营分析或第二账务事实源；这些另立任务包。

## 变更文件摘要

- `infra/compose.admin-lab.yaml`
- `infra/.env.admin-lab.example`
- `infra/Caddyfile`
- `infra/admin-lab/*`
- `tests/admin_lab/*`
- `tools/admin-lab/*`
- `upstream/sub2api/backend/internal/lab/*`
- 前端认证存储命名空间及 lab 入口相关文件

## 新鲜验证

以下命令在候选 HEAD 上执行：

```text
bash tests/admin_lab/smoke_admin_lab.sh                         PASS
bash tests/admin_lab/*.sh                                       PASS
(cd upstream/sub2api/backend && go test ./internal/lab -count=1) PASS
(cd upstream/sub2api/frontend && pnpm exec vitest run \
  src/admin-lab-entry.spec.ts src/utils/authStorage.spec.ts --run) PASS (2/2)
(cd upstream/sub2api/frontend && pnpm build)                    PASS
(cd upstream/sub2api/backend && go build ./cmd/server)           PASS
docker compose ... config --quiet                                PASS
docker run caddy:2.10.2-alpine caddy adapt ...                  PASS
git diff --check                                                 PASS
```

`smoke_admin_lab.sh` 是静态/进程内 smoke，默认不启动服务、不调用生产地址；设置 `LAB_SMOKE_LIVE=1` 才执行 HTTP live 探针。

## 现场阻断与未验证项

- 本地首次 live Compose 启动曾暴露两个真实问题，已在候选中修复并加入合同测试：PostgreSQL 18 卷必须挂载 `/var/lib/postgresql`；Python mock healthcheck 必须访问 IPv4 loopback `127.0.0.1`。
- 修复后重新尝试 `up -d --build --wait` 时，前端/后端镜像构建长时间无输出，已中断；因此未形成完整 lab API/gateway live smoke 证据。
- 当前环境没有可复用的生产 `sub2api_default` 外部网络；live 验证需由根总控在受控宿主环境使用实际 lab gateway network 执行，不能把测试网络伪装成生产网络。
- 未执行生产 Caddy reload、生产蓝绿发布或线上验收。
- `downtime_required`：`unverified until root preflight`。

## 迁移、配置与发布

- 数据库迁移：无新增迁移；lab 使用官方 Sub2API 迁移集合在独立 PostgreSQL 上初始化。
- 生产配置/数据：候选未写生产数据库、Redis、支付、上游、通知或对象存储。
- 生产配置变化：若根总控批准发布，将增加 `/admin/lab/` Caddy 路由和独立 lab Compose 服务；需根预检确认宿主网络和 reload 方式。
- 发布链：只能从合并并验证后的 `main` 使用既有本地/宿主蓝绿链执行；不得从候选直接部署，不使用 GitHub Actions。

## 回滚

1. 停止并删除 `sub2api-admin-lab` Compose project 及其独立卷。
2. 从生产 Caddy 配置移除 T53 `/admin/lab/` matcher/handler，reload 生产 Caddy。
3. 保持生产数据库迁移和生产服务不变；不做生产数据恢复或逆迁移。
4. 若发布链失败，保留候选 worktree、失败日志和宿主记录，不删除证据。

## 根总控下一步

1. 在候选上复核 diff 和本交接。
2. 在合并后的 `main` 重新运行直接相关验证和发布预检。
3. 仅当预检明确允许且 `downtime_required=false` 时进入既有蓝绿发布；若为 `true`，在任何停服/切换前暂停等待授权。
4. 完成 lab live smoke、生产路由回归和线上专项验收后，才可更新总账为完成并清理候选。
