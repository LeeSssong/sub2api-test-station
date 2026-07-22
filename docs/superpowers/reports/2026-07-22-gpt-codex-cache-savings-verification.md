# GPT/Codex 缓存让利只读基线验证

**日期：** 2026-07-22

**结论：** 本地实现与只读门禁通过；生产 24 小时自然流量门禁未执行，不据此改价或部署

## 范围

本轮只实现并验证三项能力：

1. relay-ops 从 Sub2API Admin Usage 接口读取普通输入、缓存创建、缓存读取和总 Token，并区分“字段真实为零”与“旧接口字段缺失”。
2. 对客户公开 OpenAI 分组中的 `gpt-*` 模型执行缓存价格门禁，要求缓存读取价明确且低于普通输入价；`gpt-5.6-*` 另要求缓存写入价明确且非负。
3. 每日运营摘要按公开组显示缓存读取、缓存写入、命中率和缓存优惠价格覆盖；异常只报告，不自动修改生产配置。

未新增回答缓存、提示词缓存、跨用户缓存或第二套用户中心。relay-ops 不读取、生成、记录或改写 `prompt_cache_key`。

## 上游事实

只读检出 `Wei-Shaw/sub2api` 标签 `v0.1.161`，提交 `19149ca196eeae4a4482e5299dc6fa4ba0b06c8c`。源码核对确认：

- `backend/internal/service/openai_gateway_usage.go` 将普通输入计算为总输入减缓存读取和缓存创建，四类 Token 互斥记录，避免重复计费。
- `backend/internal/service/billing_service.go` 分别使用输入、输出、缓存创建和缓存读取价格；GPT-5.6 缓存写入与读取有独立价格。
- `backend/internal/handler/openai_gateway_handler.go` 使用客户端 `prompt_cache_key` 参与会话哈希，并由原生粘性调度保持账号亲和。
- `frontend/src/components/admin/usage/UsageStatsCards.vue` 和 `UsageTable.vue` 已展示缓存 Token、缓存创建成本、缓存读取成本、标准费用和实际费用。

因此本项目不 fork Sub2API 网关，不复制计费或粘性路由；新增代码只验证这些原生能力持续满足本站的让利口径。

## 自动化证据

以下命令在本地仓库执行：

```bash
docker run --rm -v "$PWD":/workspace -w /workspace/relay-ops-service \
  golang:1.24-alpine go test ./internal/sub2api ./internal/cachepolicy ./internal/dailyreport -count=1

docker run --rm -v "$PWD":/workspace -w /workspace/relay-ops-service \
  golang:1.24-alpine go test ./... -count=1

docker run --rm -v "$PWD":/workspace -w /workspace/relay-ops-service \
  golang:1.24-bookworm sh -c 'go test -race ./... -count=1 && go vet ./...'

git diff --check
docker compose -f infra/compose.yaml config --quiet
```

结果：

- 缓存契约、价格门禁和日报聚焦测试全部通过。
- relay-ops 全量普通测试全部通过。
- relay-ops 全量 race 测试和 `go vet` 全部通过。
- `git diff --check` 通过。
- Compose 配置检查退出码为 0；本机未设置 `D04_TOTAL_BUDGET_USD`，只产生现有空值警告。

Alpine 镜像第一次运行 `-race` 因未启用 CGO 停止，随后使用现有 `golang:1.24-bookworm` 镜像完成同一全量 race 验证。这是测试运行时差异，不是代码失败。

## 隐私与写入审计

- 新增代码只接收 Sub2API 的组、渠道价格和聚合 Usage 结构。
- 报告门禁代码在展示前去除组 ID、模型和区间细节，只保留稳定错误类别。
- 没有读取或保存提示词、响应正文、文件、原始 API Key 或缓存键。
- 没有调用 Sub2API 写接口，没有修改账号、分组、路由、价格、倍率、余额、Key、用户或数据库。
- 没有发起模型请求、重建容器、部署镜像或发送飞书消息。

## 剩余生产门禁

本轮不能证明真实用户已经获得预期节省。进入生产启用或对外宣称前仍须：

1. 连续观察至少 24 小时，并为 `GPT-Pro`、`GPT-Plus` 各取得至少 20 个成功自然请求。
2. 对同步与 SSE 样本逐笔核对 Sub2API Token 分类、用户余额扣减和上游账单。
3. 证明缓存读取确实按公开 `cache_read_price` 扣费，普通输入、缓存写入和输出没有串类。
4. 与同模型、同渠道基线比较：成功率下降不超过 0.5 个百分点，TTFT P95 上升不超过 10%。
5. 缓存字段缺失率无异常，未出现跨用户数据或原始缓存键日志。

上述证据齐备前保持现有生产价格、路由和注册状态，不用合成高负载或付费流量制造通过样本。
