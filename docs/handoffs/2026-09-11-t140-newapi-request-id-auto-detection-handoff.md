# T140 交接报告

## 状态

`READY_FOR_ROOT_REVIEW` 候选，未合入根 `main`，未推送，未部署，未触碰生产账号或服务器运行数据。

## 交付内容

- 显式请求头配置优先。
- OpenAI API Key 的非官方自定义 BaseURL 或可信 NewAPI 身份自动读取 `X-Oneapi-Request-Id`。
- NewAPI 倍率写入要求原生 Sub billing 明确 `unsupported`。
- 原生 `ok`、临时失败、未知或缺失快照均不会触发 NewAPI 倍率接管。
- 无 migration、配置批量变更、历史回填或凭据变化。

## Git 与测试

- 基线：`14caa8b3c`
- 分支：`codex/t140-newapi-request-id-auto-detection`
- 最终提交：`a63b6eb626d050321e771e03e96aa07bc0da64e3`
- `go build ./internal/service`：通过
- `go build ./cmd/server`：通过
- `gofmt`、`git diff --check`：通过
- `go test ./internal/service`：被主线既有缺失符号 `resolveCompositeModelOwnership`、`decodeCodexManifestModels` 阻断

## 部署边界

部署必须由根发布总控从干净且已推送的 `main` 发起，并取得用户明确的主站授权语义。线上验证按用户要求仅使用 SSH、服务器配置、发布流水、日志和数据库只读查询，不使用页面读取。

## 回滚与未验证项

回滚使用上一已验证蓝绿槽或 Git 前向修复，不删除业务数据。未验证项是完整 service 测试执行和部署后自然请求产生的真实 NewAPI 请求 ID/倍率注册证据。
