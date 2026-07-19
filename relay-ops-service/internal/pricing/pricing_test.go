package pricing

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

func TestFetcherHashesGzipAndSkipsUnchangedBodies(t *testing.T) {
	t.Parallel()
	body := []byte(`{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1.25","output":"10"}]}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write(body)
	_ = writer.Close()
	client := &http.Client{Transport: roundTripper(func(*http.Request) *http.Response {
		return response(http.StatusOK, compressed.Bytes(), map[string]string{"Content-Type": "application/json", "Content-Encoding": "gzip"})
	})}
	fetcher := Fetcher{Client: client, Resolver: publicResolver{}, Now: func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }}

	first, changed, err := fetcher.Fetch(context.Background(), "https://pricing.example/models", "")
	if err != nil || !changed || !bytes.Equal(first.Body, body) || first.ContentHash == "" {
		t.Fatalf("first = %#v changed=%v err=%v", first, changed, err)
	}
	second, changed, err := fetcher.Fetch(context.Background(), "https://pricing.example/models", first.ContentHash)
	if err != nil || changed || second.ContentHash != first.ContentHash || second.Body != nil {
		t.Fatalf("second = %#v changed=%v err=%v", second, changed, err)
	}
}

func TestFetcherRejectsSSRFRedirectsAndOversizedBodies(t *testing.T) {
	t.Parallel()
	if _, _, err := (Fetcher{Resolver: publicResolver{}}).Fetch(context.Background(), "https://127.0.0.1/pricing", ""); !IsUnsafeURL(err) {
		t.Fatalf("private URL error = %v", err)
	}

	redirectClient := &http.Client{Transport: roundTripper(func(*http.Request) *http.Response {
		return response(http.StatusFound, nil, map[string]string{"Location": "https://127.0.0.1/private"})
	})}
	if _, _, err := (Fetcher{Client: redirectClient, Resolver: publicResolver{}}).Fetch(context.Background(), "https://pricing.example/models", ""); !IsUnsafeURL(err) {
		t.Fatalf("redirect error = %v", err)
	}

	largeClient := &http.Client{Transport: roundTripper(func(*http.Request) *http.Response {
		return response(http.StatusOK, bytes.Repeat([]byte("x"), 513), map[string]string{"Content-Type": "text/html"})
	})}
	if _, _, err := (Fetcher{Client: largeClient, Resolver: publicResolver{}, MaxBytes: 512}).Fetch(context.Background(), "https://pricing.example/models", ""); !IsBodyTooLarge(err) {
		t.Fatalf("large body error = %v", err)
	}
}

func TestExtractorReadsStructuredJSONAndCommonHTML(t *testing.T) {
	t.Parallel()
	extractor := CompositeExtractor{}
	jsonResult := FetchResult{URL: "https://sub2api.example/pricing", ContentType: "application/json", Body: []byte(`{
		"multiplier":"0.10x",
		"models":[{"model":"gpt-5.6-sol","input_price":"1.25","output_price":"10","cache_read_price":"0.125","tier":">272k"}]
	}`)}
	evidence, err := extractor.Extract(jsonResult)
	if err != nil || evidence.AdvertisedMultiplier == nil || *evidence.AdvertisedMultiplier != 1_000 || len(evidence.Models) != 1 {
		t.Fatalf("JSON evidence = %#v, %v", evidence, err)
	}
	if evidence.Models[0].ModelID != "gpt-5.6-sol" || evidence.Models[0].Tier != ">272k" || evidence.Models[0].CacheRead != "0.125" {
		t.Fatalf("JSON model = %#v", evidence.Models[0])
	}

	htmlResult := FetchResult{URL: "https://newapi.example/pricing", ContentType: "text/html", Body: []byte(`
		<html><body><p>当前倍率 0.05x</p><table><thead><tr><th>模型</th><th>输入价格</th><th>输出价格</th></tr></thead>
		<tbody><tr><td>gpt-5.5</td><td>$2.00</td><td>$8.00</td></tr></tbody></table></body></html>`)}
	evidence, err = extractor.Extract(htmlResult)
	if err != nil || evidence.AdvertisedMultiplier == nil || *evidence.AdvertisedMultiplier != 500 || len(evidence.Models) != 1 {
		t.Fatalf("HTML evidence = %#v, %v", evidence, err)
	}
	if evidence.Models[0].Input != "2.00" || evidence.Models[0].Output != "8.00" {
		t.Fatalf("HTML model = %#v", evidence.Models[0])
	}
}

func TestSemanticDiffDetectsMultiplierAndModelPriceChanges(t *testing.T) {
	t.Parallel()
	oldMultiplier := domain.MultiplierBPS(700)
	newMultiplier := domain.MultiplierBPS(1_000)
	previous := Evidence{AdvertisedMultiplier: &oldMultiplier, Models: []ModelPrice{{ModelID: "gpt-a", Input: "1", Output: "8"}, {ModelID: "gpt-removed", Input: "2", Output: "9"}}}
	current := Evidence{AdvertisedMultiplier: &newMultiplier, Models: []ModelPrice{{ModelID: "gpt-a", Input: "1.25", Output: "8"}, {ModelID: "gpt-added", Input: "3", Output: "10"}}}

	diff := Diff(previous, current)
	if diff.Multiplier == nil || diff.Multiplier.Before != 700 || diff.Multiplier.After != 1_000 {
		t.Fatalf("multiplier diff = %#v", diff.Multiplier)
	}
	if len(diff.AddedModels) != 1 || diff.AddedModels[0] != "gpt-added" || len(diff.RemovedModels) != 1 || len(diff.PriceChanges) != 1 {
		t.Fatalf("diff = %#v", diff)
	}
	if !diff.SemanticChange() {
		t.Fatal("semantic change not detected")
	}
}

func TestExtractorReportsUnparseableAndSnapshotStaleness(t *testing.T) {
	t.Parallel()
	_, err := (CompositeExtractor{}).Extract(FetchResult{URL: "https://example.com/pricing", ContentType: "text/html", Body: []byte(`<html><body>pricing unavailable</body></html>`)})
	if !IsUnparseable(err) {
		t.Fatalf("error = %v", err)
	}
	if !IsStale(time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC), time.Date(2026, 7, 19, 1, 16, 0, 0, time.UTC), 15*time.Minute) {
		t.Fatal("snapshot should be stale")
	}
}

type publicResolver struct{}

func (publicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.20")}}, nil
}

type roundTripper func(*http.Request) *http.Response

func (fn roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request), nil
}

func response(status int, body []byte, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(string(body)))}
}
