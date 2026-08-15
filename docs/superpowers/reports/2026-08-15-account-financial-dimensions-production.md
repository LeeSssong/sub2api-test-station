# T11 经营页三层视图与异常空态生产验收

日期：2026-08-15

## 发布身份

- 根 `main`：`d17968ab95cb5f9db2a7374c59222b8b01c0e46f`
- source/tested tree：`05ce07aa4f58346ab25ae9db37baa2d95569662a`
- 迁移哈希：`d3fe99bba69b0cf0cca8a7f5ec45499921f3496f58dd74c3a671d90a653589b5`
- 测试证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-15-main-d17968ab9-t11-account-financial-dimensions-v1.json`
- 生产记录：`/var/lib/sub2api/release-records/20260815T135236Z-production-75345.json`
- 生产镜像：`ghcr.io/leesssong/xingqiao-sub2api:release-d17968ab95cb5f9db2a7374c59222b8b01c0e46f-ed98d0cfdebbf30f8c22ac0e4a693977ba83278a73513b4f83fe409411d5f24d`

宿主记录为 `succeeded/promoted`、`rolled_back=false`；发布控制器返回
`downtime_required=false`。生产 `release-state` 为 schema v2，活动槽 `blue`、
活动上游 `sub2api-blue:8080`，source commit/tree、迁移哈希以及 blue/worker
镜像均与本地证据一致。

## 线上专项验收

- 公网 `/healthz`、`/readyz`、`/health` 均返回 HTTP 200，内容分别为
  `alive`、`ready`、`ok`。
- 管理员登录态进入 `/admin/operations/account-profitability`，中文标题、范围、
  摘要、分组维度和账号表格均正常显示。
- 7 天全站真实数据可见：收入、支出、盈利、利润率、异常流水和用户未消费余额
  均有明确值；全站摘要在切换分组后保持不变。
- 分组 Tab 显示生产现有 7 个分组；选择 `GPT-Pro` 后显示“分组摘要：GPT-Pro”、
  18 个账号、分组金额、异常数、未分摊说明和真实账号行。
- 点击 `SheApi-0.2` 的异常数后，最终 URL 为
  `/admin/usage?tab=cost-exceptions&review=pending&range=7d&account_id=147`；
  成本异常 Tab 处于选中状态。当前筛选无待处理记录时显示“暂无数据”，未出现
  空白内容区。
- 页面重新加载捕获 34 个请求，其中经营数据请求为原生
  `/api/v1/admin/operations/account-financial?range=today`；`/xingqiao/**` 请求为 0。
- 390x844 精确移动视口下 `documentElement.scrollWidth === clientWidth === 390`，
  分组摘要和账号表格可见，页面级无横向溢出。

生产截图：

- `output/playwright/t11-account-financial-production-desktop.png`
- `output/playwright/t11-account-financial-production-mobile-390x844.png`

## 样本与验证边界

- 当前生产响应没有未归属流水，因此未自然显示“未归属”Tab；未篡改生产数据。
  未归属投影和不重复计数由同一发布树的后端专项测试覆盖。
- 未在生产主动制造财务接口故障。错误提示和重试、异常 loading/data/empty/error
  四态由同一发布树的前端 42/42 定向测试及本地受控故障注入覆盖；生产自然验证
  覆盖了 data 和 empty。
- 用户明确要求跳过 fresh 全分支独立终审。该项记录为流程豁免，不宣称终审 PASS；
  已完成的逐任务独立复审、合并后专项验证和生产验收继续有效。

## 回滚与清理

T11 无数据库迁移、依赖或运行配置变化。如需回滚，使用既有蓝绿链切回上一版
green 镜像，不需要数据回滚。生产验收闭环后才允许归档并删除候选 worktree/分支。
