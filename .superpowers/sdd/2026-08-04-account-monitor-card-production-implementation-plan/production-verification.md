# 账号监控 V3 生产闭环验证

**日期：** 2026-08-04
**结果：** 已完成（已推送、已部署、已验证生效）

| 验证项 | 生产证据 |
|---|---|
| 推送与发布来源 | 最终分支提交为 `05985e62ec88b04d1e647a815eecdb1cf1155776`，tree 为 `c37b383bf54e485d7393ff0793e30dd03f5e2328`，已推送。 |
| release-state | `source_commit=05985e62ec88b04d1e647a815eecdb1cf1155776`；`source_tree=c37b383bf54e485d7393ff0793e30dd03f5e2328`；`migrations_hash=337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76`；`active_slot=green`。 |
| 运行镜像 | image：`release-05985e62e…-0d10260b…`；image ID：`sha256:0d10260b745e2086326977303b15f6eb78e8e03de7858fe356dec046bf0e10e8`。 |
| 共享服务保持 | PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 与 release-state 一致，均未重建。 |
| 页面与窗口接口 | 生产页面返回 200；24 小时、7 天、30 天接口均返回对应 `range`、7 个分组、66 个账号。 |
| 分组与卡片 | 七项原生分组汇总均存在；卡片包含优先级、评分、排名、采购成本、倍率；7 个分组均为 `rank_order_violations=[]`。 |
| 实时并发 | 并发端点中账号 `20`、`118`、`21` 均返回 `current=0`、`limit=10`。 |
| 迁移 | `procurement_cost_cny`、`procurement_cost_effective_at` 列及采购成本非负约束均存在。 |
| 状态投影收口 | #118：`service_state=available`、`eligible=true`、评分 66、排名 5、24 小时 79 请求/0 失败；#119：`service_state=available`、`eligible=true`、评分 59、排名 7、24 小时 1037 请求/0 失败。#119 最新 probe 仍为 failed，但不再覆盖达到真实请求门槛的窗口状态。 |
| 单卡刷新 | 对 #118/#119 的单卡 POST 均成功；刷新后 `checked_at` 分别推进至 `2026-08-04T18:21:21.906086Z`、`2026-08-04T18:21:22.233675Z`，证明卡片刷新不再停留在旧检查时间。 |

独立的营收、账务、对账和飞书事项不属于本次 V3 闭环，仍保持各自原有状态。
