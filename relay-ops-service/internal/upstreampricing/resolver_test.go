package upstreampricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

func TestResolveReturnsExplicitPricingSourceAndMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer server.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"Pro-SHUAI-0.17":"codexPro分组"}}]}`)
	resolution, ok := newResolver(t, path).Resolve(context.Background(), "Pro-SHUAI-0.17")
	if !ok || resolution.PricingURL != server.URL || resolution.Multiplier != .17 {
		t.Fatalf("resolution = %#v, %v", resolution, ok)
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

func TestLookupRejectsRedirectDowngradeToHTTP(t *testing.T) {
	// 初始 URL 是 https 不够：Go 默认 client 跟随重定向且允许降级到明文，
	// RequireHTTPS 必须对每一跳生效。
	var plainHit atomic.Bool
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHit.Store(true)
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer plain.Close()

	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/api/pricing", http.StatusFound)
	}))
	defer secure.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+secure.URL+`","accounts":{"A":"codexPro分组"}}]}`)
	client := secure.Client()
	resolver := &Resolver{ConfigPath: path, HTTP: client, TTL: time.Minute, RequireHTTPS: true}
	if _, ok := resolver.Lookup(context.Background(), "A"); ok {
		t.Fatal("302 降级到明文 HTTP 时必须拒绝，不得返回倍率")
	}
	if plainHit.Load() {
		t.Fatal("明文跳转目标不应被请求到")
	}
	if client.CheckRedirect != nil {
		t.Fatal("不得修改调用方注入的 client（副作用会污染共享 client）")
	}
}

func TestLookupNegativeCachesUpstreamFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	path := writeConfig(t, `{"upstreams":[{"pricing_url":"`+server.URL+`","accounts":{"A":"codexPro分组","B":"codexPro分组兜底"}}]}`)
	resolver := newResolver(t, path)
	now := time.Now()
	resolver.Now = func() time.Time { return now }

	resolver.Lookup(context.Background(), "A")
	resolver.Lookup(context.Background(), "B")
	if calls != 1 {
		t.Fatalf("上游故障被请求 %d 次，负缓存 TTL 内应只请求 1 次", calls)
	}

	// 负缓存过期后必须重试，不能把上游永久拉黑
	now = now.Add(negativeTTL + time.Second)
	resolver.Lookup(context.Background(), "A")
	if calls != 2 {
		t.Fatalf("负缓存过期后被请求 %d 次，应重试第 2 次", calls)
	}
}

func TestLookupSlowUpstreamDoesNotBlockOtherUpstream(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pricingBody))
	}))
	defer fast.Close()

	path := writeConfig(t, `{"upstreams":[`+
		`{"pricing_url":"`+slow.URL+`","accounts":{"S":"codexPro分组"}},`+
		`{"pricing_url":"`+fast.URL+`","accounts":{"F":"codexPro分组"}}]}`)
	resolver := newResolver(t, path)

	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		resolver.Lookup(context.Background(), "S")
	}()
	// 等慢请求真正挂在上游上（拿不到锁的话 fast 那边会超时暴露问题）
	time.Sleep(50 * time.Millisecond)

	fastDone := make(chan bool, 1)
	go func() {
		_, ok := resolver.Lookup(context.Background(), "F")
		fastDone <- ok
	}()
	select {
	case ok := <-fastDone:
		if !ok {
			t.Fatal("fast 上游的 Lookup 应成功")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("慢上游持锁期间阻塞了另一个上游的 Lookup")
	}

	unblock()
	<-slowDone
}
