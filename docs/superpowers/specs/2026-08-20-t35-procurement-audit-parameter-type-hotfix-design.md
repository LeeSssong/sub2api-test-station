# T35 采购审计 PostgreSQL 参数类型热修规格书

## 1. 任务身份与设计分类

- 任务：T35“采购保存 PostgreSQL 参数类型热修”。
- 工作区：`/Users/gongtengxinwen/.codex/worktrees/eeb6/sub2api搭建`。
- 分支：`codex/t35-procurement-audit-type-hotfix`。
- 精确基线：根 `main@101357776e1af9dbf83df282afd96cdb284ffcf4`，tree `e92a60dccd37b5b2e8123613d9085af8be66b723`。
- 分类：这是已有采购事务中三条同类 SQL 的 bounded hotfix。项目交付门禁要求保留正式规格、计划、TDD 和根总控审批，因此规格阶段按完整 brainstorming 证据、方案比较和分段设计执行；产品方案已由用户批准，本规格不重新打开产品决策。
- 权限边界：本 worktree 只维护 T35 的规格、计划、实现、直接测试和交接；根任务是唯一发布总控。本任务不修改根 `main`、全局队列/总账、发布证据或生产状态，不自行合并、推送、部署或触碰生产。

## 2. 现状证据与根因

生产实现位于 `upstream/sub2api/backend/internal/service/account_procurement_profitability.go`。T30 已将三条采购审计写入的 `request_id` 从 `$3` 改为 `LEFT($3,64)`，满足 `audit_logs.request_id VARCHAR(64)`，但以下动态值仍只出现在 `jsonb_build_object` 的多态参数位置：

1. 结算审计：`jsonb_build_object('account_id',$4,'reason',$5)`；
2. 清空审计：`jsonb_build_object('account_id',$4,'cleared',true)`；
3. 录入/修改审计：`jsonb_build_object('account_id',$4,'cost_cny',$5,'quota_usd',$6)`。

`jsonb_build_object` 的 value 参数是多态 `any`。在 PostgreSQL 解析参数化语句时，未带 cast 的 `$4/$5/$6` 没有来自其他 SQL 表达式的确定类型；Go 运行时参数值的类型不足以替代服务端的 parse-time 类型推断。因此 SQL 在审计步骤报 `pq: could not determine data type of parameter $4`（最小化 PREPARE 探针从第一个未定型参数开始报错），而审计步骤位于版本台账和账号投影更新之后、事务提交之前，最终由延迟 `Rollback` 回滚整笔事务。

现有 `account_profitability_test.go` 使用 sqlmock。它能验证事务顺序、参数和值，却不会让 PostgreSQL 解析生产 SQL；T30 的 `TestProcurementAuditBoundsRequestIDToAuditSchema` 也只检查源文件是否包含一次 `LEFT($3,64)`。因此两者都可能在生产 SQL 无法解析时继续通过。

2026-08-20 使用与仓库 integration harness 相同系列的原生 PostgreSQL 18 容器执行最小 PREPARE 探针：

- 未定型 `jsonb_build_object('account_id',$1,'cost_cny',$2,'quota_usd',$3)` 以 `could not determine data type of parameter $1` 失败；
- `$1::bigint`、`$2/$3::double precision` 后成功生成 JSON；
- 清空形态 `$1::bigint` 和结算形态 `$1::bigint/$2::text` 均成功。

这证明故障属于 PostgreSQL 参数推断，而非 handler、幂等键生成、账务公式、迁移或前端状态问题。

## 3. 目标与非目标

### 3.1 目标

1. 让采购录入/修改、清空和结算三条生产审计 SQL 在 PostgreSQL 中稳定解析并执行。
2. 保持审计与采购台账、账号投影处于同一事务：任一步骤失败都不提交局部状态。
3. 保持 T23–T30 已有台账、幂等、成本计算、错误映射、handler 和前端合同不变。
4. 使用真实 PostgreSQL 执行生产 service 路径建立 RED/GREEN 证据，并保留最窄的源码合同守门；sqlmock 只承担事务分支和失败注入，不作为参数推断的唯一证据。

### 3.2 非目标

- 不新增或修改数据库迁移、Ent schema、列、索引或约束。
- 不回填历史记录，不改写生产数据，不修订既有审计记录。
- 不改变采购成本、预计可用额度、确认成本、剩余成本、损失或利润公式。
- 不改变请求/响应 DTO、HTTP 状态、错误文案、幂等键、账号回读或前端交互。
- 不修改其他页面、渠道监控、调度、发布脚本、配置或 GitHub Actions。

## 4. 方案比较与选择

### 方案 A（选择）：在三条生产 SQL 的多态参数使用点显式 cast

直接为账号、金额/额度和原因分别增加 `::bigint`、`::double precision` 和 `::text`。优点是修改面最小，类型与 Go 输入及数据库事实一致，不改变 JSON 键、数值语义、调用参数、事务或接口；真实 PostgreSQL 回归可以直接执行原生产 service。风险仅是未来复制同类 SQL 时遗漏 cast，由精确源码合同和原生 PostgreSQL测试共同守门。

