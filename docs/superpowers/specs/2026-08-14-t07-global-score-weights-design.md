# T07 全局评分设置设计

## 状态

- 日期：2026-08-14。
- 任务：T07 全局评分设置。
- 当前阶段：`DESIGNING`，仅完成 Superpowers brainstorming 和正式规格书。
- 当前分支：`codex/t07-global-score-weights`。
- 基线：本 worktree 从 `main@efa0ef54cb432e784796add380727bc5366d2d06` 创建。
- 约束：未获得用户对本规格书的书面批准前，不调用 `writing-plans`，不编写运行时代码，不派实现者，不修改生产、根 `main`、全局队列、项目总账、GitHub Actions、`external-primary` 或其他任务包。

## 问题证据与当前行为

T07 的目标页面是原生账号监控。现状证据如下：

- `docs/project/native-sub-incremental-delivery-constraints.md` 要求全局评分权重与分组评分权重独立持久化，复用现有分组评分设置交互，不改变既有排序之外的调度行为。
- `docs/project/native-sub-task-package-queue.md` 中 T07 的范围为：全局权重持久化/API、账号监控全局设置按钮、复用分组评分弹窗；默认权重为成本 15、成功率 45、首字延迟 20、总延迟 20。
- `upstream/sub2api/backend/internal/service/account_monitor_types.go` 中 `DefaultAccountMonitorScoreWeights` 同时包含四项权重和四项服务指标阈值；默认值为 `15/45/20/20`，阈值默认为 TTFT `1000/5000 ms`、总耗时 `10000/60000 ms`。
- `upstream/sub2api/backend/internal/service/account_monitor_service.go` 当前全局账号监控评分使用 `DefaultAccountMonitorScoreWeights`，分组评分使用各分组自己的 `group.ScoreWeights`。
- `upstream/sub2api/backend/internal/repository/account_monitor_repo.go` 当前只有 `account_monitor_group_score_weights` 分组持久化，包含四项权重和四项阈值。
- `upstream/sub2api/backend/internal/server/routes/admin.go` 中现有分组 GET 是普通管理员访问，分组 PUT/DELETE 使用 `stepUpAuth`。
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue` 已提供四项权重、合计校验、恢复默认、保存/取消，以及“服务指标评分范围”四个阈值字段。
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue` 当前只在选中具体分组时显示评分权重入口；未进入具体分组的全局视图没有评分设置入口。

因此缺口是：全局视图无法编辑全局评分权重，后端也没有独立全局权重持久化或 API；全局排序刷新后只能回到默认权重。

## 目标

1. 在未进入具体分组时提供全局评分权重设置入口。
2. 为全局评分权重提供独立持久化和管理员 API。
3. 复用现有分组评分设置弹窗的交互骨架，但全局模式只编辑四项权重。
4. 默认全局权重为：成本 `15`、成功率 `45`、TTFT `20`、总耗时 `20`。
5. 保存或恢复默认后，全局账号排序立即使用新权重刷新。
6. 页面刷新后，全局权重仍从持久化配置恢复。
7. 全局权重与分组权重互不覆盖。

## 非目标

- 不修改分组权重语义，不迁移、重写或回填已有分组权重数据。
- 不增加评分指标。
- 不修改高级调度算法、Top-K、配额余量权重或生产请求调度。
- 不让全局评分权重参与生产路由。
- 不把 TTFT 目标/上限、总耗时目标/上限纳入 T07 的全局 API、UI 或持久化。
- 不修改生产、根 `main`、全局队列/总账、GitHub Actions、`external-primary` 或其他任务包。

## 用户、边界与术语

- 用户：管理后台账号监控页面的管理员。
- 全局视图：账号监控未选中具体分组时的全站账号列表/排序视图。
- 分组视图：账号监控选中具体分组后的分组列表/排序视图。
- 全局权重：只包含 `cost`、`success`、`ttft`、`latency` 四个整数权重，合计必须为 `100`。
- 阈值：`ttft_target_ms`、`ttft_limit_ms`、`latency_target_ms`、`latency_limit_ms`。这些字段继续使用既有默认值，不进入 T07 全局配置。

