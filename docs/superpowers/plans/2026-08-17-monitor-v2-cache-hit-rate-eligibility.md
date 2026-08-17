# Monitor V2 缓存命中率有效样本口径修正实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正 Monitor V2 缓存命中率的分子/分母，只统计成功且具备文本 Token Prompt Cache 语义的用量流水，同时保持账务、API 和前端合同不变。

**Architecture:** 在现有 `monitorV2Repository.GetCacheStats` 的 SQL 聚合层集中应用缓存样本谓词：`actual_cost > 0`，并接受显式 `billing_mode='token'` 或历史空计费模式且无图片/视频字段的兼容流水。服务层、API 响应和其他费用/用量聚合不变；通过 SQL mock 场景矩阵锁定边界，再做仓储/服务聚焦验证与只读生产交叉核对。

**Tech Stack:** Go, `database/sql`, PostgreSQL, `sqlmock`, `testify`, 既有 Monitor V2 service/repository 测试与本地/宿主发布链。

## Global Constraints

- 只修改 Monitor V2 缓存统计仓储查询及其直接相关测试；不改账务、价格、倍率、缓存策略、Luna 禁用策略或前端 API 合同。
- 不新增数据库迁移、配置项、历史回填、生产数据写入、第二事实源或 GitHub Actions。
- 任务必须在独立 worktree 中实施；根 `main`、全局队列、项目进度总账、发布证据和生产状态仅由唯一发布总控修改。
- 仅运行直接相关的 repository/service 测试、必要的后端 compile-only/build、`gofmt` 和 `git diff --check`；不运行全仓、压力、soak、mutation 或无关浏览器矩阵。
- 发布预检预期 `downtime_required=false`；若实际为 `true`，在任何停机、迁移、重启或切换前停在授权门禁。
- 上线后只读核验 24h 与 7d 窗口，确认失败占位和图片/视频/按次流水不进入缓存样本。

---

## 文件变更地图

- Modify: `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go` — 在 `request_count` 与 `hit_count` 两个聚合 FILTER 中复用同一缓存样本谓词。
- Modify: `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go` — 更新 SQL mock，并加入成功 Token、未命中 Token、失败占位、图片、视频/按次、历史空 `billing_mode` 六类样本断言。
- Inspect only: `upstream/sub2api/backend/internal/service/monitor_v2_service.go` 及其测试 — 确认百分比计算、空样本和响应结构无需变化；若聚焦测试暴露直接回归，只补最小测试修复，不扩大范围。
- Create: `docs/superpowers/specs/2026-08-17-monitor-v2-cache-hit-rate-eligibility-design.md`（已完成）— 记录问题证据、谓词、验收矩阵和回滚条件。
- Create: `docs/superpowers/plans/2026-08-17-monitor-v2-cache-hit-rate-eligibility.md`（本文件）— 记录实施步骤与验证命令。

---

### Task 1: 为缓存样本谓词补齐失败测试（RED）

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go`

**Interfaces:**
- Consumes: 当前 `GetCacheStats` SQL mock，返回列仍为 `group_id`, `evidence_available`, `request_count`, `hit_count`。
- Produces: 能约束 SQL 必须同时出现 `actual_cost > 0`、`billing_mode` 分支和图片/视频字段兼容条件的测试。

- [x] **Step 1: 扩展 SQL mock 正则，先要求新谓词存在。**

  将 `TestMonitorV2RepositoryGetCacheStatsGroupsRows` 的 `ExpectQuery` 改为匹配两个聚合都带同一语义：

  ```text
  actual_cost > 0
  billing_mode = 'token'
  billing_mode IS NULL OR billing_mode = ''
  image_count / video_count / image_input_tokens / image_output_tokens
  cache_read_tokens > 0
  ```

- [x] **Step 2: 增加场景化统计测试数据。**

  在同一 SQL mock 返回聚合结果前，用表格注释明确期望六类流水的归类；至少覆盖：成功 Token 命中（分子+分母）、成功 Token 未命中（仅分母）、`actual_cost=0` 失败占位（排除）、成功 `billing_mode=image`（排除）、成功 `billing_mode=video/per_request`（排除）、历史空 `billing_mode` 且图片/视频字段全零（纳入）。断言 `request_count`/`hit_count` 与现有三组 group 行一致且不改变 `EvidenceAvailable`。

- [x] **Step 3: 运行 RED。**

  Run:

  ```bash
  cd /Users/gongtengxinwen/Documents/sub2api搭建/upstream/sub2api/backend
  go test ./internal/repository -run 'TestMonitorV2RepositoryGetCacheStats' -count=1
  ```

  Expected: FAIL，因为当前仓储 SQL 尚未包含上述筛选谓词，`sqlmock` 的新正则无法匹配。

---

### Task 2: 在仓储 SQL 中实现最小修正（GREEN）

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go`

**Interfaces:**
- Consumes: Task 1 的 SQL mock 正则和场景矩阵。
- Produces: `GetCacheStats` 返回结构不变；`request_count` 与 `hit_count` 仅基于同一缓存样本谓词。

- [x] **Step 1: 提取 SQL 语义谓词并分别嵌入两个 FILTER。**

  在 `COUNT(ul.id)` 的 FILTER 中加入：

  ```sql
  ul.actual_cost > 0
  AND (
    ul.billing_mode = 'token'
    OR (
      (ul.billing_mode IS NULL OR ul.billing_mode = '')
      AND COALESCE(ul.image_count, 0) = 0
      AND COALESCE(ul.video_count, 0) = 0
      AND COALESCE(ul.image_input_tokens, 0) = 0
      AND COALESCE(ul.image_output_tokens, 0) = 0
    )
  )
  ```

  `hit_count` 在该谓词后继续追加 `AND ul.cache_read_tokens > 0`。平台条件、时间边界、LEFT JOIN、分组和返回列保持原样；不要把谓词移动到 JOIN `ON`，避免改变无样本分组的 `evidence_available` 和零计数返回行为。

