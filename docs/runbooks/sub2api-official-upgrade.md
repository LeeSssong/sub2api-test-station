# Sub2API 官方版本升级手册

## 原则

官方 Release 的版本、asset、checksum、官方 commit 和镜像标签是唯一事实源。管理员点击升级后，沿用官方发现、下载、校验和版本解析逻辑；星桥定制不再覆盖官方核心源码。外置控制面与薄适配器按独立合同发布，官方版本只需通过合同和兼容门禁。

## 正常流程

1. 本地/宿主脚本发现官方候选并记录 tag、commit、asset、checksum、合同版本和迁移哈希。
2. 在非活动槽位加载候选，运行后端/前端/updater/控制面合同测试、`go vet`、构建、镜像平台与标签校验。
3. 执行迁移分类门禁：expand-only 才能继续；破坏性迁移进入 `blocked`，不得自动切换。
4. 对账号、盈利、账务、对账、权限、筛选、分页、排序、CSV 和故障降级执行新旧双读对比。
5. 候选 readiness 达标后，管理员二次确认才允许蓝绿提升；提升和回退使用现有 root-owned 宿主执行器。
6. 切换后验收 `/healthz`、`/readyz`、管理员身份、2FA 会话、`/v1/models`、worker、控制面新鲜度、容器身份和迁移状态。

## 生产前只读预检

在生产主机执行：

```bash
sudo SUB2API_RELEASE_STATE=/var/lib/sub2api/release-state \
  SUB2API_RELEASE_ENV_FILE=/opt/sub2api/production/release.env \
  /usr/local/libexec/preflight-sub2api-externalization.sh
```

预检只读 `release-state` 和 `release.env`，验证文件权限、JSON schema、活动槽位/上游配对、镜像身份和迁移哈希一致性。成功结果必须包含 `status:"ready"`、`update_performed:false` 和 `promotion_performed:false`。失败时保持活动槽位不变，修复并重新预检。

## 回退与异常

资格、迁移、双读、健康或管理员验收任一失败，停止升级并保留候选、日志和证据。不要强行覆盖当前镜像、跳过 checksum、手改 release-state 或删除失败记录。蓝绿切换后若公共验收失败，立即按现有回退脚本恢复上一个活动槽位；若 schema 已发生破坏性变化，先走备份恢复和人工事故流程。

发布链继续使用本地/宿主脚本，不新增或依赖 GitHub Actions。
