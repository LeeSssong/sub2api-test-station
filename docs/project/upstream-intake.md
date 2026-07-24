# 现有上游 API 填报与接入指南

**适用对象：** `UP01` 及后续上游中转 API。  
**当前状态：** 只完成离线模板和校验，不登录、不充值、不发送真实请求。真实动作受 D13 延后。

## 1. 文件边界

- 仓库示例：`config/upstreams/UP01.example.yaml`，所有值均为虚构示例。
- 真实非敏感资料：复制为 `config/upstreams/UP01.local.yaml`；该路径已被 Git 忽略。
- 真实 API Key：只在 Sub2API 管理界面或批准的密钥存储中录入，不写 YAML、Git、聊天或报告。
- `secret_ref` 只说明密钥应放在哪里，例如 `sub2api-admin://accounts/UP01`，它本身不是密钥。

## 2. 填报顺序

1. 从上游后台或客服资料核对 Base URL、鉴权头和模型列表。
2. 只登记已经实际确认的模型；`public_name` 是本站对外名称，`upstream_name` 是上游真实名称。
3. 统一把输入、输出、缓存读取和缓存写入价格换算为每百万 Token；不支持的缓存价格保留空值。
4. 登记供应商并发、RPM、TPM、超时、429、余额查询、最低充值和退款规则。
5. 将再分发许可记录为 `allowed`、`conditional`、`prohibited` 或 `unknown`，不要把“API 可以调用”当作“允许转售”。
6. 将 `reviewed_at`、`evidence.checked_at` 和资料来源更新为本次核对结果。

普通结构校验：

```bash
ruby ops/validate-upstream.rb config/upstreams/UP01.local.yaml
```

准备进行真实小额测试前，把 `readiness` 改为 `ready_for_live_test` 并运行：

```bash
ruby ops/validate-upstream.rb --live-ready config/upstreams/UP01.local.yaml
```

校验通过只说明资料完整且不含明显密钥，不代表上游可用、价格准确、允许转售或已经充值。

## 3. Sub2API 字段映射

依据 Sub2API v0.1.155 源码，API Key 上游使用以下映射：

| YAML 字段 | Sub2API 位置 | 注意事项 |
|---|---|---|
| `connection.base_url` | 新建账号 → API Key → Base URL | 路径按上游文档填写；只有启用 URL allowlist 时才需将 host 加入名单 |
| `connection.secret_ref` | 不直接录入 | 根据引用位置取得真实 Key，只粘贴到管理界面的 API Key 输入框 |
| `sub2api.platform` | 平台 | OpenAI 兼容上游通常选 `openai`；按真实协议选择，不按模型品牌猜测 |
| `sub2api.account_type` | 账号类型 | 本模板固定为后端值 `apikey`，界面显示 API Key |
| `sub2api.priority` | 优先级 | 数字越小优先级越高；首个上游使用默认 50 即可 |
| `sub2api.concurrency` | 账号最大并发 | 不得超过供应商并发上限；首轮测试建议 1–2 |
| `sub2api.rate_multiplier` | 账号成本统计倍率 | 不等于用户售价或用户分组倍率；首轮保持 1.0 |
| `models` | 模型白名单/映射 | 同名时建立一对一白名单；异名时映射 `public_name` → `upstream_name` |
| `sub2api.pool_mode` | 账号重试设置 | 首上游先关闭；真实错误语义验证后再开启，防止重复计费 |

OpenAI API Key 账号开启“自动透传”时，账号的模型白名单/映射可能不生效。首轮闭环必须关闭自动透传或另行验证其模型限制，不能仅凭界面已填写白名单就认为限制生效。

## 4. 只属于供应商台账的字段

`limits.rpm`、`limits.tpm`、`request_timeout_seconds`、`daily_cost_cap`、余额、最低充值、退款和再分发许可不是本模板确认过的 Sub2API 单账号原生字段。它们用于：

- 决定账号并发和重试上限；
- 设置人工告警、充值和熔断规则；
- 计算 L1-4 成本与售价；
- 判断该渠道是否可以进入商业售卖。

在真实部署版本确认相应控制确实生效前，不得把“已记录”写成“已强制执行”。

## 5. 管理界面录入顺序（延后执行）

1. 先建立独立账号名和独立分组，不与订阅账号池混合。
2. 选择协议对应平台和 API Key 类型，录入 Base URL。
3. 从受控位置取得 Key 并直接录入；不在终端回显，不截图包含 Key 的页面。
4. 建立最小模型白名单/映射，设置并发、优先级和倍率。
5. 首次创建后先保持最小流量，按 `upstream-live-acceptance.md` 验收。
6. 验收失败时停用该账号，保留错误类型、时间和请求 ID，不保存完整 Prompt 或 Key。

## 6. 当前待用户提供的非敏感资料

- 上游名称或工作代号。
- Base URL 和鉴权方式，不含 Key。
- 已购买/可用模型及价格表。
- 并发、RPM、TPM、余额查询、最低充值、退款和 429 说明。
- 再分发/转售条款或客服回复的位置。

这些资料未提供不阻塞其他离线准备；UP01 继续保持“待盘点、未充值、未验证”。
