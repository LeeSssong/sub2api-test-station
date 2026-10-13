# 全工作区整合与主站蓝绿发布（2026-10-13）

- 目标：64.83.10.67；将全部现有非 main 工作区改动归档到根 main、推送 origin/main、蓝绿发布；执行中追加用户授权的 Sol 收费恢复。
- 合入：codex/intelligence-drawing-judge 5b33ad709、codex/safari-artwork-zoom 03a9cc438、codex/restore-sol-long-fee 668e27ffd。其他 ticket-gateway 与 sol-500k 工作区提交原已在 main；全部分支/工作区保留为已合入历史证据，不重复合并。最终检查各工作区干净，无 main..HEAD 独有提交。
- 最终应用来源：main commit 7fc6074886940e19abbdf3d775cf533926fd7e59，tree ce4502cc8eaded324b3d01135afbd65978cbb6d8。推送、fetch 和来源门禁确认根 main 干净、非 detached、与 origin/main commit/tree 一致。本记录为部署后的文档提交。
- 改动：智商监测轻量图片判定、Safari 作品缩放修复；恢复 Sol 订阅长上下文附加费，保留 500k 默认上下文和原账号调度。无 SQL 迁移；迁移 hash e5d79ffa00d518804211a877ede6c7cb54277ada8c76e975526dca622445a6de 与现网一致。
- 运行修复：默认 Docker seccomp 阻止 Chromium namespace，真实 Linux 截图失败。提交 3365d1274 纳入基于 Moby 默认规则的 profile，仅额外允许 clone/unshare/setns；不关闭 Chromium 沙箱、不使用 privileged 或 SYS_ADMIN。精确 profile 下非 root、断网真实 PNG 尺寸/像素冒烟通过 1.17s，最终镜像冒烟通过 1.41s。
- 构建：从已推送 main 的 Git archive 本地构建前端和 linux/amd64 Go 二进制，上传校验后在原生宿主封装。复用有来源的旧运行依赖并新增 Chromium/中文字体；未使用服务器目录或容器源码。追加收费版复用同源前端和已验证运行依赖。初次追加镜像遗漏更新来源标签，在发布前使用缓存重新生成元数据，错误标签镜像未部署。
- 制品：第一轮 manifest sha256:c3fc0a354e9d8361a179ba8ff382c9d8343683953dc479dbab9c7aa308036680；最终 manifest sha256:2ec4645390d205f523b071f4b8eddd84f0e64073f245e099bbca598bd5766b5d，config sha256:9ef123c26dbdc67c6655f014ca2f3f474c442246a8f397e88ae02119673d3a14。seccomp 文件 SHA256 6429cafaa78f6b45cc00a5784fcc03eac99c6a306987b3f56a1d6d06382be4a1。
- 验证：Safari Vitest 28/28、绘图前端 7/7、Go 绘图相关服务测试 11.479s、vue-tsc 和前端生产构建通过（Vite 19.25s）。复用收费恢复 RED/GREEN 证据：相关服务回归 2.985s，覆盖272000/272001/500000边界。无冲突，无数据库/调度变化，不重复无关全量回归。未调用真实付费视觉判题，未用实际生产扣费验证收费。
- 线上：两轮候选就绪后切流，健康/readyz、认证版本2.10.5、来源标签通过；智商监测管理接口读取成功；Sol模型目录 context_window=500000。API与worker第一轮更新，worker先排空；第二轮仅API更新。第一轮脚本暂停/恢复 reauth worker 并重建容器但镜像不变；数据库、Redis、detector、首页及独立codex2api未更换。
- 时间：第一轮宿主21:46:06开始，约21:46:48切流，旧连接289秒自然排空、未强制终止；最终轮21:55:01开始、约21:55:20切流，旧连接排空299秒、forced=true，旧API已停止。构建/上传未全程精确分段计时；未声称零请求波动。
- 回滚：使用 /usr/local/libexec/deploy-sub2api-blue-green-host.sh --rollback --record /var/lib/sub2api/release-records/20261013T135501Z-production-225696.json，并提供原发布环境；第一轮记录20261013T134606Z-production-203466.json。旧制品和运行配置备份保留，未回滚。
- 证据：本地受保护 ~/.config/sub2api/release-evidence/all-worktrees-20261013/ 与 final/；宿主 /var/lib/sub2api/release-staging/all-worktrees-20261013/。测试站未查询、未同步。
- 未解决事项：诊断时错误读取了包含敏感值的宿主 Compose 片段，已提示用户；本文不含敏感值，尚未轮换相关凭据，需要后续受控轮换。视觉模型准确率、费用与真实判题结果仍未实测。
