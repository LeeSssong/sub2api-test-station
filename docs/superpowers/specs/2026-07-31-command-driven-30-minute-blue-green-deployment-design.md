# 30 分钟指令驱动蓝绿生产部署设计

**日期：** 2026-07-31

**状态：** 已批准，直接进入实施
**取代范围：** 本设计取代此前可复用蓝绿设计中的长观察、多阶段晋升流程；旧设计只保留事故约束和兼容性研究价值。

## 1. 目标

任务实施并测试完成后，系统等待用户明确发出“部署生产”指令。从指令发出起 30 分钟内，完成不可变镜像构建、生产备用槽启动、内部验证、Caddy 无中断切流和公网验收。

如果预检判断本次变更必然造成服务不可用，必须在任何生产变更前停止，向用户说明原因、预计不可用时长和回滚方案。只有用户另行明确授权“允许停机部署”后才能继续。

## 2. 固定约束

- 没有明确生产部署指令，不得修改生产运行状态。
- 部署不重复同一代码树已经通过的开发测试；代码树发生变化则拒绝部署并要求重新测试。
- 不停止 Caddy、PostgreSQL、Redis 或其它未变更的共享服务。
- 不使用 docker compose down，不删除生产卷，不自动恢复数据库。
- 新版本必须先在非活动槽启动，旧版本继续承接公网流量。
- 候选槽必须禁用 scheduler、monitor、queue consumer 等单例后台任务，只提供 API 和前端流量。
- 切流前失败不得影响活动槽；切流后失败必须优先把流量切回旧槽。
- 无法证明新旧版本可并行、可回切或可在 30 分钟内完成时，必须停止。

## 3. 触发语义

实施和测试完成只产生“待部署”状态。用户在当前任务中发出“部署生产”“发布生产”或等价明确指令后，才授权：

1. 构建当前已验证 Git tree 的生产镜像；
2. 向生产主机传输或拉取该不可变镜像；
3. 启动非活动槽并执行内部验收；
4. 切换公网流量、观察和必要时回切；
5. 记录精确 commit、Git tree、镜像 digest、槽位和验收结果。

普通生产部署指令不包含停机授权。

## 4. 蓝绿架构

生产环境定义两个可交换的 Sub2API API 槽和一个唯一后台进程：

- sub2api-blue
- sub2api-green
- sub2api-worker

任一时刻只有一个槽接收公网流量。Caddy 从 root-only 状态文件解析活动 upstream，只允许 sub2api-blue:8080 或 sub2api-green:8080。

切流前必须完成 Caddy 配置校验；切流使用 graceful reload，不重建 Caddy。两个 API 槽共享 PostgreSQL、Redis 和必要的只读配置，并始终以 API-only 模式运行。`sub2api-worker` 是唯一允许执行数据库迁移、启动 scheduler/monitor、消费共享队列和运行其它全局单例后台任务的进程。

应用启动角色由 `SERVER_PROCESS_ROLE` 明确指定：

- `all`：兼容现有单体部署，运行 HTTP、迁移、请求级后台任务和全局单例后台任务；
- `api`：运行 HTTP/API 和当前实例请求链路必需的本地后台任务，不执行迁移、secret bootstrap、simple-mode seed、启动期 Setting 迁移、调度器、监控器或共享队列消费；
- `worker`：执行迁移和全局单例后台任务，不承接公网流量；内部 HTTP 健康入口可以保留，但不能加入 Caddy upstream。

请求级本地后台任务和全局共享后台任务必须逐项分类，不能用一个粗粒度总开关关闭所有 `Start`。例如，API 请求产生的 deferred account last-used flush、当前实例的 usage writer、内容审核请求 worker 等可留在 `api`；token refresh、过期清理、聚合、monitor、scheduled test、backup、billing probe、Prompt Audit 共享任务消费等只在 `worker/all` 启动。Prompt Audit 的 API 槽只加载配置和入队，唯一 worker 负责消费。

`api` 角色以只读方式核对当前镜像内嵌迁移集合：`schema_migrations` 中必须存在所有已知迁移且 checksum 匹配；该检查不得执行 DDL、INSERT、secret bootstrap 或 seed。缺失迁移、checksum 不匹配，或镜像迁移集合 hash 与当前活动发布不一致时，自动蓝绿流程必须在生产变更前输出 `downtime_required=true`。第一版不自动推断 expand/contract 兼容性。

发布记录至少包含：

- active_slot 和 previous_slot；
- commit 和 Git tree；
- image 和 image_digest；
- started_at、cutover_at 和 verified_at；
- result 和失败阶段。

## 5. 30 分钟流程

### 0–5 分钟：预检

- 获取全局发布锁；
- 验证部署 Git tree 与最后测试证据一致；
- 读取活动槽、镜像 digest、Compose 身份和共享服务健康；
- 检查 CPU、内存、磁盘、PostgreSQL 连接和 Redis 连接余量；
- 判断迁移、卷、网络、密钥、共享服务和后台任务兼容性；
- 在生产变更前识别必然停机风险。

### 5–15 分钟：构建与传输