### 方案 B：Go 端预序列化完整 `extra`，以单个 `$N::jsonb` 写入

Go 先构造并 marshal 审计 JSON，再把完整 JSON 作为一个参数传给 SQL。它也能消除多态推断，但会改变三条 SQL 的参数布局，引入新的序列化错误路径，并把 PostgreSQL 原生数值编码改为 Go JSON 编码；对本次热修过宽。

### 方案 C：抽取新的审计 repository/helper 或 SQL 常量层

统一审计写入可提高复用性，但会改变 service 边界和调用结构，需要更大的事务接口设计。问题只有三处已知 SQL，重构会扩大回归面，且并不比就地 cast 更直接。

结论：采用用户已批准的方案 A。方案 B/C 不进入本任务。

## 5. 精确 SQL 设计

仅修改三条审计语句的 `jsonb_build_object` value 参数，其他 SQL 文本、参数次序和值保持原样。

### 5.1 采购结算

目标片段：

```sql
jsonb_build_object(
  'account_id', $4::bigint,
  'reason',     $5::text
)
```

- `$4` 对应 `ProcurementSettlementInput.AccountID int64`。
- `$5` 对应已通过词表校验的 `Reason string`。
- 保留 `NULLIF($1,0)`、`POST`、原路径、`LEFT($3,64)`、状态码 200 和既有 action。

### 5.2 采购清空

目标片段：

```sql
jsonb_build_object(
  'account_id', $4::bigint,
  'cleared',    true
)
```

- `$4` 对应 `ProcurementConfigInput.AccountID int64`。
- `true` 已是 PostgreSQL boolean literal，不增加无意义 cast。
- 保留清空投影、`cost_pending` 版本、actor、request id 和 action。

### 5.3 采购录入/修改

目标片段：

```sql
jsonb_build_object(
  'account_id', $4::bigint,
  'cost_cny',   $5::double precision,
  'quota_usd',  $6::double precision
)
```

- `$4` 对应 `AccountID int64`。
- `$5/$6` 对应 service 已校验为有限值的 `float64` 成本和额度。
- cast 只解决参数推断；审计 JSON 仍记录与本次实际台账/投影写入相同的 `nextCost/nextQuota`，不重新计算、不改舍入规则。

## 6. 数据流、幂等与失败/回滚语义

### 6.1 录入/修改

service 继续按既有顺序锁账号和当前版本、计算剩余成本/额度、结束旧版本、创建新 active 版本、更新 `accounts` 投影、插入审计，最后提交。显式 cast 只让最后的审计 SQL 可解析。审计、版本或投影任一步失败时函数返回错误，延迟 `Rollback` 撤销此前写入；不得为了“保存成功”而跳过审计或把审计移出事务。

### 6.2 清空

service 继续结束旧版本、清空账号投影、创建 `cost_pending` 版本、写 `cleared=true` 审计并提交。审计失败时必须恢复到事务前状态：旧版本仍有效、投影仍保留、无新 pending 版本、无审计。修复后四部分原子提交。

### 6.3 结算

service 继续校验 reason、幂等键、OAuth 账号、当前版本和永久不可用状态，计算并写入 settlement/loss 后写审计。审计失败时 settlement 状态、`ended_at/settled_at/loss_cny/settlement_request_id` 和审计全部不提交；修复后共同提交。

### 6.4 幂等与错误合同

- 相同 request id 且相同账号的录入/清空继续在发现既有版本后直接提交只读事务，不重复审计或创建版本。
- 相同 settlement request id 且相同账号继续直接返回成功；跨账号复用继续冲突。
- 输入错误、账号不存在、额度已消耗冲突、账号尚可用、数据库错误及 handler 的中文诊断/202 partial-success 合同均不变。
- 不引入 catch-and-continue、独立审计事务、重试、补写或后台修复。

## 7. TDD 与验证设计

### 7.1 真实 PostgreSQL RED/GREEN（参数推断主证据）

优先复用 `internal/repository/integration_harness_test.go` 的原生 PostgreSQL/testcontainers、全量迁移和 `integrationDB`。跨包 harness 适用：repository 测试包已经依赖 `internal/service`，新 build-tag integration 测试可直接调用 `service.NewAccountProfitabilityService(integrationDB)` 的生产路径，无需导出 SQL、增加 test hook 或复制 service 实现。

新增一个聚焦 integration 测试，使用唯一账号/request id 并覆盖三个子用例：

1. **录入/修改**：调用真实 `UpdateProcurementConfig`。修复前应在审计 parse 阶段得到 `pq: could not determine data type of parameter $4`；测试在最终报告失败前查询并证明账号投影、版本和审计均未留下局部写入。修复后断言 active 版本、账号投影和含 numeric `account_id/cost_cny/quota_usd` 的审计 JSON 原子落库。
2. **清空**：直接准备一个有效 active 版本和账号投影，再调用真实清空。修复前证明审计错误后旧版本/投影保持、无 pending 版本和审计；修复后断言旧版本结束、投影为空、新 `cost_pending` 版本和 `cleared=true` 审计共同提交。
3. **结算**：直接准备永久不可用 OAuth 账号及 active 版本，再调用真实 `SettleProcurement`。修复前证明 settlement 字段和审计均回滚；修复后断言 settled 状态、settlement request id、loss 和含 string reason 的审计共同提交。

