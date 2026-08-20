# T42 Monitor V2 时间新鲜度与刷新可靠性验证

- 基线：`main@3ac10d8473923a9b017c4826024680c4361e8323`
- 分支：`codex/t42-monitor-v2-freshness`
- 状态：READY_FOR_ROOT_REVIEW（未合并、未推送、未部署）

## 实现

- 原生 `account_monitor_results.checked_at` 通过单条 Monitor V2 SQL 投影为分组级 `source_updated_at`，沿 service/handler 可选字段输出；无最新探测时省略。
- 合同版本保持 `7`，24/28/30 固定时间桶、状态/指标和可见性语义保持不变。
- Monitor V2 卡片显示“探测于 <时间>”或“暂无最新探测”；时间线文件保持 T41 负责的现状。
- 周期/窗口 GET 失败时保留旧快照并在 5 秒后重试；取消、隐藏和卸载仍清理计时器，请求成功后恢复配置刷新间隔；不再因周期读取瞬时失败触发 fallback。

## 验证命令

```text
go test ./internal/service ./internal/handler ./internal/repository -run 'MonitorV2|monitor v2' -count=1  PASS
go test ./internal/service ./internal/handler ./internal/repository -run 'TestAccountMonitorRepositoryProjectMonitorV2Groups|TestMonitorV2' -count=1  PASS
pnpm vitest run src/features/monitor-v2/__tests__  8 files / 37 tests PASS
pnpm run typecheck  PASS
pnpm run build  PASS
 git diff --check  PASS
```

## 迁移、配置、停机

- 数据库迁移：无。
- 配置变化：无。
- `downtime_required=false`；仅源码、测试和文档变化。

## 回滚与风险

回滚为恢复本分支提交。新增字段为可选，旧 v7 客户端兼容；剩余风险是未执行生产线上验收，等待根总控合并、发布预检与专项验证。
