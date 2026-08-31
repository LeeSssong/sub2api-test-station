# T98 恢复检查点

日期：2026-08-31
阶段：`CONTEXT_RECOVERY -> IMPLEMENTING`（仅候选 worktree）

## 当前现场

- 根工作区：`/Users/gongtengxinwen/Documents/sub2api搭建`
- 根分支/HEAD/tree：`main` / `43ffa2353ed96da668d0846753f472fea922d07d` / `d5ff2d1eb48edb0e68451c432ee0666b73bbf841`（本轮只读核对时；根随后仍可能前进）
- 根 `origin/main` 与 HEAD 的 commit/tree 一致，根工作区干净（本轮恢复核对时）。
- T98 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-feishu-upstream-balance-notification`
- T98 分支/HEAD/tree：`codex/t98-feishu-upstream-balance-notification` / `d0a4c9a1bdbb31c10109d5ae4288c6388aa019ec` / `508174ab2618972be000ac0f854b5c296d323416`。
- 候选相对当前根 `main` 的 ahead/behind 必须以本轮收尾时重新读取为准；本次核对时根已从 `5e6ccee14` 前进到 T105 提交 `43ffa2353`，候选仍以共同祖先 `788aae3c2` 为基线且不得直接进入整合。
- 当前候选未提交文件（无暂存）：本恢复检查点；代码与回归测试已提交在 `d0a4c9a1b`，不得覆盖或丢弃。
- 本轮未发现发布、合并、推送、迁移、停机、重启或切槽命令正在运行；仅有其他窗口的长期 ssh-agent/测试背景进程，未操作它们。
- 候选工作树 `git diff --check` 当前通过；最近提交为 `d0a4c9a1b fix: guard upstream balance event observations`。

## 已确认

- T98 规格、计划、验证报告和原交接已完整存在；实现提交链已在候选中，直接相关历史验证记录为 service 15/15、notify 7/7、repository 8/8、migration 1/1、converter 4/4、server build 通过。
- T103 已在根 `main` 标记 `ABANDONED`；native-only 账号槽位约束仍是永久门禁，不恢复 admission/slow-session 或其他账号级并发控制。
- 账号 ID `231` 的生产软删除事实（`status=error`、`deleted_at=2026-08-16 14:49:43+08`）与 T98 migration 文件编号 231 无关。
- 历史 T98 退役/清理动作不可重放：不得再次停止旧 writer、DROP 旧表、覆盖/安装 secret、发送真实消息或启动旧 sender。候选中的清理命令仍是受控、默认 count-only。
- T98 旧通知清理与 relay-ops 退役、secret 转换准备、旧发布均已有历史执行证据；本轮不重放。账号 ID `231` 的软删除事实（`status=error`、`deleted_at=2026-08-16 14:49:43+08`）与 migration 文件编号 `231` 完全无关。
- 既有报告中的生产/验收状态、旧表不存在、migration 231 未应用、通知开关关闭等均只能作为历史线索；本候选不触碰生产，也不把历史报告当作当前线上授权或写入依据。

## 未确认/待处理

- 当前根 `main@43ffa2353` 已只读核对；不执行线上状态写入或发布检查，待根总控按发布门禁另行核验主站/验收站版本、锁、槽位和迁移状态。
- 已确认并已合入候选的代码修复：冲突 BaseURL 按 scope 隔离（`b3b4be899`），事件观测单调性/CAS 与 resolved 水位保护（`d0a4c9a1b`）。
- 直接验证证据：仓储 `TestUpstreamBalanceEventRepository*`、service、notify、migration 目标测试均 fresh 通过；`go build ./cmd/server`、relay-ops 全包测试与三个二进制构建通过。
- 未验证项：候选刷新到当前 `main` 后的全套直接测试、最终 diff/敏感扫描，以及根总控的迁移/发布预检。
- 需核对项目总账中“历史已清理/已发布”描述与当前宿主事实的时间差；不以旧总账或聊天摘要重放任何动作。

## 恢复后的唯一安全动作

唯一下一动作：在本候选将 `d0a4c9a1b` 与已有 T98 实现一起刷新到最新根 `main@43ffa2353`，仅解决本任务范围冲突并重跑全部直接验证。任何 migration 231、secret/开关、旧表清理、停机、发布或真实飞书发送仍停在根总控授权门禁。

## 停止与回滚边界

本检查点不授权合并、推送、部署、迁移、停机、重启、切槽、生产配置写入、生产数据删除或真实消息发送。保留当前 worktree、未提交内容和证据；不得 reset、checkout、clean、覆盖或删除现场。

## 恢复账本（本轮）

- 已执行且绝不能重放：历史旧通知退役/清表、旧 sender 发布、secret 安装/转换和任何真实飞书投递；这些动作只作为已完成历史证据。
- 仅候选代码/测试：`b3b4be899` 与 `d0a4c9a1b` 的隔离/CAS 修复及回归测试已提交；恢复 checkpoint 仍未提交。
- 已完成（仅候选本地）：仓储 CAS 修复、resolved watermark 回归；仓储 T98 定向测试通过。尚未执行：候选刷新、根合并/推送/部署、migration 231 应用、通知开关启用和生产/验收真实消息验收。
- 回滚方式：在候选中保留现有提交和未提交测试，若修复失败只停止并记录，不 reset/clean；发布前任何代码回滚由根总控在 `main` 形成可审计修复，清表后不得恢复旧通知。
