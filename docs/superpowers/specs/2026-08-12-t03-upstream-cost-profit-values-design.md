# T03 上游扣费与利润始终有值设计

## 目标

管理员在原生用量详情查看一条 Sub/New 流水时，只要已通过现有精确请求 ID 规则命中上游原生账单记录，“上游实际扣费”和“利润”必须是数值。上游账单明确返回扣费时使用原值；该字段为空、缺失或 `null` 时按 `0`；利润始终由后端按 `本站实际扣费 - 上游实际扣费` 计算。

## 既有链路与根因

- 管理员详情调用既有 `/api/v1/admin/usage/:id/upstream-cost`，普通用户没有该接口。
- `SubUpstreamCostService` 已使用本地 `upstream_request_id`、上游记录 `upstream_request_id` 与本地 `request_id` 的严格优先级精确匹配 Sub/New 原生账单。
- 前端只展示后端 `status=confirmed` 的 `upstream_actual_cost` 与 `profit`，并且已能把 confirmed zero 显示为金额，不会把 `0` 当成空白。
- 当前 Sub 记录把 `actual_cost` 解码为非空 `float64`，New 记录把 `quota` 解码为 `json.Number`；空字符串或 `null` 会分别导致解码失败或后续换算失败，最终返回 `unavailable`，因此管理员看到 `-`。

## 设计

### 原生数值归一化

- 为 Sub 的 `actual_cost` 和 New 的 `quota` 使用同一个只服务于上游账单字段的可空 JSON 数值类型。
- 接受 JSON number 与数值字符串。
- JSON `null`、缺失字段和空字符串表示原生空白，归一为零。
- 非空但不是有效有限数值的字段仍视为上游响应不可用；不得静默归零。
- New 的 `quota_per_unit` 是单位换算元数据，不是“实际扣费”字段；缺失或非法时仍保持 `unavailable`，不伪造账单。

### 命中与状态

- 只有严格精确匹配到一条原生账单记录后，才应用空白即零规则并返回 `status=confirmed`。
- 凭据缺失、端点/鉴权/网络/响应失败、分页失败、没有精确记录仍返回 `status=unavailable`，成本和利润保持 `null`。
- 不改变匹配顺序、时间窗、页数、端点发现或凭据读取。

### 权限与前端

- 继续使用管理员专属路由；普通用户 DTO、接口和弹窗不新增成本字段。
- 前端现有 confirmed-zero 投影与格式化已满足展示要求，不修改页面结构和交互。

## 非目标

- 不做估算、模糊匹配、对账状态、异常标记、历史回填或 relay-ops。
- 不新增数据库字段、迁移、配置或 GitHub Actions。
- 不修改账号调度、倍率、客户计费或上游凭据。

## 验收

1. Sub 精确记录的明确扣费返回原值与数值利润。
2. Sub 精确记录的 `null`、缺失或空字符串扣费返回 `0` 与数值利润。
3. New 精确记录的明确 quota 返回换算后的扣费与数值利润。
4. New 精确记录的 `null`、缺失或空字符串 quota 返回 `0` 与数值利润。
5. 非空非法原生数值、单位换算元数据缺失或原生记录未命中仍返回不可用。
6. 管理员处理器返回上述数值；普通用户接口与 DTO 不暴露成本和利润。

## 发布属性

- 预期无迁移、无配置、无停机，`downtime_required=false`。
- 候选线程只做到 `READY_FOR_ROOT_REVIEW`；不得合并、推送或部署。
