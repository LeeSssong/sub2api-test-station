# T03-R1 发布预检（根任务专属，候选阶段）

状态：`NOT_RELEASED`；`downtime_required=unverified until root preflight`。

候选：`codex/t03-r1-upstream-cost-persistence@32f8ec12572ad9d2e50eab054788d4ec0bf05454`
tree：`ab19d7807763e1fcd03d0d0cd3f8f49d335f9fbb`
merge base：`19492c57da24270eb2b3e9b5d9727c2865aebb9e`
迁移 222 SHA-256：`47f786d6b2b020d0211a17d4ccd2bc6bb3774a315f483fdc0ac45657c9ee738e`

## 根任务必须 fail-closed

- 只能在精确 `AUTHORIZE_MERGE_TO_MAIN` 后将候选合并到当时干净且未漂移的 main。
- 合并后必须重跑专项测试、构建、migration/Ent 同步和 `git diff --check`。
- 宿主必须接受迁移 hash 并输出明确 `downtime_required=false`；否则在迁移、重启或蓝绿切换前停止。
- independent final review 必须为 APPROVE，且当前管理员 handler 的 read-time upstream fallback 必须被移除或得到明确的审查处置。
- 发现 GitHub Actions、非 expand-only 迁移、external-primary、T05 或生产/上游调用即停止。

## 回滚

候选阶段保留分支、提交、证据和 `stash@{0}`。若未来合并后预检或线上验证失败，保留候选和已应用独立表，按 reviewed 蓝绿方式回退到前一已验证镜像；不得 destructive rollback 或删除表。
