# 非 main 工作区整合与生产发布交接

日期：2026-08-09

## 范围

- 所有本地分支相对最终 `main` 的独有提交数必须为 0。
- 已并入 GPT 分组基线、Resend SMTP 诊断、Sub 上游实际成本、监控当前配置历史边界及跨分支 UsageLog 兼容修复。
- 根目录既有未跟踪发布证据保持原样，不纳入提交、不覆盖、不删除。
- 本轮不执行生产迁移、容器重启、蓝绿切换或配置写入。

## 发布身份与迁移

- 镜像仓库：`ghcr.io/leesssong/xingqiao-sub2api`
- 生产当前迁移哈希：`9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0`
- 候选迁移哈希：`1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98`
- 发布器仅允许上述精确哈希对进入最长 300 秒维护路径；其他迁移集合继续 fail closed。
- 最终 `source_commit`、`source_tree`、测试命令和生产参数由推送后生成的 `release-evidence/sub2api-release-ready-<commit>.json` 与同名 `.env` 冻结。

## 生产参数

- SSH 目标：`ubuntu@43.133.75.82`
- SSH 端口：`2222`
- SSH 私钥：`/Users/gongtengxinwen/.ssh/tencent_lighthouse_seoul_sub2api`
- known hosts：`/Users/gongtengxinwen/.ssh/known_hosts`
- 传输模式：优先 `preloaded`，由发布脚本绑定候选镜像 ID、归档 SHA-256、源码提交、源码树和迁移哈希。
- 维护授权来源哈希：`9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0`

## 后续执行边界

用户发出部署指令后，不再重复常规构建、单元测试、类型检查或发布合同回归；直接使用冻结证据进入生产发布。仍不可省略发布脚本内建的 release identity、镜像/迁移一致性、共享容器身份、`/healthz`、`/readyz`、公共路由和失败回滚门禁。
