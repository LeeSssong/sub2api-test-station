# 可复用 Sub2API 蓝绿发布设计

**日期：** 2026-07-31
**状态：** 设计完成，待实施计划与演练
**适用范围：** `/opt/sub2api/production` 的 Sub2API 应用发布
**不包含：** PostgreSQL、Redis、Caddy 镜像升级、relay-ops 功能升级、账号级配置

## 1. 目标

建立一套以后每次 Sub2API 发布都能复用的蓝绿机制：

- 公网请求切换期间不停止 Caddy；
- PostgreSQL、Redis、Caddy 和 relay-ops 不随 Sub2API 发布重建；
- 候选版本先在生产网络内独立启动和验收；
- 切流失败可在数秒内恢复旧 upstream；
- 切流稳定后将候选版本晋升到标准 `sub2api` 服务；
- 最终删除候选冗余容器，但保留旧镜像和发布记录以便回滚；
- 禁止再次执行整栈 `docker compose up -d --force-recreate`。

本设计优化的是用户可见影响，而不是单纯缩短命令执行时间。任何候选启动、迁移、健康检查或配置错误都应发生在旧版本仍持续服务的阶段。

## 2. 事故约束

2026-07-31 两次中断证明以下模式不可接受：

```text
同时重建 Sub2API、relay-ops、Caddy
  → relay-ops 配置失败
  → Caddy 等待 relay-ops healthy
  → 公网入口无法恢复
```

根因边界：

- 新 relay-ops 对通知策略新增了严格字段要求；
- Native P0/P1 Bridge 尚未完成只读数据库配置和完整服务接线；
- Caddy 被 Compose `depends_on` 纳入 relay-ops 故障域；
- 发布命令一次替换多个有依赖关系的服务。

因此，Sub2API 蓝绿发布不得包含 relay-ops 或 Caddy 镜像更新。relay-ops 必须保持单活，并使用单独的配置兼容门禁和发布流程。

## 3. 方案比较

### 3.1 推荐：临时候选槽 + 标准服务晋升

每次发布创建临时 `sub2api-candidate`：

```text
阶段 A

Caddy ──> sub2api（旧标准服务）
              │
              ├── PostgreSQL
              └── Redis

sub2api-candidate（新版本，尚未接公网）
              ├── PostgreSQL
              └── Redis
```

候选通过内部验收后，Caddy 热 reload：

```text
阶段 B

Caddy ──> sub2api-candidate（新版本）

relay-ops ──> sub2api（旧标准服务）
```

公网稳定后，在 Caddy 仍指向 candidate 时升级标准服务：

```text
阶段 C

Caddy ──> sub2api-candidate（持续服务）

relay-ops ──> sub2api（标准服务原地升级为新版本）
```

标准服务通过验收后，Caddy 热切回 `sub2api`，再删除 candidate：

```text
最终状态

Caddy ──> sub2api（新标准服务）
relay-ops ──> sub2api（新标准服务）

sub2api-candidate 已删除
```

优点：

- 每次发布结束后仍恢复标准服务名，现有 relay-ops 和运维工具无需长期感知槽位；
- Caddy 始终运行，标准服务升级期间公网由 candidate 承接；
- 不需要长期维护两个活动槽和动态 Docker DNS alias；
- 候选删除后资源占用恢复到单实例；
- 下一次发布可以完全复用同一流程。

### 3.2 不采用：永久 blue/green 双槽

长期保留 `sub2api-blue` 和 `sub2api-green` 会让 relay-ops、备份、运维脚本和人工排障都必须理解“当前活动槽”。Docker network alias 切换也容易出现短暂多地址解析和缓存问题。

### 3.3 不采用：复制完整生产 Compose project

第二个 project 会生成独立网络和命名卷；第二个 Caddy 无法同时绑定 80/443，也可能复制 scheduler 和通知发送器。该方案扩大状态面和故障域，不适合当前单机生产环境。

## 4. 固定架构

### 4.1 保持单实例的服务

以下服务在 Sub2API 发布全过程中容器 ID 必须不变：

