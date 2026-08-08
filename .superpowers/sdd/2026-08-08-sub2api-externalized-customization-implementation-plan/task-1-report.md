# Task 1 实施报告：Sub2API 定制基线冻结

## 状态

本地实施完成，提交前验证通过；本任务没有推送、生产写入、部署或线上状态变更。根据项目总账口径，事项保持“进行中”。

## 交付内容

- `docs/contracts/sub2api-customization-inventory.yaml`
  - 冻结八项定制能力：主页/文档、监控与排名、盈利分析、账务与对账、余额与倍率采集、请求完成事件、官方升级发布、管理员认证与导航。
  - 每项记录 `capability`、owner、源码路径、API、Ent/运行时表、前端路由、事实源、验收证据和旧/目标实现。
  - owner 限定为 `core`、`adapter`、`control_plane`、`host`。
  - 记录当前生产 source commit/tree、Sub2API/Caddy/PostgreSQL/Redis/Worker immutable image 身份、迁移哈希、active slot、release-state 路径和生产验收报告引用。
- `docs/contracts/admin-experience-contract.md`
  - 固定同域登录、原生 2FA、原 URL、字段语义、筛选、排序、分页、刷新、CSV 和控制面不可用时的只读快照/原生页面降级。
- `tests/contracts/baseline_manifest_test.rb`
  - Ruby 契约测试校验清单记录字段、owner 枚举、运行时 manifest 身份和管理员合同黄金路由/要求。
- `docs/project/project-progress.md`
  - 新增本轮 Task 1 进行中登记，明确尚未推送、部署和线上验证。

## 验证

```text
ruby -Itests tests/contracts/baseline_manifest_test.rb
PASS capabilities=8 required_admin_routes=8

ruby -c tests/contracts/baseline_manifest_test.rb
Syntax OK

git diff --check -- docs/contracts/sub2api-customization-inventory.yaml docs/contracts/admin-experience-contract.md tests/contracts/baseline_manifest_test.rb
passed
```

## 证据与限制

运行时身份引用 2026-08-08 账号监控/成本生产验证报告，并沿用 2026-08-01 蓝绿验证中未重建的共享容器 immutable identity。本文档只冻结已记录的只读证据，不声称当前工作树已部署，也不包含任何凭据、Cookie、Bearer 或 API key。后续任务必须在候选环境重新核对 release-state、镜像和迁移哈希后才能进入发布门禁。
