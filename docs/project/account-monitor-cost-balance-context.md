# 账号监控成本、余额与评分权重任务上下文合同

**生效日期：** 2026-08-06

**适用范围：** 本功能的实施代理、任务审查代理、最终审查代理和生产部署协调者。

## 强制恢复入口

任何代理开始工作、上下文压缩后恢复或接管前，必须完整读取：

1. 本合同。
2. `docs/superpowers/specs/2026-08-06-account-monitor-cost-balance-and-score-weights-design.md`。
3. `docs/superpowers/plans/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan.md`。
4. `docs/project/account-monitor-v3-acceptance-contract.md`。
5. `docs/project/project-progress.md` 顶部本轮登记。
6. `.superpowers/sdd/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan/progress.md`。
7. 自己收到的任务 brief；聊天摘要不得替代上述文件。

代理首条报告必须包含：

```text
CONTEXT_ACK=2026-08-06-account-monitor-cost-balance
TASK=<task number and name>
BASE_COMMIT=<40-hex commit>
ALLOWED_FILES=<exact task file list>
DEPLOYMENT_GATE=no-agent-deploy
```

缺少以上确认的实现或审查结果无效。实施代理和审查代理不得推送、部署、修改生产、修改唯一总账或扩大文件范围。

## 决策优先级

发生冲突时按以下顺序裁决：

1. 用户在当前任务中的最新明确指令。
2. 本合同。
3. 本轮设计规格和实施计划。
4. 账号监控 V3 不可漂移合同中未被本合同覆盖的条款。
5. 当前实现与历史文档。

无法判断时停止对应任务并交回协调者，不得折中拼接新旧方案。

## 本轮覆盖旧 V3 合同的内容

旧 V3 合同仅在以下三点被本轮明确覆盖，其他布局、真实请求窗口、排名、调度边界和中文展示规则继续有效：

1. 具体分组汇总从七项增加为八项；第八项是当前评分权重和编辑入口。
2. OpenAI API Key 卡片可以增加“上游余额”指标；非 API Key 卡片不渲染空占位。
3. OpenAI 成本来源由账号类型决定：API Key 使用倍率，非 API Key 使用 `采购成本 CNY / 预计可用额度 USD`；旧的按有效期和统计窗口摊销采购成本算法废止。

评分与余额仍只影响账号监控展示，不得写入调度器；营收、利润、账务和对账不得恢复到账号监控页。

## 已确认功能边界

- OpenAI `account_type=apikey` 显示并编辑倍率；手工保存写 `manual_override`，可恢复 `upstream_managed`。
- 其他 OpenAI 账号显示采购成本和预计可用额度，默认草稿 60 USD 未保存前不参与评分。
- 混合分组将两类账号转换为同单位成本倍率后使用同一成本得分公式。
- 缺预计额度只让成本项为 0，质量维度继续计分和排名。
- 余额不参与评分，只对 OpenAI API Key 账号展示。
- 余额与 Sub2API 声明倍率跟随健康探测；New API 付费倍率测量 6 小时自动刷新，单卡可强制刷新。
- 辅助刷新失败保留最后有效值，不得把成功的健康探测改成失败。

## 基线事实

- 实施 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-monitor-cost-balance`。
- 分支：`codex/account-monitor-cost-balance`。
- 最新远端基线：`origin/main@69caeaf816e3e01f9e0c6059c3c5262a4a12c2f6`。
- 生产 Sub2API 来源提交：`05985e62ec88b04d1e647a815eecdb1cf1155776`。
- 生产与最新远端的 `upstream/sub2api` 子树均为 `fc455d6aecfdb07ab90587000d7c5e77902f5bb6`，功能代码一致。
- 主 checkout 的本地 `main` 有未合并历史和无关未跟踪文件，禁止在该目录实现、rebase、reset 或部署。
- 每个任务开始前必须重新 `git fetch origin --prune`，确认任务基线仍包含最新 `origin/main`；若远端出现新的 `upstream/sub2api` 变更，协调者先做基线合并和回归，再派发任务。

## 小步执行与审查

- 每个计划任务由一个新的实施代理完成，只改任务文件并提交一个可独立审查的提交。
- 每个任务提交后由新的独立审查代理逐条核对设计规格、本合同、diff 和目标测试。
- 审查不通过时只派发该任务的修复，不提前开始依赖它的下一任务。
- 协调者在每次提交和审查后更新 SDD `progress.md`；代理不得自行把任务标为通过。
- 所有任务通过后再做一次整分支审查和一次聚焦验证，不运行无关全仓库验证。
- 不实现灰度流量、长期观察、额外运维平台或与本功能无关的重构。

## 生产与停机门禁

- 只有协调者可以推送和部署。
- 生产连接直接使用已配置的 `ssh -o BatchMode=yes sub2api-prod`，不得重新索取、复制或输出凭据。
- 用户已授权本功能在不停止服务的前提下完成生产部署；部署只允许既有零停机原子切换，不做逐比例灰度。
- 部署前先比较候选迁移哈希与生产 `/var/lib/sub2api/release-state`。若既有发布器返回 `downtime_required=true`，或任何步骤需要停止 API、worker、PostgreSQL、Redis 或 Caddy，必须在生产变更前立即停止并报告，不得绕过门禁、手改 release-state 或改用临时部署路径。
- 零停机发布成功后只做本功能必要的健康、接口和页面验证，不附加 24/48 小时观察任务。
- 未同时满足已推送、已部署和线上验证生效，总账保持“进行中”。
