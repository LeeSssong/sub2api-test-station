# Sub 上游逐请求真实扣费设计

## 目标

管理员打开一条用量详情时，系统自动复用该请求所用账号已经保存的上游 Sub `base_url` 和 `credentials.api_key`，读取上游原生 `/v1/usage/records` 中对应流水的 `actual_cost`，并按唯一公式展示：

```text
利润 = 本站实际扣费 - 上游 Sub 实际扣费
```

管理员区不再重复展示用户/请求区已有的“本站请求 ID”。

## 已确认根因

1. OpenAI `/v1/responses` 转发结果已从上游响应头取得 `x-request-id`，但 `openai_gateway_usage.go` 创建 `UsageLog` 时没有写入 `UpstreamRequestID`。
2. Sub 原生 `/v1/usage/records` 已声明 `upstream_request_id` 和逐笔 `actual_cost`，但 `GatewayHandler.UsageRecords` 组装响应时漏掉了 `UpstreamRequestID`。
3. `actual_cost` 不在生成响应正文中；它属于上游 Sub 的账单流水，需要用现有 API Key 调用 `/v1/usage/records` 获取。
4. 当前前端错误地经 relay-ops 查询成本，因而引入额外授权、映射和账务数据前置条件；本需求不需要这些条件。

## 数据链

### 请求 ID

- 本站 `usage_logs.request_id` 继续保存本站幂等/追踪 ID，不改变现有语义。
- 本站 OpenAI 用量记录把转发结果中的上游 `x-request-id` 写入 `usage_logs.upstream_request_id`。
- 上游 Sub 的 `/v1/usage/records` 返回每条流水自身的 `request_id`、其最终供应商的 `upstream_request_id` 和 `actual_cost`。

### 自动匹配

后台按本站用量 ID加载本地流水及其账号，仅在服务器内部读取账号凭据。查询上游时使用 `Authorization: Bearer <账号已保存 api_key>`，以本站流水时间前后各 10 分钟作为有界窗口，每页最多 1000 条并限制最多 10 页。

匹配顺序：

1. 本站 `upstream_request_id` 与上游流水 `upstream_request_id` 精确相等。
2. 本站 `upstream_request_id` 与上游流水 `request_id` 精确相等，以兼容上游 Sub 直接把其本站 ID 回传为响应头的版本。
3. 本站 `request_id` 与上游流水 `request_id` 精确相等，作为 Sub-to-Sub 透传同一客户端请求 ID 时的历史兼容回退。

所有匹配均已被该账号的 API Key 和窄时间窗限定；不按模型、Token 数或金额进行模糊猜测。

## 管理接口

新增管理员专用接口：

```http
GET /api/v1/admin/usage/:id/upstream-cost
```

返回结构：

```json
{
  "usage_id": 42,
  "local_request_id": "client:...",
  "upstream_request_id": "req_...",
  "site_actual_cost": 0.00688,
  "upstream_actual_cost": 0.004,
  "profit": 0.00288,
  "status": "confirmed"
}
```

上游不可达、响应非法、账号缺少 URL/API Key 或窗口内没有匹配流水时，接口仍返回本站金额和稳定的 `status: "unavailable"`、`reason` 枚举；`upstream_actual_cost` 与 `profit` 为 `null`。不得用本站标准费用、账号倍率成本或任何估算值代替上游真实扣费。本站用量不存在仍按 404 返回。

## 前端

管理员详情顶部只保留三项财务摘要：本站实际扣费、上游实际扣费、利润。管理员信息区删除重复的本站请求 ID，并移除 relay-ops 的“成本依据、估算、待对账、计入成本、毛利状态”等复杂口径；上游请求 ID、账号、渠道和上游端点等运维字段保留。

普通用户详情和接口不调用、不返回任何上游账号凭据或上游成本。

## 安全与边界

- 不新增授权、密钥、账号映射或五类账务数据。
- 已保存 API Key 只在后台请求头中使用，禁止写入响应、日志或错误信息。
- 只请求账号既有 `base_url`；该地址本来已被该账号用于模型转发。
- HTTP 请求使用超时、响应体大小限制和分页上限，管理员关闭详情或请求超时时可取消。
- 不回填历史数据库。历史流水能通过精确回退匹配则显示真实值，否则诚实显示不可用。
- 不改变客户扣费、定价、倍率、账号配额或任何聚合报表。

## 验收

- 新 OpenAI 流水持久化上游响应 `x-request-id`。
- Sub `/v1/usage/records` 原样返回管理员/对账所需的 `upstream_request_id`。
- 后台使用本地流水账号的既有 Bearer API Key 查询上游，并只在精确匹配后使用其 `actual_cost`。
- 后台返回的 `profit` 严格等于本地 `actual_cost - upstream actual_cost`，上游真实费用为 0 时仍是已确认数据。
- 管理员界面没有第二个“本站请求 ID”，也没有估算成本或 relay-ops 授权依赖。
- 普通用户接口和界面行为不变。

