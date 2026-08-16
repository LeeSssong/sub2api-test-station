# T14 用量详情上游扣费字段兼容实现报告

## 交付状态

候选实现完成，等待最终 whole-branch review 与根总控授权；未合并、未推送、未部署。

## 基线与提交

- 任务基线：`main@4f31ec3dd010dc3d2b6c5caaacadddce1adb84a2`
- 分支：`codex/t14-usage-detail-field-compat`
- 源码实现 tip：`11271c034`（包含实现、定向测试和类型修复）；随后仅追加文档证据修复。
- 规格/计划：T14 approved spec 与 `8e3d0e17a` plan

## 变更

- `getCostEvidence()` 在前端 API 边界将 PascalCase 与 snake_case 字段统一为既有 snake_case 类型；snake_case 优先，空值和数值精度原样保留。
- 添加 API contract tests：两种命名、混合优先级、空/缺失/primitive/array 响应和网络错误传播。
- 添加真实 API-boundary 到 `UsageDetailDialog` 的 PascalCase focused test，以及既有管理员成本/利润、不可用和权限隔离回归覆盖。
- 组件逻辑、后端 DTO、账务口径、持久化、权限和 API 路径未改。

## 验证

- focused API/UI tests：34 passed across 3 files。
- `pnpm typecheck`：PASS。
- `pnpm build`：PASS。
- `git diff --check 4f31ec3dd..HEAD`：PASS（规格文档既有行尾空白已清理）。
- 变更范围仅限 T14 规格/计划、前端 API 和直接组件/API 测试；后端、迁移、配置、Actions、其他页面、生产数据均无命中。

## 发布与回滚

- 无迁移、无配置变化、无依赖变化；预期 `downtime_required=false`。
- 根总控授权后从合并并验证的 `main` 进入既有蓝绿发布链。
- 回滚为上一已验证生产提交重新发布，不涉及数据回滚。

## 未验证项

尚未推送、部署或进行线上管理员验收；这些动作只由根总控在候选最终复审通过后执行。
