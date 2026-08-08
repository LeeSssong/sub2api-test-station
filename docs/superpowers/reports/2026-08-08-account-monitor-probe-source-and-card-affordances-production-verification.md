# 账号监控探测口径与卡片交互生产验证

验证日期：2026-08-08（Asia/Shanghai）

## 发布身份

- source commit：`c69d31da51de00347f77305208bebd630fd6daf0`
- source tree / tested tree：`11eeb8b2cf5eec54f0d39397e349dd0566280ca7`
- migrations hash：`aee795202a3dd14c191c5e395add6beb58942950bf530d9961ae80a359998429`
- image：`ghcr.io/leesssong/xingqiao-sub2api:release-c69d31da51de00347f77305208bebd630fd6daf0-cda98e2f51b06beff850f982d00ab0050420e5a51fbed01bde7269f25c77ce2c`
- image id：`sha256:cda98e2f51b06beff850f982d00ab0050420e5a51fbed01bde7269f25c77ce2c`
- transport：现有蓝绿 host executor 的 preloaded 镜像链路（未使用 GitHub Actions）

专项证据文件：`/Users/gongtengxinwen/Documents/release-staging/sub2api-monitor-evidence-v2.json`

## 线上验证

- `https://api.xingqiaolab.top/healthz`：HTTP 200。
- `https://api.xingqiaolab.top/readyz`：HTTP 200。
- `/api/v1/admin/accounts/monitor`（管理员 `x-api-key`）：HTTP 200；返回 71 个账号，其中 61 个 `evidence_source=monitor_probe`，26 个主动探测最新状态为失败；真实调用 `request_count/error_count` 仍独立返回。
- API 账号对象包含 `score_breakdown`、`evidence_source`、`homepage_url`；手工倍率账号返回 `multiplier.source=manual`，原生倍率账号返回 `multiplier.source=declared`。
- 失败探测账号仍保留探测窗口成功率、失败状态和 `eligible=false`，没有被真实调用成功率覆盖。
- 蓝绿执行器输出：`result=succeeded`、`active_slot=green`、`downtime_required=false`。
- PostgreSQL、Redis、Caddy 容器身份未变化；green API 与 worker healthy，旧 blue API 保持 healthy 作为回滚槽。

## 功能验收

- 账号卡片主质量指标文案和样本数明确为主动探测；调用明细仍使用真实请求窗口。
- 手工成本/倍率显示右上角感叹号并提示“手工维护”；原生 billing 成本标题也暴露“上游原生”提示。
- 账号名称使用 `target="_blank"`、`rel="noopener noreferrer"` 跳转 `homepage_url`。
- 评分区域悬浮提示展示成本、探测成功率、TTFT、总耗时分项及合计。
- 前端专项测试：AccountMonitorCard 25 项、AccountMonitorView 26 项，共 51 项通过；前端生产构建通过（仅既有动态导入/chunk size 警告）。

## 结论

本次功能已通过本地专项验证、生产蓝绿部署和线上健康/API 验证，发布无需停机；项目进度可标记为“已完成”。
