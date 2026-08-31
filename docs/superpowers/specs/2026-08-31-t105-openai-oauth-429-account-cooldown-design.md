# T105 OpenAI OAuth 429 账号级原生限流恢复

## 问题与证据

OpenAI OAuth 的瞬时 429 在 `openai_account_runtime_block_fastpath.go` 中进入同账号重试窗口；`RateLimitService.handle429` 为避免提前阻断重试，在窗口内不调用 `SetRateLimited`。现有 handler 的同账号重试预算约 5 秒，短于 2 分钟窗口，因此预算耗尽后会直接将账号加入当前请求的 `failedAccountIDs` 并切号，数据库没有账号级限流状态，后续请求仍可反复命中同一账号并返回 502。

## 目标与非目标

目标是：同账号重试耗尽或跨账号 failover 前，对 OpenAI OAuth 账号调用 Sub 原生账号级 `SetRateLimited`；可靠 reset 时间优先，无可靠 reset 时使用 5 分钟；运行时 blocker 与持久状态一致，并保留管理员 `clear-rate-limit` / `recover-state` 恢复入口。

非目标：不做模型级限流，不改变 OpenAI capacity shed、图片专用限流、model-not-found、非 OAuth 429、Spark shadow 429 或请求级 `failedAccountIDs` 语义；不移除同账号有限重试。

## 方案与选择

方案 A（推荐）：在公共 OpenAI failover 转换点调用新的账号级持久化辅助方法，使用可选 `SetRateLimitedIfLater` 防止短 fallback 覆盖更长官方 reset。方案 B 是把持久化逻辑放进每次 429 处理，会破坏同账号重试；方案 C 是只延长内存 blocker，无法影响跨请求调度。选择 A。

## 控制流与契约

首次瞬时 OAuth 429：保持现有 retry window，`SetRateLimited` 调用数为 0。handler 仍可在有限预算内重试同账号。预算耗尽、不可安全重试或准备切换账号时，调用账号级恢复方法：解析可靠 reset（Retry-After / OAuth quota reset）并使用该时间，否则 `now + 5m`；只延长已有更晚 reset。随后写入 runtime blocker，清理请求窗口状态，并继续现有 failover。管理员清除限流时沿用原生 `ClearRateLimit`，同时清理 runtime blocker。

## 验收矩阵

覆盖窗口内零持久化、重试耗尽持久化、跨账号前持久化、无 reset 五分钟 fallback、官方 reset 优先、runtime/持久状态一致、管理员清除恢复，以及至少一个非 Responses 路径；capacity shed、图片专用、模型级和 shadow 分支保持原行为。

## 测试、发布与回滚

先运行直接 Go 单元测试验证 RED，再实现并运行 service/handler 相关测试、`gofmt`、`git diff --check`，必要时构建 server。候选达到 `READY_FOR_ROOT_REVIEW` 后刷新到最新 `main`，由根总控合并、推送并按既有本地/宿主发布链部署。回滚使用发布链上一已验证槽位；不得从候选直接部署。

## 批准记录

用户已确认仅账号级、复用 Sub 原生限流状态、保留有限同账号重试、可靠 reset 优先、无可靠 reset 固定 5 分钟，以及管理员可手动恢复。
