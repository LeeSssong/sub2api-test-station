# T03-R1 全分支终审证据草稿

状态：`PENDING_INDEPENDENT_REVIEW`。本文件不是 APPROVE，也不产生 `READY_FOR_ROOT_REVIEW`。

候选：`codex/t03-r1-upstream-cost-persistence@32f8ec12572ad9d2e50eab054788d4ec0bf05454`
tree：`ab19d7807763e1fcd03d0d0cd3f8f49d335f9fbb`
merge base：`19492c57da24270eb2b3e9b5d9727c2865aebb9e`

## 独立 reviewer 必须核对

1. Task 1–7 与批准规格/计划一致，旧 `usage_logs` 直接扩字段未回归。
2. Sub/New 仅响应后单次终态登记，无回填、补查、重试、估算或读时上游 HTTP。
3. 222 仅新增独立表/索引，官方 `usage_logs` 结构/插入语义未改变。
4. 覆盖截止点、异常核对、OAuth、北京自然日、财务公式、审计和管理员鉴权正确。
5. 使用记录异常 Tab 的 URL、筛选/分页/导出/批量核对、evidence detail/source 和 NewAPI empty-quota 证据完整；普通用户不泄露。
6. 无 GitHub Actions、external-primary、relay-ops 主账务路径或 T05 越界。
7. 明确裁决已知 blocker：`internal/handler/admin/usage_handler.go` 中 `SubUpstreamCostService.GetByUsageID` nil-service fallback 是否阻断管理员读取零 upstream HTTP 合同。

任务级测试证据见同日期 `task-review`。`stash@{0}` 保留，未恢复或删除。终审前不得合并 main、推送、部署或启动 T05。
