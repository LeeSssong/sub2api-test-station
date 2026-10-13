# Sol 500k 与订阅计费主站发布（2026-10-13）

- 结果：64.83.10.67 蓝绿发布成功，保留 ranxi 2.10.5，仅移植两项 Sol 改动；未回滚。测试站未查询、未同步。
- 上游来源：Wei-Shaw/sub2api commits 07185812655ec2e0740ca76882879c0b9ae5ee0a（500k 默认目录）与 43768680d38ccdbd8fffc2cb91d3c1fc778a61c6（订阅长上下文附加费豁免）。保留上游明确上下文、API Key 计费及自定义区间价；适配本地 Fast、响应模型重算和原始上游成本路径。
- 来源：codex/sol-500k-billing 快进合入根 main，推送/fetch 后干净、非 detached 且 commit/tree 与 origin/main 一致。部署 commit bc127991346fe82ca1b187cff364adc68aeea617；tree af8b7682dc9b009c33f2570bc6d828add35f2a03。已合入分支作为历史证据，不再重复合并。本记录是部署后文档提交。
- 制品：ghcr.io/leesssong/xingqiao-sub2api:release-bc127991346fe82ca1b187cff364adc68aeea617-81c4606034154471deab273355a4286e5a15b84690fa614effeb74c60f1d8210；manifest digest sha256:81c4606034154471deab273355a4286e5a15b84690fa614effeb74c60f1d8210；config digest sha256:21156ebba49d2f23b48bedeb8f527f7d85c4735f630b0bd5b754d046e2bfe748。二进制 SHA256 7b55a9efde2d67a94fb88085f8e47c7d4f59ec696258bab2b124062840e63c36。从推送 main 的 Git archive 编译；核实源码及依赖不变后复用上次内嵌前端和运行时，镜像构建一次。
- 验证：旧代码 RED 复现 272k 与订阅附加费；移植后 Go unit 选择 GPT61Sol、OAuth manifest、订阅豁免、RecordUsage、统一计费、搜索/图片计费及模型倍率回归通过（4.240s）。覆盖边界、别名、API Key、缓存、Fast 重算、自定义区间价格。首次 GREEN 编译发现本地额外调用签名需适配，补齐后通过。linux/amd64 embed 构建及 git diff --check 通过；未做无关全量测试。
- 数据库/服务：SQL 字节未变，迁移 hash e5d79ffa00d518804211a877ede6c7cb54277ada8c76e975526dca622445a6de 与现网一致，无迁移。仅 API blue→green；worker、reauth、detector、PostgreSQL、Redis、Caddy、首页、独立 codex2api 容器 ID/镜像未变；Caddy 仅 reload。
- 线上：认证版本 2.10.5，来源标签/状态匹配；health、readyz 通过；认证模型目录返回 gpt-6.1-sol context_window=500000、max_context_window=872000。未触发真实长上下文消费，计费验证来自本地回归，未声称实际生产扣款验证。
- 耗时：镜像上下文 5.2s、COPY 1.6s、导出 3.4s；宿主北京时间 20:35:26 开始，约 20:35:47 切流，旧实例排空记录 301s、forced=true（300 秒期限后停止残留连接）。编译/上传未单独精确计时。
- 回滚入口：宿主使用记录对应环境执行 /usr/local/libexec/deploy-sub2api-blue-green-host.sh --rollback --record /var/lib/sub2api/release-records/20261013T123526Z-production-79421.json。旧 8081c05fa 镜像与配置保留，未回滚。
- 证据：受保护本地 ~/.config/sub2api/release-evidence/sol-500k-20261013/，含补丁、RED/GREEN、构建来源、发布日志、宿主记录、线上模型目录。无已知未解决发布问题。
