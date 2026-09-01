# T96 分组账号基线生产调整报告

- 日期：2026-09-01
- 目标来源：`docs/superpowers/specs/2026-08-31-t96-group-account-baseline-unified-quality-scheduling-design.md`
- 生产 API：Sub2API 原生管理员接口 `POST /api/v1/admin/accounts/bulk-update` 与 `PUT /api/v1/admin/accounts/:id`
- 前快照：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-09-01-t96-group-account-baseline/before.json`
- 后快照：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-09-01-t96-group-account-baseline/after-api.json`

## 结果

已通过原生账号管理接口精确替换 68 个仍存在且有明确 T96 目标的账号分组，并将这 68 个账号的账号级 Sub 原生 `priority` 从原值调整为 `50`。生图 7 个账号保持原生生图组，Pro 与专属 Pro 的确定账号集合保持镜像。

历史目标名单中的 `131、152、153、273、293` 当前已不存在，接口返回账号不存在或列表不再包含，因此未伪造写入。2026-08-31/09-01 新增的 `294、295、296、298、299、300、301` 不在 T96 质量排名报告中；其中 `294、295、296、298、301` 当前属于多个公开文本组，`299、300` 当前属于特惠组，均保留现状，待形成同一质量口径证据后再归组。

## 调整明细

以下为账号原分组 -> 新分组；同一账号的 `priority` 均为原值 -> `50`。

### GPT-Pro + 【专属】GPT-PRO

`Pro-SHUAI-0.17`、`pro-Auv`、`plus-XN`、`apizh-0.12`、`ai8-plus`、`特惠2-mosshub`、`plus-mosshub`、`海豚科技1`、`pwtk-plus`、`CX-Pro`、`Vokly-pro`、`CallAI-pro`、`SHUAI-plus1-0.12`、`loveapi-pro`、`xian-plus`、`xian-pro`、`mmc-pro`。

### GPT-Plus

`Pro20x-SHUAI-0.2`、`Plus-WAWAZZ`、`特惠-佛爷-0.08`、`pro-XN`、`特惠-SHUAI-0.08`、`SheApi-0.08`、`apizh-0.15`、`海豚科技2`、`never-plus`、`never-pro`、`haha-pro`、`合租巴士-特惠-0.08`、`NV-PRO`、`loveapi-特惠`、`loveapi-plus2`、`makeup-特惠`、`makeup-特惠2`、`makeup-特惠3`。

### GPT-特惠

`plus-a6-0.1`、`plus-Auv`、`pro20x-Auv`、`plus-猫哥`、`pro-猫哥`、`pro20x-猫哥`、`SheApi-0.2`、`ai8-plus`、`ai8-pro`、`pwtk-特惠1`、`pwtk-pro`、`云桥-特惠1`、`CallAI-特惠`、`河图-plus`、`河图-pro`、`baitumax-pro`、`风岚-plus`、`风岚-pro`、`星辰-plus2`、`合租巴士-pro`、`NV-plus`、`星辰AI-plus2`、`loveapi-plus1`、`云桥-特惠`、`柚子-plus`、`云桥-pro`。

### 生图

`生图-SHUAI`、`生图-XN`、`生图-CX`、`haha-生图`、`合租巴士-生图`、`云桥-生图`、`moss-生图`：分组未变；账号级 `priority` 按原生接口统一为 `50`。

## 未调整账号

| 账号 ID | 账号名 | 当前分组 | 当前 priority | 原因 |
|---:|---|---|---:|---|
| 294 | 8月31日04.11发车-7d-ivoratkinson4458+lppk7 team-397540-UVJS-F2DC02C0151C1 | Pro、专属 Pro、Plus、特惠 | 1 | 新增账号，不在 T96 历史质量排名中 |
| 295 | susanleet522@gmail.com | Pro、专属 Pro、Plus、特惠 | 1 | 新增账号，不在 T96 历史质量排名中 |
| 296 | bettymoorex749@gmail.com | Pro、专属 Pro、Plus、特惠 | 1 | 新增账号，不在 T96 历史质量排名中 |
| 298 | barbaracartery286@gmail.com | Pro、专属 Pro、Plus、特惠 | 1 | 新增账号，不在 T96 历史质量排名中 |
| 299 | 风岚-炸弹 | 特惠 | 1 | 新增账号，暂无完整排名证据 |
| 300 | pw-特惠2 | 特惠 | 1 | 新增账号，当前仅有有限探测证据 |
| 301 | art0 | Pro、专属 Pro、Plus | 1 | 新增账号，成本/排名证据不完整 |

## 校验结论

- 确定目标账号的分组集合已精确替换；多余关系仅来自上述 7 个新增账号。
- Pro 与专属 Pro 的确定目标集合相同。
- 生图集合为 7 个，且与确定文本集合无交集。
- 确定目标账号账号级 `priority` 不再有非 50 值。
- 生产前后未修改账号状态、可调度状态、并发、倍率、凭据或模型配置。
- 原生 `PUT` 接口重建 `account_groups.priority` 为组内序号；现有公开原生接口不接受关系级 priority=50 字段。本报告中的“Sub 原生优先级”指账号级 `accounts.priority`。
