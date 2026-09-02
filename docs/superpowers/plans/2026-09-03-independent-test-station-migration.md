# 独立测试站迁移实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将当前 `/admin/lab/` 测试站及测试数据迁移到 `ubuntu@49.51.203.200`，以独立裸 IP 根站点运行，并创建私有 GitHub 代码仓库。

**Architecture:** 新服务器独立运行 Docker Compose、Caddy/Nginx、API、worker、detector、PostgreSQL、Redis 和应用数据卷。旧测试站只在确认主站容器身份后停止，使用逻辑备份恢复而非复制主站卷；旧站和备份保留作为回滚源。

**Tech Stack:** Ubuntu, Docker Engine/Compose, PostgreSQL `pg_dump/pg_restore`, Redis persistence, Caddy/Nginx, GitHub CLI/keyring。

**Spec:** `docs/superpowers/specs/2026-09-03-independent-test-station-migration-design.md`

## Global Constraints

- 只停止测试站专用 Compose project；主站 `sub2api`、PostgreSQL、Redis、Caddy 不得停止。
- 部署来源、仓库和凭据必须与主站隔离；不使用 GitHub Actions。
- 备份、密钥和真实数据只存放在本机/服务器受限目录，不进入 GitHub。
- 裸 IP 仅作初期受限验证；长期公网管理员访问需补充 HTTPS/VPN/来源限制。

### Task 1: 新服务器基线

**Files/Systems:** 新服务器 `/opt/sub2api-test-station`、系统 SSH/UFW/Docker 配置。

- [ ] 使用 `test_service.pem` 以 `ubuntu` 登录，校验主机指纹和磁盘。
- [ ] 安装 Docker Engine、Compose plugin、Caddy；创建专用部署用户/目录并设置权限。
- [ ] 配置 UFW 仅开放 SSH、HTTP、HTTPS；确认 IPv4/IPv6 监听。
- [ ] 创建独立 Compose project/network/volume 命名，不能复用 `sub2api`、`sub2api_default` 或主站路径。
- [ ] 保存不含秘密的主机基线记录。

### Task 2: 旧测试站冻结与备份

**Files/Systems:** 旧测试站宿主、受限本机备份目录。

- [ ] 只读列出 Docker Compose project，明确测试站 project/container 名称，并核对主站容器仍健康。
- [ ] 停止测试站 project；禁止使用不带 `--project-name` 的全局 `docker compose down`。
- [ ] 使用 `pg_dump -Fc` 导出测试站 PostgreSQL 全库，记录 schema 版本、表行数和 SHA-256。
- [ ] 导出 Redis 持久化数据和 `/app/data`/上传数据，记录容量和 SHA-256。
- [ ] 保存测试站配置变量名清单；秘密只通过受保护文件传输。
- [ ] 在临时数据库中恢复备份并执行只读一致性检查，失败则停止后续动作。

### Task 3: 独立根路径拓扑

**Files:** `infra/compose.admin-lab.yaml`, `infra/admin-lab/gateway.conf`, 新站 Caddy 配置、部署脚本。

- [ ] 将前端 `VITE_APP_BASE_PATH` 改为 `/`，API base path 改为 `/api/v1`，Cookie/localStorage 使用新站命名空间。
- [ ] 将 Compose project、service、network、volume、deploy root 改为测试站专用新机身份。
- [ ] 移除对主站外部 Docker network 的依赖；上游/支付/通知仅允许测试站配置的目标。
- [ ] 配置 Caddy/Nginx 监听 IPv4 与 IPv6 根路径 `/`，保留 `/health`、`/readyz` 等健康端点。
- [ ] 添加启动、停止、备份、恢复和版本核对脚本，所有停止命令要求显式 project 名。

### Task 4: 新机部署与数据恢复

**Systems:** 新服务器独立 Compose project 和数据卷。

- [ ] 传输镜像/代码、Compose、配置模板和已校验备份，传输后再次校验 SHA-256。
- [ ] 生成新站数据库、Redis、JWT、CSRF、Cookie 等独立秘密。
- [ ] 启动 PostgreSQL/Redis，应用迁移并恢复 PostgreSQL；恢复 Redis 和应用数据。
- [ ] 启动 API、worker、detector、gateway 和边缘代理，等待全部健康检查通过。
- [ ] 记录 source commit/tree、镜像 digest、容器身份和数据恢复结果。

### Task 5: 功能与隔离验收

- [ ] 通过 IPv4 和 IPv6 验证 `/`、登录/退出、管理员权限和会话。
- [ ] 验证用户、账号、分组、余额/账单、API、流式响应、worker、detector 和文件数据。
- [ ] 重启应用栈，确认数据与会话策略符合预期且无卷丢失。
- [ ] 只读核对新站未连接主站数据库、Redis、网络、卷或生产凭据。
- [ ] 同时只读核对主站 `sub2api`、PostgreSQL、Redis、Caddy 健康，确认未被迁移操作停止。

### Task 6: GitHub 私有仓库

- [ ] 用本机已登录的 `gh` 创建私有仓库（默认名 `sub2api-test-station`，如已存在则先核对归属）。
- [ ] 推送去秘密化代码、Compose/Caddy 模板、迁移/恢复脚本和运维文档。
- [ ] 用 `git grep`、秘密扫描和 `.gitignore` 检查，确保无 `.env`、备份、PEM、token、API key、支付/上游凭据或真实数据。
- [ ] 记录仓库地址、GitHub 账号、凭据存储位置、权限范围和轮换要求；不记录明文凭据。

### Task 7: 收口与回滚窗口

- [ ] 生成迁移报告，包含备份 checksum、行数/容量对账、验收结果、source commit/tree 和运行身份。
- [ ] 保留旧测试站、原始备份和恢复证据，观察期内禁止删除。
- [ ] 若验收失败，停止新站专用 project 并恢复旧测试站；不得触碰主站服务。
- [ ] 仅在用户明确要求且完成长期 HTTPS/访问控制方案后，替换裸 IP 入口。