RED 必须来自未修改生产 SQL 的真实 PostgreSQL 解析错误，而不是手写错误、sqlmock 返回值或只读源码断言。执行时用 `CI=1` 让 Docker 缺失显式失败，`-v` 输出需显示 integration harness 实际启动并运行目标测试，不能把“跳过容器”记录成通过。

建议命令：

```bash
cd upstream/sub2api/backend
docker info
CI=1 go test -tags integration ./internal/repository \
  -run 'TestAccountProcurementAuditParametersUsePostgreSQLTypes' -count=1 -v
```

### 7.2 源码合同与 sqlmock 事务分支

扩展现有采购审计源码守门，精确断言：

- 三条审计 SQL都保留 `LEFT($3,64)`；
- 结算含 `$4::bigint/$5::text`；
- 清空含 `$4::bigint` 和 `cleared=true`；
- 录入/修改含 `$4::bigint/$5::double precision/$6::double precision`；
- 不出现旧的未定型对应片段。

现有 sqlmock 成功、幂等和冲突测试继续运行。增加或收紧三条审计失败注入，断言 service 返回原错误并执行 `Rollback` 而非 `Commit`。这些测试负责快速锁住事务控制流；真实 PostgreSQL integration 测试负责参数解析，二者不得互相替代。

聚焦命令：

```bash
cd upstream/sub2api/backend
go test ./internal/service \
  -run 'Test(UpdateProcurementConfig|SettleProcurement|ProcurementAudit)' -count=1
go test ./internal/handler/admin \
  -run 'TestAccountHandler.*Procurement' -count=1
go build ./cmd/server
```

### 7.3 格式、范围与零迁移守门

实施候选必须执行：

```bash
gofmt -w \
  internal/service/account_procurement_profitability.go \
  internal/service/account_profitability_test.go \
  internal/repository/account_procurement_audit_type_integration_test.go
git diff --check
git diff --name-only 101357776e1af9dbf83df282afd96cdb284ffcf4...HEAD
git diff --exit-code 101357776e1af9dbf83df282afd96cdb284ffcf4...HEAD -- \
  migrations ent/schema frontend .github/workflows
```

允许的实现范围只有：

- `upstream/sub2api/backend/internal/service/account_procurement_profitability.go`；
- `upstream/sub2api/backend/internal/service/account_profitability_test.go`；
- 新增最窄的 `upstream/sub2api/backend/internal/repository/account_procurement_audit_type_integration_test.go`；
- T35 自身规格、计划与交接文档。

如真实 PostgreSQL测试可在不增加第三个测试文件的更窄既有位置复用同一 harness，允许收窄文件数，但不得退化为 sqlmock-only 或 source-only。

## 8. 迁移、发布预期与回滚

- 数据库迁移：零；migration set/hash 应与基线一致。
- 历史回填/生产数据改写：零。
- 构建产物：仅后端 Go 实现和测试变化；前端合同和静态资产不变。
- 发布预期：`downtime_required=false`，最终以根总控在合并后 `main` 上执行的受控发布预检为准。
- 代码回滚：根总控回退/反向应用 T35 实现提交并沿既有蓝绿链发布上一可用镜像；因为无迁移和数据形状变化，不需要数据库逆向迁移。
- 运行回滚：发布链异常时保持或恢复原活动槽。T35 不自行操作活动槽、宿主或生产。
- 已成功写入的新审计/采购事务是正常业务事实，不因代码回滚删除；修复前失败事务已经由 PostgreSQL 原子回滚，无半成品清理步骤。

## 9. 完成门槛与交接

实施阶段只有在以下条件全部成立后才可向根总控报告 `READY_FOR_ROOT_REVIEW`：

1. 真实 PostgreSQL RED 明确复现参数推断错误及事务零残留；GREEN 覆盖录入、清空、结算三条生产 service 路径。
2. 三条 SQL 精确 casts 与本规格一致，所有直接相关 unit/integration/handler 测试通过。
3. 后端构建、gofmt、`git diff --check` 和范围/零迁移守门通过。
4. worktree 干净，候选提交和验证证据绑定同一 HEAD。
5. 未修改全局队列/总账、发布证据、生产状态、根 `main`，未推送、合并或部署。

## 10. 规格自审

- 占位符扫描：通过；无未决产品选择或待补内容。
- 一致性：根因、三条精确 casts、测试预期和回滚语义一致。
- 范围：单一后端 SQL 热修，可由一个后续 implementation plan 完成。
- 歧义：`account_id=bigint`、`cost_cny/quota_usd=double precision`、`reason=text` 已逐参数固定；清空 boolean literal 保持原样。
- 证据质量：参数推断以真实 PostgreSQL 生产路径为主，源码合同和 sqlmock 仅作补充，不存在 sqlmock 假绿门槛。
