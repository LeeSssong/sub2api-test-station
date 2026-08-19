# T29 Monitor V2 二态健康展示与统一指标口径实施计划

## Task 1: 用失败测试锁定 v6 合同

- 更新 service、repository、handler、frontend API 和组件夹具为 v6 目标结构。
- 先新增断言：旧百分比字段缺失、状态只允许二态、时间线只含二态探测、Pro 置顶且为旗舰。
- 运行聚焦测试并保留预期失败结果。

## Task 2: 实现统一性能查询

- 将 `MonitorV2Repository` 改为批量 `GetPerformanceStats`。
- 使用 `(group_id, primary_model)` scope CTE 和单一 eligible CTE 计算 TTFT、总延迟、TPS。
- 资格统一为时间窗、分组、主模型、正实际成本、文本 Token 语义及完整性能证据。
- 更新 sqlmock 测试，确认三个指标共用同一 `sample_count`。

## Task 3: 收敛后端二态服务合同

- 将 Monitor V2 状态收敛为 `operational | unavailable`。
- 从最新主动监控选择主模型并批量读取性能统计。
- 删除成功率和缓存率服务字段；简化时间线字段。
- 稳定识别 Pro/旗舰并置顶。
- 更新 handler DTO 和 JSON 测试，升级合同版本为 6。

## Task 4: 收敛前端健康展示

- 更新 types/API 运行时校验，拒绝旧百分比字段。
- 从卡片删除有效调用、成功率、缓存命中率和所有百分比格式化逻辑。
- 增加 Pro 旗舰徽标；状态只显示运行中/服务不可用。
- 时间线按点的二态状态绘制绿色/红色固定柱，不显示请求结果和百分比。
- 整体状态改为二态并使用相同文案与颜色。

## Task 5: 验证、视觉核对与交接

- 运行 Monitor V2 service/repository/handler tests 和前端 Monitor V2 Vitest。
- 运行前端 typecheck/build、必要后端 build、gofmt、`git diff --check`。
- 启动本地页面，使用桌面与 390px 截图核对百分比、状态、Pro 顺序、时间线和横向溢出。
- 写候选 handoff 并提交到 T29 分支，随后仅交回根发布总控串行审查、合并、推送、预检、蓝绿发布和线上验收。
