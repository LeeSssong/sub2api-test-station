# L1-2 Sub2API 离线部署基线实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Do not use subagents unless the user explicitly authorizes delegation.

**Goal:** 在不购买服务器、域名或上游额度的前提下，生成可在 SRV01 2C2G 假定主机上部署的 Sub2API/Caddy/PostgreSQL/Redis 配置，并完成本地静态和容器级验证。

**Architecture:** 单节点 Docker Compose；只有 Caddy 暴露 80/443，Sub2API、PostgreSQL 和 Redis 仅在 Compose 网络中可见。所有密钥由本地脚本生成到 Git 忽略文件，版本使用 2026-07-15 核验的不可变镜像摘要；生产域名、上游和凭据到位后只替换环境值，不改拓扑。

**Tech Stack:** Docker Compose v2、Sub2API v0.1.155、PostgreSQL 18 Alpine、Redis 8 Alpine、Caddy 2.10.2 Alpine、Bash、OpenSSL。

## Global Constraints

- 遵守 D13：不执行真实付款、充值、购买、收款、实名或商户开通。
- SRV01 只是未购买的假定资产；本地通过不能标记为生产部署完成。
- 不在 Git、普通文档、聊天或 shell history 中保存真实 API Key、密码、Cookie、OAuth Token、2FA 恢复码或支付密钥。
- 不使用 `latest`；镜像锁定到本计划记录的 digest，升级必须重新验证。
- 2 GiB 主机默认配 2 GiB swap；PostgreSQL、Redis 和应用连接池使用低内存参数。
- 80/443 之外不向公网暴露任何容器端口；应用入口由 Caddy 统一承载。
- 上游默认只允许 HTTPS 和显式 allowlist，禁止私网目标。
- 仅信任 Docker 默认私网 `172.16.0.0/12` 的代理头；全局和网关请求体上限均为 16 MiB。
- 开始代码实施前遵守 `using-git-worktrees`：取得隔离工作区同意，不在 `main` 直接实施。

## File Map

| 路径 | 责任 |
|---|---|
| `.gitignore` | 排除 `infra/.env`、本地备份和运行时产物 |
| `infra/compose.yaml` | 四个服务、网络、卷、健康检查、低内存默认值和日志轮转 |
| `infra/.env.example` | 非敏感变量名、假定域名和明确的示例值 |
| `infra/Caddyfile` | HTTPS、SSE 反代、响应头和访问日志 |
| `ops/generate-env.sh` | 从示例生成权限为 600 的本地 `.env` 和随机密钥，不覆盖已有文件 |
| `tests/infra/validate-baseline.sh` | Compose、镜像锁定、端口、安全默认值和密钥泄漏静态检查 |
| `docs/runbooks/offline-deployment-baseline.md` | 本地验证、生产替换项、上线命令和未验证边界 |

## Locked Images

| 服务 | 镜像 |
|---|---|
| Sub2API | `weishaw/sub2api@sha256:5433a314b1dacce7882d0739a6ec24bdec1419a93fba5a34bdecad950137cbb5` |
| PostgreSQL | `postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15` |
| Redis | `redis:8-alpine@sha256:9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005` |
| Caddy | `caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d` |

摘要在 2026-07-15 通过 Docker Registry manifest 响应核验。Sub2API GitHub 最新发布为 v0.1.155；Docker Hub 没有 `v0.1.155` 标签，因此以当日 `latest` 的多架构 digest 固定。实施时必须运行版本检查，若镜像内版本不是 v0.1.155，停止并重新选择摘要。

---

### Task 1: 先建立会失败的基础设施契约测试

**Files:**
- Create: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: 计划中的文件路径和安全约束。
- Produces: `bash tests/infra/validate-baseline.sh`，任一契约不满足时非零退出。

- [x] **Step 1: 创建静态验证脚本**

`tests/infra/validate-baseline.sh` 的初始完整内容：

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "missing $1"
}

require_fixed() {
  local needle=$1
  local file=$2
  rg -Fq -- "$needle" "$file" || fail "missing expected value in $file: $needle"
}

require_file infra/compose.yaml
require_file infra/.env.example
require_file infra/Caddyfile

docker compose \
  --env-file infra/.env.example \
  -f infra/compose.yaml \
  config --quiet || fail 'docker compose config failed'

images=(
  'weishaw/sub2api@sha256:5433a314b1dacce7882d0739a6ec24bdec1419a93fba5a34bdecad950137cbb5'
  'postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
  'redis:8-alpine@sha256:9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005'
  'caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d'
)

for image in "${images[@]}"; do
  require_fixed "image: $image" infra/compose.yaml
