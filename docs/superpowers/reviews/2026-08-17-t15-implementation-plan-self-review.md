# T15 实施计划自审

## 结论

计划覆盖规格的数据库、原生模型选择、sidecar 边界、持久化去重、固定时隙、admin API、卡片交互和候选交接。任务按可独立验证的纵向边界拆分，允许内联执行。

## 自审结果

- 规格覆盖：所有批准合同均映射到 Task 1–4；许可证门禁与禁止部署映射到 Global Constraints 和 Task 5。
- 占位符扫描：无 TBD/TODO/“类似前文”等不可执行占位描述。
- 类型一致性：service/repository/sidecar/API/UI 均使用 `AccountModelDetection*` 前缀；状态枚举统一为 `untested/queued/running/normal/abnormal/insufficient/failed/unsupported`。
- 兼容性：现有 monitor 构造器保持可在 detection=nil 时工作，避免扩大旧测试修改。
- 验证范围：只包含 migration、focused backend、focused frontend、typecheck/build 与 diff check；未加入全仓或额外 reviewer。

## 刷新复核（2026-08-17）

- 目标基线已刷新到根 `main@b59baac1434f54024c5d5ea15e1d6f804d511aae`；唯一内容冲突为 `cmd/server/wire_gen.go`，已由 `go generate ./cmd/server` 重新生成，未手工拼接生成物。
- 生成依赖同时包含 T15 `AccountModelDetection*` provider 与主线 `OpenAISharedHealth`/ops scheduler provider；T15 范围文件未被主线改写。
- sidecar HTTP 测试改为内存 `RoundTripper`，仅替换测试传输层以适配当前沙箱禁止 loopback listener 的环境，仍覆盖鉴权、路径、请求体、非 2xx、非法状态和敏感摘要过滤；生产 sidecar 实现未因此改变。
- 许可证门禁保持：PolyForm Noncommercial detector 核心、基线和报告未复制进 Sub；未配置合法商业 detector 时 catalog fail-closed，生产接入仍待书面授权或独立实现。
- 结论：T15 直接相关功能与验证闭环，状态恢复为 `READY_FOR_ROOT_REVIEW`；`downtime_required` 仍须由根 `main` 合并后的发布预检判定。
