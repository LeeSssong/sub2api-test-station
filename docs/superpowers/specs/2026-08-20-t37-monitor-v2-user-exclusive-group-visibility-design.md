# T37 Monitor V2 当前用户专属分组可见性设计

## 状态

- 阶段：`DESIGNING`
- 基线：`main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`
- 规格结论：根总控已批准
- 实施门禁：实施计划获根总控批准前，不修改业务代码

## 问题证据与当前行为

登录用户访问 `GET /api/v1/monitor-v2` 时，路由已经经过 JWT 认证，但 handler 没有使用认证主体的 `userID`。当前数据流是：

1. `MonitorV2Handler.Snapshot` 从 Gin context 读取 `user_role`。
2. 角色为 `admin` 时传入 `MonitorV2ScopeAdmin`，其他角色传入 `MonitorV2ScopePublic`。
3. `MonitorV2Service.Snapshot` 调用 `GroupRepository.ListActive()`。
4. admin scope 保留全部 active 分组，包括当前用户没有原生访问权的专属分组。
5. 服务把这些分组 ID 交给 T34 的 `ProjectMonitorV2Groups`，从 `account_monitor_results` 生成卡片并返回前端。

基线已有两项稳定复现：

- `TestMonitorV2HandlerUsesAdminScopeFromTrustedRoleContext` 证明 admin role 被转换为 admin scope。
- `TestMonitorV2SnapshotPreservesGroupOrderAndVisibility` 证明 admin scope 返回 active 专属分组，而 public scope排除它。

两项测试在基线均通过，因此问题不是前端缓存或渲染，而是后端把“管理员角色”错误解释成“拥有全部专属分组”。这也意味着只在前端隐藏会继续让接口泄露专属分组名称、平台、倍率、状态和原生探测聚合。

Sub 已有原生授权事实：`APIKeyService.GetAvailableGroups(ctx, userID)`。它综合 active 分组、`user_allowed_groups`、分组类型与有效订阅，回答当前用户可绑定的分组。T37 不重建授权表或新的权限规则。

## 目标

1. 所有当前 active 的非专属分组继续对登录用户可见。
2. active 专属分组仅在当前登录用户出现在 Sub 原生可访问分组结果中时可见。
3. admin role 本身不增加任何专属分组可见权；管理员与普通用户使用同一当前用户授权事实。
4. 在调用 T34 原生探测投影前裁剪分组 ID，使不可见专属分组既不被查询，也不进入响应。
5. 保持 Monitor V2 v7 的字段、排序、状态、指标、刷新间隔以及 `24h/7d/30d` 的 24/28/30 固定桶合同。

## 非目标

- 不修改 `user_allowed_groups`、订阅、用户角色、分组配置或分组成员。
- 不修改账号主动探测、`account_monitor_results`、T34 聚合 SQL、状态判定、评分、调度或账务。
- 不新增前端隐藏逻辑，不修改 Monitor V2 页面布局、文案或 CodexRadar。
- 不新增迁移、配置项、缓存、权限事实源或生产数据写入。
- 不修改全局队列、项目进度总账、根 `main`、发布证据或生产状态。

## 影响用户与边界条件

- 普通登录用户：公开分组不变；只看到自己原生有权访问的专属分组。
- 管理员登录用户：公开分组不变；专属分组同样只按该管理员用户自身的原生授权显示。
- 没有任何专属授权的用户：只看到 active 公开分组。
- 拥有部分专属授权的用户：只看到授权交集，不看到其他专属分组。
- 授权指向 inactive/disabled 分组：不显示，因为 Monitor V2 仍只投影 active 分组。
- `GetAvailableGroups` 返回的公开分组不决定公开可见性；公开分组仍由 `ListActive()` 保持全部可见，避免把订阅型公开分组意外改成仅订阅用户可见。
- 对专属订阅型分组，是否可见继续服从 `GetAvailableGroups` 的有效订阅语义。

## 方案比较

### 方案 A：MonitorV2Service 注入原生可访问分组读取器（采用）

让 handler 从认证主体取 `userID`，服务同时读取 `ListActive()` 与 `GetAvailableGroups(userID)`。服务保留所有 active 非专属分组，并仅在原生可访问 ID 集合中保留 active 专属分组；裁剪完成后才调用 T34 原生探测投影。

优点：权限裁剪位于后端投影边界；精确复用原生授权事实；公开分组语义保持不变；不可见分组不会进入探测查询；handler 不承担领域过滤。缺点：Monitor V2 service/provider 增加一个窄依赖，并需要最小 wire 更新。

### 方案 B：handler 调用 APIKeyService 后过滤完整快照（不采用）

先让 MonitorV2Service 按旧 admin/public scope 生成完整快照，再由 handler 删除未授权卡片。

