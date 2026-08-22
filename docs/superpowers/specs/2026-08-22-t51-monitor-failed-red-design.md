# T51 Monitor V2 检测失败红色状态设计

## 问题与证据

`account_monitor_results` 时间桶查询当前只投影 `operational/unavailable`。桶内只有 `failed` 结果和桶内没有任何结果都会得到 `unavailable`，同时延迟均可能为 `NULL`。前端再用 `unavailable && latency_ms === null` 推断 NO DATA，导致真实检测失败被显示为灰色无数据。

## 目标与非目标

- 检测成功：绿色 UP。
- 至少有一次检测但没有成功：红色 DOWN。
- 没有任何检测结果：灰色 NO DATA。
- 不改主动探测、账号资格、可用率计算、评分、调度、迁移或配置。

## 方案

选择在时间线点增加必填布尔字段 `has_result`。仓储按 `COUNT(sr.id) > 0` 生成；空桶为 `false`，任何成功或失败结果桶为 `true`。前端只用该字段判断 NO DATA，不再用延迟是否为空猜测。

备选方案一是新增第三个 `no_data` status，但会混合健康状态与证据状态。备选方案二是给失败虚构延迟，数据语义错误。两者均不采用。

## 合同与兼容

Monitor V2 合同从 v7 升至 v8，`timeline[].has_result` 为必填 boolean。前后端同一次原子发布，不保留静默降级，避免旧负载继续误判。

## 验收与发布

- 仓储投影保留 `has_result`。
- 前端失败且无延迟时仍为红色 DOWN。
- 空桶保持灰色 NO DATA。
- Monitor V2 直接相关 Go/Vitest、typecheck、build 和 diff-check 通过。
- 无迁移、无配置、无生产数据写入，预计 `downtime_required=false`；从合并后的根 `main` 走既有蓝绿链。
