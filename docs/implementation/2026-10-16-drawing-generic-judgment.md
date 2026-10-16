# 泛化绘图判定（本地候选，未部署）

- 分支：codex/drawing-no-face-judgment；基线：96c22b6c5ac7adea2837e7bc03f415532cad099b。
- 泛化规则检查主体类别、题目中的动作和场景、明显结构问题。不认脸，不比较姓名、发型、肖像相似度，不枚举具体动作或场景。
- unknown 显示“无法判断”；error 显示“判定异常”，涵盖截图、请求、解析、配置及判定超时问题。生成状态与统计保持独立。
- 历史已知技术原因仅作只读展示映射；不修改历史记录，不自动改为通过，不重新调用模型。
- 无数据库迁移。

## 验证

- 新增回归先在旧代码失败，再修改实现。
- go test -tags unit ./internal/service -run 'TestDrawing|TestIntelligence' -count=1：通过。
- 前端 intelligence 组件测试：4 个文件、12 项通过。
- vue-tsc --noEmit：通过。git diff --check：通过。
- 已有 Browserslist 及 Node localStorage 警告不影响测试。
- 单元测试替代了上游模型请求；未调用真实付费模型，实际识别准确率未验证。未执行新浏览器视觉验收。
- 未合并、推送或部署。主站及测试站未修改。