- PostgreSQL；
- Redis；
- Caddy；
- relay-ops。

发布脚本在任何 mutation 前记录这些容器 ID，切流后和清理后再次比较。任一身份变化均判定发布失败。

### 4.2 候选服务

候选服务固定命名为：

```text
sub2api-candidate
```

它必须：

- 使用与生产相同的 Compose project：`sub2api`；
- 接入现有 `sub2api_default` 网络；
- 使用固定 digest 或本机已检查的不可变镜像；
- 使用同一 PostgreSQL 和 Redis；
- 使用与标准服务相同的全局配置，但不得包含账号级覆盖；
- 不暴露宿主机端口，只允许生产网络内部访问；
- 使用独立健康检查；
- 将日志输出到 stdout，禁止与标准服务同时写
  `/app/data/logs/sub2api.log`。

候选不能直接以完整 `RUN_MODE=standard` 运行。当前 Sub2API 启动会
启动 scheduler、账号/渠道监控、用量和幂等清理、订阅/支付过期处理、
指标聚合、告警和其它后台 runner；旧服务与 candidate 并行会造成重复
清理、刷新、监控、通知或队列处理。

因此蓝绿机制的首个工程前置条件是新增并测试一个明确的 candidate/API
进程模式：

- 仍提供 HTTP、SSE、WebSocket 和只读管理接口；
- 禁止所有后台 scheduler、monitor、cleanup、expiry、aggregation、
  alert、refresh 和通知 runner；
- 不执行数据库 migration，只执行 schema/迁移兼容性预检；
- 不执行 entrypoint 的递归 `chown` 或其它共享数据卷初始化副作用；
- 通过显式环境变量或配置键启用，默认标准服务行为完全不变；
- 启动日志输出进程模式和每个被禁用的 runner，便于发布门禁核对；
- 启动时输出版本化 runner capability manifest。manifest 必须由当前
  wiring 中所有 `Start()`、worker、subscriber 和定时注册点生成，不能
  使用“其它 runner”汇总项；当前清单至少覆盖 email queue、auth-cache
  invalidation、usage-record、deferred/timing-wheel、billing flush、
  scheduled test、backup、scheduler、monitor、cleanup、expiry、
  aggregation、alert、refresh、通知、UserMessageQueue、并发 slot
  cleanup、Ops metrics collector、Ops system log sink、AuditLog、
  ingress-reject aggregator、batch-image cleanup/worker、quota flusher
  和 Prompt Audit。candidate capability check 必须逐项断言为
  `disabled`，缺项、未知项或状态不确定均失败关闭。

在这个进程模式实现并通过单元、集成和双实例竞态测试之前，蓝绿发布
只能停留在演练阶段，不能接生产流量。

### 4.3 `/app/data` 处理

生产 `/app/data` 包含：

- 安装标记和 `config.yaml`；
- 模型价格文件；
- 支持页面和图片；
- 运行日志。

候选应挂载同一数据卷，以保证支持页面、模型价格和配置读取一致。为避免并发写同一日志文件，candidate 强制配置：

```text
LOG_OUTPUT_TO_STDOUT=true
LOG_OUTPUT_TO_FILE=false
```

标准 `sub2api` 服务继续使用现有日志策略。实施测试必须证明 candidate 配置不会修改标准服务的日志输出设置。

共享数据卷不是并发安全性的替代品。候选模式必须禁止启动期
递归 `chown`，并为配置、页面、价格文件和运行日志分别增加读写契约
测试；任何未列入契约的文件写入都使 candidate 启动门禁失败。

候选与旧版本并行期间还必须满足共享状态契约：

- PostgreSQL schema、JSON/枚举、usage/billing 写入由旧版和新版共同可读；
- Redis key 前缀、TTL、序列化格式和锁语义由旧版和新版共同兼容；
- 候选产生的 usage、cache、auth 和队列数据在旧镜像回滚后仍可读取；
- 新版本不得写入旧版本无法解析的字段或 Redis payload；
- 必须有旧/新并发读写和“候选写入后回滚旧镜像”集成测试；
- 测试覆盖真实迁移、Redis key、SSE、WebSocket、usage 和 billing 最小路径。

