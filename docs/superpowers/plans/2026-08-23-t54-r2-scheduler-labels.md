# T54-R2 调度预设语义、中文参数与有界校验实施计划

## 目标

在不改变调度行为的前提下，修正 T54 设置页的中文预设展示、参数标签、区间和说明，并让前后端对旧超界值采用一致的有界归一化。

## 任务 1：后端名称与归一化

文件：`backend/internal/service/settings_view.go`、`setting_parse.go` 及相关 service tests。

1. 先补失败测试：三项内置预设返回中文名称且 ID 不变；合法边界通过；超界/NaN/Infinity 被拒绝；旧读取值按字段范围归一化；冷落阈值保留 0 兼容语义。
2. 将内置预设名称集中定义，避免 handler/frontend 重复数值或名称。
3. 抽取或补齐统一的读取归一化函数；写入仍严格拒绝非法新值，读取旧值允许夹断/默认回退。
4. 保持 available presets、group policy 快照、旧模式转换和自定义预设引用规则不变。
5. 运行定向 Go 测试和 `go build ./cmd/server`，提交一个后端功能提交。

## 任务 2：前端中文元数据与控件约束

文件：`frontend/src/api/admin/settings.ts`、`SettingsView.vue`、`i18n/locales/zh/admin/settings.ts` 及 SettingsView 测试。

1. 先补失败测试：预设显示“体验优先/体验均衡/利润优先”；参数标签不出现英文；每个参数显示区间和说明；输入控件拥有有限 min/max；预设模式参数 disabled。
2. 在 API 层增加展示元数据或集中常量，保留英文内部 key 作为类型契约。
3. 更新中文 locale，所有可见标签不超过 5 个汉字；说明用短中文句子。
4. 更新参数渲染：权值、TopK、公平参数均使用统一字段元数据；候选池用中文固定选项；预设名称优先使用服务端中文名，并以 ID 作为稳定 key。
5. 确认自定义/预设切换、原生分组下拉和保存命名预设不回归。
6. 运行定向 Vitest、typecheck、build 和 diff-check，提交一个前端功能提交。

## 任务 3：联合验证与 handoff

1. 在候选 worktree 汇总两个实现提交，检查 diff 中没有算法、S1/S2、重试、sticky、Monitor V2、迁移或生产配置变更。
2. 运行后端/前端直接相关测试及必要构建门禁。
3. 写 `docs/handoffs/2026-08-23-t54-r2-scheduler-labels-handoff.md`，列出提交、测试证据、兼容性、`downtime_required=false` 预期、风险和回滚方式。
4. 将候选状态交给根总控，禁止在候选直接合并、推送或部署。

## 变更边界

- 不修改 `openai_account_scheduler.go` 的选择、重试和预算逻辑。
- 不修改 Monitor V2 数据源或体验卡旁证展示。
- 不新增依赖、迁移、GitHub Actions 或生产业务数据写入。
