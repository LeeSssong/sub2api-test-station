# 倍率监控迁移与滚动窗钳制 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给滚动窗历史拉取加上下限钳制，并把倍率变化告警从已废弃的 `accounts.rate_multiplier`（生产恒为 1，4 天零触发）迁移到 schema v2 的可信倍率。

**Architecture:** `RollingWindowLimitFor` 补齐与 `HistoryLimitFor` 同款的上下限。`opsmonitor.Service` 新增窄接口 `MultiplierSource`，`evaluateMultiplier` 改吃 schema v2 `multiplier.value`；随后从 `accountquality` 与 Ruby 采集脚本移除废弃字段，并清理存量基线记录。

**Tech Stack:** Go 1.24（module `example.invalid/relay-ops-service`）、Ruby、PostgreSQL。

## Global Constraints

- 只有 `multiplier.status == "ok"` 且 `value != nil` 且 `*value > 0` 的倍率才可信；其余状态不建基线、不告警、不参与比较。
- 倍率不可用不得退化为「倍率变为 0」或「倍率消失」类告警 —— 拿不到值只是没有证据，不是变更事件。
- 迁移后必须清理 22 条存量 `multiplier_baseline` 记录（当前值全为 `1x`），否则首轮会把 `1x → 0.16x` 误报成倍率变更。
- 不得修改 `internal/accounthealth/` 之外任何与本次无关的判定逻辑。
- Go 测试在 `relay-ops-service/` 目录下运行。

## 生产事实（实施依据，已核实）

- `relay_ops.incidents` 中 22 条 `site:account:N:multiplier_baseline`，`current_value` 全为 `1x`，`sample_count > 1` 的为 **0** 条 —— 该告警从未触发过。
- `accounts.rate_multiplier` 生产实测 12 个账号全部为 `1`。
- 真实倍率区间 `0.05x–0.25x`，来自 `account-monitors` 的 schema v2 `multiplier.value`。

---

### Task 1: 滚动窗历史条数钳制

**Files:**
- Modify: `relay-ops-service/internal/accounthealth/dayslice.go:60-66`
- Test: `relay-ops-service/internal/accounthealth/dayslice_test.go`

**Interfaces:**
- Changes: `RollingWindowLimitFor(intervalSeconds int) int` 增加下限 `12`、上限 `200`

`HistoryLimitFor` 有 `historyLimitMin=100` / `historyLimitMax=2000` 两道钳制，`RollingWindowLimitFor` 一道都没有。管理员把 `interval_seconds` 调到 1 秒 → `limit = 5400` → 11 个账号每 5 分钟各拉 5400 条 → 撞上客户端 `maxResponseBytes`（2 MiB）→ `getStrict` 返回 `errResponseTooLarge` → 告警作业 `continue` 跳过该账号 → 全部账号落入空窗回退 → **判定静默退回 7 天累计口径**，刚修复的窗口问题复活，且只在日志留一行。

下限 `12`：1 小时窗至少要 12 个样本才有判定意义（对应当前 300 秒间隔）。上限 `200`：足以覆盖 18 秒间隔，再密的探测间隔属于配置异常，不应让告警作业跟着放大请求量。

- [ ] **Step 1: 写失败测试**

在 `dayslice_test.go` 的 `TestRollingWindowLimitFor` 里补充边界用例（若该测试不存在则新建）：

```go
func TestRollingWindowLimitForIsClamped(t *testing.T) {
	cases := []struct {
		name     string
		interval int
		want     int
	}{
		{"当前生产间隔", 300, 18},
		{"非法值回退 300 秒", 0, 18},
		{"负值回退 300 秒", -5, 18},
		{"密集探测撞上限", 1, 200},
		{"60 秒间隔在区间内", 60, 72},
		{"18 秒间隔恰好触顶", 18, 200},
		{"稀疏探测撞下限", 3600, 12},
		{"超长间隔仍给下限", 86400, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RollingWindowLimitFor(tc.interval); got != tc.want {
				t.Fatalf("RollingWindowLimitFor(%d) = %d, want %d", tc.interval, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -run TestRollingWindowLimitForIsClamped -v`
Expected: FAIL，`RollingWindowLimitFor(1) = 5400, want 200`

- [ ] **Step 3: 加钳制**

在 `dayslice.go` 的常量块补两个常量，并改写函数：

```go
	rollingLimitMin = 12
	rollingLimitMax = 200
```

