# T90 账号监控卡片交接

## 状态

READY_FOR_ROOT_REVIEW（未合并、未推送、未部署）

## 基线与工作区

- 基线：`main@84dc3c40a13a776ce87bff1a1f0973599d630cd9`
- 分支：`codex/t90-account-monitor-real-request-card`
- Worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t90-account-monitor-real-request-card`

## 本次实现

- 新增真实请求证据、成功率/TTFT P95 全站排名、分组利润率与排名、上游倍率来源、24 桶真实性能时间线 DTO。
- 原生仓储增加 usage_logs + ops_error_logs 批量去重聚合；错误优先，主动探测不进入真实请求统计。
- 卡片增加真实请求成功率、TTFT P95、利润率、Sub 原生优先级、上游声明倍率和真实性能柱状图；保留详情、账号操作、主动探测与手动模型探测入口。
- 详情弹窗增加账号对象全部可见字段展示，移除额外脱敏提示。

## 验证

- `go test ./internal/repository ./internal/handler/admin -run 'AccountMonitor' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过。
- `git diff --check`：通过。
- 既有 `AccountMonitorCard.spec.ts` 有 2 项旧版断言与 R2“移除说明性文字/改用真实请求柱图”冲突，未为旧断言回退新设计；需根审阅决定是否更新测试契约。
- `go test ./internal/service -run 'AccountMonitor'` 中两个与账号池时间状态相关的既有测试失败，失败点不在 T90 新增路径。

## 迁移/配置/发布

- 无数据库迁移。
- 无配置变化。
- 未写生产数据，未触碰主站。
- `downtime_required`：未执行发布预检；由根任务在合并后判断。

## 回滚

删除本候选提交或回退到基线 `84dc3c40a`；无数据回滚需求。

## 剩余风险

- 分组利润率已提供分组批量查询，但需要根审阅确认账务字段与站内收入口径的最终映射。
- 详情弹窗按接口实际返回对象展示；接口未返回的秘密不会由前端自行读取。
