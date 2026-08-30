# Monitor V4 有效失败与截尾平均实施计划

- [x] 在 `account_monitor_repo.go` 排除客户端责任和明确模型不支持错误。
- [x] 将 TTFT/总耗时计算改为前后 5% 截尾平均，保留现有返回字段。
- [x] 更新 SQL 合同回归断言。
- [ ] 运行 Go Monitor V4 定向测试（本机 Go 1.27 工具链下载被网络 EOF 阻断）；`git diff --check` 已通过。
- [x] 生产只读重算新口径并记录结果：GPT-Pro `5.12s / 4.75s`、GPT-Plus `6.90s / 13.75s`、GPT-特惠 `5.86s / 10.80s`（TTFT/总耗时）。
