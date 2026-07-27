# Sub2API 官方版本无人值守候选准备设计

**日期：** 2026-07-28  
**状态：** 已确认架构，待实现  
**运行平台：** GitHub Actions  
**镜像仓库：** 私有 GHCR  
**检查频率：** 每 6 小时

## 问题

官方 Sub2API 已实现管理后台自动检查更新，但检查只在管理员页面加载或手动刷新时
发生。后端的 20 分钟 Redis TTL 只是请求缓存，不是后台调度器；如果没有管理员打开
页面，生产服务不会主动发现 GitHub Release。

星桥生产使用包含源码定制的合格镜像，不能直接运行官方原版镜像。每次官方升级仍需
合并官方变更、运行测试、构建合格镜像并把候选镜像送到生产机。当前人工流程虽然安全，
但耗时受本机构建环境和 SSH 大文件传输影响，也无法真正无人值守。

## 目标

建立一条不依赖管理员访问后台、不依赖本地 Mac 在线的自动准备流水线：

1. 每 6 小时查询一次 `Wei-Shaw/sub2api` 最新稳定 GitHub Release。
2. 对新版本自动执行官方源码三方合并、全量测试和 `linux/amd64` 镜像构建。
3. 把合格镜像推送到私有 GHCR，并由生产机按不可变 digest 拉取。
4. 在生产机验证平台、资格标签、二进制版本和当前运行容器不变。
5. 全部成功后，把合格源码提交以 fast-forward 方式推进到 `main`。
6. 通过飞书发送准备完成或失败的事实卡片。
7. 永远停在“候选镜像已就绪”，生产切换仍只由管理员在现有后台手动确认。

## 非目标

- 不调用 `/api/v1/admin/system/update` 或任何宿主更新 API。
- 不修改生产 `compose.yaml`、`.env`、Caddy 配置或数据库。
- 不重建、重启、停止或切换任何生产容器。
- 不在生产机编译源码或积累 BuildKit 构建缓存。
- 不自动处理源码冲突、失败测试或不匹配的已有版本标签。
- 不把官方后台页面加载事件作为可靠触发器。
- 不使用本地 Codex 自动化或本地 Mac 作为长期调度器。
- 本阶段不自动删除 GHCR 历史版本，也不执行 Docker image、volume 或 system prune。

## 总体流程

```text
GitHub Actions schedule / workflow_dispatch
  -> 查询官方 latest stable Release
  -> 与 main 中 XINGQIAO_UPSTREAM.md 比较
  -> 在无秘密 Job 中三方合并、测试、构建
  -> 发布不可变 GHCR 镜像和审计分支
  -> 生产受限 SSH 命令 pull + inspect + isolated --version
  -> 证明运行容器和 Compose 未变化
  -> 以 compare-and-swap fast-forward 推进 main
  -> 飞书发送成功或失败事实卡片
```

计划任务使用非整点 cron，避免 GitHub Actions 整点拥堵。`workflow_dispatch` 只用于
人工重试同一“官方最新稳定版”流程，不能指定任意旧版本或跳过安全门禁。

## 官方版本发现

版本事实源保持为：

```text
GET https://api.github.com/repos/Wei-Shaw/sub2api/releases/latest
```

发现 Job 必须：

- 使用 GitHub API JSON，而不是解析网页或 Release 邮件；
- 验证 `draft=false`、`prerelease=false`；
- 验证 tag 为 `v?数字.数字[.数字][-预发布][+构建]`，但拒绝 prerelease；
- 解析 tag 最终指向的 40 位 commit，支持 annotated tag；
- 保存 Release 名称、正文、发布时间、HTML URL、tag、version 和 commit；
- 把完整 Release JSON 作为 artifact 传递，正文不得进入 shell 命令或
  `GITHUB_OUTPUT`；
- 把 `main` 当前 SHA 固定为本次 compare-and-swap 基线。

如果 `upstream/sub2api/XINGQIAO_UPSTREAM.md` 已记录同一官方版本和 commit，且生产
候选状态也已匹配，则本次为幂等 no-op，不发送重复成功卡片。

## 自动三方合并

流水线复用已经验证过的合并方法：