done

if rg -n ':latest([[:space:]]|$)' infra/compose.yaml; then
  fail 'mutable latest tag is forbidden'
fi

ports_owner=$(awk '
  /^  [a-zA-Z0-9_-]+:/ { service=$1 }
  /^    ports:/ { print service }
' infra/compose.yaml)
[[ "$ports_owner" == 'caddy:' ]] || fail 'only caddy may publish host ports'
require_fixed 'ports: ["80:80", "443:443"]' infra/compose.yaml

for setting in \
  'DATABASE_MAX_OPEN_CONNS=20' \
  'DATABASE_MAX_IDLE_CONNS=5' \
  'POSTGRES_MAX_CONNECTIONS=60' \
  'POSTGRES_SHARED_BUFFERS=128MB' \
  'REDIS_MAXCLIENTS=1000' \
  'REDIS_POOL_SIZE=64' \
  'REDIS_MIN_IDLE_CONNS=5' \
  'SERVER_TRUSTED_PROXIES=172.16.0.0/12' \
  'SERVER_MAX_REQUEST_BODY_SIZE=16777216' \
  'GATEWAY_MAX_BODY_SIZE=16777216'; do
  require_fixed "$setting" infra/.env.example
done

for setting in \
  'SECURITY_URL_ALLOWLIST_ENABLED: "true"' \
  'SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP: "false"' \
  'SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS: "false"' \
  'SERVER_TRUSTED_PROXIES: ${SERVER_TRUSTED_PROXIES:-172.16.0.0/12}' \
  'SERVER_MAX_REQUEST_BODY_SIZE: ${SERVER_MAX_REQUEST_BODY_SIZE:-16777216}' \
  'GATEWAY_MAX_BODY_SIZE: ${GATEWAY_MAX_BODY_SIZE:-16777216}'; do
  require_fixed "$setting" infra/compose.yaml
done

require_fixed 'reverse_proxy sub2api:8080' infra/Caddyfile
require_fixed 'flush_interval -1' infra/Caddyfile

git check-ignore -q infra/.env || fail 'infra/.env is not ignored'
if git ls-files --error-unmatch infra/.env >/dev/null 2>&1; then
  fail 'infra/.env must never be tracked'
fi

if rg -n -i \
  '(sk-[a-z0-9]{16,}|whsec_[a-z0-9]{16,}|BEGIN [A-Z ]*PRIVATE KEY|Bearer[[:space:]]+eyJ|Cookie:[[:space:]]*[^[:space:]])' \
  infra; then
  fail 'possible secret found in controlled infrastructure files'
fi

printf 'PASS: infrastructure baseline contracts\n'
```

- [x] **Step 2: 运行测试并确认正确失败**

Run: `bash tests/infra/validate-baseline.sh`  
Expected: `FAIL: missing infra/compose.yaml`，证明测试在生产配置不存在时会失败。

---

### Task 2: 实现低内存 Compose、环境模板和 Caddy 入口

**Files:**
- Create: `.gitignore`
- Create: `infra/compose.yaml`
- Create: `infra/.env.example`
- Create: `infra/Caddyfile`
- Test: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: Task 1 的契约测试和 Locked Images 表。
- Produces: 可由 `docker compose` 解析的四服务拓扑。

- [x] **Step 1: 写入 Git 排除规则**

`.gitignore` 至少包含：

```gitignore
infra/.env
backups/
*.local
```

- [x] **Step 2: 写入非敏感环境模板**

`infra/.env.example` 使用以下确定值；`example-only-*` 只用于 Compose 解析和本地隔离验证，禁止部署：

```dotenv
SITE_ADDRESS=api.example.com
TZ=Asia/Shanghai
SERVER_TRUSTED_PROXIES=172.16.0.0/12
SERVER_MAX_REQUEST_BODY_SIZE=16777216
GATEWAY_MAX_BODY_SIZE=16777216
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=example-only-postgres-password
POSTGRES_DB=sub2api
POSTGRES_MAX_CONNECTIONS=60
POSTGRES_SHARED_BUFFERS=128MB
DATABASE_MAX_OPEN_CONNS=20
DATABASE_MAX_IDLE_CONNS=5
REDIS_PASSWORD=example-only-redis-password
REDIS_MAXCLIENTS=1000
REDIS_POOL_SIZE=64
REDIS_MIN_IDLE_CONNS=5
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=example-only-admin-password
JWT_SECRET=example-only-jwt-secret
TOTP_ENCRYPTION_KEY=example-only-totp-key
SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS=api.example-upstream.com
```

- [x] **Step 3: 写入 Compose 拓扑**

`infra/compose.yaml` 必须满足以下精确配置：

```yaml
name: sub2api

services:
  sub2api:
    image: weishaw/sub2api@sha256:5433a314b1dacce7882d0739a6ec24bdec1419a93fba5a34bdecad950137cbb5
    restart: unless-stopped
    expose: ["8080"]
    environment:
      AUTO_SETUP: "true"
      SERVER_HOST: 0.0.0.0
      SERVER_PORT: "8080"
      SERVER_MODE: release
      RUN_MODE: standard
      SERVER_TRUSTED_PROXIES: ${SERVER_TRUSTED_PROXIES:-172.16.0.0/12}
      SERVER_MAX_REQUEST_BODY_SIZE: ${SERVER_MAX_REQUEST_BODY_SIZE:-16777216}
      GATEWAY_MAX_BODY_SIZE: ${GATEWAY_MAX_BODY_SIZE:-16777216}
      DATABASE_HOST: postgres
      DATABASE_PORT: "5432"
      DATABASE_USER: ${POSTGRES_USER}
      DATABASE_PASSWORD: ${POSTGRES_PASSWORD:?required}
      DATABASE_DBNAME: ${POSTGRES_DB}
      DATABASE_SSLMODE: disable
      DATABASE_MAX_OPEN_CONNS: ${DATABASE_MAX_OPEN_CONNS:-20}
      DATABASE_MAX_IDLE_CONNS: ${DATABASE_MAX_IDLE_CONNS:-5}
      REDIS_HOST: redis
      REDIS_PORT: "6379"
      REDIS_PASSWORD: ${REDIS_PASSWORD:?required}
      REDIS_DB: "0"
      REDIS_POOL_SIZE: ${REDIS_POOL_SIZE:-64}
      REDIS_MIN_IDLE_CONNS: ${REDIS_MIN_IDLE_CONNS:-5}
      ADMIN_EMAIL: ${ADMIN_EMAIL}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD:?required}
      JWT_SECRET: ${JWT_SECRET:?required}
      TOTP_ENCRYPTION_KEY: ${TOTP_ENCRYPTION_KEY:?required}
      TZ: ${TZ:-Asia/Shanghai}
      SECURITY_URL_ALLOWLIST_ENABLED: "true"
      SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP: "false"
      SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS: "false"
      SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS: ${SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS:?required}
    volumes:
      - sub2api_data:/app/data
    depends_on:
      postgres: {condition: service_healthy}
      redis: {condition: service_healthy}
    healthcheck:
      test: ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 45s
    logging: &logging
      driver: json-file
      options: {max-size: "20m", max-file: "5"}

  postgres:
    image: postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15
    restart: unless-stopped
    command: ["postgres", "-c", "max_connections=${POSTGRES_MAX_CONNECTIONS:-60}", "-c", "shared_buffers=${POSTGRES_SHARED_BUFFERS:-128MB}", "-c", "effective_cache_size=512MB", "-c", "maintenance_work_mem=32MB"]
    environment:
      PGDATA: /var/lib/postgresql/data
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?required}
      POSTGRES_DB: ${POSTGRES_DB}
      TZ: ${TZ:-Asia/Shanghai}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
    logging: *logging

  redis:
    image: redis:8-alpine@sha256:9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005
    restart: unless-stopped
    command: ["sh", "-c", "exec redis-server --save 60 1 --appendonly yes --appendfsync everysec --maxclients ${REDIS_MAXCLIENTS:-1000} --requirepass \"$$REDIS_PASSWORD\""]
    environment:
      REDIS_PASSWORD: ${REDIS_PASSWORD:?required}
      REDISCLI_AUTH: ${REDIS_PASSWORD:?required}
      TZ: ${TZ:-Asia/Shanghai}
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
    logging: *logging

  caddy:
    image: caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d
    restart: unless-stopped
    environment:
      SITE_ADDRESS: ${SITE_ADDRESS:?required}
    ports: ["80:80", "443:443"]
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      sub2api: {condition: service_healthy}
    logging: *logging

