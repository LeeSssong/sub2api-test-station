# T53 管理员隔离测试站原生复用基线

- 任务：T53 管理员隔离测试站
- 基线：`main@ca92211e9341e5ae2baba49666604bcd049c6754`
- 工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t53-admin-lab`
- 分支：`codex/t53-admin-lab`
- 状态：`IMPLEMENTING`

## 原生复用结论

| 能力 | 复用方式 | 证据 |
|---|---|---|
| 管理员路由 | 直接复用原生 `RegisterAdminRoutes`、admin middleware 和 handler/DTO | `upstream/sub2api/backend/internal/server/router.go:127-130`、`.../server/routes/admin.go` |
| 登录/Token | 直接复用原生 `/api/v1/auth` 路由、JWT bearer 校验和登录响应 | `.../server/routes/auth.go`、`.../handler/auth_handler.go` |
| 前端 API | 复用原生 Axios client、API response unwrap、Authorization 注入和请求标识 | `upstream/sub2api/frontend/src/api/client.ts` |
| 前端登录 | 复用原生 auth API/store 行为；lab 只增加 base path 和浏览器存储命名空间，不能共用生产 localStorage token | `upstream/sub2api/frontend/src/api/auth.ts`、`.../stores/auth.ts` |
| 数据库 | 复用当前迁移集合，在独立 PostgreSQL 数据库初始化；不导出生产数据 | `infra/compose.yaml`、Sub setup/migration 入口 |
| Redis | 复用原生 Redis client/config，指向独立 lab 实例，不共享生产 DB/index | `infra/compose.yaml`、原生 config |
| 反向代理 | 在现有 Caddy fallback 前增加 `/admin/lab/` matcher；生产 `/api/*` 和 `/admin/*` 规则不改语义 | `infra/Caddyfile` |
| 账务事实源 | 不新增；后续额度账本仍必须单独复用/扩展原生事实源 | 项目全局约束 1.1 |

## 不复用项

- 不复用生产 PostgreSQL、Redis、Cookie/token、JWT/CSRF secret、文件卷、支付实例、上游凭据、通知出口。
- 不复制生产用户、订单、余额、usage logs 或 API Key；测试数据仅由 `LAB_ONLY` 版本化种子生成。
- 不创建第二套账务、经营统计或控制面。

## 验证

```text
bash tests/admin_lab/native_reuse_inventory.sh
native reuse inventory: PASS
```

直接相关基线检查通过；下一步进入独立 Compose 与 fail-closed guard。
