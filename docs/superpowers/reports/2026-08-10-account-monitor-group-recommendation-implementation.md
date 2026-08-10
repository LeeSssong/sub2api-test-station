# 账号监控卡片“推荐分组”实现报告

## 范围与状态

本轮在账号监控只读投影和现有账号卡片 header 元信息同行增加“推荐分组”。推荐固定使用滚动 7 天主动探测数据，测试组显示推荐、继续观察、暂缓迁入或暂不建议入组；正式组只对 `active + schedulable` 账号分析，并仅在明确建议迁往其他正式档位时显示推荐文字与叹号 Tooltip。

当前状态为本地实现和验证完成，尚未合并到 `main`、推送、生产部署或线上验证，项目总账继续保持“进行中”。本轮没有修改生产账号分组、优先级、调度状态、评分权重或数据库。

## 实现结果

- 后端新增纯推荐评估器与可选 `group_recommendation` 投影；账号监控 schema version 递增，旧客户端可忽略新增字段。
- 推荐质量只读取主动探测成功率、TTFT、完整响应耗时及当前成本证据，不使用真实请求质量数据回退。
- 推荐窗口固定为滚动 7 天；页面的 `24h/7d/30d` 选择仍只控制原有卡片展示指标。
- `GPT-Pro` 与 `【专属】GPT-Pro` 共用质量档位；Codex Auth 正常时默认推荐 Pro；生图、Claude、自测均不会成为推荐目标。
- 利润门槛为公开 Pro/Plus `0.05x`、特惠/专属 Pro `0.02x`，先于质量档位判断。
- 前端只在原有“平台 / 当前分组 / 调度状态”同行追加字段；正式组明确迁移时显示“推荐：目标分组 + !”，Tooltip 支持鼠标与键盘焦点。
- 没有新增迁移按钮、定时器、数据库迁移或分组写入路径；管理员仍通过既有分组编辑能力人工迁移。

## 提交

```text
13cbb2108 feat: evaluate account group recommendations
40c8007b3 feat: project account group recommendations
688f93b2b feat: show account group recommendations
663551bb8 fix: harden account group recommendations
```

## 验证

2026-08-10 重新执行以下完整验证，全部退出码为 0：

```text
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin ./internal/repository -count=1
go vet ./...

cd upstream/sub2api/frontend
pnpm test:run
pnpm typecheck
pnpm build
```

结果：

- 后端 `internal/service`、`internal/handler/admin`、`internal/repository` 测试通过；全仓 `go vet ./...` 通过。
- 前端 231 个测试文件、1647 项测试全部通过；类型检查和生产构建通过。
- `git diff --check` 通过。
- Vitest/Vite 输出仍包含仓库既有的 `router-link`、jsdom 网络、Browserslist 版本、动态/静态 import 和 chunk size 警告；不影响退出码，未发现由本功能新增的失败。

## 浏览器证据

本地 Vite 页面使用 3 个 mock 账号验证了正式迁移、测试组观察和正式匹配三种状态：

- 桌面 `1440x1000`：正式迁移卡片显示“推荐：GPT-Pro + !”；测试组显示“继续观察”；匹配当前 Pro 的正式账号不显示推荐。Tooltip 可由键盘焦点触发。
- 窄屏 `390x844`：`documentElement.scrollWidth = clientWidth = 390`；三张卡片宽度均为 `334px`，右边界为 `362px`；推荐字段宽度分别为 `92.89px` 和 `40px`，`scrollWidth = clientWidth`，没有横向溢出或遮挡。
- desktop header 高度约为 `79/75/75px`；包含正式迁移推荐的首张卡片只比普通卡片增加约 `4px`。窄屏同行信息按现有 flex 规则自然换行，没有新增独立区块。

截图：

- `output/playwright/account-group-recommendation/desktop.png`
- `output/playwright/account-group-recommendation/narrow.png`

浏览器控制台的两条错误来自 smoke mock 未覆盖公告数组和 `/setup/status`，分别为 `all.slice is not a function` 与 HTTP 500；账号监控接口与本功能页面投影已被 mock 正常覆盖，不属于本次实现回归。

## 边界复核

- 无数据库 migration、scheduler/worker/cron 改动。
- 无账号 `group_ids`、`priority`、调度状态或评分权重写入。
- 无真实请求质量 fallback；真实请求次数继续只作为原卡片展示信息。
- 无硬编码生产分组 ID；按规范化分组名和现有分组配置识别档位。
- 无自动迁移 API、按钮或后台任务。
- 生图、Claude、自测不成为 GPT 正式推荐目标。
- 单账号推荐失败只清空该行推荐，不使整个账号监控列表失败。

## 独立审查与修复

首次 whole-branch 审查阻止合并并发现 3 个 P1 和 2 个 P2：Pro/专属 Pro 别名只保留首个组约束、模型不可用未进入硬故障、Codex Auth 绕过质量门槛、生图账号排除不足，以及前后端测试组别名归一化不一致。

修复提交 `663551bb8` 采用测试先行关闭这些问题：

- 公开 Pro 与专属 Pro 的全部成本上限和质量上限同时生效，利润余量分别为 `0.05x` 与 `0.02x`，组顺序不影响目标或首要原因。
- `model_unavailable`、model not found 与 HTTP 404 在测试组返回暂缓迁入，正式组保持静默。
- Codex Auth 仅在通过 Pro 质量和已配置成本门槛后立即推荐；失败时测试组保留 Pro 目标并使用 `hold`。
- 当前组或最新探测模型属于生图/Claude 时不生成 GPT 推荐。
- 前端按后端相同的大小写与 ASCII 空格规则识别测试组。

二次独立审查又发现 Pro 别名遍历顺序会改变 `reason_codes[0]`；最终提交已改为先聚合所有别名质量失败，再按固定的成功率、TTFT、完整响应顺序选择首要原因，并加入正反组顺序回归。修复后独立复审未报告剩余 correctness/regression finding。

## 剩余工作与风险

- 仍需在合并后的 `main` 重跑并版回归、构建与发布预检，再按项目蓝绿链部署并完成线上验证。
- 当前截图基于本地 mock 数据，不替代生产真实账号投影验收。
- 推荐依赖现有主动探测样本和账号成本证据；成本缺失、样本不足或探测过期时会保守保持观察/暂缓，而不会自动迁入正式组。
- 生图/Claude 排除目前依赖当前分组名和最新主动探测 `model_id` 中的稳定关键词；未来若出现名称含 `image` 的纯文本模型，应改用结构化模型能力字段。