## 方案比较与选择

### 方案 1：独立全局权重 API 与 singleton 持久化，复用弹窗模式

新增全局权重 GET/PUT/DELETE API；新增独立 singleton 存储，仅保存四项权重和审计字段。前端为现有评分弹窗增加 `mode="group" | "global"`，分组模式保持八个字段，全局模式只显示四项权重。全局视图新增设置入口，保存或恢复默认后重新加载账号监控投影。

优点：范围清晰，全局与分组互不污染；复用已有交互和校验；后端排序可统一从服务层取权重；刷新后保留。
缺点：需要新增一次 expand-only migration 和一组 API/测试。

### 方案 2：把全局权重作为特殊分组记录

在现有 `account_monitor_group_score_weights` 中使用特殊 `group_id` 或虚拟分组表达全局权重。

优点：复用现有表和 repository 形态。
缺点：会污染分组语义，需要绕开外键或引入虚拟分组，容易误触分组迁移与删除语义，不符合“全局与分组独立”的边界。

### 方案 3：只前端本地存储

把全局权重保存到浏览器 localStorage，前端本地重排。

优点：改动最小，无迁移。
缺点：不满足独立持久化/API；不同管理员、不同浏览器不一致；后端返回的得分和排序无法成为统一事实。

### 选择

选择方案 1。它最贴合队列范围和现有代码结构：新增全局单例配置，复用现有弹窗和刷新模型，后端仍是排序事实源，分组语义不变。

## 架构与组件边界

### 后端

- service：在账号监控服务中新增全局权重读取、保存和恢复默认方法。全局账号评分从该方法获取权重；仅当全局记录不存在时回退默认 `15/45/20/20`。
- repository：新增全局权重 singleton 的读、写、删方法。真实存储错误必须向上传递，不能伪装成默认权重。
- handler：新增全局权重 GET/PUT/DELETE handler，复用分组权重请求校验结构中的四项权重字段，但不接受阈值字段作为全局配置。
- routes：全局 GET 使用普通管理员访问；PUT/DELETE 使用现有 `stepUpAuth`。

### 前端

- API client：新增 `getGlobalScoreWeights`、`updateGlobalScoreWeights`、`resetGlobalScoreWeights`。
- `AccountMonitorGroupScoreDialog.vue`：参数化为分组/全局两种模式。分组模式保持当前交互、标题、八个字段和事件载荷；全局模式只显示四项权重、当前合计、恢复默认、取消、保存。
- `AccountMonitorView.vue`：只在全局视图显示全局评分设置入口；具体分组视图继续显示现有分组权重入口。保存或恢复默认成功后刷新当前全局投影。

## 端到端数据与控制流

1. 管理员进入账号监控全局视图。
2. 打开全局评分弹窗时，前端调用独立 GET `/admin/account-monitors/global-score-weights`，读取四项全局权重；账号监控投影 DTO、`AccountMonitorSchemaVersion` 和既有投影接口保持不变。
3. 管理员点击全局评分设置入口。
4. 前端打开评分弹窗的全局模式，展示四项权重；没有自定义记录时展示默认 `15/45/20/20`。
5. 管理员修改权重并保存。
6. 前端调用全局 PUT API；后端校验四项权重为有限、非负整数且总和为 `100`。
7. 后端原子替换 singleton 记录并返回保存后的四项权重和审计字段。
8. 前端关闭或保持弹窗状态后重新加载账号监控全局投影，使得得分和排序由服务端用新权重重算。
9. 管理员刷新页面后，打开全局评分弹窗再次调用独立 GET，后端继续读取 singleton 全局权重并用于全局排序。
10. 管理员点击恢复默认时，前端调用 DELETE；后端删除 singleton 记录并返回默认四项权重，随后前端刷新全局投影。

分组视图不经过上述全局 API；分组 GET/PUT/DELETE、分组阈值、分组排序语义保持原状。

