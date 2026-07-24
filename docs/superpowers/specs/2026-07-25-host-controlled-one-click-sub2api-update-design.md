# 宿主机受控的一键 Sub2API 更新设计

**日期：** 2026-07-25  
**状态：** 已确认  
**生产入口：** `ssh sub2api-prod`  
**生产目录：** `/opt/sub2api/production`  
**生产 Compose project：** `sub2api`

## 目标

继续完整使用 `weishaw/sub2api` 官方镜像和官方版本检查界面。管理员收到
官方 GitHub Release 更新提示后，点击原来的“立即更新”，先看到一次确认窗口，
然后选择“现在升级”或仅为该版本安排一次北京时间升级。实际变更由宿主机完成，
不再替换容器 writable layer 中的 `/app/sub2api` 二进制。

## 已确认的用户体验

1. 官方 `GET /api/v1/admin/system/check-updates` 保持直通 Sub2API，所以登录、
   打开版本面板或手动刷新时仍显示官方最新稳定版、版本说明和发布时间。
2. 点击官方“立即更新”按钮时，浏览器阻止原动作并显示星桥确认窗口。
3. 窗口显示当前版本、此次目标版本和北京时间，并提供：
   - `现在升级`
   - `本次定时升级`
   - `取消`
4. 本次定时使用 `Asia/Shanghai`，只能选择未来时间，最多存在一个未执行计划。
   新计划需要先明确替换旧计划；计划可取消，服务器重启后仍保留。
5. 计划与当时确认的官方版本和 immutable Docker digest 绑定。若执行前出现更新的
   官方版本，本次计划仍只安装已确认的目标，不自动跨版本。
6. 升级开始后页面显示已受理状态。容器重建会短暂断开当前请求；页面通过状态接口
   恢复显示最终成功、失败或已回滚结果。

## 架构

### 官方版本检查

官方 Sub2API 继续从 `Wei-Shaw/sub2api` GitHub Releases 查询最新版并使用现有
20 分钟 Redis 缓存。这个路径不经过新的更新服务，因此官方更新提示不会因宿主机
执行层而漂移。

### 无定制镜像的浏览器扩展

Caddy 对 Sub2API SPA 的 HTML 导航请求返回一个模板：模板通过内部子请求取得官方
`index.html`，原样输出后追加同源 `/xingqiao-update-ui.js`。JS/CSS/API 请求仍直接
反代官方容器。注入脚本独立于官方 bundle，主要职责是：

- 观察官方版本下拉面板；
- 捕获“立即更新”按钮的点击，不修改官方 bundle；
- 查询官方更新信息；
- 显示确认/北京时间单次调度窗口；
- 调用宿主更新 API，显示计划和执行状态；
- 页面结构变化时 fail closed：不放行原容器内二进制更新，并显示运维提示。

Caddy 同时截获 `POST /api/v1/admin/system/update`。来自旧页面、脚本失效或直接 API
调用的请求也只能进入宿主更新服务，永远不会到达官方容器的 in-place updater。
`POST /rollback` 和 `POST /restart` 继续被明确拦截；Docker 回滚必须使用镜像级流程。

### 宿主更新服务

新增 root-owned systemd 服务 `sub2api-updater.service`，通过
`/run/sub2api-updater/updater.sock` 提供 Unix socket HTTP。Caddy 只读挂载该运行时
目录并反代以下接口：

- `POST /api/v1/admin/system/update`：创建立即或本次定时任务。
- `GET /api/v1/admin/system/host-update/status`：读取当前/最近任务状态。
- `DELETE /api/v1/admin/system/host-update/schedule`：取消尚未开始的任务。

服务保存 `/var/lib/sub2api-updater/state.json`，使用原子 rename 和 `0600` 权限。
状态包含 operation ID、管理员 ID、目标版本、immutable image ref、计划时间、阶段、
结果和无秘密错误摘要。服务启动时重新加载待执行计划；已到期计划立即执行。

### 身份和请求边界

每个 API 请求必须同时满足：

- `Authorization: Bearer ...`；
- `Origin: https://api.xingqialab.top`；
- `X-Admin-UI-Request: 1`；
- JSON mutation 请求使用 `Content-Type: application/json`；
- `Sec-Fetch-Site` 缺失或为 `same-origin`。

