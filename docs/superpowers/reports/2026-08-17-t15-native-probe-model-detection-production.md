# T15 账号监控原生探测模型与异步模型检测生产收口

## 发布身份

- 发布源：已推送根 `main@3e5f9393d948603019fdde212957efdbbad0d715`
- source/tested tree：`deadf8ec212b05c4555a108ba0b627bb12030112`
- 迁移哈希：`bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951`
- 0600 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-3e5f9393d-t15-maintenance-ready-v1.json`
- 宿主发布记录：`/var/lib/sub2api/release-records/20260817T174502Z-production-2353131.json`
- 结果：`succeeded/promoted`，`rolled_back=false`
- 活动槽：`green`，活动上游 `sub2api-green:8080`
- 不可变镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-3e5f9393d948603019fdde212957efdbbad0d715-9cc6d0c6d5e9cd992c2f1807c3ecd327b75289dc50d260f7b52f9b69c2f323f3`

## 维护门禁与发布

- 普通预检因新增 `225_account_model_detection.sql` 返回 `downtime_required=true`、`reason_code=migration_set_changed`、预计不可用 300 秒，并在生产变更前停止。
- 用户于 2026-08-17 明确授权停机部署。
- 根总控以 TDD 补齐唯一精确 `MAINTENANCE_11`：生产迁移集 `aaebed88f7fb712e1f518e73cc89bd44eb214f365f3b49f003598c93883a4604` 到候选集 `bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951`。未授权、错误 old/new 与未知目标均继续在停服前 fail-closed。
- 维护链仅停止 API/worker；PostgreSQL、Redis、Caddy 保持运行和原容器身份。候选 worker 应用迁移后恢复 green API，最终 API/worker 使用同一镜像、healthy、restart 0。

## 线上验收

- 公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。
- 未认证 `/api/v1/admin/account-monitors/1/models` 与 `/api/v1/admin/account-monitors/1/detection` 均返回 401，证明新增路由存在且保持管理员认证隔离。
- PostgreSQL `account_model_detection_settings` 与 `account_model_detection_runs` 两表均存在。
- API 与 worker 均未配置 `SUB2API_MODEL_DETECTOR_URL` 或 `SUB2API_MODEL_DETECTOR_TOKEN`。
- 管理员登录态 `/admin/accounts/monitor` 正常加载全站 87 个账号；账号卡片出现“模型检测”状态行和“修改连接测试模型”入口。
- `CX-Pro #190` 弹窗正常展示连接测试模型清单、检测模型清单、最近状态、保存和立即检测入口；因 sidecar 未配置，检测模型显示“检测器暂不支持”、最近状态为“不支持”，符合 fail-closed 与许可证边界。

## 保留边界

- 未取得商业书面授权或合法独立 detector 实现前，生产继续不配置 detector URL/token。
- 本次未执行 live sidecar、压力、mutation、soak 或无关浏览器矩阵；未修改生产账号、分组、评分、调度、价格、倍率或账务数据。
- 回滚依据为宿主保留的上一 blue 槽、上一不可变镜像、release-state、最终记录及维护 `.partial` 机制；迁移 225 为 add-only，不自动删除新表。