```go
// RollingWindowLimitFor sizes the history page for the one-hour alert window.
// The clamps matter: without an upper bound a one-second probe interval asks
// for 5400 entries, blows past the client's response ceiling, and every
// account silently falls back to the cumulative window — reviving the very
// staleness this window was added to remove.
func RollingWindowLimitFor(intervalSeconds int) int {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultIntervalSecond
	}
	needed := int(math.Ceil(float64(rollingWindowSeconds) / float64(intervalSeconds)))
	limit := int(math.Ceil(float64(needed) * rollingLimitSlack))
	if limit < rollingLimitMin {
		return rollingLimitMin
	}
	if limit > rollingLimitMax {
		return rollingLimitMax
	}
	return limit
}
```

- [ ] **Step 4: 验证通过并提交**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ ./internal/app/ -count=1`
Expected: PASS（`app` 包里有断言 `limit == 18` 的端到端测试，300 秒间隔下钳制不改变该值）

```bash
git add relay-ops-service/internal/accounthealth/
git commit -m "fix: clamp rolling window history limit"
```

---

### Task 2: 倍率告警改用可信倍率

**Files:**
- Modify: `relay-ops-service/internal/opsmonitor/service.go`（`Service` 结构体、`Run` 的账号循环、`evaluateMultiplier`）
- Modify: `relay-ops-service/internal/app/app.go:199-201`（`configuredSiteMonitor`）与 `:289`（注入点）
- Test: `relay-ops-service/internal/opsmonitor/service_test.go`

**Interfaces:**
- Produces: `opsmonitor.MultiplierSource` 接口 —— `ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)`
- Produces: `opsmonitor.Service.Multipliers MultiplierSource` 字段
- Changes: `evaluateMultiplier` 的调用方从 `quality.Accounts[].RateMultiplier` 改为投影中的可信倍率
- Changes: `configuredSiteMonitor(reader, quality, multipliers, state, notifier)` 增加一个参数

`Service.Reader` 是 `opsmetrics.Reader`，拿不到 `account-monitors`。新增窄接口而不是扩宽 `Reader`，避免波及 `opsmetrics` 的其他实现者。app.go 第 234 行构造的 `reader`（`*sub2api.HTTPReader`）同时满足两者，直接注入即可。

- [ ] **Step 1: 写失败测试**

在 `service_test.go` 追加：

```go
type stubMultiplierSource struct {
	projection sub2api.AccountMonitorProjection
	err        error
}

func (s stubMultiplierSource) ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error) {
	return s.projection, s.err
}

func multiplierProjection(accountID int64, value *float64, status string) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Accounts: []sub2api.AccountMonitorAccount{{
			AccountID: accountID, Name: "Pro-SHEN-0.16",
			Multiplier: sub2api.AccountMonitorMultiplier{Value: value, Source: "declared", Status: status},
		}},
	}
}

func TestEvaluateMultiplierUsesTrustworthyValue(t *testing.T) {
	value := 0.16
	source := stubMultiplierSource{projection: multiplierProjection(26, &value, "ok")}
	service, repo := newTestSiteMonitor(t, source)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	record, found, err := repo.Get(context.Background(), "site:account:26:multiplier_baseline")
	if err != nil || !found {
		t.Fatalf("baseline not created: found=%v err=%v", found, err)
	}
	if record.CurrentValue != "0.16x" {
		t.Fatalf("CurrentValue = %q, want 0.16x（必须是可信倍率而非废弃的 1x）", record.CurrentValue)
	}
}

func TestEvaluateMultiplierSkipsUntrustworthyStatus(t *testing.T) {
	for _, status := range []string{"failed", "stale", "unsupported", "unavailable"} {
		t.Run(status, func(t *testing.T) {
			source := stubMultiplierSource{projection: multiplierProjection(26, nil, status)}
			service, repo := newTestSiteMonitor(t, source)

			if err := service.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
				t.Fatalf("status=%s 时不得建立基线（拿不到值不是变更事件）", status)
			}
		})
	}
}

func TestEvaluateMultiplierSkipsNonPositiveValue(t *testing.T) {
	zero := 0.0
	source := stubMultiplierSource{projection: multiplierProjection(26, &zero, "ok")}
	service, repo := newTestSiteMonitor(t, source)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
		t.Fatal("倍率 0 是坏数据，不得建立基线")
	}
}