## API 与字段契约

### GET `/admin/account-monitors/global-score-weights`

返回当前全局权重。若 singleton 记录不存在，返回默认权重。

```json
{
  "cost": 15,
  "success": 45,
  "ttft": 20,
  "latency": 20,
  "updated_by": 0,
  "updated_at": null,
  "is_default": true
}
```

`updated_by`、`updated_at`、`is_default` 为建议响应字段，用于前端区分默认和自定义；排序和校验只依赖四项权重。默认回退时 `updated_by=0`、`updated_at=null`、`is_default=true`。自定义记录存在时 `is_default=false`。

### PUT `/admin/account-monitors/global-score-weights`

请求体只接受四项权重：

```json
{
  "cost": 15,
  "success": 45,
  "ttft": 20,
  "latency": 20
}
```

响应为保存后的全局权重：

```json
{
  "cost": 15,
  "success": 45,
  "ttft": 20,
  "latency": 20,
  "updated_by": 123,
  "updated_at": "2026-08-14T00:00:00Z",
  "is_default": false
}
```

PUT 语义是原子替换 singleton 记录。请求中若带有 `ttft_target_ms`、`ttft_limit_ms`、`latency_target_ms`、`latency_limit_ms`，后端不得将其持久化或纳入全局评分；推荐按未知字段不参与处理，测试必须确认这些字段不会改变全局阈值或存储。

### DELETE `/admin/account-monitors/global-score-weights`

删除 singleton 自定义记录，返回默认四项权重：

```json
{
  "cost": 15,
  "success": 45,
  "ttft": 20,
  "latency": 20,
  "updated_by": 0,
  "updated_at": null,
  "is_default": true
}
```

DELETE 不写入默认记录；“没有记录”就是默认状态。

### 校验

- 四项权重必须是有限整数。
- 四项权重必须非负。
- 四项权重总和必须精确等于 `100`。
- 非法输入返回 `400`，不改变已存储全局权重。
- GET 只有在记录不存在时返回默认；存储读取失败必须返回错误。

## 持久化与迁移

需要 expand-only migration。

新增独立 singleton 表，建议名称为 `account_monitor_global_score_weights`：

```sql
CREATE TABLE IF NOT EXISTS account_monitor_global_score_weights (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    cost_weight SMALLINT NOT NULL CHECK (cost_weight >= 0),
    success_weight SMALLINT NOT NULL CHECK (success_weight >= 0),
    ttft_weight SMALLINT NOT NULL CHECK (ttft_weight >= 0),
    latency_weight SMALLINT NOT NULL CHECK (latency_weight >= 0),
    updated_by BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cost_weight + success_weight + ttft_weight + latency_weight = 100)
);
```

实现可按现有迁移编号规则生成下一号 migration。该迁移只新增表，不回填，不触碰 `account_monitor_group_score_weights`，不新增阈值列，不改已有分组记录。

存储语义：

- 0 行：使用默认 `15/45/20/20`。
- 1 行：使用该行四项权重。
- PUT：`INSERT ... ON CONFLICT (singleton) DO UPDATE`。
- DELETE：删除 singleton 行。

## 失败与安全语义

- GET 存储读取失败：返回错误，页面提示读取失败，不静默回退默认。
- PUT/DELETE 校验失败：返回 `400`，旧配置保持不变。
- PUT/DELETE 存储失败：返回错误，旧配置保持不变，前端保留弹窗草稿并显示错误。
- PUT/DELETE 成功但投影刷新失败：持久化结果仍然有效；前端提示刷新失败，弹窗保留错误状态或允许用户重试刷新，当前列表不得假装已经按新排序完全同步。
- 权限：GET 需要普通管理员访问；PUT/DELETE 需要现有 step-up 权限；操作者从认证上下文获取并写入 `updated_by`。
- 安全边界：API 不返回凭据或账号敏感字段；全局权重仅影响账号监控展示评分和排序，不影响生产调度。

## UI 交互与状态

