# 2026-08-09 生产更新与非 main 工作区清理记录

## 生产发布基线

- 上一生产提交：`abb87a0a8ba4d57cfcf8e38065c5459825062346`
- 本次应用生产提交：`3fb79f5291961a99a50d13b3306937a8db156b04`
- 应用 tree：`380600ff2b5718343d64f4729f984f4d9e3ea2ac`
- 部署记录：`20260809T154902Z-production-2353963`，`result=succeeded`，`state=promoted`，`rolled_back=false`
- 活动槽：`blue`，上游：`sub2api-blue:8080`
- 迁移哈希：`1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98`
- 线上验收：`/healthz` alive，`/readyz` ready，首页 HTTP 200，管理 API 未认证 HTTP 401；API/worker healthy 且 restart count 为 0；PostgreSQL/Redis/Caddy 容器身份保持。

## 本次更新内容

1. **上游故障韧性与恢复可观测**：并入缓存感知的重试/故障转移、account-model 短暂故障分类、half-open/cooldown 恢复、流式读错误传播、恢复排除和告警/审计指标；重试计费记录改为幂等并持久化 attempt 元数据。
2. **用量详情的上游 Sub 真实成本与利润**：持久化上游 request ID，按本次请求的上游账号查询 Sub 原生用量流水，管理员用量详情展示上游实际扣费与本站利润，移除重复请求 ID 展示。
3. **Monitor V2 当前配置边界**：监控只纳入当前启用且绑定有效活动分组的渠道；端点、凭据、模型或分组身份变化时建立新的历史起点，防止旧配置污染当前 7 天可用率。
4. **GPT 文本账号分组基线**：并入基于真实请求优先、探测补足的 Pro/Plus/特惠分组分析、利润门槛、主力/次级/备用基线与生产应用记录。正在补跑的 14 个异常账号仍由保护工作区继续处理。
5. **Resend SMTP 诊断证据**：并入 SMTP 约 20 秒超时的受控复现、审查与 fail-closed 结论；本包未把证据不足的 deadline 方案当成已修复行为。
6. **发布与迁移门禁收口**：新增 usage attempt reconciliation 与 channel monitor history boundary 迁移；仅 allowlist 生产当前哈希到本次候选哈希的精确维护过渡，保留镜像、迁移、健康、共享容器身份与回滚门禁。
7. **图片账号成本配置工具兼容性**：发布包包含对生产 Compose project 名的脚本兼容修复与回归。

## 保护例外

- 任务：“制定上游账号测试方案”
- 工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/gpt-group-baseline-apply`
- 分支：`codex/gpt-group-baseline-apply`
- 保留基线：`ea36d5b2443dd06d97f835279f2a92271ca14029`
- 未提交内容：`docs/project/project-progress.md` 新增 14 个异常 GPT 账号补跑复核登记。

## 待清理工作区恢复点

- `/private/tmp/sub2api-monitor-release.Wg2PIv` → detached `abb87a0a8ba4`
- `/private/tmp/sub2api-release-20260809` → detached `d8cdc50d472a`
- `/private/tmp/sub2api-release-abb87` → detached `33e5e25f0ae2`；其 patch-id `ee605422a2ed996a1255e49cf0ba6ef1134fd086` 与 `main` 中 `d9c68f86b72d59797b571e916c969121e21dff60` 完全一致。
- `/private/tmp/sub2api-resilience-integration-20260809` → `codex/upstream-resilience-integration@525a2d95ddb7`
- `/Users/gongtengxinwen/Documents/sub2api-upstream-resilience-spec` → `codex/upstream-resilience-implementation@e55fc3c9ea22`
- `.worktrees/gpt-group-baseline-analysis` → `codex/gpt-group-baseline-analysis@ba30a0bafa09`
- `.worktrees/image-account-fixed-cost` → `codex/image-account-fixed-cost@15cf3303d6b8`
- `.worktrees/monitor-v2-current-config` → `codex/monitor-v2-current-config@218bb9f1d5be`
- `.worktrees/monitor-v2-minimal-fix` → `codex/monitor-v2-minimal-fix@fd9d4e95d726`
- `.worktrees/release-ready-3fb79f529` → detached `3fb79f529196`
- `.worktrees/release-ready-c8e9df44a` → detached `c8e9df44a014`
- `.worktrees/release-ready-d73e82254` → detached `d73e82254dd0`
- `.worktrees/resend-email-configuration` → `codex/resend-email-configuration@a555b7d3e46c`
- `.worktrees/resend-smtp-timeout` → `codex/resend-smtp-timeout@191435b4ca8c`
- `.worktrees/usage-upstream-actual-cost` → `codex/usage-upstream-actual-cost@b246e0c668dd`

上述候选在清理前均为干净工作区；除已证明 patch 等价的 detached `33e5e25f0` 外，相对 `main` 的独有提交数均为 0。已缺失的 `/private/tmp/sub2api-monitor-release.LTDml9`、`/private/tmp/sub2api-release-192da`、`/private/tmp/sub2api-release-94f8b4d8e.IHqR8z` 只清理 Git 注册记录。

## 清理结果

- 上述 15 个实际存在的待清理 worktree 已通过 `git worktree remove` 删除，删除前逐个复核 `dirty=0`。
- 9 个对应命名分支均确认 `main..branch=0`，已通过非强制的 `git branch -d` 删除。
- 3 条目录已缺失的临时 worktree 注册已由 `git worktree prune` 清理。
- 当前 `git worktree list` 仅剩根目录 `main` 与受保护的 `.worktrees/gpt-group-baseline-apply`；本地分支也仅剩 `main` 和 `codex/gpt-group-baseline-apply`。
- 受保护工作区仍位于 `ea36d5b2443dd06d97f835279f2a92271ca14029`，`docs/project/project-progress.md` 的 14 账号补跑登记仍完整保留。
- 清理后生产 `https://api.xingqiaolab.top/healthz` 返回 `alive`，`/readyz` 返回 `ready`。
