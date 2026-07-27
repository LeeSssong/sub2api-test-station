# 上游分组倍率兜底 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 当 Sub2API 的 schema v2 倍率不可用时，用上游公开定价接口的分组倍率兜底，让那些永远测不出倍率的账号也能算出利润。

**Architecture:** 新增 `internal/upstreampricing` 包：读一份「账号 → 上游分组名」映射配置，按需拉取上游 `/api/pricing` 的 `group_ratio`（带缓存），解析出倍率。`dailyreport` 与 `opsmonitor` 共用它做兜底。配置文件每次读取时重新加载，运维改完无需重启。

**Tech Stack:** Go 1.24（module `example.invalid/relay-ops-service`）。

## Global Constraints

- 兜底倍率必须标注来源为 `upstream_pricing`，与 `declared` / `measured` 区分开，绝不能伪装成 Sub2API 自己测出来的。
- 配置里存的是**分组名**不是倍率数值。倍率每次从上游实时解析，上游调价自动跟随。硬编码数值会重蹈 `rate_multiplier` 恒为 1 的覆辙。
- 分组名在上游 `group_ratio` 里查不到时，该账号退回「无法核算」，**绝不能拿一个错数字去算利润**。
- 配置文件缺失或格式错误时整份忽略、记一条日志，不得影响其他账号，更不得让日报或告警作业失败。
- 上游请求必须是 HTTPS、有超时、有响应体大小上限。上游不可达时静默跳过兜底，不是变更事件。
- 倍率 `<= 0` 一律视为坏数据跳过，与既有 `trustworthyMultiplier` 规则保持一致。
- Go 测试在 `relay-ops-service/` 目录下运行。

## 生产事实（实施依据，已核实）

shuaiapi 的 `GET /api/pricing` 公开返回：

```json
{"group_ratio": {"codexPro分组": 0.17, "codexPro分组兜底": 0.2,
                 "plus高不稳定分组": 0.08, "CC专用分组-禁非流等": 0.85, ...}}
```

四个账号的映射（用户已确认，且四个分组名在上游均存在）：

| 账号 | 分组 | 当前倍率 |
|---|---|---|
| `Pro-SHUAI-0.17` | `codexPro分组` | 0.17 |
| `Pro20x-SHUAI-0.2` | `codexPro分组兜底` | 0.2 |
| `特惠-SHUAI` | `plus高不稳定分组` | 0.08 |
| `claude-SHUAI` | `CC专用分组-禁非流等` | 0.85 |

这些账号的 Sub2API 倍率恒为 `measured/failed`：shuaiapi 异步结算用量，测算读 `after` 时计数未变，恒定 `quota delta must be positive`。改不了 —— 项目不 fork Sub2API，生产用官方镜像。

`AccountMonitorAccount` 投影里**没有 base_url**，所以配置必须自带上游 pricing URL。

---

### Task 1: upstreampricing 解析与配置加载

**Files:**
- Create: `relay-ops-service/internal/upstreampricing/resolver.go`
- Test: `relay-ops-service/internal/upstreampricing/resolver_test.go`

**Interfaces:**
- Produces: `Config{Upstreams []UpstreamMapping}`、`UpstreamMapping{PricingURL string, Accounts map[string]string}`
- Produces: `LoadConfig(path string) (Config, error)`
- Produces: `ParseGroupRatios(body []byte) (map[string]float64, error)`
- Produces: `Resolver{ConfigPath string, HTTP *http.Client, Now func() time.Time, TTL time.Duration}`
- Produces: `(*Resolver).Lookup(ctx context.Context, accountName string) (*float64, bool)`

配置文件格式：

```json
{
  "upstreams": [
    {
      "pricing_url": "https://api.shuaiapi.com/api/pricing",
      "accounts": {
        "Pro-SHUAI-0.17": "codexPro分组",
        "Pro20x-SHUAI-0.2": "codexPro分组兜底",
        "特惠-SHUAI": "plus高不稳定分组",
        "claude-SHUAI": "CC专用分组-禁非流等"
      }
    }
  ]
}
```

`Lookup` 语义：账号不在任何映射里 → `(nil, false)`；在映射里但上游拉取失败、分组名查不到、或倍率 `<= 0` → `(nil, false)`；成功 → `(&value, true)`。**任何失败都返回 false，绝不返回猜测值。**

