# T98 恢复检查点

日期：2026-08-31
阶段：`CONTEXT_RECOVERY -> IMPLEMENTING`（仅候选 worktree）

## 当前现场

- 根工作区：`/Users/gongtengxinwen/Documents/sub2api搭建`
- 根分支/HEAD/tree：`main` / `788aae3c27bd18d60c16aaa2c1cfea2a8581d8e4` / `7f55d7e8c0943f56e2e013d19ef24078bc28765c`
- 根 `origin/main` 与 HEAD 的 commit/tree 一致，根工作区干净。
- T98 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-feishu-upstream-balance-notification`
- T98 分支/HEAD/tree：`codex/t98-feishu-upstream-balance-notification` / `bee0ded9bf569d804486608b7cf93de78bb8a290` / `bcdbf4be292c0b0b9601e3c08b97c5b891646c7f`
- T98 worktree 干净、无未跟踪文件；候选是当前根 `main` 的祖先，根相对候选前进 16 个提交，不能直接进入整合/发布。
- 未发现本地发布、合并、推送、迁移、停机、重启或切槽命令正在运行。

## 已确认

- T98 规格、计划、验证报告和原交接已完整存在；实现提交链已在候选中，直接相关历史验证记录为 service 15/15、notify 7/7、repository 8/8、migration 1/1、converter 4/4、server build 通过。
- T103 已在根 `main` 标记 `ABANDONED`；native-only 账号槽位约束仍是永久门禁，不恢复 admission/slow-session 或其他账号级并发控制。
- 账号 ID `231` 的生产软删除事实（`status=error`、`deleted_at=2026-08-16 14:49:43+08`）与 T98 migration 文件编号 231 无关。
- 历史 T98 退役/清理动作不可重放：不得再次停止旧 writer、DROP 旧表、覆盖/安装 secret、发送真实消息或启动旧 sender。候选中的清理命令仍是受控、默认 count-only。
- 宿主只读回报显示：生产/验收服务健康，旧 relay-ops notification 表不存在，T98 migration 231 尚未应用，通知开关关闭且未挂载 upstream-balance secret；该回报仍需本轮独立复核其时间点和发布状态。

## 未确认/待处理

- 需在只读范围确认主站/验收站当前 source commit/tree、release lock、活动槽、migration hash、sender/secret 实际状态，以及旧清理是否已完成；不得写入生产。
- 需对照根 `main@788aae3c2` 审查 T98 实现是否仍符合批准规格；若继续修改，必须只在本候选 worktree，且先刷新到最新 `main` 后重跑直接相关测试。
- 需核对项目总账中“历史已清理/已发布”描述与当前宿主事实的时间差；不以旧总账或聊天摘要重放任何动作。

## 恢复后的唯一安全动作

完成上述只读复核后，在候选 worktree 刷新到根 `main@788aae3c2`，审查 T98 diff/规格并运行直接相关验证；如剩余工作触及 migration 231、secret/开关、旧表清理、停机或真实飞书发送，则停在根总控授权门禁并更新交接。

## 停止与回滚边界

本检查点不授权合并、推送、部署、迁移、停机、重启、切槽、生产配置写入、生产数据删除或真实消息发送。保留当前 worktree、未提交内容和证据；不得 reset、checkout、clean、覆盖或删除现场。