func TestEvaluateMultiplierSilentWhenSourceFails(t *testing.T) {
	source := stubMultiplierSource{err: errors.New("unavailable")}
	service, repo := newTestSiteMonitor(t, source)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("倍率源故障不得让整个巡检失败: %v", err)
	}
	if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
		t.Fatal("读取失败时不得建立基线")
	}
}
```

`newTestSiteMonitor(t, source)` 需要你按 `service_test.go` 中既有的 `Service` 构造与 fake 仓库写法实现，返回 `(opsmonitor.Service, incidents.Repository)`，其中 `Reader` 用文件里已有的 fake reader（提供 account 26 为 active+schedulable），`Multipliers` 用传入的 source。先读该文件现有 helper 再动手，不要另起一套 fake。

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/opsmonitor/ -run TestEvaluateMultiplier -v`
Expected: 编译失败，`unknown field Multipliers in struct literal`

- [ ] **Step 3: 改造 Service**

在 `service.go` 增加接口与字段：

```go
type MultiplierSource interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
}
```

`Service` 结构体追加 `Multipliers MultiplierSource`。

把 `Run` 中账号循环里原先的倍率分支：

```go
		if account.RateMultiplier != nil {
			if err := s.evaluateMultiplier(ctx, object, *account.RateMultiplier, quality.ObservedAt); err != nil {
				return err
			}
		}
```

整段删除。改为在账号质量循环**之前**单独跑一轮倍率评估：

```go
	if err := s.evaluateMultipliers(ctx, active, now); err != nil {
		return err
	}
```

并新增：

```go
// evaluateMultipliers watches the trustworthy schema v2 multiplier. The old
// implementation watched accounts.rate_multiplier, which is fixed at 1 in
// production — 22 baselines, zero changes in four days. An upstream price
// change never reached this tripwire.
func (s Service) evaluateMultipliers(ctx context.Context, active map[int64]struct{}, now time.Time) error {
	if s.Multipliers == nil {
		return nil
	}
	projection, err := s.Multipliers.ListAccountMonitors(ctx)
	if err != nil {
		// 读不到倍率证据不是变更事件，静默跳过本轮
		return nil
	}
	for _, account := range projection.Accounts {
		if _, ok := active[account.AccountID]; !ok {
			continue
		}
		value := trustworthyMultiplier(account.Multiplier)
		if value == nil {
			continue
		}
		item := object{kind: "account", id: account.AccountID}
		if err := s.evaluateMultiplier(ctx, item, *value, now); err != nil {
			return err
		}
	}
	return nil
}

// trustworthyMultiplier mirrors the daily report's rule: status must be ok,
// the value present and positive. Sub2API only rejects value < 0, so a zero
// can reach us, and a zero would look like a price change to zero.
func trustworthyMultiplier(m sub2api.AccountMonitorMultiplier) *float64 {
	if m.Status != "ok" || m.Value == nil || *m.Value <= 0 {
		return nil
	}
	return m.Value
}
```

`evaluateMultiplier` 自身不需要改。

- [ ] **Step 4: 更新注入点**

`app.go` 的 `configuredSiteMonitor` 增加参数：

```go
func configuredSiteMonitor(reader opsmetrics.Reader, quality opsmonitor.QualitySource, multipliers opsmonitor.MultiplierSource, state *incidents.Machine, notifier opsmonitor.MessageSender) opsmonitor.Service {
	return opsmonitor.Service{Reader: reader, Quality: quality, Multipliers: multipliers, Incidents: state, Notifier: notifier}
}
```

调用点（第 289 行附近）改为：

```go
	siteMonitor := configuredSiteMonitor(reader, siteAccountQualitySource, reader, incidentMachine, notifier)
```

- [ ] **Step 5: 验证并提交**

Run: `cd relay-ops-service && go build ./... && go test ./... -count=1 2>&1 | tail -15`
Expected: 全包通过

```bash
git add relay-ops-service/internal/opsmonitor/ relay-ops-service/internal/app/app.go
git commit -m "feat: watch trustworthy multiplier for price changes"
```

---

### Task 3: 移除废弃倍率字段

**Files:**
- Modify: `ops/collect-account-quality-pulse.rb:333-335`、`:345`、`:356-364`、`:366-368`、`:392`
- Modify: `relay-ops-service/internal/accountquality/result.go:41`、`:63`、`:115`、`:166`、`:226`
- Modify: `relay-ops-service/internal/http/templates/ops.html:11`
- Test: `relay-ops-service/internal/accountquality/result_test.go`

**Interfaces:**
- Removes: `accountquality.AccountRecord.RateMultiplier`、`accountquality.AccountView.Multiplier`、`accountquality.formatMultiplier`
- Removes: Ruby 端 `rate_multiplier` 方法及其在 sample / summary 中的输出
- Removes: `/ops` 账号质量表的「倍率」列

