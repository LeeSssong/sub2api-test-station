# T19 Monitor V2 缓存命中率有效样本口径修正生产收口

## 发布身份

- 发布源：已推送根 `main@949f200f3ad6fc0455cef7788abdc941a756c65f`
- source/tested tree：`47df2074c89762182213254f6fae6f9d1210ed5e`
- 迁移哈希：`bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951`（无变化）
- 0600 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-949f200f3-t19-cache-eligibility-v1.json`
- 宿主记录：`/var/lib/sub2api/release-records/20260817T181347Z-production-2379500.json`
- 结果：`succeeded/promoted`，`rolled_back=false`，`downtime_required=false`
- 活动槽：`green`，活动上游 `sub2api-green:8080`
- 不可变镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-949f200f3ad6fc0455cef7788abdc941a756c65f-891da5170d353328748a6a51e7567c7db5143a499743ee028956cf6c76078ec8`

## 线上交叉验收

API 两个窗口的 `generated_at` 均为 `2026-08-17T18:22:36Z`，`contract_version=5`。生产 PostgreSQL 使用与实现完全相同的 `actual_cost > 0`、Token/历史空 billing_mode、图片视频字段全零谓词。

| 窗口 | 分组 | API eligible/cache samples | SQL eligible | API 推导 hit | SQL hit |
|---|---|---:|---:|---:|---:|
| 24h | GPT-Pro | 612 | 612 | 600 | 600 |
| 24h | GPT-Plus | 128 | 128 | 123 | 123 |
| 24h | GPT-特惠分组 | 2771 | 2771 | 2713 | 2713 |
| 7d | GPT-Pro | 4878 | 4878 | 4779 | 4779 |
| 7d | GPT-Plus | 28591 | 28591 | 27765 | 27765 |
| 7d | GPT-特惠分组 | 6499 | 6499 | 6290 | 6290 |

- API 24h/7d 均 HTTP 200；公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。
- API/worker 使用同一不可变镜像；PostgreSQL、Redis、Caddy 容器身份保持不变。
- 无迁移、无配置、无生产数据写入；账务、价格、倍率、缓存策略和 API/前端合同未改变。

## 保留边界

- SQL 使用 API `generated_at` 作为截止点，避免实时流量在两次读取之间造成样本漂移。
- 回滚依据为上一活动 blue 槽、上一不可变镜像和 release-state/release-record；本任务无数据回填或不可逆写入。