1. 从 `XINGQIAO_UPSTREAM.md` 读取当前官方 base tag 和 commit。
2. 在临时官方仓库检出该 base commit。
3. 用当前 `upstream/sub2api` 快照覆盖 base 并提交为“星桥定制”提交。
4. 合并目标官方 commit。
5. 任何文本冲突立即失败，不做基于猜测的自动冲突解决。
6. 把干净合并结果同步回根仓库的 `upstream/sub2api`。
7. 更新 `XINGQIAO_UPSTREAM.md` 和当前状态记录。
8. 以固定作者身份创建候选源码提交。

候选提交的父提交必须是发现阶段固定的 `main` SHA。Release `published_at` 用作固定
提交时间，使同一输入的重试得到稳定提交。合并脚本只允许修改
`upstream/sub2api/**` 和明确列出的版本记录文件；官方源码不能覆盖 `.github`、
`ops`、`infra`、`relay-ops-service` 或仓库其他目录。

## 测试和构建门禁

候选源码必须通过：

- 后端 `go test ./... -count=1`；
- 后端 `go vet ./...`；
- 前端冻结锁文件安装；
- 前端完整测试；
- 前端正式生产构建；
- Caddy 缓存规则测试；
- 宿主更新脚本契约测试；
- `sub2api-updater` 完整 Go 测试；
- `git diff --check`；
- 合并范围和来源元数据校验。

正式镜像固定：

- 平台：`linux/amd64`；
- 构建参数 `VERSION=<version>`；
- 构建参数 `COMMIT=<official-commit>`；
- 前端构建堆 `FRONTEND_NODE_MAX_OLD_SPACE_SIZE=3072`；
- `--provenance=false` 和 `--sbom=false`，生成单平台 manifest；
- 标签：
  - `com.xingqiao.sub2api.qualified=true`
  - `com.xingqiao.sub2api.upstream.version=<version>`
  - `com.xingqiao.sub2api.upstream.commit=<official-commit>`
  - `com.xingqiao.sub2api.source.commit=<candidate-source-commit>`

构建 Job 不拥有仓库写权限、GHCR 写权限、生产 SSH 密钥、飞书凭据或其他业务秘密。
它只输出候选 Git bundle、单平台 OCI 镜像和机器可读验证报告 artifact。

## Job 权限隔离

流水线拆成相互隔离的 Job：

1. `discover`：只读 GitHub 内容和官方 Release。
2. `prepare`：无秘密地合并、测试和构建，不执行生产操作。
3. `publish`：只加载已经验证的 OCI artifact，检查标签并推送 GHCR；不执行候选
   仓库脚本或候选镜像。
4. `stage-production`：只持有专用受限 SSH 密钥和当前 Job 的短时
   `packages:read` token，只能调用生产候选加载器。
5. `advance-source`：只读取候选 Git bundle，在远端 `main` 仍等于固定基线时执行
   fast-forward；绝不 force push。
6. `notify`：只检出触发本次工作流的可信源码提交并使用飞书凭据，不检出或执行
   未通过门禁的候选代码。

所有第三方 GitHub Actions 使用完整 commit SHA 固定，不使用浮动 `@main` 或
`@v4` 引用。工作流设置单一 concurrency group，新的定时触发不能并发准备另一个
版本，也不取消已经进入生产候选交付阶段的运行。

## 私有 GHCR 发布

镜像命名：

```text
ghcr.io/leesssong/xingqiao-sub2api:upstream-<version>
ghcr.io/leesssong/xingqiao-sub2api@sha256:<manifest-digest>
```

发布规则：

- 使用当前仓库作用域的 `GITHUB_TOKEN` 写入私有 GHCR；
- 同一版本 tag 首次发布后视为不可变；
- 如果 tag 已存在，只有 digest 和四项资格标签全部一致才可幂等复用；
- 已存在但内容不同的 tag 视为供应链冲突并失败，禁止覆盖；
- 生产交付只使用 digest，不使用可变 tag；
- 审计分支使用 `automation/sub2api-upstream-<version>`，保存候选源码提交。

