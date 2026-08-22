# T52 调度公平性与管理员实时参数页设计

## 1. 背景与原生能力

生产只读证据显示，`GPT-Pro` 近 24 小时 341 次请求中前 6 个账号承载 334 次（97.9%）；27 个可调度账号中多 个 priority=3 账号最近使用时间停留在 2026-08-17。`GPT-特惠分组` 近 24 小时 880 次请求中前 2 个账号承载 499 次（56.7%）。当前高级调度已启用，DB `lb_top_k` 覆盖为空，运行时使用配置默认 `lb_top_k=7`。

原生能力已经提供：账号资格过滤、S1/S2 veto、sticky、评分权重、固定 Top-K、`last_used_at` 和管理员 `settings` API/SettingsView。T43 关闭了 adaptive Top-K，但固定 Top-K 与 priority/score 排序仍会让浅池中低优先级账号长期无法进入候选。因此本任务只扩展原生调度和原生设置，不建设第二套控制面或账号事实源。

## 2. 目标与非目标

### 目标

- 在不破坏健康/余额/故障域/sticky/重试语义的前提下，让长期未使用的健康账号获得可解释、可控的探索机会。
- 为管理员提供实时生效的公平调度参数，并在每个字段旁显示作用、范围和效果。
- 支持全局默认值以及按分组覆盖值；未配置分组覆盖时回退全局值。
- 展示当前生效值，避免管理员误以为保存后需要重启。

### 非目标

- 不改账号准入、S1/S2 veto、sticky 绑定、并发上限、重试预算、计费、usage_logs 或评分事实源。
- 不新增数据库迁移；参数继续存储在原生 `settings` key/value 表。
- 不强制严格 round-robin，不保证每个账号每个请求等量使用；硬资格和 sticky 永远优先。
- 不新增普通用户入口、外部控制面或 GitHub Actions。

## 3. 方案比较

1. **纯全池 round-robin**：覆盖最宽，但忽略质量、负载和上下文，容易把慢/临界账号直接推入主流量；不选。
2. **只调大 Top-K**：改动小，但 priority/score 仍可能让低优先级账号长期饥饿；不选。
3. **混合公平调度（推荐）**：主车道保留现有评分/Top-K；探索车道从全部合格账号按最久未使用排序，按比例或饥饿阈值插入。既保留质量保护，也能让浅池健康账号恢复覆盖；选择此方案。

## 4. 参数合同

全局 settings key：

- `openai_advanced_scheduler_candidate_pool_mode`: `top_k`、`all_eligible`、`hybrid`，默认 `hybrid`。
- `openai_advanced_scheduler_exploration_ratio`: 0-100 的整数百分比，默认 `20`。仅影响新请求；sticky 命中仍优先。
- `openai_advanced_scheduler_starvation_threshold_seconds`: 0 或 300-86400 秒，默认 `21600`（6 小时）。健康账号超过阈值未使用时进入探索优先队列；0 表示关闭硬阈值。
- `openai_advanced_scheduler_fairness_weight`: 0-10 的非负小数，默认 `2`。对探索排序中的 idle-age 归一化分数加权；0 表示只按最久未使用。
- `openai_advanced_scheduler_group_overrides`: JSON 对象，键为 group id，值只允许覆盖上述四项；空对象表示没有覆盖。

运行时有效值按 `group_id` 选择覆盖；覆盖字段缺失则回退全局。配置非法时整个更新 fail-closed，保留旧值。配置缓存沿用现有 5 秒 settings cache，保存成功后立即失效，目标生效延迟不超过 5 秒。

## 5. 调度数据流

1. 现有 `listSchedulableAccounts`、模型/能力、S1/S2 和利润控制过滤先执行。
2. sticky、forced retry 和半开探测路径保持原顺序，不进入公平探索分支。
3. `top_k`：保持当前固定 Top-K 行为。
4. `all_eligible`：所有合格账号参与当前评分排序，仍使用现有加权选择。
5. `hybrid`：
   - 若存在超过 starvation threshold 的合格账号，选择最久未使用的健康账号作为探索候选；
   - 否则按 exploration ratio 从全部合格账号建立探索候选，其余请求走现有 Top-K；
   - 探索候选只改变候选顺序，不绕过负载/资格/并发最终检查；抢槽失败继续走现有选择预算和 fallback。
6. 选择成功后沿用现有 `last_used_at` 更新和 scheduler metrics；不新增账务或审计事实源。

## 6. 管理员页面

复用 `SettingsView.vue` 的 OpenAI 高级调度区域，增加公平性分组。每个参数显示：字段名称、当前生效值、输入控件、单位/范围，以及一行小号说明“它控制什么”和“调大/调小会怎样”。

- 候选池模式使用 select，并说明三种模式对质量与覆盖的取舍。
- 探索比例、饥饿阈值和公平性权重使用 number input；保存前显示范围校验。
- 分组覆盖使用分组下拉 + 覆盖表格；每行可清除单字段，清除后回退全局。
- 页面显示当前全局值、选中分组的有效值和“保存后约 5 秒内生效”提示。
- 中英文 locale 同步；移动端单列，不产生横向滚动。

## 7. API 与兼容性

继续使用 `GET/PUT /api/v1/admin/settings`。响应增加全局配置、有效值字段和分组覆盖 JSON；旧客户端省略新字段时由服务端保留旧值。新安装默认 hybrid/20%/6h/2，现有实例缺失 key 时按同样默认回显。

## 8. 测试策略

- Go：参数解析/范围校验/JSON 覆盖回退；`top_k` 兼容行为；`all_eligible` 覆盖所有合格账号；`hybrid` 的 threshold、ratio、sticky 优先和资格过滤；设置更新立即读取。
- 前端：字段说明存在、模式切换和范围校验、分组覆盖编辑/清除、旧 payload 回显、移动端布局合同。
- 直接门禁：相关 Go package tests、`go build ./cmd/server`、相关 Vitest、`pnpm typecheck`、`pnpm build`、`git diff --check`。

## 9. 发布、回滚与验收

- 无迁移、无生产数据回填；预期 `downtime_required=false`。
- 发布后读取管理员 settings API，确认默认/保存值和分组覆盖生效；观察 24h 账号覆盖率、最大账号占比、最久未使用账号和 Top-K 过滤率。
- 回滚优先将新参数恢复为空/旧 `top_k` 模式；二进制回滚沿用上一已验证蓝绿镜像。
- 失败时保留候选 worktree 和证据，不清理，不绕过发布链。

## 10. 验收矩阵

| 场景 | 预期 |
|---|---|
| sticky 会话请求 | 继续命中既有 sticky 账号，公平探索不抢占 |
| Pro 浅池存在 >6h 未用账号 | hybrid 将其放入探索候选，且仍经过最终资格/并发检查 |
| 所有账号刚使用过 | 按 20% ratio 进入全池探索，其余按旧 Top-K |
| group override 只覆盖 ratio | 该组 ratio 使用覆盖，其他字段回退全局 |
| 非法 ratio/threshold/JSON | PUT 400，旧配置保持不变 |
| 关闭高级调度 | 新公平覆盖不生效，回到配置/默认 Top-K |
| 旧客户端不发送新字段 | 现有调度设置不被清空 |

