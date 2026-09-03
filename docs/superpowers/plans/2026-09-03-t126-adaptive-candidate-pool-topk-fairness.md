# T126 自适应候选池、Top-K 与公平性 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** 为原生 OpenAI 调度器增加有界、窗口化的候选池/Top-K/公平性自适应策略。

**Architecture:** 在现有候选过滤与统一质量排序之后增加纯策略决策层；策略读取窗口化运行时指标，输出有效模式、Top-K 与探索比例，继续复用现有 selection order、decision details 和原生利润门禁。

**Tech Stack:** Go、现有 Sub2API service/config 测试。

**Spec:** `docs/superpowers/specs/2026-09-03-t126-adaptive-candidate-pool-topk-fairness-design.md`

## Global Constraints

- 不改变统一质量评分定义或评分权重。
- 不改变 Sub 原生利润控制；异常保持 fail-open。
- 不新增数据库、外部控制面、调度配置页或平行账务源。
- 自适应结果受硬上下限保护，异常回退静态配置且不阻断请求。

### Task 1: 自适应策略模型与纯函数

**Files:**
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go`

- [ ] 增加有界策略配置和规范化默认值。
- [ ] 编写候选不足、分数差距、故障域集中、NaN/Inf、上下限与单档位滞后的失败测试。
- [ ] 实现纯策略函数，输出 effective mode、Top-K、exploration ratio 和 reason。
- [ ] 运行定向 service/config 测试并提交。

### Task 2: 接入调度决策与观测

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_scheduler_log.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go`

- [ ] 在硬过滤后调用策略，保持排除账号、sticky 和原生利润门语义。
- [ ] 将有效参数与触发原因写入既有 decision details/log projection。
- [ ] 覆盖集成场景和配置关闭回归。
- [ ] 运行 gofmt、定向测试、`go build ./cmd/server` 与 `git diff --check`，提交并生成交接。
