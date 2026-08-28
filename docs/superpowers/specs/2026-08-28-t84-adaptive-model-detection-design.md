# T84 模型检测跨 6 小时窗口自适应升档设计

## 批准记录

- 2026-08-28：用户确认自动检测只在下一次 6 小时窗口升级，不在当前检测结束后立即追加请求。
- 2026-08-28：用户确认每次升级仍必须满足当前北京时间 5 分钟真实请求为空；失败、异常等非可疑结果不触发升级。

## 当前行为与问题

T83 已将定时模型检测限制为 00:00、06:00、12:00、18:00 窗口内的 low run，并在入队前检查当前 5 分钟 `usage_logs` 空桶。当前生产没有自动 `low -> medium -> high` 调用方；历史保留的 `shouldEscalateDetection`/`EnqueueEscalationHigh` 不能作为有效调度事实。

本次只增加跨窗口的自适应档位选择，并补齐真正发送上游请求前的第二次空桶检查，避免“入队时为空、执行时已出现真实请求”仍发探针。

## 目标

1. 每个自然 6 小时检测窗口每个账号最多一个自动 run。
2. 默认 profile 为 `low`（19 个许可检测请求）。
3. 只有已完成检测且具备明确冲突证据的结果才在下一个自然 6 小时窗口升一级：`low -> medium -> high`。
4. `high` 完成任意最终结果后，下一个窗口回到 `low`。
5. `failed`、超时、检测器不可用、账号/API Key 不可用、usage 查询失败和 `insufficient` 均不升级、不立即重试；当前档位保持不变，直到得到新的最终结果。
6. 每次升级窗口入队前、任务实际发送请求前都必须确认当前 Asia/Shanghai 5 分钟桶没有真实用户请求；桶繁忙则不访问上游、不消耗档位，等待下一窗口。

## 可疑判定

仅以下组合视为 `suspicious`：

- `juice_status == mismatch`；或
- `fingerprint_status == mismatch` / `strong_conflict`；或
- `fingerprint_candidate` 非空且不同于 `claimed_model`。

单独的 `status == abnormal` 但没有上述证据不升档；`insufficient` 永不直接升档。

## 状态推导

不新增运行表；为账号/分组主动探测开关新增向后兼容 migration `229_active_probe_switches.sql`。`RunDueSlots` 读取账号最近完成 run，按最近一次自动 run 的 profile 和结果推导下一窗口 profile：

| 最近一次自动结果 | 下一窗口 |
| --- | --- |
| 无历史 | low |
| low + suspicious | medium |
| medium + suspicious | high |
| high + 任意最终状态 | low |
| normal | low |
| insufficient / failed | 保持最近 profile，不升级 |

当窗口因真实请求繁忙或 usage reader 出错而跳过时，不创建新 run，因此档位自然保留。

## 数据流与失败语义

`RunDueSlots` 计算 6 小时 slot、读取最近历史、检查空桶、创建唯一 `slot_key` run。`RunQueued/execute` 在 claim 前后再次读取账号和当前桶；若已变忙，完成为脱敏的 `probe_bucket_busy` 跳过记录或安全回置队列，绝不调用 sidecar。成功调用后只保存既有 detector 结果，不追加同窗口任务。

手动 low 检测、历史查询和 v4.1.1 detector prompt/hash 合同保持不变。

## 验收与测试

- 00/06/12/18 窗口分别验证 low、medium、high 选择和 slot 去重。
- low 可疑 -> 下一窗口 medium；medium 可疑 -> 下一窗口 high；high 完成 -> 下一窗口 low。
- normal、insufficient、failed 不升档；无立即追加任务。
- 入队后桶变忙时不发上游请求，下一窗口仍使用原档位。
- 直接相关 Go service/repository tests、`go build ./cmd/server`、gofmt、diff-check 和现有 Python adapter tests 通过。

## 发布边界

包含向后兼容数据库迁移 `229_active_probe_switches.sql`，默认值保持主动探测开启；无配置迁移和生产数据回填。发布仍须从已验证 `main` 按验收站全局约束执行；本轮代码未推送、未部署。
