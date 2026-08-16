# OAuth 图片编辑上传 MIME 兼容热修生产收口

## 结论

- 状态：`DONE`，OAuth 精确线上重放保留一项安全约束说明。
- 最终发布提交：`3d4580c55f106193617865c59c42dbc603fee435`。
- source/tested tree：`5e5e3cecdcdaa4a36573c423c2f29b003260f0c8`。
- 迁移集合：`d3fe99bba69b0cf0cca8a7f5ec45499921f3496f58dd74c3a671d90a653589b5`，与发布前生产一致，`downtime_required=false`。
- 0600 资格证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-16-main-3d4580c55-oauth-images-mime-hotfix-v1.json`。
- 宿主发布记录：`/var/lib/sub2api/release-records/20260816T042531Z-production-727142.json`，`result=succeeded`、`state=promoted`、`rolled_back=false`。
- 活动槽：`blue`；活动上游：`sub2api-blue:8080`。

## 实现与最小门禁

- OAuth Responses 私有 MIME helper 对空 MIME 和规范化后的 `application/octet-stream` 按文件字节识别；仅接受识别为 `image/*` 的内容，无法识别或非图片内容 fail closed。
- 仅 image/mask 两个 OAuth Responses 上传 call site 使用该 helper；共享 helper、Grok、API-key multipart、错误合同、前端、依赖、配置、迁移和发布脚本未改。
- focused OAuth/API-key/multipart service tests、后端 compile-only、gofmt、diff/范围/禁区/邮箱 guards 均通过；未运行无关全仓、压力、mutation、长 soak 或重复浏览器验证。

## 发布与即时验收

- 根 `main@3d4580c55` 已推送到 `origin/main`，发布镜像同时包含已上线的 T11-R1 与本热修，不回滚前序功能。
- 预加载发布前发现生产宿主缺少 allowlist 内固定网络探针镜像；已仅恢复 `curlimages/curl@sha256:94e9e444bcba979c2ea12e27ae39bee4cd10bc7041a472c4727a558e213744e6` 的宿主镜像缓存并核对 image ID/RepoDigest。未启动额外容器、未切换业务状态、未修改生产数据。
- 公网 `/healthz`、`/readyz`、`/health` 均 HTTP `200`。
- 事故 API key `50`、`group_id=19` 使用 `gpt-image-2` 和 `application/octet-stream` PNG 调用 `/v1/images/edits`，返回 HTTP `200`、`data_count=1`，未出现 `unsupported MIME type`；API-key 链路回归通过。
- OAuth 精确公网 `/v1/images/edits` 未强行重放：生产唯一允许生图的 group 19 已按 10:02 的事故规避移除全部 OAuth 账号，账号 222/223 当前位于不允许生图的 group 20/6。为遵守“不改生产分组、不改生产数据”，未临时恢复账号、未新建/轮换 key、未绕过权限。OAuth octet-stream 编辑行为由与发布 tree 绑定的 focused tests 覆盖；该线上子项明确记为安全不可执行，不倒称线上 PASS。

## 回滚与后续

- 回滚方式：切回发布前 green 应用/worker 镜像和 Caddy 上游；无数据库回滚或数据清理。
- 可恢复 bundle：`/Users/gongtengxinwen/Documents/sub2api-archives/oauth-images-edit-mime-compat-de462d348.bundle`，权限 0600，`git bundle verify` 通过，SHA-256 `fa1b990ff68bd79da09bfbeafce81c2638c194586fe9e7b69fe044c9b4c378c8`。
- 已删除干净、已合并的 OAuth 候选 worktree、本地候选分支和本次临时 release worktree；历史、冻结 worktree 与根目录受保护内容均未修改。
- 生产临时分组规避继续保留，后续是否恢复 OAuth 账号到生图组必须另行评估，不属于本热修。
- 下一串行任务包为 T09“官方更新冲突停止与人工处理”；不得与本次收口并行进入发布车道。