volumes:
  sub2api_data:
  postgres_data:
  redis_data:
  caddy_data:
  caddy_config:
```

- [x] **Step 4: 写入 Caddy 配置**

`infra/Caddyfile`：

```caddyfile
{$SITE_ADDRESS} {
	encode zstd gzip

	reverse_proxy sub2api:8080 {
		flush_interval -1
		transport http {
			dial_timeout 10s
			response_header_timeout 300s
			keepalive 30s
		}
	}

	header {
		-Server
		X-Content-Type-Options nosniff
		Referrer-Policy no-referrer
	}

	log {
		output stdout
		format json
	}
}
```

- [x] **Step 5: 运行契约测试**

Run: `bash tests/infra/validate-baseline.sh`  
Expected: 所有静态检查和 `docker compose config --quiet` 通过。

---

### Task 3: 生成本地密钥文件且拒绝覆盖

**Files:**
- Create: `ops/generate-env.sh`
- Modify: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: `infra/.env.example`。
- Produces: 权限为 `0600` 的 `infra/.env`，每个密钥独立随机生成。

- [x] **Step 1: 先扩展失败测试**

在 `tests/infra/validate-baseline.sh` 的最终 `PASS` 输出之前增加：

```bash
require_file ops/generate-env.sh

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT
GENERATED_ENV="$TEMP_DIR/.env"

