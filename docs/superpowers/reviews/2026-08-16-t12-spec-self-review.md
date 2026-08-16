# T12 规格自审（修订）

审查对象：`2026-08-16-t12-native-probe-cost-design.md`。

- **身份约束已闭环：** 已核对当前 `usage_logs` 的 `user_id`、`api_key_id`、`account_id` 必填外键；修订方案不再写无身份 probe 行，也不放宽约束、不使用占位 ID。
- **事实源边界已闭环：** 独立 append-only `account_probe_cost_logs` 只服务本站探测成本，明确不替代、不混入 `usage_logs`；用户余额、用户用量、普通管理员统计和六项财务继续只读 `usage_logs`，无需依赖隐含来源过滤。
- **现有字段边界已闭环：** `usage_completeness` 沿用当前 `complete/partial/unknown` 语义；本方案不虚构不存在的 `usage_source`。probe 表自身即来源隔离边界，`probe_run_id` 唯一约束防止同一 attempt 重复计入。
- **数据与迁移已闭环：** 需要 add-only 新表/索引/约束 migration；不做历史迁移/回填的含义已明确为不生成、更新或重算历史业务数据。发布预检仍必须输出 `downtime_required`，若为 true 需人工确认。
- **财务安全已闭环：** probe 成本永不进入账号计费、用户扣费、利润、利润率或余额；缺失 Token/价格只显示不完整，不估算。
- **USD 与兼容已闭环：** 现有未消费金额字段、接口名称和 DTO 完全保持；T12 只维持/校正经营页 USD 两位显示，不新增字段、alias、deprecation、余额迁移或其他页面改造。
- **范围无漂移：** 仅覆盖探测写入链路、account-financial 读模型/API 和经营页 UI；未包含调度、用户入口、历史数据、外部控制面、余额 DTO 重构或部署动作。

结论：`PASS（待根总控书面审阅）`。本轮仍未调用 `writing-plans`，未写实现/迁移/测试，未访问生产。