GHCR 历史镜像保留策略不在本阶段自动执行。单个镜像约几十 MB，生产加载器先检查
至少 5 GiB 可用空间且根分区低于 85%；不满足时失败并通知，不执行宽泛清理。

## 生产受限候选加载器

生产机安装 root-owned 候选加载器和专用 SSH forced-command 包装器。GitHub Actions
使用独立 SSH 密钥；该密钥不能获得交互 shell、端口转发、代理转发、PTY 或任意命令
执行能力。

forced command 只接受：

```text
prepare <ghcr-digest-ref> <version> <official-commit> <source-commit>
```

加载器执行：

1. 验证参数格式和固定 GHCR 仓库前缀。
2. 记录当前 Sub2API 容器 ID、image ID、启动时间、状态、health、重启数和生产
   Compose SHA-256。
3. 验证磁盘阈值。
4. `docker pull --platform linux/amd64 <exact-digest-ref>`。
5. 验证 OS/架构、四项资格标签和不可变 image ID。
6. 使用以下隔离约束执行 `/app/sub2api --version`：
   - `--network none`
   - `--read-only`
   - `--cap-drop ALL`
   - `--security-opt no-new-privileges`
7. 给同一 image ID 添加本地标签
   `xingqiao-sub2api:upstream-<version>`，供现有宿主更新器解析。
8. 再次验证运行容器全部身份字段和 Compose SHA-256 完全不变。
9. 原子写入无秘密候选状态记录。

加载器不得调用 Docker Compose、宿主更新器、更新 API、数据库客户端或容器 restart。
它不得打印 registry token、SSH 密钥、环境文件或飞书凭据。

生产不保存长期 GHCR PAT。`stage-production` 使用当前 Job 的短时
`GITHUB_TOKEN`，其权限限定为 `contents:read` 和 `packages:read`。token 只通过 SSH
标准输入传给 forced-command，不进入命令参数、环境文件或日志。加载器在 `0700`
临时目录中创建 Docker config，完成 login、digest pull 和校验后立即 logout 并删除
目录；任意退出路径都执行清理。

## 源码推进

只有 GHCR 发布和生产候选校验成功后，`advance-source` 才能推进源码：

- 远端 `main` 必须仍精确等于发现阶段记录的 base SHA；
- 候选提交必须是该 base SHA 的直接后代；
- 推送必须是 fast-forward；
- 远端发生任何并发变化时立即失败，下一轮从新 `main` 重新准备；
- 不使用 force push，不自动覆盖人工提交；
- 审计分支在失败时保留，成功后也至少保留到对应版本完成生产人工更新。

因此“静默准备完成”同时意味着候选源码已经成为仓库当前事实，不需要再执行人工
合并或 PR 操作。唯一仍需人工决定的运行时动作是现有管理后台里的更新确认。

## 飞书通知

新增共享的候选准备卡片 renderer，并复用现有 Feishu webhook Client、脱敏、
30 KiB 限制和 HTTPS URL 校验。

成功卡片标题：

```text
Sub2API <version> 候选镜像已静默准备
```

成功卡片只报告事实：

- 官方版本、名称和发布时间；
- 官方 commit 和 Release 链接；
- 官方 Release 正文摘要；
- 合并结果和完整测试门禁结果；
- 候选源码 commit；
- GHCR immutable digest；
- 生产本地 image ID 和资格校验结果；
- 当前运行版本、容器 image ID、health、启动时间和“未发生切换”；
- Compose SHA-256 未变化；
- 明确声明未调用更新 API、未修改 Compose、未操作数据库。

卡片不包含“下一步”“请点击更新”或固定操作建议。

失败卡片标题：

```text
Sub2API <version> 候选准备失败
```

失败卡片包含失败阶段、稳定错误类别、官方 Release 事实、GitHub Actions 运行链接和
能够证明的生产未变化状态。错误输出必须归一化和脱敏，不发送任意 shell stderr。
同一版本、同一失败阶段的连续重试只发送第一次；阶段变化或最终成功可以再次发送。

## 幂等、重试和状态

