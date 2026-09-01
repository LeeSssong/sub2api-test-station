# T91-A 根总控审核结论

**审核日期：** 2026-09-01

## 结论

本任务**不能直接进入最终业务开发阶段**，但可以继续进入 **T91-A 的 acceptance schema 准备/只读基线补齐阶段**。

源码映射报告已达到“真实路径与入口已核验”的要求；不过，规格书第 10.0 和第 12 节要求的 acceptance 运行态基线尚未执行，且报告中的部分仓库/远端 SHA 已过时。因此当前结论为：

> **条件通过：允许补齐 Task 0 acceptance 基线；不允许直接修改 schema、Ent、迁移或运行时代码。**

## 必须先完成的事项

1. 在 acceptance 站按项目全局约束执行只读基线：paid 足够、paid 不足但 gift 足够、两者不足、历史负 opening、失败请求、零费用请求、重复支付回调、重复 API 请求、重复兑换、旧自动核销码。
2. 把基线的环境版本、查询时间、脱敏输出和失败场景记录到独立报告；不得读取或写入敏感凭据。
3. 刷新 Task 0 报告中的实际 `main`/`origin/main` SHA、工作区状态和当前 worktree 盘点。
4. 明确并记录统一十进制适配器的接口边界；在适配器合同未落档前，不得生成新金额字段的 Ent 代码。
5. 对 acceptance 基线报告和更新后的 Task 0 报告完成根总控复核后，才可放行 T91-A schema/Ent/迁移实现。

## 已确认无需改动的规格口径

- `attempted_quota_usd` 是应扣额度，`delta_usd` 是实际扣费；未扣差额不进入余额公式、不形成用户欠款。
- 新账务域使用 `decimal.Decimal` 与 `numeric(20,8)`；旧 Ent `float64` 必须经统一适配器进入新域。
- paid 先于 gift、桶内 FIFO；`migration_opening` 不参加 FIFO、退款或管理员扣减。
- 退款 outbox 事件、worker、claim/retry/dead-letter、unknown/reconciling 和回调定位属于 T91-D，T91-A 只核验可复用设施。
- `failed` 仅表示明确不可重试终态；渠道结果不确定时不得释放 reservation。

## 不构成的授权

本结论不构成 acceptance migration、生产迁移、历史回填、真实退款调用、双写、cutover、推送或部署授权。
