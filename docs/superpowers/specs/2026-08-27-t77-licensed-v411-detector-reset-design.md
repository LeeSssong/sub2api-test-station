# T77 授权 v4.1.1 指纹制品与检测历史清库设计

## 问题与证据

当前生产 sidecar 为 `native-1`：它只核验模型目录和一次 `/responses` 返回，并将行为指纹初始化为不可用。生产 `account_model_detection_runs` 有 3,676 条：3,144 条旧格式，532 条带 T70 结构字段但均无真实 `fingerprint_status`；没有一条真实行为指纹结果。该表没有外键引用，大小约 2.9 MiB。

用户已于 2026-08-27 明确确认：持有 `chen-006/gpt56_api_detector` v4.1.1 的线下商业授权，可以使用该制品；并授权删除上述 3,676 条记录，不做迁移或回填。

## 目标

1. 使用官方 v4.1.1 发布资产 `gpt56_api_detector_github_upload.zip`，SHA-256 必须为 `70c0c2f092e66cd219f2384e08872e5bedb4559e427c2e320d0070186376f865`。
2. 保持 Sub 原生账号监控、任务队列、`account_model_detection_runs` 和管理员记录面板作为唯一运营事实源。
3. 在既有私网 `/v1/catalog`、`/v1/detect` 合同中返回 v4.1.1 的 Juice、指纹状态、候选、相似度、计划数和有效样本。
4. 在可恢复备份后删除生产中全部 3,676 条 `detector_version` 不为 `4.1.1` 的历史运行记录；不写回、不转换、不合成任何历史证据。

## 非目标

- 不修改账号调度资格、熔断、计费、用量或质量评分。
- 不扩大管理员 UI 或重做 T73 的筛选和展示。
- 不保存 API Key、Base URL、Authorization、题面、原始回答、原始请求/响应或 detector 本地 SQLite 文件。
- 不伪造指纹，v4.1.1 无有效样本时必须返回真实证据不足或不可用。

## 方案与选择

| 方案 | 取舍 | 结论 |
| --- | --- | --- |
| 仅改现有 Go `native-1` | 不能产生行为指纹 | 不选 |
| 另建独立控制面 | 会重复任务、历史和权限边界 | 不选 |
| 同镜像运行 v4.1.1 合同包装器 | 复用原生调度、存储、部署和 UI，仅替换证据引擎 | 选定 |

## 数据与控制流

1. 账号检测服务向私网 sidecar 发送已存在的运行 ID、申报模型、实际请求模型、临时 API Key/Base URL、档位、模式和触发原因。
2. `/app/model-detector` 验证内部 bearer token，并用 v4.1.1 单次官方档位执行检测；每次运行使用私有临时目录，结束即删除。
3. 包装器仅提取受限摘要：Juice 状态、行为指纹状态、最强候选、模型匹配度、计划数、有效样本、总体结论和分类错误码。
4. Sub 现有 sidecar client 再次执行 body、字符串、深度和敏感键限制，随后写入原有 run 表及当前投影。
5. 管理员面板照常读取原生历史接口；成功样本显示两项真实证据，失败样本显示不可用原因而不显示伪造零值。

## 接口契约

`/v1/catalog` 返回版本 `4.1.1` 和 v4.1.1 支持的 Sol/Terra/Luna 模型集合。`/v1/detect` 接收既有 `AccountModelDetectionRequest`，并返回：

```json
{
  "status": "normal|abnormal|insufficient",
  "profile": "low|medium|high",
  "planned_requests": 19,
  "valid_samples": 0,
  "evidence_state": "complete|insufficient|unavailable",
  "juice_status": "pass|mismatch|insufficient|possible_non_gpt",
  "fingerprint_status": "strong_match|unclear|unavailable",
  "fingerprint_candidate": "gpt-5.6-sol",
  "fingerprint_similarity": {"gpt-5.6-sol": 0.98},
  "detector_version": "4.1.1"
}
```

`fingerprint_similarity` 是匹配度，不是路由概率。未被 v4.1.1 支持的申报模型返回 `insufficient` 与受限错误码，不把其误判为账号异常。

## 安全、失败与兼容

- Docker 构建下载固定 URL、校验固定 SHA-256，并保留上游 `LICENSE`、`NONCOMMERCIAL_NOTICE_CN.md` 与 Required Notice；用户的线下授权不提交到仓库。
- 包装器只监听 Compose 私网 `:8090`，要求现有 bearer token；健康检查不暴露授权或检测内容。
- 每次检测禁用 upstream retention，临时目录在 `finally` 清理，日志只能记录脱敏分类错误。
- 请求超时、非法响应、受限模型或 v4.1.1 内部错误均返回 `insufficient/unavailable`，不影响主 API 请求链。
- 现有 45 秒 sidecar 客户端超时必须提高到能容纳高档检测，但仍受有界 HTTP、计划数和上游超时控制。

## 数据清理与回滚

删除脚本只能在生产宿主执行，必须：

1. 创建 `pg_dump -t account_model_detection_runs` 的受限可恢复导出，记录 SHA-256 和行数；
2. 使用单个事务、`ACCESS EXCLUSIVE` 锁和期望计数 3,676；
3. 确认该表仍没有被外键引用，只删除 `detector_version IS DISTINCT FROM '4.1.1'` 的行；
4. 在提交前验证删除数和剩余非 v4.1.1 行数均为零；失败时回滚；
5. 不访问或修改任何其他表。

如发布失败，恢复上一个蓝绿镜像；如清库后需要恢复，使用受限导出只恢复该表，且必须由单独的人工操作批准。不会把历史数据自动回填。

## 验收矩阵

| 场景 | 预期 |
| --- | --- |
| v4.1.1 资产哈希错误 | Docker 构建失败 |
| 无 token / 错 token | sidecar 拒绝 |
| medium 运行完成 | 有计划数、有效样本、Juice 和指纹字段 |
| 指纹强烈指向其他模型 | 返回候选和匹配度，综合结论异常 |
| 网络/协议失败 | 返回真实 `unavailable`，不持久化敏感数据 |
| 删除前计数变化 | 脚本失败并不删除 |
| 删除成功 | 备份存在、删除 3,676 行、非 v4.1.1 行为 0 |
| 发布后 | sidecar 版本为 4.1.1，健康检查和一次真实检测均通过 |

## 测试、发布与批准

先写 adapter、Docker 和清理 guard 的失败测试，再完成最小实现。定向验证覆盖 Go sidecar 客户端、adapter 映射、镜像构建、Compose/宿主发布合同和清理脚本 fail-closed 行为；随后执行 Go build、相关前端测试、类型检查、生产构建和发布预检。预检 `downtime_required=true` 时停止等待授权；`false` 时走既有无停机蓝绿链。

用户已确认商业授权和精确删除范围；本规格据此进入实施。
