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
