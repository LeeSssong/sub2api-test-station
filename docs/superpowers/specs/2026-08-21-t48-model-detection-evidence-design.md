# T48 模型映射/替换双证据检测与上游返回值展示设计

状态：已批准设计。批准依据：用户于 2026-08-21 批准“目录证据 + 主动响应/指纹证据”判定矩阵，并补充要求“当指纹或模型不匹配时，明确写出上游返回的是什么”。

## 1. 问题证据与当前行为

- 当前 `backend/cmd/model-detector/main.go` 只调用上游 `GET /v1/models`。申报模型在目录中就返回 `normal/match`，不在目录中就返回 `abnormal/model_not_advertised`。
- 当前实现没有发送单次主动模型请求，因此不知道上游响应体实际声明的 `model`，也不能仅凭 `/models` 目录证明请求由哪个模型处理。
- 当前一旦目录不匹配，页面直接显示原始 `model_not_advertised`；`Juice: mismatch` 也没有说明证据来源，容易被理解为“模型无法使用”而不是“发现疑似映射/替换证据”。
- T15 现有合同已保留 `juice_summary`、`fingerprint_candidate`、`fingerprint_similarity` 和稳定错误码；T48 优先小幅扩展该合同，不新建平行检测系统。

## 2. 目标

1. 将上游模型目录、单次主动响应声明模型和行为指纹候选作为独立证据源，不互相冒充。
2. 当模型或指纹不匹配时，页面显示：请求/申报模型、上游响应声明的 `model`、指纹候选、相似度、目录摘要和综合结论。
3. 上游响应未包含 `model` 时明确显示“上游未返回 model 字段”，不从目录或指纹候选猜测填充。
4. 将原始机器码收进“技术详情”语义；首屏使用“疑似映射”、“疑似替换”、“高风险不一致”或“证据不足”。
5. 不保存 API Key、Base URL、Authorization、完整提示词、完整输出、原始响应体或无限模型目录。

## 3. 非目标

- 不宣称检测结果是上游承认或密码学证明。
- 不复制、打包或重写历史 PolyForm Noncommercial 检测器的核心、基线或报告逻辑。
- 不修改账号连接探测、质量评分、调度、余额、计费、分组建议或原生模型登记。
- 不保存完整 `/models` 响应；仅保存有界计数和最多 10 个脱敏模型 ID 摘要。
- 不在本任务中合并、推送、发布或修改生产数据；T47-R2 仍占用发布单车道。

## 4. 方案比较与选择

### 方案 A：只修改前端文案

可以快速将 `model_not_advertised` 翻译为“疑似映射”，但仍然无法回答“本次请求上游返回了什么模型”，不满足用户补充要求。不选。

### 方案 B：目录证据 + 主动响应模型 + 可选指纹证据（选定）

`native-1` 先读取 `/v1/models`，再向选定模型发送有界、低输出的 `/v1/responses` 主动探针，只提取顶层 `model` 和必要的有界协议元数据，不保存输出。T15 现有指纹候选/相似度字段继续接收独立合法检测器证据；`native-1` 没有可验证行为基线时必须返回“指纹未检测”，不得把响应 `model` 冒充为行为指纹候选。

该方案可立即补齐“上游响应了什么”，同时保留将来接入独立合法行为指纹引擎的合同。

### 方案 C：直接引入历史第三方指纹工具

可能产生更多指纹候选，但存在明确的商业许可边界，且会把未审查基线带入生产。不选。

## 5. 证据与判定合同

### 5.1 证据包

sidecar 使用现有 `juice_summary` 返回有界 `evidence_v1` 包，避免数据库迁移：

```json
{
  "evidence_version": "model-detection-evidence-v1",
  "requested_model": "gpt-5.6-sol",
  "catalog": {
    "status": "match|missing|unavailable",
    "returned_count": 3,
    "returned_models": ["gpt-5.4", "gpt-5.6-terra"]
  },
  "active_response": {
    "status": "match|mismatch|missing|unavailable",
    "returned_model": "gpt-5.4"
  },
  "fingerprint": {
    "status": "match|mismatch|unavailable",
    "candidate": "gpt-5.4",
    "similarity": 0.98
  },
  "verdict": "verified|suspected_mapping|suspected_replacement|high_risk_inconsistent|insufficient"
}
```

- `returned_models` 最多 10 个，每个 ID 最多 128 字节，按稳定排序保存。
- `active_response.returned_model` 只能来自单次主动响应顶层 `model`；不存在时留空并记为 `missing`。
- `fingerprint.candidate` 只能来自指纹引擎。`native-1` 没有独立指纹基线时使用 `unavailable`，不用 `returned_model` 伪造候选。
- 为保持旧客户兼容，已有 `fingerprint_candidate` / `fingerprint_similarity` 继续保留，前端优先读标准字段，其次读 `evidence_v1.fingerprint`。

### 5.2 判定矩阵

| 目录 | 主动响应 `model` | 指纹 | 综合结论 |
|---|---|---|---|
| 命中 | 匹配 | 匹配或未检测 | `verified` |
| 缺失 | 匹配 | 匹配或未检测 | `suspected_mapping` |
| 命中 | 不匹配 | 任意 | `suspected_replacement` |
| 任意 | 匹配 | 不匹配 | `suspected_replacement` |
| 缺失 | 不匹配 | 不匹配或候选与响应模型冲突 | `high_risk_inconsistent` |
| 证据请求失败且无其他可用证据 | 不可用 | 不可用 | `insufficient` |

