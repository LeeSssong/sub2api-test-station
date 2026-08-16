# T12 设计交接（修订）

- 任务：T12 经营页本站探测花费与排序 / USD 字段优化
- 状态：`READY_FOR_ROOT_REVIEW`（仅 docs-only；等待根总控书面审阅/批准）
- 基线：`main@c42b5b8cc`（本 worktree 以最新 main 为基线）
- 当前 worktree：`/Users/gongtengxinwen/.codex/worktrees/1475/sub2api搭建`
- 分支：`codex/t12-native-probe-cost-design-recovery`
- 规格：[2026-08-16-t12-native-probe-cost-design.md](../superpowers/specs/2026-08-16-t12-native-probe-cost-design.md)
- 自审：[2026-08-16-t12-spec-self-review.md](../superpowers/reviews/2026-08-16-t12-spec-self-review.md)
- 唯一承载选择：新增独立 append-only `account_probe_cost_logs`，仅记录本站探测成本；绝不替代或混入 `usage_logs` 用户计费事实源。因 `usage_logs.user_id/api_key_id/account_id` 均必填，旧的“无身份 probe 行写 usage_logs”方案已废弃。
- 迁移语义：需要 add-only 新表/索引/约束 migration；“不做历史迁移/回填”是不生成或改写历史业务数据。发布预检必须输出 `downtime_required`，若为 true 在动作前停下等待人工确认。
- 范围：探测写入链路、account-financial 读模型/API、经营页 UI；保留六项排序、本站探测卡片/账号列、USD 两位展示和内部原始精度。现有未消费金额字段/接口名/DTO 保持兼容，只校正经营页显示语义；不新增 alias/deprecation、余额字段迁移或其他页面改造。
- 明确隔离：用户余额、用户用量、普通管理员统计、账号计费、用户扣费、利润和利润率均继续只读 `usage_logs`；probe 表只提供独立探测字段，普通用户 DTO 永不返回。
- 本轮禁止：未调用 `writing-plans`；未写实现、迁移、测试；未修改 `docs/project/*`；未合并、push、部署或访问生产。
- 根总控下一步：审阅规格与自审；批准后另行决定是否进入 writing-plans 阶段。若退回，继续在本 worktree 修订并生成新 docs-only commit。