- 全局视图显示评分设置入口，位置复用分组摘要中评分权重入口的视觉语言。
- 分组视图继续显示分组评分权重入口，全局入口不可见。
- 全局弹窗标题表达“全局评分规则”；分组弹窗标题继续表达“分组评分规则”。
- 全局弹窗仅显示四项权重输入和当前合计，不显示“服务指标评分范围”区块。
- 全局弹窗说明继续强调四项权重只影响监控展示排序，不改变生产调度。
- 保存按钮在总和不是 `100`、输入非法或保存中时禁用。
- 恢复默认调用全局 DELETE；成功后刷新全局投影。
- 分组弹窗现有八字段、阈值校验、保存和恢复默认行为保持不变。

## 兼容策略

- 旧部署升级后没有 singleton 记录，行为等同当前默认权重，不影响现有页面加载。
- 新代码回滚后，新增 singleton 表和记录成为惰性遗留数据；旧代码不读取它，不影响旧路径。
- 分组权重表、分组阈值、分组 GET/PUT/DELETE 不变。
- 全局默认阈值仍来自既有 `AccountMonitorDefaultTTFTTargetMS`、`AccountMonitorDefaultTTFTLimitMS`、`AccountMonitorDefaultLatencyTargetMS`、`AccountMonitorDefaultLatencyLimitMS`，但 T07 不提供编辑入口和持久化。
- 保持账号监控投影 DTO、`AccountMonitorSchemaVersion` 和既有投影接口不变；全局权重通过打开弹窗时调用独立 GET 获取。

## 场景化验收矩阵

| 场景 | 操作 | 预期 |
| --- | --- | --- |
| 首次进入全局视图 | 打开账号监控全局视图并打开评分设置 | 显示四项默认权重 `15/45/20/20`，合计 `100` |
| 保存全局权重 | 输入合法权重并保存 | API 持久化成功，全局投影刷新，账号得分和排序按新权重更新 |
| 刷新保留 | 浏览器刷新后再次打开全局评分设置 | 显示上次保存的全局权重 |
| 恢复默认 | 点击恢复默认 | singleton 记录被删除，弹窗回到 `15/45/20/20`，全局排序刷新 |
| 非法输入 | 输入负数、非整数、非有限值或总和不为 `100` | 前端阻止提交或后端返回 `400`，旧配置不变 |
| 分组隔离 | 修改全局权重后进入某个分组 | 分组仍使用自己的权重和阈值 |
| 全局隔离 | 修改某个分组权重后回到全局视图 | 全局仍使用全局权重 |
| 阈值排除 | 在全局保存请求中夹带阈值字段 | 阈值字段不被持久化，不改变全局评分阈值 |
| 权限 | 未完成 step-up 时调用 PUT/DELETE | 请求被现有 step-up 权限拦截 |
| 存储失败 | GET 发生数据库错误 | 页面显示错误，不回退默认 |
| 刷新失败 | PUT/DELETE 成功后列表刷新失败 | 配置已生效；页面提示刷新失败，不伪造新排序 |

## 测试策略

### 后端

- repository 测试：singleton GET 记录不存在、GET 已存在、PUT upsert、DELETE、数据库错误透传、约束拒绝非法总和。
- service 测试：默认回退仅发生于 not found；存储错误不回退；PUT/DELETE 校验 actor 和权重；全局账号评分使用全局权重。
- handler/routes 测试：GET/PUT/DELETE 路径、请求/响应字段、`400` 校验、PUT/DELETE step-up 权限。
- 回归测试：分组权重 GET/PUT/DELETE、分组阈值、分组排序不变。

### 前端

- API client 测试或 mock 覆盖全局 GET/PUT/DELETE。
- `AccountMonitorGroupScoreDialog.vue` 测试：global mode 只渲染四项权重；group mode 继续渲染四项权重和四项阈值；保存载荷按 mode 区分。
- `AccountMonitorView` 测试：全局入口只在全局视图出现；分组入口只在分组视图出现；保存和恢复默认后刷新投影；失败时弹窗错误和草稿保留。
- 现有账号监控页面测试继续覆盖分组评分设置，确保不回归。