若无法证明该契约，候选只能运行内部无写探针，不能接公网流量。

### 4.4 数据库迁移

候选启动前必须生成新旧迁移差异报告。只有满足以下条件才允许继续：

- 新迁移为 expand-only；
- 不删除旧字段、旧表、旧索引或旧约束；
- 不修改旧版本无法理解的枚举或不可逆数据；
- 旧标准服务在新 schema 下仍可工作；
- migration runner 使用数据库锁，两个实例不会重复并发执行迁移。

如果不能证明双版本兼容，蓝绿发布必须在切流前终止，不能用“可以回滚镜像”代替数据库兼容证明。

## 5. Caddy 无中断切流

### 5.1 稳定环境占位符

生产 Caddyfile 将所有 Sub2API upstream 目标改为同一个 Caddyfile
环境占位符：

```caddyfile
reverse_proxy {$SUB2API_UPSTREAM:sub2api:8080} {
	flush_interval -1
	# 原有 header_down 和 transport 保持不变
}
```

不存在自定义 Caddy 插件或第二个配置文件。默认值始终是标准服务
`sub2api:8080`，因此 Caddy 重启时安全地回到标准服务。

候选切流时只在运行中的 Caddy 容器执行：

```bash
caddy_id=$(docker compose ps -q caddy)
docker exec -e SUB2API_UPSTREAM=sub2api-candidate:8080 \
  "$caddy_id" caddy reload --adapter caddyfile --config /etc/caddy/Caddyfile
```

标准服务切回时省略环境变量或显式传入
`SUB2API_UPSTREAM=sub2api:8080`。发布记录保存当前运行目标，Caddy
重启后默认回到标准服务，因此不会在不知情时继续把流量送到候选。

所有 Sub2API upstream 均使用此占位符；原有 `flush_interval`、header
和 transport 参数必须保留。发布脚本不得使用全局字符串替换修改
Caddyfile，只允许校验占位符数量和目标列表。

### 5.2 原子切换

切流顺序固定为：

1. 不改写宿主机 Caddyfile；使用临时环境变量生成待适配配置；
2. 使用 Compose 动态解析得到 Caddy service/container ID；
3. 验证所有 Sub2API `reverse_proxy` 路由都使用占位符，目标只能是：
   - `sub2api:8080`
   - `sub2api-candidate:8080`
4. 在运行中的 Caddy 容器执行带 `--adapter caddyfile` 的 `caddy validate`，
   并将同一环境变量传给 `caddy reload`；
5. 执行 `caddy reload`；
6. 查询 Caddy admin API 的 active config，确认所有相关 upstream 已切到目标；
   admin API 不可达、返回无法解析或与目标不一致时，状态机进入
   `MANUAL_CONFIRM_REQUIRED`，禁止继续观察、晋升或清理；
7. 验证公网请求和重启 watchdog；若 Caddy 意外重启，默认回到标准 upstream，
   发布状态必须回到人工确认，而不是继续清理。

`caddy reload` 失败时，Caddy 继续使用已加载的旧配置；因为宿主机
Caddyfile 未改写，不需要文件恢复。不得通过重建 Caddy 容器尝试修复。

## 6. 发布状态机

每次发布创建唯一 release ID，并将状态写入只允许 root 写入的发布记录：

```text
PRECHECKED
CANDIDATE_STARTED
CANDIDATE_VERIFIED
PUBLIC_SWITCHED
OBSERVING
CANONICAL_PROMOTED
PUBLIC_SWITCHED_BACK
CANDIDATE_STOPPED
CANDIDATE_REMOVED
COMPLETED
ROLLED_BACK
FAILED_SAFE
```

状态只能按下列合法转移推进。脚本重入时读取状态和当前真实容器/Caddy
upstream；两者不一致时必须停止并要求人工审计，禁止根据“预期状态”
自动覆盖生产。

