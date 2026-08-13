# T03-R1 Task 4 fix round 3 implementation report

状态：本地实现与专项验证完成，等待独立复审；未合并、未推送、未部署、未进行线上验证。

## 范围

修复 `task-4-rereview-r2.md` 唯一 Important：`ReviewSelected` 在前序 `CreateReview` 已成功提交后，后续输入 `validateMoney` 失败时，只记录 aggregate `failed`，没有逐行审计已提交 rows。

本轮仅修改 Task 4 service implementation/test、项目进度总账和本报告；未修改 API、UI、schema、migration、generated Ent、`usage_logs` 或 Task 5+。

## TDD 证据

- RED：新增 `TestAccountFinancialServiceReviewSelectedAuditsCommittedRowsBeforeLaterValidationError` 后，原实现运行 `go test ./internal/service -run TestAccountFinancialServiceReviewSelectedAuditsCommittedRowsBeforeLaterValidationError -count=1` 失败，实际仅产生 `audits=1`。
- GREEN：修复后同一用例通过。

## 实现

- `ReviewSelected` 保存每个已提交结果对应的输入身份，保证审计使用该行自己的 `ReviewedBy` 与 `RequestID`。
- 后续 validation 失败时，先对 `out` 中每个已提交结果逐行写入 `updated`/`skipped` 审计，再对当前失败输入单独写入 `failed=1` 审计。
- repository failure 路径同步使用逐行输入身份，避免沿用失败行身份污染前序审计；成功路径也按行身份记录。

## 验证

通过：

- `go test ./internal/service -run 'TestAccountFinancialServiceReviewSelectedAuditsCommittedRows(BeforeLaterError|BeforeLaterValidationError)$' -count=1`
- `go test ./internal/service -run 'TestAccountFinancialService(ReviewSelected|AuditsValidationFailures|AuditsEveryMutation)' -count=1`
- `go vet ./internal/service`
- `git diff --check`

未运行：PostgreSQL integration（既有环境阻断 `panic: rootless Docker not found`）。

`downtime_required=false`（仅本地 service/test 代码，无迁移、配置或部署变更）。

