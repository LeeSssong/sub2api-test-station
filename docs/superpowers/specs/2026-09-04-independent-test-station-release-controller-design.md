# 独立测试站发布控制器设计

## 问题与现状

独立测试站运行于 `sub2api-test-station` Compose project，入口为 `http://49.51.203.200/`，部署根目录为 `/opt/sub2api-test-station/`。当前只有克隆时使用的临时链路；仓库内旧 `ops/release-sub2api-acceptance.sh` 面向历史 `/admin/lab` 拓扑，不能发布新站。测试站当前 release 由 `release-state.json` 记录 source commit/tree，运行 API、worker、detector 使用带提交标识的不可变镜像。

## 目标与非目标

目标是新增一个 fail-closed 的本地/宿主发布链：从根目录干净 `main` 构建 Linux/amd64 应用镜像，生成独立测试站 release bundle，传输至测试站并以专用 Compose project 原子切换；保存 source commit/tree、镜像 digest、旧 release 回滚信息，并执行健康检查。

非目标包括主站发布、旧 `/admin/lab` 发布、数据库/Redis/对象存储迁移、生产凭据复制、GitHub Actions、业务代码修改和真实功能验收。

## 方案与选择

方案 A：复用旧验收脚本，仅替换路径。不采用，因为它仍依赖旧环境变量、旧 Caddy 拓扑和旧 project，容易误触主站或旧验收实例。

方案 B：新增专用本地脚本和远端执行器，复用现有镜像构建与 Compose 原子切换习惯。采用此方案，边界最清晰，能在本地完成来源门禁并在远端只操作一个 project。

方案 C：仅推送 Git 后在服务器直接拉取并构建。不采用，因为服务器 Git 工作树和凭据来源不可作为发布来源，且无法保证制品与 `main` tree 可追溯。

## 架构与控制流

`ops/release-sub2api-test-station.sh` 在本地执行：校验当前 worktree 为 `main`、干净且 `HEAD/tree` 与 `origin/main` 一致；校验固定 SSH alias、0600 key/known_hosts；通过 `docker buildx` 构建应用镜像并 `docker save`，计算 archive SHA-256；复制专用 compose/Caddy 和非秘密配置模板，生成临时 bundle。脚本只接受固定远端 `sub2api-test-station`、根目录前缀 `/opt/sub2api-test-station/` 和 project `sub2api-test-station`，并经 SSH 将 bundle 交给远端执行器。

`ops/deploy-sub2api-test-station-host.sh` 通过 stdin 接收并解析参数，在远端 root 下校验 staging 路径、bundle checksum、source metadata 和 compose identity；将镜像导入独立 Docker daemon，创建带 source commit 名称的 release 目录，写入仅含非秘密运行元数据的 `.env`，执行 `docker compose ... up -d --remove-orphans`，等待 API/worker/detector/Caddy 健康；成功后原子更新 `release-state.json`，失败则保留新目录并恢复旧 Compose 状态，不删除独立 named volumes。

## 接口与字段

本地脚本环境变量：`RELEASE_WORKTREE`、`TEST_STATION_SSH_TARGET`（固定默认 `sub2api-test-station`）、`TEST_STATION_SSH_KEY`、`TEST_STATION_KNOWN_HOSTS`、`TEST_STATION_DEPLOY_ROOT`（固定前缀）、`TEST_STATION_ENV_FILE`。远端执行器参数：`--staging-root`、`--image-archive`、`--image-sha256`、`--compose`、`--caddy`、`--env-file`、`--source-commit`、`--source-tree`、`--deploy-root`。

状态字段为 `source_commit`、`source_tree`、`image_digest`、`release_dir`、`previous_release_dir`、`project_name`、`result`、`updated_at`；不得写入密码、token、私钥或完整 env。

## 失败、安全与回滚

来源、路径、project、checksum、SSH 身份或健康检查任一失败即停止，且在远程 staging 之外不写入。Compose 只使用 `--project-name sub2api-test-station` 和活动 release 内的专用 compose 文件；禁止 `docker compose down -v`、删除 `sub2api-test-station-*` volumes、调用主站脚本或读取旧验收 env。切换失败时保持旧 release 运行并保留新 release 和证据；人工回滚通过旧 `release-state.json` 指向的 compose 文件执行，不从旧 commit 临时构建。

## 兼容、验收与测试

现有测试站数据卷和独立管理员保持不变；应用镜像 tag 绑定 source commit，Caddy 使用现有独立站配置。契约测试覆盖：非 `main`、脏树、origin 漂移、错误目标身份、非 0600 凭据、错误 compose project、路径穿越、checksum 失败、健康失败回滚，以及成功命令只使用独立 project。上线后核对 release-state、六服务 healthy、`/health`、`/readyz` 和根路径。

## 发布与回滚条件

只有根目录 `main` 已推送且 `HEAD/tree == origin/main` 时才可发布测试站。发布不构成主站授权，也不自动同步主站。失败保留候选 worktree、bundle 和远端 release；回滚使用上一已验证 release 的 compose/Caddy 与状态文件。

## 未决事项

本设计不改变测试站当前 `.env` 的秘密值；发布脚本只读取远端既有 env 的变量名和值摘要，不打印内容。真实支付、消费、上游和通知仍需管理员另行验收。
