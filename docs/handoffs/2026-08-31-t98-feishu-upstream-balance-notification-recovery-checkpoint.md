# T98 恢复检查点

日期：2026-08-31
阶段：`READY_FOR_ROOT_REVIEW -> INTEGRATING`（候选已准备；仅在根总控授权下整合）

## 当前现场

- 根工作区：`/Users/gongtengxinwen/Documents/sub2api搭建`
- 根分支/HEAD/tree：`main` / `43ffa2353ed96da668d0846753f472fea922d07d` / `d5ff2d1eb48edb0e68451c432ee0666b73bbf841`（本轮只读核对时；根随后仍可能前进）
- 根 `origin/main` 与 HEAD 的 commit/tree 一致，根工作区干净（本轮恢复核对时）。
- T98 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-feishu-upstream-balance-notification`
- T98 分支/HEAD/tree：`codex/t98-feishu-upstream-balance-notification` / `34aed9938f4d4cc7c375c56057e8bc143b82db5b` / `8f7da9accdee563ea61ad4f229ac9d8c6a0764ea`。
- 候选已在本地刷新合入当前根 `main@43ffa2353ed96da668d0846753f472fea922d07d`；相对 `main` 为 ahead 6、behind 0，共同祖先为 `43ffa2353`。用户现已明确授权“合并到 main，然后快速部署到主站”，但候选仍不得直接构建或发布。
- 当前候选未提交文件（无暂存）：本恢复检查点与交接增量；代码与回归测试已提交在 `d0a4c9a1b`，不得覆盖或丢弃。
- 本轮未发现本任务的发布、合并、推送、迁移、停机、重启或切槽命令正在运行；另有其他窗口的独立 Go 测试进程，未操作它们。
- 候选工作树干净，`git diff --check` 通过；最近提交为 `34aed9938 Merge branch 'main' into codex/t98-feishu-upstream-balance-notification`。

## 已确认

- T98 规格、计划、验证报告和原交接已完整存在；实现提交链已在候选中，直接相关历史验证记录为 service 15/15、notify 7/7、repository 8/8、migration 1/1、converter 4/4、server build 通过。
- T103 已在根 `main` 标记 `ABANDONED`；native-only 账号槽位约束仍是永久门禁，不恢复 admission/slow-session 或其他账号级并发控制。
- 账号 ID `231` 的生产软删除事实（`status=error`、`deleted_at=2026-08-16 14:49:43+08`）与 T98 migration 文件编号 231 无关。
- 历史 T98 退役/清理动作不可重放：不得再次停止旧 writer、DROP 旧表、覆盖/安装 secret、发送真实消息或启动旧 sender。候选中的清理命令仍是受控、默认 count-only。
- T98 旧通知清理与 relay-ops 退役、secret 转换准备、旧发布均已有历史执行证据；本轮不重放。账号 ID `231` 的软删除事实（`status=error`、`deleted_at=2026-08-16 14:49:43+08`）与 migration 文件编号 `231` 完全无关。
- 既有报告中的生产/验收状态、旧表不存在、migration 231 未应用、通知开关关闭等均只能作为历史线索；本候选不触碰生产，也不把历史报告当作当前线上授权或写入依据。

## 未确认/待处理

- 当前根 `main@43ffa2353` 与 `origin/main` commit/tree 一致，已完成候选刷新；发布前仍须由根总控核验主站/验收站版本、锁、槽位、来源和预检结果。
- 已确认并已合入候选的代码修复：冲突 BaseURL 按 scope 隔离（`b3b4be899`），事件观测单调性/CAS 与 resolved 水位保护（`d0a4c9a1b`）。
- 直接验证证据（刷新前后代码树未改变 T98 运行时代码）：仓储 `TestUpstreamBalanceEventRepository*`、service、notify、migration 目标测试均通过；`go build ./cmd/server`、relay-ops 全包测试与三个二进制构建通过；退休/secret 合同测试通过。
- 未验证项：根总控合并后的再次门禁、迁移/发布预检、验收站/主站线上功能和真实飞书投递；若预检为 `downtime_required=true`，必须在停机/迁移前取得单独授权。
- 需核对项目总账中“历史已清理/已发布”描述与当前宿主事实的时间差；不以旧总账或聊天摘要重放任何动作。

## 恢复后的唯一安全动作

唯一下一动作：根总控确认 `main` 未漂移且无其他任务占用整合车道后，以候选 `34aed9938` 合并到根 `main`，执行 post-merge 门禁、推送和发布预检；预检为 `downtime_required=true` 时立即暂停。

## 停止与回滚边界

本检查点不授权合并、推送、部署、迁移、停机、重启、切槽、生产配置写入、生产数据删除或真实消息发送。保留当前 worktree、未提交内容和证据；不得 reset、checkout、clean、覆盖或删除现场。

## 恢复账本（本轮）

- 已执行且绝不能重放：历史旧通知退役/清表、旧 sender 发布、secret 安装/转换和任何真实飞书投递；这些动作只作为已完成历史证据。
- 仅候选代码/测试：`b3b4be899` 与 `d0a4c9a1b` 的隔离/CAS 修复及回归测试已提交；候选刷新合并提交为 `34aed9938`；checkpoint/交接增量当前待提交。
- 已完成（仅候选本地）：仓储 CAS 修复、resolved watermark 回归、候选刷新和直接验证；尚未执行：根授权合并、推送、预检后的部署/迁移、通知开关启用和生产/验收真实消息验收。
- 回滚方式：在候选中保留现有提交和未提交测试，若修复失败只停止并记录，不 reset/clean；发布前任何代码回滚由根总控在 `main` 形成可审计修复，清表后不得恢复旧通知。
