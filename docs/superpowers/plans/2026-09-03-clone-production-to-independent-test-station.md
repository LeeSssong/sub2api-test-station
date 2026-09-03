# 独立测试服务器直接克隆主站制品实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将主站当前运行的 Sub2API 与 homepage/Caddy 制品逐字节复制到独立测试服务器，同时保留测试站独立数据、凭据、网络和卷。

**Architecture:** 主站宿主仅用于只读锁定并导出当前活动镜像和静态制品；独立测试服务器使用测试站专用 Compose/Caddy 配置加载这些制品。切换前保存当前栈与数据恢复点，失败时只回滚 `sub2api-test-station` project。

**Tech Stack:** Docker Engine/Compose、Caddy、PostgreSQL 18、Redis 8、SSH/SCP、SHA-256、Bash。

**Spec:** `docs/superpowers/specs/2026-09-03-clone-production-to-independent-test-station-design.md`

## Global Constraints

- 不修改 Sub2API、homepage 或主站业务代码，不重新构建主站应用制品。
- 不复制主站 PostgreSQL、Redis、应用数据、管理员、支付、上游、通知或其他 secrets。
- 只停止或替换独立服务器的 `sub2api-test-station` Compose project。
- 所有秘密继续位于本机或服务器 0600 文件；Git 和聊天中只记录绝对路径、用途和验证方法。
- 主站全程只读，操作前后必须核对活动 source/image ID 与健康。

---

### Task 1: 锁定主站制品和测试站恢复点

**Files:**
- Create locally outside Git: `~/.config/sub2api/test-station-clone/manifest.json`
- Create on test host: `/opt/sub2api-test-station/backups/<timestamp>/`

**Interfaces:**
- Consumes: 主站 `/var/lib/sub2api/release-state`、运行容器、独立测试站现有 Compose/卷。
- Produces: 主站 source/image/static manifest 和测试站回滚备份 checksum。

- [ ] 只读读取主站活动槽、Sub2API image ID、source commit/tree、migration hash、Caddy image ID 和首页文件 manifest。
- [ ] 读取独立测试站容器、镜像、Compose、网络和卷身份，确认所有停止目标均属于 `sub2api-test-station`。
- [ ] 导出测试站 PostgreSQL、Redis RDB、应用数据、Compose/Caddy 和脱敏配置变量名，计算 SHA-256。
- [ ] 在主站与测试站分别记录服务健康；任何主站异常立即停止。

### Task 2: 导出并传输主站运行制品

**Files:**
- Create locally outside Git: `~/.config/sub2api/test-station-clone/artifacts/<source>/`
- Create on test host: `/opt/sub2api-test-station/releases/<source>/`

**Interfaces:**
- Consumes: Task 1 锁定的 Sub2API 和 Caddy/homepage image IDs。
- Produces: 经双端 SHA-256 校验的 Docker archives。

- [ ] 在主站以 image ID 导出当前活动 Sub2API 镜像和当前 Caddy/homepage 镜像，不读取或归档容器 secrets/卷。
- [ ] 将归档传回本机受限目录并计算 SHA-256。
- [ ] 将归档传输到独立测试服务器受限 release 目录并再次校验 SHA-256。
- [ ] 加载镜像并核对 image ID、source labels 和静态首页 manifest 与主站一致。

### Task 3: 生成独立测试站运行配置

**Files:**
- Create on test host: `/opt/sub2api-test-station/releases/<source>/compose.yaml`
- Create on test host: `/opt/sub2api-test-station/releases/<source>/Caddyfile`
- Reuse on test host: `/opt/sub2api-test-station/.env`

**Interfaces:**
- Consumes: 主站镜像引用、测试站现有 0600 `.env`、测试站卷名称。
- Produces: `docker compose config` 可验证的独立拓扑。

- [ ] 从主站运行拓扑提取 API/worker/detector/Caddy/homepage 路由结构，改写为 `sub2api-test-station` project 和独立网络/卷。
- [ ] 配置裸 IP HTTP 根首页、`/login`、`/admin/*`、`/api/*`、`/ws/*`、`/health` 与 `/readyz`；移除 `/admin/lab` 依赖。
- [ ] 保留测试站数据库/Redis/管理员/cookie/provider/mock 值，只引用现有 0600 `.env`，不输出值。
- [ ] 运行 `docker compose config`，断言没有主站 project、network、volume、bind path 或 production secret path。

