# refactorUIUXv0.1 独立测试站发布控制器补强设计

> 日期：2026-09-20  
> 状态：已批准。批准依据：`refactorUIUXv0.1` 的 PD-19 合同已冻结，用户在完成汇报后明确要求“继续”。  
> 基线：`origin/main@645ce06834698cebcd6707836a234d7096e0a081`，tree `d16f0f3f13d2f837587b73455d22aa3ac9eef540`。

## 1. 问题证据与当前行为

独立测试站已经有专用发布入口 `ops/release-sub2api-test-station.sh` 和宿主执行器 `ops/deploy-sub2api-test-station-host.sh`，但当前实现与早期设计文档存在四个影响回退可信度的差距：

1. 发布前不生成 PostgreSQL、Redis 和 app-data 的逐次备份。
2. `release-state.json` 的 `previous_release_dir` 恒为 `null`；脚本不核对活动 API 容器实际使用的 Compose 文件。
3. `image_digest` 保存的是镜像 tar 归档 SHA-256，不是加载后 Docker image ID，且远端未验证 tag 指向预期 image ID。
4. 候选 `up -d` 或健康检查失败时脚本直接退出，不恢复上一 release；健康门禁只匹配容器状态字符串，没有验证 `/readyz` 的 JSON 语义。

当前切换是同一 Compose project `sub2api-test-station` 的原地更新，不是蓝绿切流。PostgreSQL、Redis 和 app-data 使用独立 named volumes；常规应用回退必须保留这些卷，不能默认回放备份覆盖发布后业务写入。

## 2. 目标与非目标

### 2.1 目标

- 在任何候选容器替换前，生成并验证本次发布专属备份集。
- 从活动 API 容器和现有状态文件共同解析上一 release，发现不一致时 fail closed。
- 同时验证并记录镜像归档 SHA-256、候选 tag 和加载后的 Docker image ID。
- 候选失败时自动重新运行上一 release 的 Compose，保留所有 named volumes，并验证旧版本恢复。
- 候选与回退都验证 `/health` 和 `/readyz`；`/readyz` 必须是 JSON、状态码正确且 `status=ready` 连续成功。
- 只有候选全部验证成功后才原子更新状态文件；失败保留候选 release、备份和诊断记录。

### 2.2 非目标

- 不把独立测试站改造成主站蓝绿拓扑。
- 不修改 sub 业务前后端、数据库 schema 或 `/readyz` 实现。
- 不发布测试站或主站，不触碰远端运行数据。
- 不自动执行数据库降级或默认恢复备份。
- 不复用主站 release、rollback 或生产 env。

## 3. 方案比较

### 方案 A：继续把所有逻辑堆入宿主执行器

优点是文件少。缺点是备份、活动版本解析、候选切换、回退和状态写入耦合在同一长脚本中，失败注入困难，容易在清理路径误删操作员证据。

### 方案 B：新增测试站专用备份助手，补强现有执行器（采用）

新增 `ops/backup-sub2api-test-station-host.sh`，只负责对活动独立测试站生成不可变备份集；宿主执行器负责活动 release 解析、镜像身份、候选启动、探针、自动应用回退和状态记录。两者都以 shell 契约测试覆盖，职责清晰，仍沿用现有专用发布链。

### 方案 C：移植主站蓝绿发布控制器

主站脚本依赖槽位、生产 Compose、Caddy upstream、迁移白名单和维护发布协议。复制会扩大范围并引入错误拓扑，违反测试站专用 project 与最小改动原则，因此不采用。

## 4. 架构与组件

### 4.1 本地发布编排器

`ops/release-sub2api-test-station.sh` 保留现有 clean `main == origin/main` 门禁。新增行为：

- 构建后读取候选 tag 的 Docker image ID。
- 计算排序后迁移集合的 SHA-256。
- 将备份助手与现有 Compose/Caddy/镜像归档一起传入远端 staging。
- 向宿主执行器传入归档 SHA、image ID、迁移集合 SHA 和备份助手路径。
- 宿主执行器返回成功前，本地脚本不声明发布成功。

### 4.2 测试站专用备份助手

`ops/backup-sub2api-test-station-host.sh` 接受活动 Compose、活动 `.env`、部署根和时间戳，只允许 project `sub2api-test-station`。它创建：

```text
/opt/sub2api-test-station/backups/<UTC timestamp>/
  postgres.dump
  redis-dump.rdb
  app-data.tar.gz
  SHA256SUMS
  metadata.json
```

备份语义：

- PostgreSQL 使用活动 Compose 的 `test-station-postgres` 执行 custom-format `pg_dump`，并用同版本容器中的 `pg_restore --list` 验证。
- Redis 在活动容器内使用容器已有的 `REDIS_PASSWORD` 执行同步 `SAVE`，再复制 `/data/dump.rdb`；密码不得进入命令输出或状态文件。
- app-data 从活动 API 容器的 `/app/data` 读取并生成归档；归档必须可列出。
- 三个制品生成 SHA256SUMS；完成集先写 `.partial-*`，全部验证后原子重命名。
- 失败只清理本次 `.partial-*`，不删除历史备份或未知文件。

备份是灾难恢复证据，不是常规应用回退自动输入。

