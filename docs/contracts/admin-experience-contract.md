# Sub2API 管理员体验合同

本合同是定制外置期间的管理员黄金样本。所有外置页面必须保持同域、同一管理员会话和可回退的原生入口；外置实现不得要求管理员重新登录、复制 Bearer、切换到 relay-ops 独立控制面或改变既有 URL。

## 入口与认证

- 同域登录：管理员从 `https://api.xingqiaolab.top/login` 登录，外置页面继续使用 Sub2API 的 `/api/v1/auth/login`、`/api/v1/auth/login/2fa` 和 `/api/v1/auth/me` 会话。
- 2FA：启用 2FA 时必须完成原生第二步验证；控制面不可绕过、缓存或重新实现 2FA。
- 原 URL：保留 `/admin/accounts/monitor`、`/admin/operations/account-profitability`、`/admin/ops`、`/admin/usage`、`/admin/groups`、`/admin/accounts`、`/admin/channels/pricing` 和 `/admin/settings`。旧 `/ops` 只做原生 `/admin/ops` 重定向。
- 权限失败：未登录返回原生登录页；非管理员返回原生 403/管理员首页；控制面不可把 401 当作主站会话失效并撤销会话。

## 页面黄金样本

每个清单能力都必须保持现有页面的字段语义和操作节奏：

- 字段：账号/分组/请求 ID、模型、实际响应模型、Token、收入、成本、倍率、余额、状态、评分、时间戳和证据来源；缺失值显示“待对账/未知”，不能用推算值冒充已确认事实。
- 筛选：支持原有账号、分组、模型、状态、时间范围和成本/对账状态筛选；筛选条件刷新后可恢复。
- 排序：支持表格列和账号卡片当前的成功率、延迟、评分、利润及更新时间排序，并保持稳定的二级 ID 排序。
- 分页：列表分页保留当前页、页大小和总数；导出必须明确是当前筛选全集，不受当前页截断。
- 刷新：页面刷新和单卡刷新显示进行中状态、最后采集时间和失败原因；不得清空上一次可审计事实。
- CSV：盈利、账务和对账列表提供 CSV 下载，列名与页面字段一致，使用 UTF-8、稳定顺序和当前筛选条件。

## 控制面降级

当外置控制面不可用时，管理员仍可登录并访问原生 `/admin`、`/admin/accounts`、`/admin/groups`、`/admin/usage`、`/admin/settings` 和 `/admin/ops`。外置页面必须显示“控制面暂时不可用”、最近一次带时间戳的只读快照和重试入口；不得伪造余额、成本、利润、倍率或对账成功。写操作只在官方 API 可用且成功审计后确认，控制面恢复后通过幂等事件重放补齐读模型。

## 验收黄金样本

验收至少覆盖：同域登录与 2FA、每个原 URL、字段/筛选/排序/分页/刷新/CSV、控制面不可用降级、管理员审计记录和蓝绿发布后的回滚入口。证据引用见 `docs/superpowers/reports/2026-08-08-account-monitor-probe-source-and-card-affordances-production-verification.md`、`docs/superpowers/reports/2026-08-08-monitor-probe-admin-cost-production-verification.md` 和 `docs/runbooks/sub2api-blue-green-production-deployment.md`。