优点：service 改动较少。缺点：不可见专属分组仍会进入 `ProjectMonitorV2Groups`；授权边界分散在 HTTP 层；其他调用者容易绕过过滤；不满足“从后端接口投影边界解决”。

### 方案 C：MonitorV2Service 直接查询 `user_allowed_groups`（不采用）

为 Monitor V2 新增 repository SQL，自己读取用户与分组授权关系。

优点：依赖表面较窄。缺点：重复 `APIKeyService.GetAvailableGroups` 已有的分组类型、订阅与用户授权语义；未来原生规则变化时会产生漂移；形成第二套权限事实。

## 采用方案与组件边界

### Handler

`MonitorV2Handler.Snapshot` 改为读取 `middleware.GetAuthSubjectFromContext(c)`：

- 认证主体存在且 `UserID > 0`：把 `userID` 传给 service。
- 主体缺失或非法：返回现有未认证错误，fail closed。
- 不再读取 `user_role`，不再构造 public/admin scope。

路由仍位于 authenticated group，handler 的主体检查是投影边界的纵深保护，不改变正常登录请求。

### Service

新增窄接口，概念签名为：

```go
type MonitorV2AvailableGroupReader interface {
    GetAvailableGroups(context.Context, int64) ([]Group, error)
}
```

`APIKeyService` 直接满足该接口。`MonitorV2Service` 保留 `GroupRepository`、T34 `MonitorV2NativeProbeReader` 与 settings reader，并新增 available-group reader。

`Snapshot` 的概念签名改为接收当前用户 ID，不再接收 role scope：

```go
Snapshot(ctx context.Context, userID int64, window MonitorV2Window, now time.Time) (*MonitorV2Snapshot, error)
```

可见性算法：

```text
activeGroups = GroupRepository.ListActive()
availableIDs = IDs(APIKeyService.GetAvailableGroups(userID))

visible(group) =
  group.status == active
  AND (
    group.is_exclusive == false
    OR group.id IN availableIDs
  )
```

服务保持 `ListActive()` 返回的稳定顺序。过滤后的 `visibleGroups` 和 `groupIDs` 一一对应，只有 `groupIDs` 被传给 `ProjectMonitorV2Groups`。

### Wire

`ProvideMonitorV2Service` 注入现有 `*APIKeyService` 作为 available-group reader。APIKey service 已属于原生服务图，不新增 provider、数据库连接或构造周期。

## 端到端数据与控制流

```text
authenticated GET /api/v1/monitor-v2
  -> JWT context AuthSubject.UserID
  -> MonitorV2Handler.Snapshot(userID, window)
  -> MonitorV2Service.ListActive()
  -> APIKeyService.GetAvailableGroups(userID)
       -> user + user_allowed_groups
       -> active groups
       -> active subscriptions
       -> native bindability rules
  -> keep every active public group
  -> keep only entitled active exclusive groups
  -> ProjectMonitorV2Groups(visible group IDs only)
       -> account_monitor_results projection
  -> existing v7 response
```

## 接口与字段合同

- HTTP 方法、路径和查询参数不变：`GET /api/v1/monitor-v2?window=24h|7d|30d`。
- `contract_version` 保持 `7`；字段结构和序列化不变。
- `groups` 的单组结构不变；本任务只收紧数组成员集合。
- `Cache-Control: no-store` 保持不变。
- 分组顺序仍由 active group repository 的现有顺序决定，过滤不重新排序。
- 24h 返回 24 桶、7d 返回 28 桶、30d 返回 30 桶。
- API 继续不返回账号 ID、账号名、凭据、单账号探测结果或授权表细节。

## 失败与安全语义

- 认证主体缺失/无效：fail closed，不生成任何 Monitor V2 快照。
- 原生可访问分组读取失败：整个请求返回错误；不得退回 public/admin role 规则，也不得假定 admin 拥有全部分组。
- active group 读取失败或 T34 原生投影失败：沿用现有错误传播，不返回部分或伪造快照。
- 用户没有专属权限：正常成功返回公开分组，不视为错误。
- 原生可访问结果含未知、重复、inactive 或非专属 ID：集合去重；inactive 仍被 active 边界排除；非专属 ID 不影响“所有 active 公开分组可见”的规则。
- 不可见专属分组 ID不得进入 `ProjectMonitorV2Groups`，避免服务内部读取超出响应权限范围的聚合。

## 兼容性、迁移与发布属性

- 前端无需修改；现有 v7 客户端只会看到更窄且正确的 `groups` 数组。
- 无数据库迁移、无 schema 变化、无配置变化、无依赖升级、无生产数据写入。
- 预计 `downtime_required=false`；实际发布预检若返回 `true`，由根总控在任何生产动作前暂停。
- 根总控负责合并后的推送、蓝绿发布和线上验证；T37 候选只到 `READY_FOR_ROOT_REVIEW`。

## 场景化验收矩阵