更新服务把 Bearer、User-Agent 和代理后的客户端地址转发给官方
`GET /api/v1/auth/me`，只接受 `role=admin`、`status=active`、正数用户 ID。
不接受管理员 API Key。日志不记录 Bearer、环境文件或数据库内容。

## 更新执行

创建任务时，服务验证目标是当前 GitHub 最新稳定 Release，并拉取
`weishaw/sub2api:<version>`。只有本地 RepoDigest 与官方 repository 匹配时，才把
`weishaw/sub2api:<version>@sha256:<digest>` 写入任务。调度时间和执行时间均不使用
可变 tag。

执行阶段使用宿主机单飞锁，调用 root-owned
`/opt/sub2api/production/ops/update-sub2api-host.sh`：

1. 证明 Linux、Docker default context、目录 `/opt/sub2api/production`、project
   `sub2api` 和现有容器身份正确。
2. 记录 Sub2API、PostgreSQL、Redis、Caddy、relay-ops、D04 容器 ID。
3. 为旧 Sub2API image ID 创建不可变时间戳回滚 tag。
4. 创建并校验 PostgreSQL custom dump、`/app/data` 归档、Compose、副本、记录数和
   `SHA256SUMS`。
5. 只替换 `compose.yaml` 的 `services.sub2api.image` 为任务中的 exact digest。
6. 执行 `docker compose ... up -d --no-deps --force-recreate sub2api`。
7. 在 180 秒内验证新 image ID、health、命名卷、`/health`、记录数不下降、唯一
   `xingqiao-support` 菜单、二维码 hash、近期致命/迁移错误为零。
8. 任一检查失败时恢复旧 image ref，并只重建 `sub2api`；数据库不自动恢复。

PostgreSQL、Redis、Caddy、relay-ops 和 D04 的容器 ID必须保持不变。更新流程不调用
`docker compose down`，不停止或重建依赖服务。

## 并发、幂等与时间

- 全局最多一个 `scheduled` 或 `running` 任务。
- 同一管理员、目标 digest 和计划时间的重复提交返回现有 operation ID。
- 运行中的任务返回 `409 UPDATE_IN_PROGRESS`；已有计划返回
  `409 UPDATE_ALREADY_SCHEDULED`，由 UI 提供替换或取消动作。
- 所有持久化时间使用 UTC RFC3339；UI 输入和显示使用 `Asia/Shanghai`。
- 计划时间至少晚于服务器当前时间两分钟，最长不超过 30 天。

## 失败处理

- GitHub 或 Docker registry 查询失败：不创建任务，不修改 Compose。
- 服务器重启：systemd 自动启动服务，重新加载计划。
- 浏览器关闭：任务状态不受影响。
- Caddy 或更新服务不可用：官方检查提示仍可读取；更新按钮显示服务不可用，不触发
  容器内更新。
- 更新成功但浏览器请求断开：状态接口是最终事实源。
- 更新失败且镜像回滚成功：状态为 `rolled_back`，保留新备份和两份镜像。
- 回滚也失败：状态为 `rollback_failed`，保留全部证据，不自动恢复数据库。

## 测试与上线

1. Go 单元测试覆盖认证、Origin/header、计划时间、单计划、幂等、重启恢复、取消、
   immutable digest、状态原子写入和 executor 结果映射。
2. Shell fixture 测试覆盖只重建 Sub2API、依赖 ID 不变、备份校验、成功、健康失败
   自动回滚、错误日志和错误镜像拒绝。
3. JS DOM 测试覆盖按钮捕获、确认、立即、北京时间定时、取消、旧计划冲突和 fail
   closed。
4. Caddy 合约测试覆盖官方检查直通、mutation 只进入 updater、SPA HTML 注入一次、
   静态资源不注入和 Unix socket mount。
5. 生产先部署 updater 和 Caddy，不实际制造新版本。通过受保护状态接口、当前版本
   `already_up_to_date` 响应、计划创建/取消和所有容器 ID 不变完成验收。

## Git 与工作区收尾

功能和生产验收完成后，把当前工作区全部非忽略内容（包括此前累计任务和
`upstream/sub2api` 官方源码快照）提交到 `codex/l1-2-offline-baseline`。获取最新
`origin/main`，在无远端分叉时 fast-forward 本地 `main`，在 `main` 上复测并推送。
当前目录是主仓库本体，不删除目录；推送后删除已合并功能分支并归档当前 Codex
任务。独立 worktree `.worktrees/xingqiao-beginner-guide` 保持不动。
