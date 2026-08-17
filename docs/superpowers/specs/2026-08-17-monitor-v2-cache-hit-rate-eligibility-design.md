# Monitor V2 缓存命中率有效样本口径修正规格

## 1. 问题证据与当前行为

Monitor V2 当前在 `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go` 中对 `usage_logs` 做分组聚合：

- 分母：时间窗内所有 OpenAI/Anthropic `usage_logs` 行；
- 分子：`cache_read_tokens > 0` 的行；
- 当前没有排除 `actual_cost = 0` 的失败占位行；
- 当前没有排除 `billing_mode = image/video/per_request` 等不具备文本 Prompt Cache 语义的成功请求。

2026-08-17 生产只读核对显示：

- 最近 24 小时约 640 条 `actual_cost = 0`、零 Token、无上游响应的失败占位行进入分母；模型包括 Luna、Sol、Terra、5.5；
- 最近 24 小时约 31 条成功的 `gpt-image-2` 图片流水进入分母，但图片请求不产生文本 Prompt Cache 命中；
- 当前 24 小时原始显示约 84.6%，按成功且缓存适用请求重算约 96.9%；7 天默认窗口原始约 89.8%，修正后约 97.3%。

## 2. 目标与非目标

### 目标

1. 将缓存命中率分母限定为“成功完成且具有文本 Token 缓存语义”的流水。
2. 将 `actual_cost = 0` 的失败/占位流水从分子、分母同时排除。
3. 将图片、视频、按次等不具备文本 Prompt Cache 语义的成功流水从缓存样本排除。
4. 保持 Monitor V2 API 响应结构、前端展示、账务字段和其他 Ops 指标不变。

### 非目标

- 不修改 `usage_logs` 数据，不做历史回填或删除；
- 不改变 `actual_cost`、账号成本、用户扣费、利润、倍率或账务聚合；
- 不调整 Luna 模型禁用策略、分组配置或错误提示；
- 不新增数据库迁移、配置项、第二事实源或外部控制面；
- 不修改缓存策略本身，只修正监控样本选择。

## 3. 方案比较与选择

### 方案 A：仅增加 `actual_cost > 0`

可排除失败占位流水，但会继续把图片/视频/按次成功请求当作缓存未命中。覆盖不完整，放弃。

### 方案 B：仅按 Token 字段大于零筛选

会误排除合法的短请求、无输出请求和部分成功的零 Token 计费类型，也无法可靠识别图片/视频计费，放弃。

### 方案 C：成功条件 + Token 计费模式条件（推荐）

使用原生 `usage_logs.actual_cost` 成功落账代理，并要求 `billing_mode = token`；对历史 `billing_mode` 为空的旧行，仅在没有图片/视频字段时按 Token 兼容回退。该方案复用已有 Sub 原生用量语义，能同时排除失败占位和非缓存图片/视频流水，不改账务事实源。

## 4. 数据与接口契约

缓存样本谓词定义为：

```sql
ul.actual_cost > 0
AND (
  ul.billing_mode = 'token'
  OR (
    (ul.billing_mode IS NULL OR ul.billing_mode = '')
    AND COALESCE(ul.image_count, 0) = 0
    AND COALESCE(ul.video_count, 0) = 0
    AND COALESCE(ul.image_input_tokens, 0) = 0
    AND COALESCE(ul.image_output_tokens, 0) = 0
  )
)
```

Monitor V2 仓储查询保持同一返回结构：

- `request_count`：满足上述谓词的样本数；
- `hit_count`：满足上述谓词且 `cache_read_tokens > 0` 的样本数；
- `evidence_available`、分组维度和时间边界保持原样。

服务层继续计算 `hit_count / request_count * 100`，前端无合同变化。

## 5. 失败与兼容语义

- `actual_cost = 0`、零 Token、无上游响应的失败占位行不计入样本；这覆盖 Luna 禁用前后的历史错误以及其他模型的 4xx/5xx/429/上游不可用错误。
- 成功重试后产生实际用量和扣费的 `Recovered upstream error` 流水仍按成功样本处理；其是否命中缓存由 `cache_read_tokens` 决定。
- 图片、视频、按次计费流水不计入缓存样本，但继续参与其他用量、费用和 Ops 聚合。
- `billing_mode` 为空的历史 Token 行仅在没有图片/视频字段时保留，避免旧数据因字段缺失全部消失。
- 无迁移、无数据写入、无历史清理；修复后历史窗口查询会自动按新谓词重算。

## 6. 场景化验收矩阵

| 场景 | `actual_cost` | `billing_mode` | `cache_read_tokens` | 计入分母 | 计入分子 |
|---|---:|---|---:|---:|---:|
| 成功 Token、发生缓存读取 | >0 | token | >0 | 是 | 是 |
| 成功 Token、未发生缓存读取 | >0 | token | 0 | 是 | 否 |
| 失败/占位、无响应 | 0 | token/空 | 0 | 否 | 否 |
| 成功图片请求 | >0 | image | 0 | 否 | 否 |
| 成功视频/按次请求 | >0 | video/per_request | 0 | 否 | 否 |
| 历史 Token 行缺失 billing_mode | >0 | 空 | 0 或 >0 | 是（无图片/视频字段） | 按 cache_read_tokens |

## 7. 测试与发布策略

- 仓储 SQL mock 覆盖以上六类样本，验证 `request_count` 和 `hit_count`；
- 保留空分组、非法时间窗、非 OpenAI/Anthropic 平台和零流量行为测试；
- 运行 Monitor V2 repository/service focused tests、受影响后端 compile-only/build、`gofmt`、`git diff --check`；
- 不运行全仓、压力、soak、mutation 或无关浏览器矩阵；
- 无迁移，发布预检预期 `downtime_required=false`；若实际返回 `true`，按全局门禁停在授权前；
- 上线后只读核验 24h/7d Monitor V2 数值、`request_count`、`hit_count`，并用生产 SQL 对比失败占位和图片样本均未计入。

## 8. 回滚条件

若 Monitor V2 API 返回错误、样本数异常为零、缓存命中率与只读 SQL 交叉核对不一致，或其他 Ops 指标出现回归，则回滚到上一活动槽/上一镜像。该任务无迁移和数据写入，回滚不需要数据恢复。

## 9. 用户批准记录

用户于 2026-08-17 明确要求“按这个方案修正”，批准采用“成功且缓存适用样本”口径，并要求将实施方案发送至“快速迭代-指挥（7）”加入任务队列。