- GitHub Actions concurrency 保证同一时间只有一个候选准备流程。
- 已匹配的 GHCR version tag 不重新推送。
- 已匹配的生产候选状态不重复 pull 或 tag。
- 已推进的 `main` 不重新合并。
- 网络和 GitHub Actions 暂时错误由下一次 6 小时调度重试。
- 同版本同阶段失败通知使用 GitHub Actions cache key 去重；缓存过期后允许再次提醒。
- 生产状态记录使用原子 rename 和 `0600` 权限。
- 任意阶段失败均不触发生产运行时回滚，因为本流程从未切换运行容器。

## 凭据和初次启用

GitHub Environment `production-candidate` 保存：

- 专用 forced-command SSH 私钥；
- SSH host、port、user 和 pinned known-host key；
- 飞书候选准备 webhook；
- 必要的非秘密生产入口变量。

GHCR 发布和生产短时只读登录都使用 Actions 自动提供的仓库 `GITHUB_TOKEN`，但分属
不同 Job 权限：发布 Job 仅有 `packages:write`，生产 Job 仅有 `packages:read`。
token 不跨 Job 持久化。

初次启用必须：

1. 合并并推送工作流及可信辅助脚本到 `main`。
2. 把 GHCR package 设为 private，并授予本仓库 Actions 写权限。
3. 在生产创建专用 SSH key pair，安装 forced-command 公钥和 loader。
4. 配置 GitHub Environment secrets/variables，不设置人工 approval gate。
5. 手动执行一次 `workflow_dispatch`，验证短时 GHCR 登录和从发现到飞书的完整路径。
6. 证明生产没有残留 GHCR credential，且运行容器、Compose 和数据库在前后均未变化。

凭据缺失、权限不足或环境保护要求人工审批时，流水线必须失败并报告配置阶段，不能
降级为生产构建、SSH 文件分片传输或无校验的镜像加载。

## 测试策略

### 合并和发现

- fixture 官方仓库验证无冲突合并、冲突失败、annotated tag、重复版本 no-op；
- 恶意 Release 正文不能进入 shell 或输出命令；
- 合并脚本不能修改允许范围外文件；
- base SHA 变化时源码推进失败且不强推。

### 镜像发布

- 资格标签、版本、官方 commit、源码 commit、平台任一不匹配均失败；
- 已存在相同 digest 可幂等复用；
- 已存在不同 digest 不可覆盖；
- build Job 环境不包含生产或飞书凭据。

### 生产加载器

- fake Docker 覆盖 pull、inspect、隔离 version check 和 tag；
- 参数注入、错误 registry、错误 digest、低磁盘、标签不匹配均 fail closed；
- 运行容器或 Compose 任一字段变化均失败；
- 测试证明没有 Compose、更新 API、数据库或 prune 调用；
- forced-command 拒绝交互 shell、额外参数和任意命令。

### 飞书

- 成功卡片包含官方正文摘要、镜像证据和生产未切换事实；
- 卡片不包含“下一步”或更新操作建议；
- 失败卡片只包含归一化错误；
- Release 正文和错误中的秘密样式被脱敏；
- 卡片满足 30 KiB 限制并保留官方 Release 链接。

### 集成与上线

- workflow 静态合约验证权限、Job 依赖、concurrency、cron 和固定 action SHA；
- GitHub Actions 手动运行使用真实私有 GHCR；
- 生产加载真实候选镜像后，现有 updater resolver 能解析本地
  `xingqiao-sub2api:upstream-<version>`；
- 前后容器 ID、运行 image ID、启动时间、health、重启数、Compose SHA-256 和公网
  `/health` 保持一致；
- 不调用更新 API，不生成宿主更新 operation。

## 完成标准

- 每 6 小时无人值守检查官方最新稳定 Release。
- 新版本无冲突且全部门禁通过时，私有 GHCR 存在不可变合格镜像。
- 生产机已按 digest 拉取、验证并添加现有 updater 可识别的本地候选标签。
- `main` 已通过 compare-and-swap fast-forward 保存合格源码。
- 飞书只发送一次事实型完成卡片，且没有固定“下一步”文案。
- 冲突、测试失败、构建失败、发布失败、生产校验失败和源码竞态均 fail closed。
- 生产运行容器、Compose、数据库和宿主更新状态完全不变。
- 本地 Mac 离线时流程仍能正常运行。
