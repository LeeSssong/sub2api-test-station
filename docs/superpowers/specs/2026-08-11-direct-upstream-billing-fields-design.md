# 上游原生逐笔扣费字段直读修正

## 目标

管理员查看单笔用量时，只读取上游原生账单字段：

- Sub2API：精确匹配原生 `/v1/usage/records` 流水后读取 `actual_cost`。
- New API：精确匹配原生 `/api/log/token` 流水后读取 `quota`，再除以原生 `/api/status` 的 `quota_per_unit`。

不引入估算、价格表换算、模糊匹配、relay-ops 账务前置条件或新的凭据。

## 已确认问题

现有服务已实现上述字段直读和精确请求 ID 匹配，但 New API 原生接口地址复用了通用地址构造器。当账号推理地址为 `https://host/api/v1` 时，移除末尾 `/v1` 后仍保留 `/api`，再拼接 `/api/log/token`，最终得到错误的 `/api/api/log/token`；`/api/status` 同样重复。这会被上游返回 404，并表现为 `endpoint_unsupported` 与空扣费。

## 修正

New API 原生管理接口的地址构造必须把推理地址的版本后缀归一化到站点根路径：

- `https://host/v1` → `https://host/api/log/token`
- `https://host/api/v1` → `https://host/api/log/token`
- `https://host/api` → `https://host/api/log/token`

仅消除与目标原生接口重复的 `/api` 前缀；不改变 Sub2API `/v1/usage/records` 地址、不改变账号探测、请求 ID 持久化或费用公式。

## 匹配与金额边界

- 只接受 `request_id` / `upstream_request_id` 精确匹配，保留现有优先级。
- Sub2API 金额只取匹配流水的 `actual_cost`。
- New API 金额只按 `quota / quota_per_unit` 计算；退款类型继续沿用现有负号语义。
- 任一原生字段缺失、非法、接口不可达或没有精确匹配时，继续返回 `unavailable`，不得用估算值补空。

## 验收

使用真实 `httptest.Server` 模拟 New API，账号 `base_url` 为 `/api/v1`；服务必须请求且只请求 `/api/log/token` 与 `/api/status`，精确匹配后返回 `status=confirmed`、正确上游实际扣费和利润。Sub2API 现有 `actual_cost` 定向测试继续通过。