- 从已验证 Git tree 构建 Linux AMD64 镜像；
- 写入 commit、tree 和 qualified 标签；
- 生成并校验 SHA-256；
- 以 digest 而非可变 tag 作为发布输入；
- 使用可恢复传输或私有 registry 将镜像送达生产。

### 15–22 分钟：候选槽验收

- 使用新 digest 只创建非活动槽；
- 确认 candidate/API-only 模式生效；
- 从生产 Docker 网络直接访问候选槽，不经过公网域名；
- 验证健康、版本、静态资源、数据库、Redis、受保护路由和有界网关冒烟；
- 候选失败时删除非活动槽，活动槽保持不变。

### 22–25 分钟：无中断切流

- 生成只指向候选槽的 Caddy 配置；
- 配置校验通过后原子更新活动 upstream；
- 执行 graceful reload；
- 旧槽继续运行。

### 25–30 分钟：公网验收

- 验证公网健康、版本、关键 API 和本次改动功能；
- 检查 HTTP 5xx、超时、容器重启和 Caddy 错误日志；
- 确认 PostgreSQL、Redis、Caddy 和其它未改服务的容器身份未变；
- 将新槽记为 active，旧槽记为 previous；
- 旧槽至少保留到当次验收完成。

## 6. 回切

- 切流前失败：停止并删除候选槽，公网继续使用旧槽。
- Caddy 校验或 reload 失败：保留旧活动状态，不停止旧槽。
- 切流后验收失败：原子切回 previous slot，再次 graceful reload，并验证旧版本。
- 回切失败：保留两槽、日志和状态记录，进入事故处理；不得自动恢复数据库或批量重建服务。

公网回滚单元是 Caddy upstream，不是数据库、Redis 或整个 Compose project。

## 7. 必须二次授权的停机门禁

出现以下任一条件时，工具必须在生产变更前退出：

- 新旧版本不能同时连接当前数据库或 Redis；
- 数据库迁移不兼容旧版本，或不能在切流前完成；
- 必须停止 PostgreSQL、Redis、Caddy 或其它共享服务；
- 必须改变生产卷、Docker 网络或 Compose project 身份；
- 候选槽不能禁用单例后台任务；
- 主机资源不足以并行运行两个槽；
- 无法证明从新槽回切旧槽后的应用和数据兼容性。

退出信息必须包含：

- downtime_required=true；
- 具体原因；
- 有界的预计不可用秒数；
- 明确的回滚单元和步骤。

只有用户看过这些信息并明确授权“允许停机部署”，才能进入单独的停机发布流程。

### 7.1 首次拓扑迁移门禁

当前生产若仍是 legacy 单 `sub2api` 服务，首次建立 `sub2api-blue`、`sub2api-green`、`sub2api-worker` 和 Caddy 活动 upstream 状态不等同于后续稳态蓝绿发布。首次迁移至少要验证：

- legacy 实例与新 worker 不会同时运行同一全局单例后台任务；
- relay-ops 改接 Caddy 内部活动路由时，`/pricing` 等公开路径不会短暂 502；
- PostgreSQL、Redis、Caddy、Compose project、网络和卷身份保持不变；
- 新拓扑失败时可切回 legacy upstream，且旧实例的数据兼容性仍成立。

只要无法在隔离演练和生产预检中证明上述条件，首次拓扑迁移必须返回 `downtime_required=true`，列出预计不可用秒数和回滚步骤，并等待用户明确授权。稳态拓扑建立以后，普通 Sub2API 版本更新才进入 30 分钟无感蓝绿路径。

## 8. 实施范围

本次实施包括：

- Sub2API candidate/API-only 运行模式；
- 唯一 `sub2api-worker` 运行角色和只读迁移核验；
- 生产 Compose 蓝绿槽定义和资源约束；
- Caddy upstream allowlist、配置校验、原子状态更新和 graceful reload；
- 单一命令的预检、构建、传输、候选验收、切流、公网验收和回切编排；
- 发布锁、状态记录、中断恢复和停机门禁；
- Shell 契约测试、Compose/Caddy 集成测试和双槽回切测试；
- 发布运行手册和 30 分钟超时报告。

本次实施不包括：

- 自动执行真实生产发布；
- 自动授权停机风险；
- 数据库自动恢复、破坏性迁移或秘密值更换；
- 与 Sub2API 蓝绿切流无关的服务重构。

## 9. 验收标准

- [ ] 没有生产指令时，工具不连接或修改生产。
- [ ] 测试 tree 与部署 tree 不一致时，构建前失败。
- [ ] 正常演练路径在 30 分钟预算内完成。
- [ ] 切流不停止 Caddy，旧槽在公网验收前不停止。
- [ ] 候选槽不运行单例后台任务。
- [ ] 候选失败不影响活动槽。
- [ ] 切流后失败可以切回旧槽。
- [ ] 未变更共享服务的容器身份保持不变。
- [ ] 必然停机条件在生产变更前输出原因、时长和回滚方案并退出。
- [ ] 只有独立停机授权可以绕过停机门禁。

## 10. 完成口径

实现、测试和隔离演练完成后，本事项只能标记为“准备完成”或“待生产验收”。只有同时满足代码已推送到服务端、机制已部署到生产、一次真实生产指令完成无感切流并验证生效，项目进度总账才能标记“已完成”。
