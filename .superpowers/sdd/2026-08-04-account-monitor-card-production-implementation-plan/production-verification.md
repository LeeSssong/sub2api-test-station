# 账号监控 V3 生产闭环验证

**日期：** 2026-08-04
**结果：** 已完成（已推送、已部署、已验证生效）

| 验证项 | 生产证据 |
|---|---|
| 推送与发布来源 | 最终分支提交为 `82095b80770236eac24adb0bdb1b80cd639675cb`，已推送。 |
| release-state | `source_commit=82095b80770236eac24adb0bdb1b80cd639675cb`；`source_tree=3678c0b8c0427996327fe77ae126640a7e216868`；`migrations_hash=337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76`；`active_slot=green`。 |
| 运行镜像 | image/image ID：`release-82095…-12585… / sha256:12585…`。 |
| 共享服务保持 | PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 与 release-state 一致，均未重建。 |
| 页面与窗口接口 | 生产页面返回 200；24 小时、7 天、30 天接口均返回对应 `range`、7 个分组、66 个账号。 |
| 分组与卡片 | 七项原生分组汇总均存在；卡片包含优先级、评分、排名、采购成本、倍率；7 个分组均为 `rank_order_violations=[]`。 |
| 实时并发 | 并发端点中账号 `20`、`118`、`21` 均返回 `current=0`、`limit=10`。 |
| 迁移 | `procurement_cost_cny`、`procurement_cost_effective_at` 列及采购成本非负约束均存在。 |

独立的营收、账务、对账和飞书事项不属于本次 V3 闭环，仍保持各自原有状态。
