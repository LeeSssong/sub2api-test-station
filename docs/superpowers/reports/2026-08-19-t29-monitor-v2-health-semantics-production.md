# T29 Monitor V2 二态健康展示与统一指标口径生产验收

## 发布身份

- Source commit: `e0b2d99b91dcbaa20b1cb4d859cd58182795c60f`
- Source/tested tree: `34ace5c193dd1c647215ed6894c7ec1945dd69b4`
- Migrations hash: `18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`
- Evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-e0b2d99b9-t29.json` (`0600`)
- Host record: `/var/lib/sub2api/release-records/20260819T102718Z-production-3917.json`

## 发布结果

既有本地/宿主蓝绿控制器返回 `downtime_required=false`、`result=succeeded`、`state=promoted`、`rolled_back=false`，活动槽为 `blue`，活动上游为 `sub2api-blue:8080`。release-state 与最终记录均绑定上述 source/tree/tested tree，迁移哈希保持不变。

活动 API、worker 与 model-detector 均使用镜像 ID `sha256:70f02ffa0ef8e555c28a3eee10a6de442bce0e9cd72457a5bcf9b9fca1f46310`，状态 healthy，重启计数为 0。PostgreSQL、Redis 与 Caddy 保持运行，重启计数为 0。

公网验证：

- `/healthz`: HTTP 200，`{"status":"alive"}`
- `/readyz`: HTTP 200，`{"status":"ready"}`
- `/health`: HTTP 200，`{"status":"ok"}`

## 合并后门禁

- `go test ./internal/repository ./internal/service ./internal/handler -run 'TestMonitorV2' -count=1` 通过。
- `go build ./cmd/server` 通过。
- Monitor V2 前端 8 个测试文件、34/34 通过。
- `vue-tsc --noEmit`、`vue-tsc -b` 与 Vite production build 通过。
- gofmt、`git diff --check`、零迁移、零发布脚本和零 GitHub Actions 范围检查通过。

## 生产专项验收

- 登录态 Monitor V2 API 实际响应 `contract_version=6`，旧 availability/cache-hit/success/eligible 字段扫描为空。
- API 卡片和时间线状态值集合严格为 `operational / unavailable`；页面中文只出现“运行中 / 服务不可用”。
- GPT-Pro 排名第一且 `is_flagship=true`；页面显示“旗舰”，没有复制 Plus 的真实指标。
- TTFT、总延迟和 TPS 逐组使用相同 `sample_count`；页面保留真实毫秒/秒、TPS 与倍率。
- 卡片、整体状态、时间线 title 与 aria 文本未出现百分号、成功率、可用率、缓存命中率、有效调用或“服务波动”。
- 1432px 页面 `innerWidth=1432`、`clientWidth=scrollWidth=1425`；390px 页面 `innerWidth=390`、`clientWidth=scrollWidth=382`。三个监控卡片均无重叠或整页横向溢出。
- 生产页面控制台没有 error/warn。

视觉证据：

- `/Users/gongtengxinwen/Documents/sub2api-archives/t28-t29-final-e0b2d99b/visuals/t29-production-desktop.png`
- `/Users/gongtengxinwen/Documents/sub2api-archives/t28-t29-final-e0b2d99b/visuals/t29-production-mobile-390.png`

## 回滚与归档

上一活动 green 槽继续保留 T28 不可变镜像 `sha256:359c1018f9bc4cf841d5659c68c5d34728526c8a5965a2642e52fd6454e11ad0`。本次无迁移或生产数据写入；回滚使用既有蓝绿控制器恢复上一活动槽和 v5 前后端组合。

T28/T29 恢复 bundle 为 `/Users/gongtengxinwen/Documents/sub2api-archives/t28-t29-final-e0b2d99b/t28-t29-refs.bundle`，SHA-256 `a7815ce5a9111b07aea9026c6456f2d830019baacc142f46a5660451f086e741`，`git bundle verify` 通过。
