# 账号监控最终审查修复报告

日期：2026-08-03（Asia/Shanghai）

## 状态

本轮只完成代码修复和本地验证，尚未推送、部署或进行生产验收。项目进度台账继续保持“进行中”。未修改 relay-ops 页面、调度 `priority` 语义或账务业务范围。

## 修复内容

- 分组质量与健康聚合现在只接收“参与监控”的账号 ID。分组合并聚合接口扩展为 `group_id + account_ids + since`，`usage_logs` 与 `ops_error_logs` 两个 SQL 数据源均按账号范围过滤；暂停/不参与监控账号不会用历史样本污染分组成功率、TTFT P50 或延迟 P95，也不会产生质量评分证据。
- `listMonitorAccounts` 使用稳定 ID 排序和首行保留策略去重，后续 `ids`、行投影、分组投影和聚合请求均只使用唯一账号。
- `AccountMonitorCard` 增加 `scope` 属性。视图在全站传入 `all`、分组传入 `group`；全站卡片不再显示“质量评分/组内排名”区块，分组仍保留。
- `AccountMonitorView.loadOperations` 为逐账号账务请求增加 6 路并发池，保留全站/分组请求范围和 generation 防旧响应覆盖语义。

## TDD 与验证

先增加失败回归并确认失败：重复账号仍返回 4 行、分组合并未收到账号范围、全站卡片仍显示评分、12 个账号请求并发数为 12。随后实现最小修复并验证：

```sh
cd upstream/sub2api/backend
go test ./internal/service
go test ./internal/repository -run '^TestAccountMonitorRepository'

cd ../frontend
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm run typecheck
```

结果：后端服务测试通过；账号监控仓储回归 7 项通过；前端 2 个测试文件、36 项通过；`vue-tsc --noEmit` 通过；`git diff --check` 通过。完整 `go test ./internal/repository` 仍有工作区原有 `usage_log_repo_request_type_test.go` 的 4 项扫描列数失败（`got 59 want 58`），与本轮账号监控改动无关。

## 风险与后续

- 新接口需要随本轮 Sub2API 后端一起发布；旧二进制不具备账号范围参数时不能与新服务层混用。
- 6 路并发是前端请求池上限，不改变后端对账接口或数据库容量；后续线上需观察账务接口延迟和失败率。
- 只有完成推送、生产部署和线上验证后，才能把项目进度台账改为“已完成”。