### 验证命令建议

实现完成后在 T07 分支运行与改动相关的后端单元测试、前端组件/页面测试、类型检查、构建、`git diff --check efa0ef54cb432e784796add380727bc5366d2d06..HEAD` 和范围检查。具体命令由后续 writing-plans 阶段根据实际文件确定。

## 发布、线上验证与回滚

T07 候选最终只能停在 `READY_FOR_ROOT_REVIEW`。只有根总控发送包含目标 `main` SHA 的 `AUTHORIZE_MERGE_TO_MAIN` 后，才允许按指令合并到根 `main`。本任务不得自行推送、部署或线上验收。

发布由根总控使用既有本地/宿主脚本链完成，不使用 GitHub Actions。

线上验证建议：

- 管理员进入账号监控全局视图，确认全局评分入口可见。
- 保存一组非默认权重，确认列表刷新后全局得分和排序变化。
- 刷新浏览器，确认弹窗读取保存后的权重。
- 恢复默认，确认回到 `15/45/20/20` 并刷新排序。
- 抽查一个分组评分设置，确认四项阈值仍可见且分组行为未变。
- 捕获网络请求，确认没有调用 `external-primary` 或 `/api/v1/xingqiao/**`。

回滚条件：

- 全局排序异常、全局和分组权重互相覆盖、权限绕过、或分组阈值被全局配置影响时，根总控应停止上线或回滚应用代码。
- migration 为 expand-only；应用代码回滚后新增表可保留，不影响旧代码。若后续确认需要清理数据，应由单独修复包处理。
- 不需要旧数据回滚，因为 T07 不改写分组表和历史监控数据。

## 风险与缓解

- 风险：复用分组弹窗时把四项阈值带入全局模式。缓解：global mode 明确只渲染四项权重，API/storage 测试确认阈值不进入全局配置。
- 风险：数据库读取失败被误判为无记录并回退默认。缓解：repository/service 区分 `sql.ErrNoRows` 和其他错误，测试覆盖。
- 风险：保存成功后前端只本地重排，和后端事实不一致。缓解：保存/恢复默认后必须重新加载全局投影。
- 风险：全局权重误用于生产调度。缓解：实现限定在账号监控评分路径，测试和 review 检查调度相关文件不变。

## 未决事项

无产品范围未决事项。

实现计划阶段需要按当前迁移目录选择具体 migration 编号，并按现有测试工具确定最终命令清单；这不改变本规格定义的字段、API、权限、失败语义或范围边界。

## 用户批准记录

- 2026-08-14：用户以“继续”指示推进 brainstorming。根总控依据已批准的任务队列边界，将纠偏后的方案及各设计段记录为与 T07 既定范围一致：仅四项全局权重独立持久化，四个服务指标阈值不进入 T07。
- 2026-08-14：上述推进记录不等同于用户对本规格书的书面批准。当前正式规格仍待用户明确审阅并书面批准；在此之前不得调用 `writing-plans`、编写实现代码或派实现者。
- 2026-08-15：用户授权根总控代审后续规格书和计划书。根总控审查本规格后要求将全局权重读取固定为打开弹窗时调用独立 GET，并保持账号监控投影 DTO、`AccountMonitorSchemaVersion` 和既有投影接口不变；上述修订验证通过即代表用户批准本规格。

## 自审记录

- Placeholder 扫描：本文没有占位标记、空章节或占位式验收。
- 一致性检查：目标、API、UI、迁移、测试和回滚均只围绕四项全局权重；分组权重和阈值保持现状。
- 范围检查：没有纳入服务指标阈值持久化、评分指标扩展、调度算法、Top-K、配额余量权重、生产发布、GitHub Actions、`external-primary` 或其他任务包。
- 歧义检查：全局默认、singleton 存储、GET/PUT/DELETE 语义、权限、失败回退、刷新失败处理和 migration 属性均已明确。
