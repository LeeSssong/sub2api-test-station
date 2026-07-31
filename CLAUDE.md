# 项目约定

## 项目进度总账（强制）

所有实施任务须在实施前登记 `docs/project/project-progress.md`，并在实施中更新状态。只有同时完成“已推送到服务端 + 已部署 + 已验证生效”才能标记“已完成”；仅本地/离线测试、代码合并或报告完成不算完成，未部署或外部阻塞保持“进行中”。

## 工作区生命周期（强制）

**每个会话必须在自己的 git worktree 里工作，任务结束时必须合并回 `main` 并删除该 worktree。**

违反这条会真实伤到人。2026-07-27 有三个会话同时直接在主工作区上改动，实际发生了：

- 一个会话的 commit 误扫入了另一个会话未完成的改动（`git add -A` 扫到了不属于它的文件）
- `git checkout` 被未提交改动挡住，无法切分支
- 一个会话为了不撞车只能中途停手，把半成品留在共享工作区里交接给别人
- 审查者反复需要区分「这个失败是我的改动造成的，还是隔壁会话的」

### 开始任务时

```bash
git worktree add ../sub2api-<任务简称> -b feat/<任务简称>
cd ../sub2api-<任务简称>
```

worktree 目录放在仓库同级，不要放在仓库内部。分支名用 `feat/` / `fix/` / `chore/` 前缀加任务简称。

### 任务完成时

按顺序做完，一步都不能省：

```bash
# 1. 确认自己的工作区干净——不能把未提交改动留给下一个人
git status --short

# 2. 合并回 main
git checkout main
git merge feat/<任务简称>

# 3. 删除 worktree 与分支
git worktree remove ../sub2api-<任务简称>
git branch -d feat/<任务简称>

# 4. 确认清理干净
git worktree list    # 应只剩主工作区
git branch           # 应只剩 main
```

若任务未做完就要交接，**先提交到自己的分支再交接**，不要把改动留在工作区里 —— 未提交的改动对别的会话是不可见的地雷。

### 例外

只读的排查、问答、看日志不需要开 worktree。一旦要改文件，就必须先开。

---

## 部署

**从 git 导出，不要 rsync 工作区。** 工作区可能含有其他会话未提交的半成品：

```bash
git archive --format=tar main <路径...> | ssh sub2api-prod 'cat > /tmp/deploy.tar'
```

### 已验证的生产连接（2026-07-30）

后续服务器操作统一从本节获取连接参数，不要从聊天记录、临时 shell 历史或其他文档猜测：

- SSH 别名：`sub2api-prod`
- 主机：`43.133.75.82`
- 端口：`2222`
- 用户：`ubuntu`
- 本机专用身份文件：`~/.ssh/tencent_lighthouse_seoul_sub2api`
- 生产部署根目录：`/opt/sub2api/production`
- Compose 项目：`sub2api`

已用只读连接验证：SSH BatchMode 登录成功；远端为 Linux；Docker context 为 `default`；`DOCKER_HOST` 为空；`sudo -n` 可用；使用生产 `/opt/sub2api/production/.env` 的 Compose 配置解析成功。文档只记录连接元数据，不记录私钥内容、密码、API Key 或 `.env` 内容。

生产操作标准前置检查：

```bash
ssh -o BatchMode=yes sub2api-prod
set -euo pipefail
test "$(uname -s)" = Linux
test "$(docker context show)" = default
test -z "${DOCKER_HOST:-}"
cd /opt/sub2api/production
test "$(pwd -P)" = /opt/sub2api/production
sudo -n true
sudo -n docker compose --project-name sub2api --env-file .env -f compose.yaml config --quiet
```

修改 `compose.yaml` 或 `.env` 前先 `cp` 一份带日期的备份。生产发布只能在该远端主机执行，不能在本机 macOS、Docker Desktop、`colima` context 或本地 `infra/.env` 上冒充生产部署。

`/ops` 页面的模板错误**编译期发现不了**（`server.go` 里是 `_ = ExecuteTemplate(...)`，错误被丢弃），改动 `ops.html` 或它依赖的结构体后，必须实际打开页面确认整页渲染完整。

## Sub2API 本体不可改

生产运行官方镜像（`xingqiao-sub2api:upstream-0.1.165`，pinned digest）。`upstream/sub2api/` 只是只读源码快照，改它不会进生产。

需要的行为必须外置到 relay-ops、Caddy、配置或其他独立发布的组件；做不到就退役该需求。详见 `docs/superpowers/specs/2026-07-24-official-sub2api-image-migration-design.md`。

## 验证

改完至少跑：

```bash
cd relay-ops-service && go build ./... && go vet ./... && go test ./... -count=1
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
ruby tests/operations/<相关测试>.rb    # 改了 ops/*.rb 时
```

`ruby -c` 只是语法检查，**不等于跑测试** —— 曾因此漏掉一个失效断言。

`tests/relay_ops/validate_relay_ops_contract.sh` 里的 `forbid` 断言是回归护栏（断言某些东西不存在），看起来像残留，删不得。
