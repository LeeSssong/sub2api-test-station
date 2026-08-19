# T26-R1 CodexRadar 三标签社区测试矩阵修复设计

## 1. 问题与证据

T26 已在 Monitor V2 中接入 CodexRadar `/api/radar-insights` 四类站长推荐，但用户已批准截图下方的“综合智能 / 软件工程能力 / 视觉空间推理”社区测试矩阵未实现。

2026-08-19 重新核对 CodexRadar 原站与公开只读接口：

- `/api/radar-insights`：四类推荐，不含全量社区矩阵。
- `/api/intelligence-efficiency-metrics`：schema 3 / `equal_latest_3`，返回软件工程模型档位的样本、IQ、平均费用和平均耗时。
- `/api/visual-spatial-reasoning`：schema 1 / `visual_spatial_reasoning_summary`，返回视觉空间模型档位的有效题数、IQ、平均费用和平均耗时。
- 原站综合 IQ 口径为同一 `model + effort` 的软件 IQ 与视觉 IQ 等权几何平均 `sqrt(software_iq * visual_iq)`；只纳入两个维度都有有效成绩的档位。综合费用/耗时按两个维度各自样本数加权，综合样本数为两维度样本数之和，更新时间取两源较早值。

根因是 T26 规格只覆盖了截图上半部的四类推荐，而非线上加载失败。

## 2. 目标与非目标

### 目标

1. 在现有 `CodexRadarRecommendations` 四类推荐下方增加三标签社区测试矩阵。
2. 展示社区众测说明、来源更新时间、全部模型/档位卡、样本数、IQ、平均费用和平均耗时。
3. 综合智能精确复刻 CodexRadar 当前组合口径；软件工程与视觉空间使用各自公开原始数据。
4. 保持固定目标、GET-only、3 秒超时、512 KiB 上限、严格校验、60 秒缓存与最近成功快照回退。
5. 390px 不产生整页横向溢出；矩阵容器内允许受控横向滚动。

### 非目标

- 不实现散点图、历史曲线、体感投票、额度雷达或截图框外内容。
- 不混入本站监控、计费、评分、模型或推荐逻辑。
- 不 iframe，不开放任意 URL 代理，不持久化第三方数据，不向 CodexRadar 发起写请求。
- 不修改 Monitor V2 本站 snapshot 口径。

## 3. 方案比较与选择

### A. 前端直连 CodexRadar

改动小，但受 CORS、超时、schema 漂移和失败回退影响，也破坏 T26 已批准的同域安全边界。不选。

### B. 扩展原有 recommendations DTO/服务，三个上游绑定一个快照

单端点简单，但社区矩阵任一上游异常会连带四类已上线推荐，并且两种 schema 与推荐 schema 耦合。不选。

### C. 新增同命名空间的 community 只读服务/DTO/endpoint（选择）

复用 T26 的 HTTP 边界与路由鉴权，但将 recommendations 与 community 缓存/快照/失败隔离。后端同时读取两个固定公开 GET 接口，严格校验后按原站口径产生三个独立 DTO；前端只消费经验证的同域响应。

## 4. 端到端数据流

1. 登录用户访问 Monitor V2，前端在加载既有推荐的同时请求 `GET /api/v1/monitor-v2/codexradar-community`。
2. handler 不接收任何上游目标、method、body 或算法参数。
3. service 仅向编译期固定的两个 HTTPS URL 发起 GET，每个响应最多 512 KiB，共用 3 秒 client timeout。
4. service 分别校验 software schema 3 与 visual schema 1，拒绝空集合、重复 `model+effort`、非有限/负数数字、超长字符串、无效时间和超出有界数量的档位。
5. software/visual 保留各自原始数值；comprehensive 只对两集合交集计算几何平均 IQ 和样本加权费用/耗时。
6. 成功值在进程内缓存 60 秒。过期后刷新失败则返回最近成功快照并标记 `stale=true`；从未成功则 503。
7. 前端严格解析三个标签与每张卡片，默认显示综合智能，切换标签仅更换本地已加载视图。

## 5. 接口与字段合同

`GET /api/v1/monitor-v2/codexradar-community`，沿用 authenticated + panel heavy rate limit。

```json
{
  "generated_at": "RFC3339",
  "stale": false,
  "tabs": [
    {
      "key": "comprehensive",
      "source_updated_at": "RFC3339",
      "points": [{
        "model": "gpt-5.6-sol",
        "effort": "low",
        "samples": 422,
        "iq": 78.29,
        "average_cost_usd": 1.85,
        "average_duration_minutes": 12.12,
        "software_samples": 336,
        "visual_samples": 86,
        "software_iq": 78.12,
        "visual_iq": 78.46
      }]
    },
    {"key": "software", "source_updated_at": "RFC3339", "points": []},
    {"key": "visual", "source_updated_at": "RFC3339", "points": []}
  ]
}
```

software/visual 不需要的组合字段使用 `omitempty`，前端不展示。标签顺序固定为 comprehensive/software/visual。

## 6. 视觉与交互

- 区域继续使用现有 CodexRadar 深色容器，在四类推荐与社区矩阵之间用细分隔线建立层级。
- 三个胶囊标签保留原站图标与中文：`🧠 综合智能`、`💻 软件工程能力`、`🧩 视觉空间推理`。
- 标题右侧显示当前标签来源更新时间；下方显示“社区众测数据 · 每多一份贡献，结果就更准确。”。
- 每张卡左上为模型 + effort，右上为 IQ，下方紧凑显示样本、费用、耗时。模型家族使用稳定强调色，不依据本站数据改色。
- 桌面使用密集自适应卡片矩阵。窄屏将矩阵放入 `overflow-x:auto` 的受控容器，每张卡保持可读最小宽度，外层 `min-width:0/overflow:hidden`。

## 7. 失败、安全与兼容

- community 失败只显示“社区测试数据暂时不可用”，不隐藏已成功加载的四类推荐。
- 不向客户端透出上游 URL、原始错误或响应体。
- 不接受查询参数转发，不发送 Cookie/凭据/请求体到第三方。
- 无数据库迁移、配置 schema 或前端路由变化。

## 8. 验收与测试

- Go service：固定两目标/GET-only、两 schema 严格校验、组合口径、60 秒缓存、任一远端失败的最近快照回退、无快照 503、超大响应拒绝。
- Go handler/routes：成功/stale/503，仅登录态可访问，路由不接受任意上游参数。
- 前端 DTO：三标签顺序、时间、有界点数、非负有限数字、固定同域 endpoint。
- 组件：默认综合、标签切换、全部卡片指标、更新时间、众测文案、stale 和失败隔离。
- 工具验证：gofmt、直接相关 Go 测试与必要 build，直接相关 Vitest、typecheck、production build、`git diff --check`。

## 9. 发布、回滚与风险

- 无迁移、无生产数据修改、无新配置，预期 `downtime_required=false`，最终以根合并后预检为准。
- 回滚为撤销 T26-R1 候选提交或使用上一已验证蓝绿槽；既有四类推荐和 Monitor V2 主体不依赖 community endpoint。
- 主要风险是第三方 schema 漂移和数据量增长；通过严格 schema、有界数量/大小、快照回退与两区域失败隔离控制。

## 10. 批准记录与自审

- 用户已明确该工作是对已批准截图范围的缺陷修复，不再询问产品批准。
- 自审结论：数据源、组合算法、失败边界、鉴权、移动溢出、非目标、测试、发布和回滚均已闭环；无待决产品问题。