- `verified` 映射现有 `status=normal`。
- `suspected_mapping` / `suspected_replacement` / `high_risk_inconsistent` 映射 `status=abnormal`。
- `insufficient` 映射 `status=insufficient`。
- 目录缺失不再使用“检测模型已失效”之类配置错误文案。

## 6. 主动探针协议

1. 使用与 `/models` 相同的上游 Base URL 和 API Key，构造 `/v1/responses`。
2. 发送非流式、无工具、有界输出探针：`model=<request_model>`、`input="Reply with exactly OK."`、`max_output_tokens=8`。
3. 仅解析顶层 `model`；输出文本、reasoning、usage 明细和原始响应体不返回 Sub、不写日志、不持久化。
4. 主动端点 401/403 为 `upstream_unauthorized`；网络/非 2xx/无效 JSON 记为主动响应 `unavailable`。如目录证据仍可用，整体不伪造模型结论，而是 `insufficient`。
5. 单次检测最多发送一次目录请求和一次主动探针，不重试，不触发工具或副作用。

## 7. 前端展示

最近结果卡按以下顺序展示：

1. **综合结论**：模型可信 / 疑似模型映射 / 疑似替换模型 / 高风险不一致 / 证据不足。
2. **请求模型**：使用已有 `claimed_model`。
3. **上游响应模型**：显示 `active_response.returned_model`；缺失时显示“未返回 model 字段”；主动请求失败时显示“未取得主动响应”。
4. **模型目录**：显示命中/未命中/不可用。未命中时显示“目录共返回 N 个模型”和有界候选摘要，但不标注为“上游响应模型”。
5. **行为指纹**：显示匹配/不匹配/未检测；不匹配时显示候选模型和相似度。
6. **技术详情**：检测器版本、时间和稳定错误码。不再把原始错误码作为首要结论。

中英文 locale 同步更新；移除对管理员无解释价值的 `Juice` 直译标签，但底层兼容字段不删除。

## 8. 失败、安全与兼容性

- sidecar 不记录请求体、API Key、Base URL、Authorization 或上游原始响应。
- Sub sidecar 客户端继续执行 64 KiB body limit、8 KiB summary limit、敏感 key 递归删除和字符串长度限制。
- 证据包无效时前端回退显示已有字段，不导致弹窗崩溃。旧历史结果继续可见，但仅显示其真实具备的证据。
- 主动探针是模型检测的计划内低 Token 请求，不进入 Sub 用户请求链、不触发工具，但可能产生上游微量计费；本任务不修改计费数据或补记为站内用量。
- 无数据库迁移、无历史回填、无配置 schema 变更。

## 9. 验收矩阵

| 场景 | 预期 |
|---|---|
| 目录含 Sol，主动响应 `model=Sol` | 模型可信；显示上游响应 Sol |
| 目录不含 Sol，主动响应 `model=Sol` | 疑似映射；同时显示目录摘要和响应 Sol |
| 目录含 Sol，主动响应 `model=5.4` | 疑似替换；明确显示请求 Sol / 返回 5.4 |
| 主动响应不含 `model` | 明确显示未返回字段；不用目录候选填充 |
| 指纹候选 5.4，相似度 0.98 | 显示“指纹更接近 gpt-5.4（98%）” |
| 响应模型 5.4，指纹候选 Terra | 高风险不一致；两个观测值均展示 |
| `/models` 不可用但主动响应可用 | 按主动证据给出有限结论，目录标注不可用 |
| 两种请求都失败 | 证据不足，不标记替换 |
| 历史旧结果 | 弹窗可正常打开，不伪造响应模型 |
| 安全扫描 | 结果/日志不含 API Key、Base URL、Authorization、prompt/output/response |

## 10. 测试、发布与回滚

- sidecar Go 测试：目录与主动响应 URL，主动请求 body 与有界输出，匹配/映射/替换/缺字段/请求失败，证据包有界摘要和凭据不泄露。
- Sub sidecar 客户端/存储测试：`evidence_v1` 通过现有 bounded summary 持久化，敏感 key 继续被删除，旧字段保持兼容。
- 前端 Vitest：五种结论、响应模型、未返回字段、目录摘要、指纹候选/相似度、技术详情和旧结果回退。
- 必要门禁：相关 Go tests、前端 focused Vitest、`pnpm typecheck`、`pnpm build`、`gofmt`、`git diff --check`。不扩大到全仓或无关测试。
- 预期 `downtime_required=false`，最终以候选合入届时最新 `main` 后的发布预检为准。T47-R2 生产收口前 T48 不进入整合/发布车道。
- 回滚为恢复上一已验证镜像；无数据迁移或生产数据回滚。

## 11. 待决事项与批准记录

- 用户已明确批准判定方向与“写出上游返回值”要求，无未决产品问题。
- 本任务不伪造行为指纹基线。当前 `native-1` 会提供目录和主动响应模型证据；真实指纹候选仍仅来自符合 T15 合同与许可门禁的独立引擎。这是准确性边界，不是用目录值代替指纹的降级。