ops/generate-env.sh "$GENERATED_ENV" >/dev/null
[[ -f "$GENERATED_ENV" ]] || fail 'environment generator did not create target'

get_value() {
  local key=$1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$GENERATED_ENV"
}

secret_keys=(
  POSTGRES_PASSWORD
  REDIS_PASSWORD
  ADMIN_PASSWORD
  JWT_SECRET
  TOTP_ENCRYPTION_KEY
)

seen_values='|'
for key in "${secret_keys[@]}"; do
  value=$(get_value "$key")
  [[ "$value" =~ ^[0-9a-f]{64}$ ]] || fail "$key is not 64 lowercase hex characters"
  case "$seen_values" in
    *"|$value|"*) fail 'generated secrets must be distinct' ;;
  esac
  seen_values="${seen_values}${value}|"
done

if stat -f '%Lp' "$GENERATED_ENV" >/dev/null 2>&1; then
  mode=$(stat -f '%Lp' "$GENERATED_ENV")
else
  mode=$(stat -c '%a' "$GENERATED_ENV")
fi
[[ "$mode" == '600' ]] || fail "generated environment mode is $mode, expected 600"

if ops/generate-env.sh "$GENERATED_ENV" >/dev/null 2>&1; then
  fail 'environment generator overwrote an existing target'
fi
```

- [x] **Step 2: 运行扩展测试并确认因脚本不存在而失败**

Run: `bash tests/infra/validate-baseline.sh`  
Expected: `FAIL: missing ops/generate-env.sh`。

- [x] **Step 3: 实现生成脚本**

`ops/generate-env.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMPLATE="$ROOT/infra/.env.example"
TARGET=${1:-"$ROOT/infra/.env"}

[[ -f "$TEMPLATE" ]] || {
  printf 'missing template: %s\n' "$TEMPLATE" >&2
  exit 1
}

[[ ! -e "$TARGET" ]] || {
  printf 'refusing to overwrite: %s\n' "$TARGET" >&2
  exit 1
}

mkdir -p "$(dirname "$TARGET")"
umask 077
TEMP_FILE=$(mktemp "${TARGET}.tmp.XXXXXX")
trap 'rm -f "$TEMP_FILE"' EXIT

POSTGRES_PASSWORD=$(openssl rand -hex 32)
REDIS_PASSWORD=$(openssl rand -hex 32)
ADMIN_PASSWORD=$(openssl rand -hex 32)
JWT_SECRET=$(openssl rand -hex 32)
TOTP_ENCRYPTION_KEY=$(openssl rand -hex 32)

while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    POSTGRES_PASSWORD=*) printf 'POSTGRES_PASSWORD=%s\n' "$POSTGRES_PASSWORD" ;;
    REDIS_PASSWORD=*) printf 'REDIS_PASSWORD=%s\n' "$REDIS_PASSWORD" ;;
    ADMIN_PASSWORD=*) printf 'ADMIN_PASSWORD=%s\n' "$ADMIN_PASSWORD" ;;
    JWT_SECRET=*) printf 'JWT_SECRET=%s\n' "$JWT_SECRET" ;;
    TOTP_ENCRYPTION_KEY=*) printf 'TOTP_ENCRYPTION_KEY=%s\n' "$TOTP_ENCRYPTION_KEY" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$TEMPLATE" > "$TEMP_FILE"

