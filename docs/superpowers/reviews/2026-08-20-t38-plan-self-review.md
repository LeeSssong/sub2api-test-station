# T38 实施计划自审

- 审查对象：`docs/superpowers/plans/2026-08-20-t38-schedulable-native-probe-score-retention.md`
- 对应规格：`docs/superpowers/specs/2026-08-20-t38-schedulable-native-probe-score-retention-design.md`
- 结论：`READY_FOR_PLAN_APPROVAL`

## 覆盖检查

- [x] RED 先锁定 pure helper 缺失和 ListWindow 当前清空行为。
- [x] GREEN 只增加共享窗口评分资格边界，不触碰状态计算、公式或排序数学。
- [x] helper 同时覆盖 latest unavailable、stale、纯失败、无样本和完整 `Account.IsSchedulable()`。
- [x] T32 paused eligible/capped 通过 current score status 保留，paused HTTP error/no evidence 不进入 T38 新分支。
- [x] global 与 group 使用同一 helper，分组成本资格继续作为附加门禁。
- [x] handler 与前端锁定“当前不可用/待确认 + 有评分/排名”的兼容组合。
- [x] 无跨窗口回退、无真实业务请求补分、无额外探测或写入。
- [x] 最小直接验证、范围扫描、回滚与根总控未验证项完整。

## 可执行性检查

- [x] 每个任务都列出精确文件、接口、命令、预期 RED/GREEN 和提交边界。
- [x] 核心 helper 的签名与最小实现已给出，后续调用点字段一致。
- [x] 条件性前端生产修改以真实 RED 为门槛，避免无关改动。
- [x] 计划执行结束状态限定为 `READY_FOR_ROOT_REVIEW`。

## 范围检查

- [x] 不修改全局队列、项目进度总账、根 `main`、发布证据或生产。
- [x] 无迁移、配置或生产数据写入。
- [x] 预计 `downtime_required=false`，根合并后预检为最终依据。

