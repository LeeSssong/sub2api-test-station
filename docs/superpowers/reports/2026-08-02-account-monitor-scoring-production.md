# 账号评分与监控入口生产报告

**生产结论：** 进行中

## 推送与运行身份

- 发布基线：`e835aa4f1422a977af55fb36fe71b80caa0ef53b`。
- 生产运行源码提交：`a0f3435885abe2a38aaa2d5fb8465dc24d85a3a1`。
- 生产运行镜像：`xingqiao-sub2api@sha256:dc5344a63881fbff40a360f613165f694b2f2c652d3c4ac0ef7ad015455699fa`。
- 活动槽位与上游：`green`、`sub2api-green:8080`。

## 迁移与基础容器

- 迁移集合由 `176e6659b45bffbf11f5e1fce7dfbaf60906fe974553d7156fdc516231f4f5d0` 受控迁移到 `c618fc284897bb24c662297ba6cb263064a1e04a024e5432f50f082ac7317408`。
- `188_account_monitor_group_score_weights.sql` 与 `193_usage_log_actual_response_model.sql` 已进入生产迁移记录。
- PostgreSQL、Redis、Caddy 容器身份保持不变：`2db52788ad73`、`c45202c0d9e6`、`1a3379491955`。

## 页面与接口验收

- `/admin/accounts/monitor` 返回 200；合法管理员会话可以打开页面。
- 分组 Tab 按倍率从 `2.00x` 降到 `0.10x`；关闭分组显示“已关闭”。
- 默认评分为成本优势 15、成功率 45、首包时间 20、总耗时 20。
- 合计 99 的保存请求返回 400；分组 6 保存 `20/40/20/20` 时分组 2 保持默认，恢复默认后分组 6 返回 `15/45/20/20`。
- 账号 20 的原生调度优先级从 7 改为 8 后立即返回 8，并已恢复为 7。

## 未验证项

- 页面加载分组数据后出现 `Cannot read properties of null (reading 'includes')`，评分弹窗和账号卡片未能在合法管理员浏览器会话中完成 UI 操作验收。
- 因页面运行时错误，不能把本次生产发布报告为完成；状态保持“进行中”。

## 回滚点

- 上一活动槽：`blue`。
- 上一镜像：`xingqiao-sub2api@sha256:5540e46c1b2088b5a83e2211ebe223e4b1f96456d42a94056b761d31ea110482`。
- 上一源码提交：`638ce8f01a9c1879f76c0415fe90ef78c782d089`。
- 恢复时使用生产 release state 和保留的 release record；不得回滚数据库、重建 PostgreSQL、Redis 或 Caddy。