Task 2 之后 `RateMultiplier` 在 Go 侧已无消费者。`/ops` 的账号质量表移除倍率列，与飞书日报保持同一口径 —— 质量视图只讲稳定性与延时，倍率属于成本维度。

**模板改动无法被编译期发现**：`server.go:424` 是 `_ = s.templates.ExecuteTemplate(...)`，错误被丢弃，字段删了但模板还引用的话，页面会静默截断成半张。删列与删字段必须同批完成，且部署后必须人工打开 `/ops` 确认整页渲染完整。

- [ ] **Step 1: 确认 Go 侧无残留消费者**

Run:

```bash
cd relay-ops-service && grep -rn "RateMultiplier\|formatMultiplier" --include="*.go" . | grep -v "_test.go"
```

Expected: 仅 `internal/accountquality/result.go` 自身命中。若 `opsmonitor` 仍有命中，说明 Task 2 未完成，**停止并上报**。

- [ ] **Step 2: 删除 Go 侧字段**

`result.go`：删除第 41 行 `RateMultiplier *float64` 字段、第 63 行 `AccountView` 中的 `Multiplier`、第 115 行 `Multiplier: formatMultiplier(...)` 赋值、第 166 行校验条件中的 `(account.RateMultiplier != nil && !finiteNonNegative(*account.RateMultiplier)) ||` 片段、第 226 行起的 `formatMultiplier` 函数。删除 `result_test.go` 中所有引用 `RateMultiplier` / `Multiplier` 的断言。

若 `finiteNonNegative` 删除后再无调用者，一并删除（先 `grep -rn "finiteNonNegative" --include="*.go" .` 确认）。

- [ ] **Step 3: 删除 /ops 倍率列**

`ops.html:11` 的表头去掉 `<th>倍率</th>`，表体去掉 `<td>{{.Multiplier}}</td>`，并把空表占位的 `colspan="8"` 改为 `colspan="7"`。

- [ ] **Step 4: 删除 Ruby 端采集**

`ops/collect-account-quality-pulse.rb`：删除 `collect_account` 中的 `multiplier = nil` 与 `multiplier = rate_multiplier(@client.account(account_id))` 两行、sample 哈希中的 `"rate_multiplier" => multiplier,`、`rate_multiplier` 方法定义、`failure_sample` 的 `multiplier` 形参及其 `"rate_multiplier" => multiplier,` 输出（调用处同步改为 `failure_sample(account_id)`），以及 summary 哈希中的 `"rate_multiplier" => current.fetch("rate_multiplier"),`。

- [ ] **Step 5: 验证并提交**

Run:

```bash
cd relay-ops-service && go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -15
ruby -c ../ops/collect-account-quality-pulse.rb
grep -c "Multiplier" internal/http/templates/ops.html
```

Expected: 全包通过；`Syntax OK`；模板中 `Multiplier` 计数为 `0`

```bash
git add ops/collect-account-quality-pulse.rb relay-ops-service/internal/accountquality/ relay-ops-service/internal/http/templates/ops.html
git commit -m "refactor: drop deprecated rate multiplier from quality evidence"
```

---

### Task 4: 清理存量基线并部署

**Files:** 无代码改动，仅生产操作。

存量 22 条 `site:account:N:multiplier_baseline` 的 `current_value` 全是 `1x`。迁移后首轮巡检会拿真实倍率（如 `0.16x`）与之比较，把 `1x → 0.16x` 误报成一次倍率变更，向飞书推 22 条假告警。必须在部署新镜像**之前**清空。

- [ ] **Step 1: 备份存量基线**

```bash
ssh sub2api-prod 'sudo bash -s' <<'OUTER'
U=$(cat /opt/sub2api/production/secrets/relay-ops-database-url)
docker run --rm -i --network sub2api_default postgres:18-alpine psql "$U" -c "\copy (select * from relay_ops.incidents where incident_key like '%multiplier_baseline%') to stdout with csv header" > /tmp/multiplier-baseline-backup.csv
wc -l /tmp/multiplier-baseline-backup.csv
OUTER
```

Expected: 23 行（22 条记录 + 表头）

- [ ] **Step 2: 删除基线记录**

`notification_deliveries` 对 `incidents` 有外键，需先删除引用这些基线的投递记录（基线状态为 `muted`，正常不会有投递，但要确认）。