### 4.3 宿主执行器

执行顺序固定为：

1. 校验 staging、source identity、Compose project/network、归档 SHA、候选 image ID 和迁移 SHA。
2. 通过 `test-station-api` 容器的 Compose `config_files` 标签解析活动 Compose；读取 `release-state.json`，要求 project、release_dir 与活动 API Compose 一致。
3. 验证上一 release 的 `compose.yaml`、`Caddyfile`、`.env` 和应用镜像仍存在。
4. 调用备份助手；备份验证失败立即停止，候选不启动。
5. 创建候选 release，复制 Compose/Caddy/受保护 env，写入候选 tag。
6. `docker load` 后验证候选 tag 的 image ID 等于本地传入值。
7. 使用候选 release 对同一 project 执行 `up -d --remove-orphans`。
8. 等待 API、worker、detector、PostgreSQL、Redis、Caddy 状态符合 Compose 健康定义。
9. 连续三次验证 `/health` 和 JSON `/readyz`，相邻尝试间隔可配置，测试模式为 0。
10. 成功后原子写入 schema v2 状态。

第 7 步之后任一失败都进入自动应用回退：用上一 release 的 Compose 与 `.env` 对同一 project执行 `up -d --remove-orphans`，再次检查六服务、`/health` 和 JSON `/readyz`。回退不执行 `down`、不删除卷、不恢复备份。

## 5. 探针合同

- `/health`：HTTP 200，JSON，`status=ok`。
- `/readyz`：HTTP 200，Content-Type 为 `application/json`，JSON 精确包含 `status=ready`。
- 候选与回退均要求连续三次成功；任何 HTML 200、JSON 解析失败、非 200、`status` 不符或超时都失败。
- 探针默认通过宿主的 `http://127.0.0.1` 访问 Caddy；测试模式允许替换命令与等待间隔。

本任务只实现发布门禁。由于当前 sub `/readyz` 仍返回 SPA HTML，控制器补强完成后也不得实际发布，必须等待后续 readiness 业务任务完成。

## 6. 状态文件合同

成功状态升级为 schema v2，至少包含：

- `schema_version: 2`
- `source_commit`、`source_tree`
- `migration_set_sha256`
- `image_archive_sha256`
- `image_id`
- `image_tag`
- `release_dir`
- `previous_release_dir`
- `backup_dir`
- `project_name`
- `result: succeeded`
- `rolled_back: false`
- `updated_at`

候选失败且自动回退成功时，不覆盖当前成功状态文件；另在候选 release 内写入权限 0600 的失败记录，包含候选身份、失败阶段、备份目录和 `rolled_back=true`，不含 env 或错误原文中的秘密。

## 7. 失败与安全语义

- 来源、路径、project、活动状态、备份、镜像身份或迁移身份不可信时，在候选启动前停止。
- 候选启动后失败必须尝试恢复上一 release；回退失败时保留所有证据并返回非零，不继续任何发布动作。
- `.env` 仅作为 Compose `--env-file` 使用和复制，不打印内容。
- 所有新目录拒绝 symlink 路径；状态、备份 metadata 与失败记录使用 0600，目录使用 0700。
- 禁止 `docker compose down`、`down -v`、`docker volume rm`、主站 project、主站脚本和旧 `/admin/lab` 路径。

## 8. 兼容性与迁移

- 现有 schema v1 状态可作为一次升级输入，但必须能解析 `release_dir`、source commit/tree 和 project；缺失上一 release 所需文件时停止。
- 候选迁移集合 SHA 只用于审计与状态记录，不证明旧二进制兼容。refactorUIUX 的每项数据库迁移仍须单独提供冻结旧 release 兼容测试。
- 自动回退只回退应用/Caddy 制品并保留当前 named volumes；备份恢复必须另行审批。

## 9. 验收矩阵

契约测试至少覆盖：

- 活动 API Compose 与状态文件一致时成功解析上一 release。
- 状态缺失、project 错误、release 不一致、上一文件缺失时在副作用前失败。
- 备份成功、空 PostgreSQL dump、损坏 Redis RDB、损坏 app-data、SHA 不一致和 symlink 路径。
- 镜像归档 SHA 正确但 image ID 不符时不启动候选。
- 候选成功时写 schema v2，记录上一 release 与备份目录。
- Compose 启动失败、worker 不健康、Caddy 不健康、`/readyz` HTML、`status=not_ready` 时恢复上一 release。
- 回退成功不覆盖旧成功状态；回退失败返回非零并保留候选和备份。
- 全部 Compose 命令都带 `--project-name sub2api-test-station`，没有破坏性卷命令。

## 10. 发布、验证与回滚条件

本任务的候选完成条件是 shell 语法、备份助手契约测试、本地编排器契约测试、宿主执行器失败注入测试和 `git diff --check` 全部通过。完成后状态只能是 `READY_FOR_ROOT_REVIEW`。

不执行合并、推送或部署。以后只有从已合入并推送的干净根 `main` 才能发布独立测试站；当前 `/readyz` 未修复前，发布必须 fail closed。

## 11. 未决事项

无产品未决项。连续成功次数固定为三次；具体超时与间隔在实现计划中以环境变量提供受限默认值，测试模式允许缩短。