chmod 600 "$TEMP_FILE"
mv "$TEMP_FILE" "$TARGET"
trap - EXIT
printf 'created %s\n' "$TARGET"
```

Run: `chmod +x ops/generate-env.sh tests/infra/validate-baseline.sh`  
Expected: both scripts are executable.

- [x] **Step 4: 运行测试并确认通过**

Run: `bash tests/infra/validate-baseline.sh`  
Expected: 生成、权限、随机性和拒绝覆盖测试全部通过。

---

### Task 4: 编写离线和真实环境边界清晰的运行手册

**Files:**
- Create: `docs/runbooks/offline-deployment-baseline.md`

**Interfaces:**
- Consumes: Compose、环境模板和 D13。
- Produces: 从本地验证到真实服务器复验的唯一操作顺序。

- [x] **Step 1: 写入本地验证流程**

手册按以下顺序给出命令和预期结果：生成临时 `.env`、把 `SITE_ADDRESS` 保持为 `api.example.com`、运行静态测试、拉取镜像、验证 Sub2API 版本、执行 `docker compose config`、启动容器、检查四个服务状态、停止并删除测试容器但保留/按需删除本地测试卷。

- [x] **Step 2: 写入真实部署前替换清单**

必须替换并复核：真实域名、管理员邮箱、上游 allowlist、五个随机密钥、服务器公网 IP、DNS A 记录、Cloudflare DNS only、80/443 防火墙、SSH 来源、2 GiB swap。手册明确禁止使用 `.env.example` 部署。

- [x] **Step 3: 写入未验证边界**

明确列出：未购买服务器、未修改 DNS、未申请证书、未录入上游 Key、未发送真实请求、未验证计费、未配置支付、未购买订阅账号。任何本地结果不能勾选这些项目。

---

### Task 5: 本地容器验证、审查和状态回写

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/superpowers/plans/2026-07-15-commercial-ai-api-relay-implementation-plan.md`
- Create: `docs/superpowers/reports/2026-07-15-l1-2-offline-baseline-verification.md`

**Interfaces:**
- Consumes: Tasks 1-4 的产物。
- Produces: 可复现验证证据和下一工作指针。

- [x] **Step 1: 运行完整静态验证**

Run: `bash tests/infra/validate-baseline.sh`  
Expected: 0 failures。

- [x] **Step 2: 验证镜像和 Compose**

Run: `docker compose --env-file infra/.env -f infra/compose.yaml pull`  
Expected: 四个 digest 对应镜像拉取成功。

Run: `docker compose --env-file infra/.env -f infra/compose.yaml config --quiet`  
Expected: exit 0。

Run: `docker run --rm weishaw/sub2api@sha256:5433a314b1dacce7882d0739a6ec24bdec1419a93fba5a34bdecad950137cbb5 --version`  
Expected: 输出包含 `0.1.155`；若不包含，停止实施并重新锁定镜像。

- [x] **Step 3: 执行隔离本地启动验证**

把测试域名临时改为 `http://localhost` 且端口映射只限本机的 override 文件放在临时目录，不提交；启动后确认 Sub2API、PostgreSQL、Redis 为 healthy，Caddy 可访问 `/health`。不得录入真实上游或支付凭据。

- [x] **Step 4: 清理本地测试资源**

Run: `docker compose --env-file infra/.env -f infra/compose.yaml down`  
Expected: 测试容器和网络删除；除非验证恢复流程，否则不使用 `-v` 删除数据卷。

- [x] **Step 5: 写验证报告并更新状态**

报告必须记录命令、退出码、镜像摘要、服务健康结果、未验证项和失败信息。主计划只将 L4-2.1.1 至 L4-2.2.4 中实际通过的离线部分标记完成；生产部署、HTTPS、上游和计费保持未完成。`current-state.md` 的下一指针更新为 L1-3 上游非敏感参数盘点与模拟渠道配置。

## Acceptance Criteria

- [x] 所有受控配置不含真实秘密，`infra/.env` 被 Git 忽略且权限为 600。
- [x] 四个镜像均用 digest 锁定，Sub2API 运行时版本确认为 0.1.155。
- [x] Compose 只有 Caddy 暴露 80/443，数据库、Redis 和应用无宿主端口。
- [x] 低内存连接参数、安全 URL 默认值和 SSE 反代配置通过自动检查。
- [x] 本地四服务启动健康，验证报告可复现。
- [x] 文档清楚区分“本地已验证”和“真实资产未购买/生产未验证”。
- [x] 未发生任何付款、购买、充值、收款、实名或商户开通。

## Risks and Rollback

- Docker Hub digest 可能与 GitHub release 发布节奏不一致；必须以运行时版本输出为准，不一致就停止。
- 2 GiB 参数是内测基线，不是容量承诺；真实主机出现已定义 OOM/内存阈值后升级到 4 GiB。
- Caddy 首次申请证书依赖真实 DNS、80/443 可达和 ACME；本地验证不覆盖该路径。
- 回滚只需停止 Compose、恢复上一份锁定配置和数据卷备份；未验证新版本前不删除旧卷。