### Task 4: 切换独立测试服务器

**Files:**
- Update symlink on test host: `/opt/sub2api-test-station/current`
- Create on test host: `/opt/sub2api-test-station/release-state.json`

**Interfaces:**
- Consumes: Task 1 恢复点、Task 2 镜像、Task 3 配置。
- Produces: 使用主站制品且测试数据独立的新运行栈。

- [ ] 停止当前 `sub2api-test-station` project，但不删除容器、网络或卷。
- [ ] 使用新 release 配置启动 PostgreSQL/Redis，核对现有测试数据卷可读且行数符合备份。
- [ ] 启动 detector、API、worker、Caddy/homepage，等待全部健康。
- [ ] 原子更新 `current` 和 0600 `release-state.json`；任一步失败时停止新栈并用 Task 1 Compose/镜像恢复旧栈。

### Task 5: 版本、功能和隔离验收

**Files:**
- Create locally outside Git: `~/.codex/release-evidence/sub2api/<date>-independent-test-station-clone.json`

**Interfaces:**
- Consumes: 新测试站运行栈和 Task 1 主站基线。
- Produces: 可审计的版本、页面、健康、数据和隔离证据。

- [ ] 对比两站 Sub2API image ID/source labels 与 homepage HTML/assets manifest。
- [ ] 通过 IPv4 和可用时 IPv6 验证 `/`、`/login`、`/health`、`/readyz`、API 登录与管理员会话。
- [ ] 只读核对测试站用户/账号/usage/账务表数量、Redis key 数和应用数据容量与备份一致。
- [ ] 核对测试站没有主站网络、卷、数据库 endpoint 或生产 secret bind；核对主站 source/image ID 与健康未变化。
- [ ] 重启测试站应用服务并重复健康、首页和数据持久化检查。

### Task 6: GitHub 和另一终端接管交付

**Files:**
- Create in Git: `docs/operations/independent-test-station-handoff.md`
- Create locally outside Git: `~/.config/sub2api/test-station-handoff.env`
- Create locally outside Git: `~/.config/sub2api/test-station-credentials-index.md`

**Interfaces:**
- Consumes: GitHub 私有仓库 metadata、本机 keyring/SSH config、测试站 release state 和凭据文件。
- Produces: 另一终端 Codex 可直接读取的接管手册和 0600 凭据索引。

- [ ] 验证私有仓库 remote、当前 GitHub 登录态、SSH alias/known_hosts/key 和测试服务器 sudo/Docker 权限。
- [ ] 仓库手册记录非敏感架构、日常查看、发布、备份、恢复、回滚和验证命令。
- [ ] 本机 0600 索引记录所有凭据文件绝对路径、变量名、用途、权限、轮换和验证命令；不复制秘密值到 Git 或聊天。
- [ ] 提供另一终端 Codex 的启动提示词，要求先读项目约束和本机凭据索引，再执行只读接管核验。
- [ ] 运行 secret scan、`git diff --check`，确认仓库无 `.env`、PEM、token、密码、备份或真实数据。

### Task 7: 候选整合与发布记录收口

**Files:**
- Modify: `docs/project/project-progress.md`
- Modify: `docs/project/native-sub-task-package-queue.md`

**Interfaces:**
- Consumes: Task 5/6 验证和交付证据。
- Produces: 根 `main` 可审计记录和候选交接。

- [ ] 提交规格、计划、非敏感手册和必要部署配置；不提交本机/宿主 secrets 或运行备份。
- [ ] 在根总控盘点后将候选合入 `main`，解决总账冲突并推送 `origin/main`；该推送不触发主站部署。
- [ ] 从最终干净且 `HEAD == origin/main` 的根 `main` 重新核对独立测试站发布记录与仓库交付内容。
- [ ] 只有 Git 推送、独立测试站部署、验证和交付手册全部完成后，将任务标记为 `DONE`。