```text
PRECHECKED -> CANDIDATE_STARTED -> CANDIDATE_VERIFIED
CANDIDATE_VERIFIED -> PUBLIC_SWITCHED -> OBSERVING
OBSERVING -> CANONICAL_PROMOTED -> PUBLIC_SWITCHED_BACK
PUBLIC_SWITCHED_BACK -> CANDIDATE_STOPPED -> CANDIDATE_REMOVED -> COMPLETED

CANDIDATE_STARTED|CANDIDATE_VERIFIED -> FAILED_SAFE
PUBLIC_SWITCHED|OBSERVING|CANONICAL_PROMOTED -> ROLLED_BACK
```

每个 release 维护 root-only lease 文件，包含 `release_id`、owner、开始
时间、租约过期时间和当前状态。`observe`、`status`、`rollback` 都必须
校验 lease；观察命令退出不会释放租约，续租失败则停止推进。另一个
release 在 lease 未过期前必须拒绝启动。

## 7. 发布流程

### 7.1 发布锁与快照

1. 获取全局 `flock`，禁止多个发布任务并行。
2. 记录时间、Git commit、镜像 digest、生产 Compose/Caddy SHA-256。
3. 记录五个服务的容器 ID、镜像、启动时间、重启次数、健康状态。
4. 记录网络和 volume 身份。
5. 完成数据库和 `/app/data` 备份并校验 SHA-256。
6. 验证公网当前健康；失败则不得开始发布。

### 7.2 候选启动

1. 验证 candidate 不存在；若存在，只允许其 Docker label
   `com.xingqiao.release.id` 和 root-only 状态文件同时匹配当前
   `release_id`、镜像 digest 和提交。
2. 在启动前执行 candidate-mode capability check，确认所有后台 runner
   均为 disabled，migration 为 preflight-only。
3. 使用 overlay 仅创建 `sub2api-candidate`。
4. 禁止 Compose 命令包含 PostgreSQL、Redis、Caddy 或 relay-ops。
5. 将 candidate overlay 与标准服务的 resolved command、environment、mount、
   network、user、security、capability/drop-cap、healthcheck、restart 和
   resource limit 做 parity diff；只有 candidate-mode、日志和 release label
   差异可以存在。
6. 预检 PostgreSQL 连接上限、Redis pool、内存、CPU 和磁盘余量，证明并行
   实例不会越过资源阈值。
7. 等待 candidate 健康，但不触碰公网 upstream。

### 7.3 候选内部验收

至少验证：

- `/health`；
- 版本、commit、镜像 digest；
- 目标 `GATEWAY_OPENAI_*` 全局环境变量；
- PostgreSQL 和 Redis 连通；
- 管理 API 只读请求；
- 模型列表；
- 使用同一专用无计费、无余额变更的探针身份，分别执行 HTTP API、SSE 和
  WebSocket 三类请求；三类请求都必须在探针前后断言 usage、billing、
  余额、账务和账号级配置计数不变；
- WebSocket `store=false` 新连接隔离；
- 监控页自动刷新设置读取；
- 日志没有 fatal、panic、migration checksum mismatch；
- candidate 重启次数为 0。

任何失败均只删除 candidate，不修改 Caddy。

### 7.4 公网切到 candidate

1. Caddy upstream 改为 `sub2api-candidate:8080`；
2. validate；
3. reload；
4. 连续执行公网健康和核心 API smoke；
5. 失败立即切回 `sub2api:8080`。

### 7.5 观察窗口

默认观察窗口为 24 小时。用户可为单次发布明确缩短，但不得低于 15 分钟。

观察期间检查：

- 公网健康连续通过；
- 以发布前 15 分钟为基线，HTTP 5xx、超时率、连接错误和 P95 延迟
  不超过基线绝对值 + 2 个百分点或相对 2 倍（取更严格者）；
- 容器重启数为 0；
- 无 fatal、panic、迁移或数据一致性错误；
- HTTP、SSE、WebSocket 均有成功样本；
- PostgreSQL、Redis 资源没有异常增长；
- relay-ops 保持旧版本单活且健康；
- 生产 Caddy、PostgreSQL、Redis、relay-ops 容器 ID 不变。

### 7.6 晋升标准服务

