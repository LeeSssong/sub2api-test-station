# 账号评分与监控入口生产报告

**生产结论：** 完成

## 推送与运行身份

- 账号监控空值修复提交：`51df96c3d86136c3ae46c30f54250f56bd30394b`，已推送到 `origin/codex/account-monitor-release`。
- 生产实际运行源码提交：`51df96c3d86136c3ae46c30f54250f56bd30394b`。
- 生产实际运行源码树：`72e1111c438826694b717015141e6e20e4ed0c75`。
- 生产实际运行镜像：`xingqiao-sub2api@sha256:e7b207e3fba2d6494db2014a9b17c7de93a07c0fbe5aca4174f5a8fee83dbfd6`。
- 活动槽位与上游：`blue`、`sub2api-blue:8080`。
- 成功发布记录：`20260802T141956Z-production-3449210.json`；没有残留 `.partial`。

## 缺陷、修复与验证

- 首轮生产页面在加载未分组账号时出现 `Cannot read properties of null (reading 'includes')`。
- 根因是后端投影使用 `append([]int64(nil), account.GroupIDs...)`，把未分组账号的 `group_ids` 序列化为 `null`。
- 修复在投影源头改为非空切片复制，保证未分组账号返回 `group_ids: []`，没有在多个前端调用点增加兼容分支。
- 新增回归测试 `TestAccountMonitorProjectionSerializesUngroupedAccountListsAsEmptyArrays`；测试在修复前明确观察到 `null`，修复后通过。
- 目标后端回归、前端 16 项回归、`go build ./...`、`pnpm run build` 和 `git diff --check` 均通过。
- `0600` 测试证据绑定提交 `51df96c3d86136c3ae46c30f54250f56bd30394b`、树 `72e1111c438826694b717015141e6e20e4ed0c75` 和迁移哈希 `c618fc284897bb24c662297ba6cb263064a1e04a024e5432f50f082ac7317408`。

## 迁移与基础容器

- 首轮受控维护发布已将迁移集合从 `176e6659b45bffbf11f5e1fce7dfbaf60906fe974553d7156fdc516231f4f5d0` 更新到 `c618fc284897bb24c662297ba6cb263064a1e04a024e5432f50f082ac7317408`。
- `188_account_monitor_group_score_weights.sql` 与 `193_usage_log_actual_response_model.sql` 已进入生产迁移记录。
- 二次修复发布的迁移集合没有变化，因此没有重复执行无关迁移，也没有进入账务或飞书发布范围。
- PostgreSQL、Redis、Caddy 容器身份在两次发布后保持不变：`2db52788ad73`、`c45202c0d9e6`、`1a3379491955`。

## 线上 API 验收

- 评分合计 99 的请求返回 400。
- 分组 6 保存 `20/40/20/20` 时，分组 2 保持 `15/45/20/20`；分组 6 恢复默认后返回 `15/45/20/20`。
- 账号 20 的原生调度优先级已验证 `7 -> 8 -> 7`，生产数据最终恢复为 7。
- 二次发布后投影返回 `schema_version=2`、55 个账号和 17 个分组；`null_group_ids=0`、非数组 `group_ids=0`，1 个未分组账号明确返回空数组。

## 线上页面与 UI 验收

- 使用合法管理员 Chrome 会话打开 `/admin/accounts/monitor`，页面完整呈现，控制台没有业务运行时错误。
- 分组 Tab 从 `2.00x` 到 `0.10x` 降序；未开放分组显示“已关闭”，并展示不会把空账号持续作为服务故障告警的语义。
- 评分弹窗正常打开，默认值为成本优势 15、成功率 45、TTFT 20、总耗时 20。
- 将成本优势改为 14 后合计 99，保存按钮禁用；改为 `20/40/20/20` 后保存按钮启用并保存成功。
- 切换到另一分组后仍显示 `15/45/20/20`，证明组间隔离；原测试分组随后通过“恢复默认”回到 `15/45/20/20`。
- 账号 20 卡片中的“全局调度优先级”已在 UI 中验证 `7 -> 8 -> 7` 即时刷新，测试值已恢复。
- 页面明确提示评分只影响监控展示排序，真实调度仍使用账号卡片中的 `accounts.priority`。

## 回滚点

- 上一活动槽：`green`。
- 上一镜像：`xingqiao-sub2api@sha256:dc5344a63881fbff40a360f613165f694b2f2c652d3c4ac0ef7ad015455699fa`。
- 上一源码提交：`a0f3435885abe2a38aaa2d5fb8465dc24d85a3a1`。
- 恢复时使用生产 release state 和保留的 release record；不得回滚数据库、重建 PostgreSQL、Redis 或 Caddy。