缓存：同一个 `pricing_url` 在 `TTL` 内只拉一次。`TTL` 默认 10 分钟（告警作业 5 分钟一轮，缓存让上游每 10 分钟才被访问一次）。

- [ ] **Step 1: 写失败测试**

创建 `resolver_test.go`：

```go
package upstreampricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const pricingBody = `{"group_ratio":{"codexPro分组":0.17,"codexPro分组兜底":0.2,"plus高不稳定分组":0.08,"CC专用分组-禁非流等":0.85,"grok公益分组":0,"auto":1},"success":true}`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mapping.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func newResolver(t *testing.T, configPath string) *Resolver {
	t.Helper()
	return &Resolver{ConfigPath: configPath, HTTP: http.DefaultClient, TTL: time.Minute}
}

func TestParseGroupRatios(t *testing.T) {
	ratios, err := ParseGroupRatios([]byte(pricingBody))
	if err != nil {
		t.Fatalf("ParseGroupRatios: %v", err)
	}
	if got := ratios["codexPro分组"]; got != 0.17 {
		t.Fatalf("codexPro分组 = %v, want 0.17", got)
	}
	if got := ratios["CC专用分组-禁非流等"]; got != 0.85 {
		t.Fatalf("CC专用分组-禁非流等 = %v, want 0.85", got)
	}
}

func TestLookupResolvesMappedAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer server.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"Pro-SHUAI-0.17":"codexPro分组"}}]}`)
	value, ok := newResolver(t, path).Lookup(context.Background(), "Pro-SHUAI-0.17")
	if !ok || value == nil {
		t.Fatal("mapped account must resolve")
	}
	if *value != 0.17 {
		t.Fatalf("value = %v, want 0.17", *value)
	}
}

func TestLookupRejectsUnknownGroupName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer server.Close()

	// 分组名写错时必须退回无法核算，绝不能拿别的值凑数
	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"Pro-SHUAI-0.17":"打错的分组名"}}]}`)
	if _, ok := newResolver(t, path).Lookup(context.Background(), "Pro-SHUAI-0.17"); ok {
		t.Fatal("分组名查不到时不得返回倍率")
	}
}

func TestLookupRejectsNonPositiveRatio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer server.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"X":"grok公益分组"}}]}`)
	if _, ok := newResolver(t, path).Lookup(context.Background(), "X"); ok {
		t.Fatal("倍率 0 是坏数据，不得返回")
	}
}

func TestLookupIgnoresUnmappedAccount(t *testing.T) {
	path := writeConfig(t, `{"upstreams":[{"pricing_url":"https://example.invalid/api/pricing","accounts":{"Other":"g"}}]}`)
	if _, ok := newResolver(t, path).Lookup(context.Background(), "Pro-SHUAI-0.17"); ok {
		t.Fatal("未映射的账号不得解析出倍率")
	}
}

func TestLookupSurvivesUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"Pro-SHUAI-0.17":"codexPro分组"}}]}`)
	if _, ok := newResolver(t, path).Lookup(context.Background(), "Pro-SHUAI-0.17"); ok {
		t.Fatal("上游不可达时不得返回倍率")
	}
}

func TestLookupSurvivesMissingConfig(t *testing.T) {
	resolver := &Resolver{ConfigPath: filepath.Join(t.TempDir(), "absent.json"), HTTP: http.DefaultClient, TTL: time.Minute}
	if _, ok := resolver.Lookup(context.Background(), "Pro-SHUAI-0.17"); ok {
		t.Fatal("配置缺失时不得返回倍率")
	}
}

func TestLookupSurvivesMalformedConfig(t *testing.T) {
	path := writeConfig(t, `{"upstreams":[{"pricing_url":`)
	if _, ok := newResolver(t, path).Lookup(context.Background(), "Pro-SHUAI-0.17"); ok {
		t.Fatal("配置格式错误时不得返回倍率")
	}
}

func TestLookupCachesPricingWithinTTL(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer server.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"A":"codexPro分组","B":"codexPro分组兜底"}}]}`)
	resolver := newResolver(t, path)
	resolver.Lookup(context.Background(), "A")
	resolver.Lookup(context.Background(), "B")
	if calls != 1 {
		t.Fatalf("上游被请求 %d 次，TTL 内应只请求 1 次", calls)
	}
}

func TestLookupRejectsPlainHTTPInProduction(t *testing.T) {
	path := writeConfig(t, `{"upstreams":[{"pricing_url":"http://untrusted.example/api/pricing","accounts":{"A":"g"}}]}`)
	resolver := newResolver(t, path)
	resolver.RequireHTTPS = true
	if _, ok := resolver.Lookup(context.Background(), "A"); ok {
		t.Fatal("RequireHTTPS 开启时明文 HTTP 必须被拒绝")
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/upstreampricing/ -v`
Expected: 编译失败，`undefined: Resolver`

- [ ] **Step 3: 实现**

创建 `resolver.go`。要点：

- `LoadConfig` 用 `json.Decoder` + `DisallowUnknownFields`，读取上限 64 KiB
- 拉取 pricing 时设 10 秒超时、响应体上限 1 MiB、`Accept: application/json`
- `RequireHTTPS bool` 字段；为 true 时非 `https` 的 `pricing_url` 直接跳过（测试用 httptest 是 http，所以默认 false，生产注入 true）
- 缓存用 `sync.Mutex` 保护的 `map[string]cachedRatios{ratios, fetchedAt}`
- `Now` 为 nil 时用 `time.Now`
- 每次 `Lookup` 都重新 `LoadConfig` —— 运维改完配置无需重启（配置文件很小，日报一天一次、告警 5 分钟一次，读取成本可忽略）

- [ ] **Step 4: 验证并提交**

Run: `cd relay-ops-service && go test ./internal/upstreampricing/ -count=1 -v`

```bash
git add relay-ops-service/internal/upstreampricing/
git commit -m "feat: resolve upstream group multipliers from public pricing"
```

---

### Task 2: 接入日报与倍率告警

**Files:**
- Modify: `relay-ops-service/internal/dailyreport/health.go`
- Modify: `relay-ops-service/internal/dailyreport/service.go`
- Modify: `relay-ops-service/internal/opsmonitor/service.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `infra/compose.yaml`、`infra/.env.example`
- Test: 对应各测试文件

**Interfaces:**
- Consumes: `upstreampricing.Resolver.Lookup`
- Produces: `config.Config.UpstreamGroupMappingFile string`（环境变量 `RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE`）
- Changes: `BuildHealthDigest` 增加一个 `fallback func(string) *float64` 参数
- Changes: `notify.AccountDetailLine.Multiplier` 在兜底来源时显示为 `0.17x（上游定价）`

兜底只在 `trustworthyMultiplier(account) == nil` 时使用。优先级：`declared` > `measured` > `upstream_pricing`。

日报文案：兜底成功的账号**不再计入** `UnsupportedAccounts`，因为它的利润已经能算出来了。

- [ ] **Step 1: 写失败测试**

`health_test.go` 追加：

```go
func TestBuildHealthDigestUsesUpstreamPricingFallback(t *testing.T) {
	projection, histories, loc, now := fixture()
	// Pro-SHUAI-0.17 的 Sub2API 倍率是 measured/failed，靠上游定价兜底
	fallback := func(name string) *float64 {
		if name == "Pro-SHUAI-0.17" {
			v := 0.17
			return &v
		}
		return nil
	}
	view := BuildHealthDigestWithFallback(projection, histories, loc, now, fallback)

	if view.Profit.UnsupportedAccounts != 0 {
		t.Fatalf("UnsupportedAccounts = %d, want 0（已被兜底，不再算不支持）", view.Profit.UnsupportedAccounts)
	}
	var line string
	for _, account := range view.Accounts {
		if account.Name == "Pro-SHUAI-0.17" {
			line = account.Multiplier
		}
	}
	if line != "0.17x（上游定价）" {
		t.Fatalf("Multiplier = %q, want 0.17x（上游定价）—— 来源必须可见", line)
	}
}

func TestBuildHealthDigestFallbackDoesNotOverrideTrustworthy(t *testing.T) {
	projection, histories, loc, now := fixture()
	// Pro-SHEN-0.16 有 declared/ok 倍率，兜底不得覆盖它
	fallback := func(string) *float64 { v := 9.99; return &v }
	view := BuildHealthDigestWithFallback(projection, histories, loc, now, fallback)

	for _, account := range view.Accounts {
		if account.Name == "Pro-SHEN-0.16" && account.Multiplier != "0.16x" {
			t.Fatalf("Multiplier = %q, want 0.16x（declared 优先于兜底）", account.Multiplier)
		}
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/dailyreport/ -run UpstreamPricing -v`
Expected: 编译失败，`undefined: BuildHealthDigestWithFallback`

- [ ] **Step 3: 实现**

`BuildHealthDigest` 保留原签名（内部转调 `BuildHealthDigestWithFallback(..., nil)`），新增带 fallback 的版本，避免改动所有既有调用点。

倍率解析顺序在 `health.go` 里集中成一个函数，`multiplierLabel` 需要知道来源才能加后缀。

`opsmonitor.Service` 增加 `Fallback func(context.Context, string) *float64` 字段，在 `evaluateMultipliers` 里对 `trustworthyMultiplier` 返回 nil 的账号尝试兜底 —— 这样上游调价也会触发倍率变化告警。

`config.go` 增加 `UpstreamGroupMappingFile`，`compose.yaml` 增加环境变量与只读挂载：

```yaml
      RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE: ${RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE:-}
```
```yaml
      - ${RELAY_OPS_UPSTREAM_GROUP_MAPPING_HOST_FILE:-/dev/null}:/run/relay-ops/upstream-group-mapping.json:ro
```

`app.go` 在 `UpstreamGroupMappingFile` 非空时构造 `Resolver{RequireHTTPS: true, TTL: 10 * time.Minute}` 并注入日报与 opsmonitor。

- [ ] **Step 4: 验证并提交**

Run: `cd relay-ops-service && go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -10`
Run: `bash tests/relay_ops/validate_relay_ops_contract.sh`

```bash
git add relay-ops-service/ infra/
git commit -m "feat: fall back to upstream group pricing for unmeasurable accounts"
```

---

### Task 3: 部署与配置落地

**Files:** 无代码改动，仅生产操作。

- [ ] **Step 1: 写配置文件**

```bash
ssh sub2api-prod 'sudo install -d -m 0755 /opt/sub2api/production/config'
cat <<'JSON' | ssh sub2api-prod 'sudo tee /opt/sub2api/production/config/upstream-group-mapping.json >/dev/null'
{
  "upstreams": [
    {
      "pricing_url": "https://api.shuaiapi.com/api/pricing",
      "accounts": {
        "Pro-SHUAI-0.17": "codexPro分组",
        "Pro20x-SHUAI-0.2": "codexPro分组兜底",
        "特惠-SHUAI": "plus高不稳定分组",
        "claude-SHUAI": "CC专用分组-禁非流等"
      }
    }
  ]
}
JSON
ssh sub2api-prod 'sudo chmod 0644 /opt/sub2api/production/config/upstream-group-mapping.json'
```

- [ ] **Step 2: 配 .env 并部署**

在生产 `.env` 追加：

```
RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE=/run/relay-ops/upstream-group-mapping.json
RELAY_OPS_UPSTREAM_GROUP_MAPPING_HOST_FILE=/opt/sub2api/production/config/upstream-group-mapping.json
```

从 git 导出构建部署（不要 rsync 工作区）。

- [ ] **Step 3: 验证**

等一轮 site-monitor（15 分钟）后确认四个 SHUAI 账号出现了倍率基线：

```bash
ssh sub2api-prod 'sudo bash -s' <<'OUTER'
U=$(cat /opt/sub2api/production/secrets/relay-ops-database-url)
docker run --rm -i --network sub2api_default postgres:18-alpine psql "$U" -c "select incident_key, current_value from relay_ops.incidents where incident_key like '%multiplier_baseline%' order by incident_key;"
OUTER
```

Expected: 基线从 7 条增至 11 条，新增的是 `0.17x` / `0.2x` / `0.08x` / `0.85x`。

日报的验证要等次日 09:02：四个 SHUAI 账号的明细行显示 `0.17x（上游定价）`，且不再出现在「上游不支持自动测算」计数里。
