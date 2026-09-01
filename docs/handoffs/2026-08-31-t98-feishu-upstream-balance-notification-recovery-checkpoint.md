# T98 恢复检查点

日期：2026-09-01
阶段：`IMPLEMENTING`（接手后继续 T98 精度缺陷诊断；不涉及发布/生产写入）

## 当前现场

- 根工作区：`/Users/gongtengxinwen/Documents/sub2api搭建`
- 根分支/HEAD/tree：`main` / `e53c98d11a0f75822064c18f72cbce47b2dffab3` / `0436dcdb180eb8a8cc5ebf41ad72d67d73a459b8`（T105 合入后的当前事实）
- 根 `origin/main` 与 HEAD 的 commit/tree 一致，根工作区干净（本轮恢复核对时）。
- T98 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-feishu-upstream-balance-notification`
- T98 分支/HEAD/tree：`codex/t98-feishu-upstream-balance-notification` / `34aed9938f4d4cc7c375c56057e8bc143b82db5b` / `8f7da9accdee563ea61ad4f229ac9d8c6a0764ea`。
- T98 代码/allowlist 已先合入并推送根 `main@b13980c98`，随后根总控又合入 T105，当前推送根为 `main@e53c98d11a0f75822064c18f72cbce47b2dffab3`、tree `0436dcdb180eb8a8cc5ebf41ad72d67d73a459b8`；当前 T98 维护目标以该最新根为准。
- 当前候选未提交文件（无暂存）：本恢复检查点与交接增量；代码与回归测试已提交在 `d0a4c9a1b`，不得覆盖或丢弃。根 `main` 当前无未提交文件；根总控的 T105 发布尝试未改变候选文件。
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

- 当前根 `main@e53c98d11` 与 `origin/main` commit/tree 一致；此前普通预检因 `migration_set_changed` fail-closed，未进入生产变更。T105 另一窗口的发布尝试已结束且未生成新的生产记录。
- 已确认并已合入候选的代码修复：冲突 BaseURL 按 scope 隔离（`b3b4be899`），事件观测单调性/CAS 与 resolved 水位保护（`d0a4c9a1b`）。
- 直接验证证据（刷新前后代码树未改变 T98 运行时代码）：仓储 `TestUpstreamBalanceEventRepository*`、service、notify、migration 目标测试均通过；`go build ./cmd/server`、relay-ops 全包测试与三个二进制构建通过；退休/secret 合同测试通过。
- 未验证项：本次维护发布、验收站/主站线上功能和真实飞书投递；用户已明确批准必要停机，本次只允许使用宿主核对出的 active hash `88a0ff14…`。
- 需核对项目总账中“历史已清理/已发布”描述与当前宿主事实的时间差；不以旧总账或聊天摘要重放任何动作。

## 恢复后的唯一安全动作

唯一下一动作：在当前干净根 `main@e53c98d11` 完成 T98 直接证据后，以 `--maintenance-authorized --maintenance-from-hash 88a0ff14…` 启动唯一维护发布；任何 hash/锁/来源漂移立即停止。

## 停止与回滚边界

本检查点不授权停机、迁移、重启、切槽、生产配置写入、生产数据删除或真实消息发送；“快速部署到主站”已执行到预检门禁为止。保留当前 worktree、提交、证据和宿主门禁输出；不得 reset、checkout、clean、覆盖或删除现场。

## 恢复账本（本轮）

- 已执行且绝不能重放：历史旧通知退役/清表、旧 sender 发布、secret 安装/转换和任何真实飞书投递；这些动作只作为已完成历史证据。
- 仅候选代码/测试：`b3b4be899` 与 `d0a4c9a1b` 的隔离/CAS 修复及回归测试已提交；候选刷新合并提交为 `34aed9938`；根发布提交为 `1827057cb`。
- 已完成：仓储 CAS 修复、resolved watermark 回归、候选刷新、根合并、推送、post-merge 直接验证和发布预检；尚未执行：停机/迁移、通知开关启用和生产/验收真实消息验收。
- 回滚方式：在候选中保留现有提交和未提交测试，若修复失败只停止并记录，不 reset/clean；发布前任何代码回滚由根总控在 `main` 形成可审计修复，清表后不得恢复旧通知。

## Latest Gate Update

- T105 外部发布尝试已安全结束，未产生新的生产发布记录；宿主仍为 active slot `green`、source `a928c671d`、migration hash `88a0ff14…`，release lock/partial 均为空。
- 根 `main` 已由 T105 窗口推进并推送到 `e53c98d11a0f75822064c18f72cbce47b2dffab3`（tree `0436dcdb180eb8a8cc5ebf41ad72d67d73a459b8`），包含 T98 `MAINTENANCE_19` allowlist；T98 维护发布必须以该最新根为目标。
- 用户已明确批准必要停机。恢复后的唯一动作是：重新核对根/远端/宿主锁与 active hash，生成绑定当前根 tree 的 T98 证据，再使用 `--maintenance-authorized` 和精确 active hash 执行；若状态漂移立即停止。

## Production Result And Prior Blocker (resolved below)

## Permission Fix Completed

- 用户已明确批准并已执行 ownership 修复：目录和五个文件均为 `1000:1000`，权限保持 `0700/0600`，内容未读取或修改。
- 仅 `sub2api-worker` 已重启并恢复 `healthy`；UID 1000 可读全部挂载文件。PostgreSQL、Redis、Caddy 保持 running 且身份未变。
- 脱敏日志计数：`notification subsystem disabled=0`、`secret_unavailable=0`、凭据签名=0、通知发送错误=0；release lock/partial=0；主站健康端点均 200。
- T98 当前不再因 secret 权限阻塞；真实群送达仍未主动制造或验证，后续不得重复重启/改 secret，除非有新的明确授权。

- 主站维护发布已成功：source `c651bcb7` / tree `79144c1c`、migration `0bda54bb`、活动槽 `green`；验收站运行同一镜像且健康。宿主 release lock/partial 均已释放。
- （历史阻塞，已解决）worker 曾因 root-owned `0700/0600` secret 无法读取并记录 `secret_unavailable`；现已按授权改为 UID 1000 ownership 并完成仅 worker 重启，当前不再报错。

## 接手记录（2026-09-01）

- 本窗口正式接手 T98 缺陷诊断；用户明确要求继续工作。当前仅处理只读证据与候选实现，不执行发布总控动作。
- 已确认根因假设：`TIMESTAMPTZ` 写入/读取为微秒精度，而 JSON `observed_at` 可保留纳秒；`time.Time.Equal` 在 lease/current 与 evaluation/lease 两处造成合法同一观测被判不匹配，静默跳过发送。
- 下一步：在候选 worktree 先补纳秒截断回归测试并验证 RED，再实现统一微秒归一化；完成后更新 handoff，交 `READY_FOR_ROOT_REVIEW`。

## 精度缺陷修复记录（2026-09-01）

- TDD RED：新增两个回归测试，验证 JSON `observed_at` 含 `123456789` 纳秒、事件账本仅保留微秒时，lease/current 与 evaluation/lease 原严格 `time.Equal` 均失败。
- 最小修复：新增 `upstreamBalanceObservationTimeEqual`，双方先 `Truncate(time.Microsecond)` 再比较；仅应用于最终投递指纹的两处时间比较，不改变余额、状态、generation、lease 或 CAS 语义。
- TDD GREEN：两个纳秒/微秒回归测试通过；`go test ./internal/service -run 'TestUpstreamBalance'` 通过；`go test -vet=off ./internal/repository -run 'TestUpstreamBalanceEventRepository'` 通过；notify 相关测试、`go build ./cmd/server`、`git diff --check` 通过。
- 未验证/基线阻断：仓储包默认 vet 仍被既有 `internal/repository/usage_log_repo_stats.go:1004` 的 `fmt.Sprintf` 参数数量错误阻断；未修改该无关文件。未执行生产/验收站/真实飞书动作。

## T105-R1 后刷新交接（2026-09-01）

- 根总控已完成 T105-R1 主站/验收站同 commit/tree 发布；T98 候选已先刷新到 `main@c828b9574`，随后因根总控新增 T106 登记再次刷新到最新 `main@b2d504797`，两次均无冲突。
- 当前候选：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-feishu-upstream-balance-notification`，分支 `codex/t98-feishu-upstream-balance-notification`，HEAD `999c3c1ee96d5ca78aa01343346d75dea1841453`，tree `40fff2f01e937f66989225b5e246a514583d6dc4`；相对当前本地 `main` ahead 6、behind 0，工作树干净。
- 刷新后直接验证：`go test ./internal/service -run 'TestUpstreamBalance' -count=1`、`go test -vet=off ./internal/repository -run 'TestUpstreamBalanceEventRepository' -count=1`、notify 相关测试、`go build ./cmd/server`、`git diff --check` 全部通过。
- 默认仓储 vet 的既有 `usage_log_repo_stats.go:1004` 格式参数错误仍未修改；本任务未执行任何生产/验收站/真实飞书动作。
- 状态：`READY_FOR_ROOT_REVIEW`。请根总控仅按单车道继续根审、整合和发布，不要从本候选直接部署。