观察通过后，公网仍由 candidate 承接：

1. 将生产 Compose 中标准 `sub2api` 镜像和全局环境更新为候选版本；
2. `docker compose config --quiet`；
3. 只执行标准 `sub2api` 服务的 `up -d --no-deps --force-recreate`；
4. 等待标准服务健康；
5. 对标准服务重复内部 smoke；
6. 验证 relay-ops 仍能访问标准服务。

该阶段标准服务短暂重启不会影响公网，因为 Caddy 仍指向 candidate。

### 7.7 切回标准服务并清理

1. Caddy upstream 热切回 `sub2api:8080`；
2. 重复公网 smoke；
3. 观察最短 5 分钟；
4. 停止 candidate；
5. 再次检查标准服务、公网和四个固定容器；
6. 删除 candidate 容器；
7. 删除 candidate 临时 overlay 和 candidate 专属临时资源；
8. 不删除共享 volume、生产网络或旧镜像；
9. 将状态标记为 `COMPLETED`。

## 8. 回滚

### 8.1 切流前失败

- 停止并删除 candidate；
- 旧标准服务和 Caddy 不变；
- 标记 `FAILED_SAFE`。

### 8.2 公网已切到 candidate

- Caddy upstream 切回 `sub2api:8080`；
- validate、reload、public smoke；
- 停止 candidate；
- 标记 `ROLLED_BACK`。

### 8.3 标准服务晋升失败

- 公网继续由 candidate 服务；
- 恢复标准服务旧 Compose 和旧镜像；
- 标准服务健康后决定：
  - 公网切回旧标准服务并回滚；
  - 或继续由 candidate 服务等待人工处理。

禁止自动恢复 PostgreSQL 数据。数据库迁移不兼容时必须在候选启动门禁阶段阻止发布。

### 8.4 切回标准服务后失败

- Caddy 立即切回仍在运行的 candidate；
- 修复或恢复标准服务；
- candidate 未停止前不得删除。

## 9. relay-ops 隔离

Sub2API 发布脚本不得升级或重建 relay-ops。

首次蓝绿工程实施同时修改生产 Compose 契约，使 Caddy 不再依赖
`sub2api` 或 `relay-ops` 的健康条件；Caddy 可以先启动，后端异常只
影响对应 upstream，不得撤掉公网入口。该修改不重建 Caddy，只作为后续
Caddy 维护和宿主机启动时的防故障契约。

后续 relay-ops 独立发布必须先完成：

- 旧通知策略缺少新增通知族字段时默认关闭，而不是启动失败；
- 未知字段继续 fail-closed；
- `native_ops_alerts_enabled=true` 时必须验证只读数据库文件；
- 增加只做 config-check、不连接数据库、不启动 scheduler、不发通知的命令；
- Compose readiness 从 `/healthz` 改为 `/readyz`；
- 保证任何时刻只有一个启用 scheduler/通知发送器的实例；
- Caddy 不再通过 `depends_on` 等待 `sub2api` 或 `relay-ops`。

这些修复不属于首次 Sub2API 蓝绿发布范围。

## 10. 删除边界

用户要求稳定后删除冗余实例。删除仅指：

- `sub2api-candidate` 容器；
- candidate 专属临时配置和发布锁文件。

不得删除：

- 标准 `sub2api`；
- PostgreSQL、Redis、Caddy、relay-ops；
- 任何共享 volume；
- `sub2api_default` 网络；
- 旧镜像；
- 备份和发布记录。

## 11. 可复用接口

发布工具提供以下命令：

```text
preflight
start-candidate
verify-candidate
switch-to-candidate
observe
promote-canonical
switch-to-canonical
cleanup-candidate
rollback
status
```

每个命令：

- 可独立重入；
- 必须校验上一状态；
- 输出机器可读 release ID、状态和失败阶段；
- 默认不自动跨越失败；
- 不接受账号 ID、账号凭据、代理绑定或分组绑定参数。

## 12. 实施文件

计划中的最小工程改动：

