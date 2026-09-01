# T107 管理员额度钱包 wiring 修复交接

- 任务：T107
- 状态：READY_FOR_ROOT_REVIEW
- 基线：b2d504797fa2fdb869f0db35f7325dca2dfa6664
- 候选工作区：/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t107-quota-wallet-fix
- 候选分支：codex/t107-quota-wallet-fix
- 实现提交：a3e8b3adb2196c6e94374923f4747bbe28789985
- 最终候选：包含本交接文档的分支 HEAD；精确 commit/tree 由发布总控接收候选时复核

## 根因

service.ProvideQuotaWalletServices 已创建 native quota wallet，但 wire_gen.go 原先仍以旧的 admin.NewUserHandler(...) 构造管理员用户 handler，没有调用 SetQuotaWalletService。因此 quota-summary、quota-ledger 和手动充值请求进入 handler 时依赖为 nil，返回 quota wallet service not available。历史提交 b80c99f9658b0dab3d8798353b67ad757466d671 的 setter 调用在官方 0.1.183 wire 重生成后丢失。

## 变更

- upstream/sub2api/backend/internal/handler/admin/user_handler.go
  - 新增 ProvideUserHandler provider，在保持旧构造器签名兼容现有测试的前提下注入 quota wallet。
- upstream/sub2api/backend/internal/handler/wire.go
  - ProviderSet 改用 admin.ProvideUserHandler，使下一次 Wire 生成继续保留注入。
- upstream/sub2api/backend/cmd/server/wire_gen.go
  - 生成结果改为 admin.ProvideUserHandler(..., v...)，其中 v 来自 ProvideQuotaWalletServices。
- upstream/sub2api/backend/internal/handler/admin/user_handler_wiring_test.go
  - 通过 provider 构造真实 handler 并请求 quota-summary，断言不再返回缺失服务错误。

无数据库迁移、配置、依赖或真实充值变化。

## TDD / 验证证据

- RED：临时移除 ProvideUserHandler 中 setter 调用后，go test ./internal/handler/admin -run '^TestProvideUserHandlerInjectsQuotaWalletService$' -count=1 失败，HTTP 期望 200、实际 500。
- GREEN：恢复 setter 后同一命令通过。
- go test ./internal/handler/admin -count=1 通过。
- go build ./cmd/server 通过。
- gofmt 已应用于改动 Go 文件；git diff --check 通过。
- 尝试 go generate ./cmd/server 时，仓库缺少 Wire 工具间接依赖的 github.com/google/subcommands go.sum 条目；用 go run -mod=mod github.com/google/wire/cmd/wire 可生成，但会重排现有无关 upstream balance provider 位置。该生成副作用已恢复，候选只保留 T107 变更。Wire 源 internal/handler/wire.go 与当前生成调用已人工核对一致。
- go test ./internal/handler/admin ./cmd/server -count=1 受现有 cmd/server/wire_gen_test.go 参数数量过时阻塞；该失败与 T107 无关。独立 server 编译仍通过。

## 未执行事项

未合并、未推送、未部署；未 SSH 写生产、未重启服务、未登录验收站、未执行真实充值，未做线上验证。downtime_required 需由发布总控预检确认；本修复本身无迁移。

## 给发布总控的精确交接

请先审查候选分支 codex/t107-quota-wallet-fix 的完整 HEAD（包含实现提交 a3e8b3adb2196c6e94374923f4747bbe28789985 与本交接文档）；确认车道后，在根目录干净 main 上合并完整候选，重跑管理员 handler 定向测试、go build ./cmd/server、格式与 diff 检查，推送并按验收站全局约束执行发布预检。只有取得用户明确主站授权后才可部署主站；发布失败按既有候选/上一已验证槽位回滚。回滚代码路径为在 main 上 revert T107 运行代码提交后重新走完整发布链。