- [x] **Step 2: 运行 GREEN 与直接相关服务测试。**

  Run:

  ```bash
  cd /Users/gongtengxinwen/Documents/sub2api搭建/upstream/sub2api/backend
  go test ./internal/repository -run 'TestMonitorV2RepositoryGetCacheStats' -count=1
  go test ./internal/service -run 'MonitorV2|monitor_v2' -count=1
  ```

  Expected: PASS；服务层百分比、空样本和 API DTO 测试不需改动。

- [x] **Step 3: 格式化并检查差异。**

  Run:

  ```bash
  gofmt -w internal/repository/monitor_v2_repo.go internal/repository/monitor_v2_repo_test.go
  git diff --check
  ```

  Expected: exit 0，diff 仅限本任务两个 Go 文件。

- [x] **Step 4: 提交候选实现。**

  ```bash
  git add internal/repository/monitor_v2_repo.go internal/repository/monitor_v2_repo_test.go
  git commit -m "fix: scope monitor v2 cache hit samples"
  ```

---

### Task 3: 候选级编译与发布前验证

**Files:**
- Inspect: `upstream/sub2api/backend/internal/service/monitor_v2_service.go`
- Inspect: `upstream/sub2api/backend/internal/handler/*monitor*v2*`（仅确认 API 合同）

- [x] **Step 1: 运行 Monitor V2 直接相关测试集合。**

  ```bash
  cd /Users/gongtengxinwen/Documents/sub2api搭建/upstream/sub2api/backend
  go test ./internal/repository ./internal/service -run 'MonitorV2|monitor_v2' -count=1
  ```

- [x] **Step 2: 运行后端 compile-only/build 门禁。**

  ```bash
  go test ./cmd/... -run '^$'
  go build ./cmd/...
  ```

  Expected: exit 0；不执行全仓测试。

- [x] **Step 3: 确认候选工作区边界。**

  ```bash
  git diff --check
  git status --short
  git diff --name-only origin/main...HEAD
  ```

  Expected: 只有 `monitor_v2_repo.go`、`monitor_v2_repo_test.go` 及本任务规格/计划（若计划在候选 worktree 保存）变化；无迁移、配置、前端或 workflow 文件。

- [x] **Step 4: 生成交接信息并停在 `READY_FOR_ROOT_REVIEW`。**

  交接必须包含候选分支/HEAD、工作区路径、测试命令及输出、`downtime_required=false` 预期、无迁移/无生产写入声明、回滚为上一活动槽；不得自行合并、推送、部署或修改全局账本。

---

### Task 4: 根总控整合、发布与线上只读验收

**Owner:** 唯一发布总控；功能候选只提供证据，不执行根操作。

- [ ] **Step 1: 盘点所有非 `main` worktree 与领先提交。**

  在合并前按项目约束检查分支、提交、脏状态和生产证据；不得越过 T15 停机门禁或 T16 冻结边界。

- [ ] **Step 2: 合并候选到已验证的根 `main` 并推送。**

  仅在当前单车道空闲、候选审查通过后合并；保留合并提交和 `origin/main` SHA 证据。

- [ ] **Step 3: 执行既有本地/宿主发布预检。**

  无迁移预期 `downtime_required=false`；若预检实际返回 `true`，立即停在授权门禁，不重启、不切换。

- [ ] **Step 4: 发布成功后做线上只读交叉核对。**

  对 Monitor V2 默认 7d 和 24h 窗口核对：

  1. API 的 `request_count`/`hit_count` 与生产只读 SQL 使用同一谓词；
  2. `actual_cost=0` 且零 Token/无上游响应的失败占位不在分母；
  3. `billing_mode=image/video/per_request` 的成功流水不在分母；
  4. 历史空 `billing_mode` 且无图片/视频字段的 Token 流水仍可计入；
  5. 其他用量/费用/利润页面的账务字段未改变。

- [ ] **Step 5: 回滚判定与收口。**

  若 API 错误、样本数异常为零、SQL/API 不一致或其他 Ops 指标回归，切回上一活动槽/上一镜像；无迁移和生产写入，不需要数据恢复。只有“已推送 + 已部署 + 已验证生效”齐备后，根总控才在总账标记 DONE。

---

## Self-review checklist

- [x] 规格中的失败占位、图片/视频/按次排除和历史空 `billing_mode` 兼容均有对应测试/实现步骤。
- [x] SQL 谓词在分母与分子中保持一致，避免出现“分子已过滤、分母未过滤”的新偏差。
- [x] 返回列、服务层百分比计算和前端 API 合同未改变。
- [x] 无 TBD/TODO/“后续补充”式占位；每个命令和预期结果均明确。
- [x] 计划未授权功能 worktree 修改根 `main`、队列、总账或生产。

## Handoff

建议登记为：

- 编号：建议 `T19`（`T18` 已被“渠道状态官方聚合/自建监控可切换”占用；最终以队列事实源分配的下一个编号为准）
- 名称：`Monitor V2 缓存命中率有效样本口径修正`
- 状态：`BACKLOG`
- 排位：当前 T15 发布车道完成后，按单车道规则排队；不打断 T15，也不解冻 T16。

规格：`/Users/gongtengxinwen/Documents/sub2api搭建/docs/superpowers/specs/2026-08-17-monitor-v2-cache-hit-rate-eligibility-design.md`

计划：`/Users/gongtengxinwen/Documents/sub2api搭建/docs/superpowers/plans/2026-08-17-monitor-v2-cache-hit-rate-eligibility.md`