- 新增 `infra/compose.sub2api-candidate.yaml`；
- 修改 `infra/Caddyfile`，增加受控 upstream 块；
- 新增 `ops/deploy-sub2api-blue-green.sh`；
- 修改 Sub2API backend 启动 wiring，增加并验证 candidate/API 进程模式；
- 修改 entrypoint/config，使 candidate 不执行递归 `chown` 和 migration；
- 新增发布状态和记录格式说明；
- 修改 `infra/compose.yaml`，解除 Caddy 对应用健康的启动依赖；
- 增加 candidate runner capability manifest 和双版本共享状态契约；
- 增加 resolved-config parity、镜像/路径 allowlist、release label/lease
  和资源容量门禁；
- 增加 shell 契约测试、Caddy validate/active-config 测试和中断恢复测试；
- 更新 `docs/runbooks/sub2api-official-image-release.md`；
- 更新 `docs/project/project-progress.md`。

首次实施不得顺带升级 relay-ops 或首页镜像。

### 12.1 变更路径与镜像 allowlist

首次生产蓝绿发布必须在任何容器 mutation 前校验提交树和镜像集合：

- 允许的运行时代码路径仅为 `upstream/sub2api/**`、`infra/compose.yaml`
  中 Sub2API 全局环境默认值，以及与 candidate/发布脚本直接相关的
  `infra/compose.sub2api-candidate.yaml`、`infra/Caddyfile`、
  `ops/deploy-sub2api-blue-green.sh`；
- 允许的镜像集合仅为标准 Sub2API 镜像 digest 与同一 digest 的
  candidate 镜像；
- `accounting-ledger`、`relay-ops-service/**`、`config/notification-policy.json`、
  `infra/Dockerfile.caddy`、homepage 资源和上游/账号/分组配置路径一旦出现在
  release diff 中，preflight 必须失败；
- 文档、测试和发布记录可以随提交存在，但不能改变运行时 allowlist；
- allowlist 失败只能停止发布并输出命中的路径或镜像，不得自动过滤后继续。

## 13. 验收标准

- candidate 启动失败不会改变公网；
- candidate 不会启动任何后台 scheduler、monitor、cleanup、expiry、
  aggregation、alert、refresh 或通知 runner；
- candidate 不会执行 migration 或共享数据卷初始化副作用；
- candidate 与旧版本的 PostgreSQL/Redis/usage/cache 数据满足双版本
  读写与回滚契约；
- Caddy 容器 ID 在完整发布中不变；
- PostgreSQL、Redis、relay-ops 容器 ID 在完整发布中不变；
- 公网切流通过 graceful reload 完成；
- Caddy 所有 Sub2API 路由均切到同一目标，active config 可查询确认；
- 标准服务升级期间公网请求由 candidate 成功承接；
- HTTP、SSE、WebSocket 均通过；
- 探针不产生 usage、billing、余额、账务或账号级配置变化；
- runner capability manifest 覆盖所有后台启动/订阅点，且 candidate 中无
  漏项、未知项或 enabled 项；
- 变更路径和镜像 allowlist 在启动 candidate 前通过；
- 切流和晋升任一阶段失败可恢复；
- 发布结束后只有一个 Sub2API 容器；
- candidate 被删除，共享资源不变；
- 全程不修改账号级配置；
- 脚本可用于后续任意固定 digest 的 Sub2API 镜像发布。

## 14. 本次发布范围

蓝绿机制实施并演练后，首次发布只包含：

- 全局 OpenAI 低延迟环境配置；
- WebSocket `store=false` 会话隔离修复；
- 随同一 Sub2API 镜像进入的监控页自动刷新能力。

明确排除：

- accounting 工作区；
- Native P0/P1 Bridge；
- relay-ops 新镜像；
- Caddy/首页镜像升级；
- 上游分流、价格变更、账号/代理/分组绑定；
- PostgreSQL、Redis 或存储结构的破坏性变更。

蓝绿机制本身属于工程代码交付，不是单纯运维脚本。只有 candidate
进程模式、数据卷写集、Caddy reload 和发布状态机都完成测试后，才能将
后续 Sub2API 版本纳入生产蓝绿发布。
