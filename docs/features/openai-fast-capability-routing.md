# OpenAI Fast 账号能力路由

## 配置

在新建或编辑 OpenAI 账号中开启「支持 Fast」。

- 「支持 Fast 的模型」填写实际上游模型名，每行一个或用逗号分隔。
- 账号普通映射、别名、compact 映射及透传模式按实际转发模型判定；透传账号请填写保留的客户端模型名。
- 留空表示此账号可用的全部模型支持 Fast。关闭后清除开关与范围。
- 只标记运营者确认实际支持 Fast 的上游。本站不自动探测或以模型名字推断能力。
- 使用已有账号 extra 字段 `openai_fast_supported` 和 `openai_fast_models`，无需数据库迁移。API 明确保存空数组或非法范围时不会启用全部模型。

## 请求行为

`service_tier=fast` 归一为 `priority`。Fast 请求只选择符合标记及模型范围、且不会被策略过滤或拦截的账号。普通请求仍可选择标记账号或未标记账号，不因能力标记被升级。

两种调度路径、批量负载、会话粘性、故障切换、账号刷新检查均执行相同条件。保留原会话键及提示词缓存键；账号合格时继续复用。换账号后上游缓存能否复用取决于提供方。

- 无合格账号：HTTP 503 `fast_unavailable`，提示「当前分组的 Fast 模式暂时异常或不支持 Fast」，并附联系管理员和 Request ID 的协助指引；不会静默转普通档。API 普通/流式返回与用户错误记录保留 Fast 专属提示，不泛化为服务繁忙；该错误不区分暂时异常与未配置支持，因此不直接断言永久不支持。
- 不可迁移的 `previous_response_id` 绑定普通账号：HTTP 409，提示新建请求或关闭 Fast。调用方明确携带可迁移上下文时可重选。
- WebSocket 后续回合开启 Fast 或更换模型：逐帧验证当前账号；不合格时明确拒绝并要求重新连接，不在已有连接内重放回合。
- 修改 Fast 标记或范围会改变账号路由指纹；已有连接会在后续准入时要求重新连接，防止使用过期能力。
- 分组强制 Fast、策略强制 priority 同样受能力筛选限制。Fast 与过滤/拦截策略冲突时明确拒绝。
- 混合分组模型目录在存在对应标记账号时展示 priority/Fast；其他模型能力继续按现有规则合并。

## 计费与生效边界

沿用现有计费：最终发送 priority 时按已有 Fast 定价；API Key 上游可信终态明确报告普通档时降为普通档。OAuth/Codex 保留既有特殊口径。第三方不声明档位时依赖运营者的支持标记及上游契约；本站不以单次响应速度证明 Fast 实际执行。

旧账号默认未标记，因此发布后 Fast 请求将停止使用这些账号。发布前应准备支持账号和范围清单，发布后配置标记，再验证对应请求；普通流量不受未标记影响。

## 本地验证（2026-10-09）

- Go `-tags unit` 直接相关服务层、处理器测试，覆盖选号/粘性/重试/模型范围/compact/WS 后续回合/标记撤销/策略及现有计费。
- 创建、编辑账号及 i18n 测试：202 项通过。
- 前端 `vue-tsc --noEmit`、四个变更 UI/工具文件 ESLint、Vite 构建通过。
- 新增字段浅深色及 375px 宽度组件预览已检查；完整后台页面未登录验收。
- 本地独立 worktree：`codex/fast-capability-routing`；没有推送、合并或部署。
- 未查询测试站或主站，未验证真实上游速度、实际收费或线上版本。

## 分组 Fast 开关重新打开后显示关闭

管理端 `GroupFromServiceAdmin` 曾遗漏 `ForceOpenAIFast`、`FreeOpenAIFast` 的字段赋值，导致数据库已开启时，管理接口仍返回两个 false。修复补齐管理 DTO 字段；用户侧 DTO 仍不暴露这两项策略。修复不改变数据库值、转发策略或计费规则。

验证：现有 `TestGroupMapperExposesForceOpenAIFastOnlyToAdmins` 修复前失败，修复后所有 `Test.*Group` DTO 测试通过；`git diff --check` 通过。本地 DTO 全包另有既存的 `TestUsageLogFromService_UsersSeeRequestedReasoningEffortOnly` 失败（预期 max、实际 xhigh），已在未修改的 main 上复现，与这两行赋值无关，本次未修改该用量逻辑。修复未部署。