```bash
ssh sub2api-prod 'sudo bash -s' <<'OUTER'
U=$(cat /opt/sub2api/production/secrets/relay-ops-database-url)
docker run --rm -i --network sub2api_default postgres:18-alpine psql "$U" <<'SQL'
begin;
delete from relay_ops.notification_deliveries
 where incident_id in (select id from relay_ops.incidents where incident_key like '%multiplier_baseline%');
delete from relay_ops.incidents where incident_key like '%multiplier_baseline%';
select count(*) as 剩余基线 from relay_ops.incidents where incident_key like '%multiplier_baseline%';
commit;
SQL
OUTER
```

Expected: `剩余基线 = 0`

- [ ] **Step 3: 构建并部署**

```bash
rsync -a --delete relay-ops-service/ sub2api-prod:/opt/sub2api/production/relay-ops-service/
rsync -a ops/collect-account-quality-pulse.rb sub2api-prod:/tmp/
ssh sub2api-prod 'sudo install -m 0644 /tmp/collect-account-quality-pulse.rb /opt/sub2api/production/ops/ && rm -f /tmp/collect-account-quality-pulse.rb'
ssh sub2api-prod 'cd /opt/sub2api/production && sudo docker build -f infra/Dockerfile.relay-ops -t sub2api-relay-ops:multiplier-migration-20260727-v1 . 2>&1 | tail -3'
ssh sub2api-prod 'cd /opt/sub2api/production && sudo cp compose.yaml compose.yaml.bak-before-multiplier-migration-20260727 && sudo sed -i "s|image: sub2api-relay-ops:merged-5ffc301-20260727-v1|image: sub2api-relay-ops:multiplier-migration-20260727-v1|" compose.yaml && sudo docker compose --env-file .env -f compose.yaml up -d --no-deps --force-recreate relay-ops'
```

- [ ] **Step 4: 部署后验证（人工确认项）**

```bash
ssh sub2api-prod 'docker inspect sub2api-relay-ops-1 --format "health={{.State.Health.Status}} restarts={{.RestartCount}}"'
ssh sub2api-prod 'docker exec sub2api-caddy-1 wget -qO- http://relay-ops:8100/healthz'
```

等待一轮 site-monitor（15 分钟）后确认基线以真实倍率重建：

```bash
ssh sub2api-prod 'sudo bash -s' <<'OUTER'
U=$(cat /opt/sub2api/production/secrets/relay-ops-database-url)
docker run --rm -i --network sub2api_default postgres:18-alpine psql "$U" -c "select incident_key, current_value, sample_count from relay_ops.incidents where incident_key like '%multiplier_baseline%' order by incident_key;"
OUTER
```

Expected: 新基线的 `current_value` 是 `0.05x` / `0.16x` / `0.25x` 这类真实值，**不再是 `1x`**；`sample_count` 全为 `1`（无误报变更）。

**必须人工打开 `https://api.xingqiaolab.top/ops` 确认整页渲染完整**（模板错误不会让容器不健康，只会让页面静默截断）。确认账号质量表存在、无「倍率」列、表格行数正常。

- [ ] **Step 5: 确认无假告警**

```bash
ssh sub2api-prod 'sudo bash -s' <<'OUTER'
U=$(cat /opt/sub2api/production/secrets/relay-ops-database-url)
docker run --rm -i --network sub2api_default postgres:18-alpine psql "$U" -c "select id, delivery_status, delivered_at from relay_ops.notification_deliveries order by id desc limit 5;"
OUTER
```

Expected: 无因倍率变更产生的新投递。若出现 22 条倍率告警，说明 Step 2 的基线清理未生效，需回滚镜像并重新清理。

## 回滚

```bash
ssh sub2api-prod 'cd /opt/sub2api/production && sudo cp compose.yaml.bak-before-multiplier-migration-20260727 compose.yaml && sudo docker compose --env-file .env -f compose.yaml up -d --no-deps --force-recreate relay-ops'
```

基线记录可从 `/tmp/multiplier-baseline-backup.csv` 恢复，但通常不需要 —— 旧镜像会自行以 `1x` 重建。

## 不在本计划范围内

- `/ops` 账号质量表的账号列仍显示 `账号 {{.AccountID}}` 而非账号名。修它需要 Ruby 采集脚本额外输出账号名，并评估账号名进入 `/ops` 页面的脱敏影响，单独立项。
- 分组告警在同一小时桶内「故障→恢复→再故障」仍吞掉第二次告警，根治需 `incidents` 包暴露事件轮次。