| 场景 | active 公开 | 获权 active 专属 | 未获权 active 专属 | inactive 分组 | 传给原生投影的 ID |
| --- | --- | --- | --- | --- | --- |
| 普通用户、无专属授权 | 显示 | 不适用 | 隐藏 | 隐藏 | 仅 active 公开 |
| 普通用户、部分专属授权 | 显示 | 显示 | 隐藏 | 隐藏 | active 公开 + 获权专属 |
| 管理员、无专属授权 | 显示 | 不适用 | 隐藏 | 隐藏 | 仅 active 公开 |
| 管理员、部分专属授权 | 显示 | 显示 | 隐藏 | 隐藏 | active 公开 + 获权专属 |
| 专属订阅已失效 | 显示 | 隐藏 | 隐藏 | 隐藏 | 不含失效专属订阅组 |
| 授权服务报错 | 不返回快照 | 不返回快照 | 不返回快照 | 不返回快照 | 不调用原生投影 |

所有成功场景还必须保持 v7 字段、顺序、`no-store`、T34 指标来源和 24/28/30 桶合同。

## 测试策略

### Handler RED/GREEN

- RED：管理员 context 只有 role、没有授权事实时不再应触发 admin-all scope；现有 admin-scope 测试应被替换为认证 `userID` 传递测试。
- 普通用户和管理员均只传 `AuthSubject.UserID`，role 不影响 service 参数。
- 缺失认证主体返回未认证错误且不调用 snapshotter。
- 保持 window 校验、`no-store` 与 v7 响应白名单测试。

### Service RED/GREEN

- active 公开分组总是保留，即使 `GetAvailableGroups` 没有返回它。
- 只保留 `GetAvailableGroups` 集合中的 active 专属分组。
- 管理员没有独立 service scope；同一 userID 的结果不随 role 改变。
- 未授权专属、inactive 分组和重复 ID 不进入 `native.groupIDs`。
- 原生授权读取错误在原生投影前返回；native reader 调用次数保持 0。
- 现有 T34 原生投影、缺失结果补全、窗口桶数、顺序和错误传播测试继续通过。

### Wire 与直接验证

- 必要 provider/wire 生成代码编译通过，确认依赖图无环。
- 运行 Monitor V2 handler/service/routes 直接相关 Go 测试。
- 运行必要 server compile/build、`gofmt`、`git diff --check` 和范围扫描。
- 不运行前端测试或构建，因为响应字段和前端代码不变；若实施实际触及前端则视为范围漂移并暂停。

## 发布、线上验证与回滚条件

根总控合并后最小发布门禁：

1. 复跑 Monitor V2 handler/service/routes 聚焦测试和必要 Go build。
2. 确认无 migration/config/frontend/GitHub Actions delta。
3. 发布预检明确输出 `downtime_required=false` 后进入既有蓝绿链。
4. 线上使用两类登录主体做 GET-only 验证：无目标专属授权的管理员不返回该专属分组；有授权用户返回该分组；公开分组与 24/28/30 桶合同保持。
5. 健康端点与 API/worker 运行状态正常。

回滚方式：由根总控使用上一活动槽/镜像执行既有蓝绿回滚。无数据迁移或生产写入，因此不需要数据回滚。

## 风险与控制

- **公开订阅组误裁剪**：若直接把 `GetAvailableGroups` 当全部可见集合，会隐藏无订阅用户的公开订阅组。控制：公开 active 分组无条件保留，原生结果只用于专属分组裁剪。
- **权限读取失败导致页面不可用**：控制：fail closed 并返回错误，禁止退回 admin-all；权限正确性优先于展示可用性。
- **角色残留造成旁路**：控制：删除 Monitor V2 public/admin scope 与 role 分支，用测试锁定 role 不参与分组可见性。
- **未授权分组仍被内部查询**：控制：测试精确断言传给 `ProjectMonitorV2Groups` 的 groupIDs 只含可见集合。
- **wire 依赖变化**：控制：最小 provider 注入并运行必要 server build。

## 仍待决事项

无产品待决事项。任务边界已明确公开分组、专属授权、管理员语义、错误策略、接口兼容、测试和发布条件。

## 规格自审

- 占位扫描：无 `TBD`、`TODO` 或未定义字段。
- 一致性：目标、算法、场景矩阵与测试均规定“公开 active 全部可见；专属 active 取原生可访问交集”。
- 范围：仅后端 handler/service/wire 与直接测试，不改前端、授权数据、探测投影或发布链。
- 歧义：明确区分公开订阅组与专属订阅组，明确授权错误 fail closed，明确裁剪发生在原生投影之前。

## 批准记录

- 2026-08-20：T37 独立任务完成现状证据、可靠复现、三方案比较、正式规格和自审。
- 2026-08-20：根总控依据用户代审授权给出 `APPROVE_SPEC_T37`，批准方案 A；下一门禁为 `APPROVE_PLAN_T37`。
