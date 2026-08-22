# T51 Monitor V2 检测失败红色状态实施计划

1. 先补仓储、服务、handler、前端合同和组件失败用例并确认 RED。
2. 在原生投影链增加 `has_result`，升级合同 v8。
3. 前端只以 `has_result` 区分 NO DATA，失败保持 unavailable 红色。
4. 运行直接相关 Go/Vitest、typecheck、build、gofmt 和 diff-check。
5. 提交候选并交给根 `main` 合并、推送、发布和线上验证。
