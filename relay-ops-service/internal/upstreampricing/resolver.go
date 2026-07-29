// Package upstreampricing resolves account cost multipliers from an
// upstream relay's public pricing endpoint (GET /api/pricing).
//
// The mapping config stores group NAMES, never ratio values: the upstream
// stays the single source of truth, so a price change upstream is picked up
// automatically without anyone editing local config. Every failure path in
// Lookup returns (nil, false) — the caller must treat that as "cannot
// account", never substitute a guessed value.
package upstreampricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// maxConfigBytes caps the mapping-config read; the file is a few KiB at most.
	maxConfigBytes = 64 << 10
	// maxPricingBodyBytes caps the upstream pricing response body.
	maxPricingBodyBytes = 1 << 20
	// fetchTimeout bounds a single pricing fetch.
	fetchTimeout = 10 * time.Second
	// DefaultTTL is how long fetched ratios are reused before refetching.
	DefaultTTL = 10 * time.Minute
	// negativeTTL is how long a failed fetch is remembered before retrying,
	// so one dead upstream costs one timeout per minute instead of one per
	// mapped account per round.
	negativeTTL = time.Minute
	// logThrottleTTL suppresses duplicate log lines for the same failure key,
	// so a report round over many accounts logs a broken config once.
	logThrottleTTL = time.Minute
)

// UpstreamMapping ties accounts (by Sub2API account name) to the group name
// they belong to on one upstream, plus that upstream's public pricing URL.
type UpstreamMapping struct {
	PricingURL string            `json:"pricing_url"`
	Accounts   map[string]string `json:"accounts"`
}

// Config is the on-disk mapping file format.
type Config struct {
	Upstreams []UpstreamMapping `json:"upstreams"`
}

type Resolution struct {
	PricingURL string
	Multiplier float64
}

// LoadConfig reads and strictly parses the mapping config at path.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open mapping config: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(io.LimitReader(f, maxConfigBytes))
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse mapping config: %w", err)
	}
	return cfg, nil
}

// pricingResponse mirrors the relevant part of GET /api/pricing.
type pricingResponse struct {
	GroupRatio map[string]float64 `json:"group_ratio"`
}

// ParseGroupRatios extracts the group→ratio table from a pricing response body.
func ParseGroupRatios(body []byte) (map[string]float64, error) {
	var resp pricingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse pricing response: %w", err)
	}
	if resp.GroupRatio == nil {
		return nil, errors.New("pricing response has no group_ratio")
	}
	return resp.GroupRatio, nil
}

type cachedRatios struct {
	ratios    map[string]float64
	err       error
	fetchedAt time.Time
}

// Resolver looks up an account's current multiplier by re-reading the mapping
// config on every call (ops edits take effect without a restart) while caching
// upstream pricing fetches per URL for TTL.
type Resolver struct {
	ConfigPath string
	HTTP       *http.Client
	// Now is the clock used for cache expiry; nil means time.Now.
	Now func() time.Time
	// TTL is the pricing-cache lifetime; <= 0 means DefaultTTL.
	TTL time.Duration
	// RequireHTTPS rejects non-https pricing URLs when true (set in production;
	// tests use plain-HTTP httptest servers, hence the false default).
	RequireHTTPS bool

	// mu guards cache and lastLogged only; HTTP fetches happen outside the
	// lock so one slow upstream cannot stall lookups against another.
	mu         sync.Mutex
	cache      map[string]cachedRatios
	lastLogged map[string]time.Time
}

// logf logs a failure at most once per logThrottleTTL per key. The daily
// report calls Lookup once per account, so an unthrottled config error would
// repeat for every mapped account in the same round.
func (r *Resolver) logf(key, format string, args ...any) {
	now := r.now()
	r.mu.Lock()
	if last, ok := r.lastLogged[key]; ok && now().Sub(last) < logThrottleTTL {
		r.mu.Unlock()
		return
	}
	if r.lastLogged == nil {
		r.lastLogged = make(map[string]time.Time)
	}
	r.lastLogged[key] = now()
	r.mu.Unlock()
	log.Printf(format, args...)
}

func (r *Resolver) now() func() time.Time {
	if r.Now != nil {
		return r.Now
	}
	return time.Now
}

// Lookup resolves accountName's multiplier from its upstream's live pricing.
// Every failure — unmapped account, unreadable config, unreachable upstream,
// unknown group name, non-positive ratio — returns (nil, false); it never
// returns a guessed or stale-beyond-TTL value.
func (r *Resolver) Lookup(ctx context.Context, accountName string) (*float64, bool) {
	resolution, ok := r.Resolve(ctx, accountName)
	if !ok {
		return nil, false
	}
	return &resolution.Multiplier, true
}

// Resolve returns both the explicitly configured production pricing source
// and the live multiplier it currently exposes for accountName.
func (r *Resolver) Resolve(ctx context.Context, accountName string) (Resolution, bool) {
	cfg, err := LoadConfig(r.ConfigPath)
	if err != nil {
		// /dev/null 挂载、文件缺失、JSON 写错都会走到这里；不记日志的话
		// 功能会永久静默失效，日报只显示「—」而无从排查。
		r.logf("config", "upstream pricing: mapping config unusable, fallback disabled: %v", err)
		return Resolution{}, false
	}

	for _, upstream := range cfg.Upstreams {
		group, mapped := upstream.Accounts[accountName]
		if !mapped {
			// 大部分账号本来就不在映射里，这是常态，不记日志。
			continue
		}
		ratios, err := r.groupRatios(ctx, upstream.PricingURL)
		if err != nil {
			// 拉取失败已在实际发请求处记过日志（负缓存命中不重复记）。
			return Resolution{}, false
		}
		ratio, found := ratios[group]
		if !found {
			r.logf("group:"+accountName,
				"upstream pricing: account %q unresolved: group %q not in %s group_ratio",
				accountName, group, upstream.PricingURL)
			return Resolution{}, false
		}
		if ratio <= 0 {
			r.logf("ratio:"+accountName,
				"upstream pricing: account %q unresolved: group %q ratio %v is non-positive",
				accountName, group, ratio)
			return Resolution{}, false
		}
		return Resolution{PricingURL: upstream.PricingURL, Multiplier: ratio}, true
	}
	return Resolution{}, false
}

// groupRatios returns the ratio table for pricingURL, served from cache
// within TTL and fetched from the upstream otherwise. Failed fetches are
// negative-cached for negativeTTL so a dead upstream is not retried for
// every mapped account in the same report round. The fetch itself runs
// outside the mutex: a slow upstream must not block lookups against others.
func (r *Resolver) groupRatios(ctx context.Context, pricingURL string) (map[string]float64, error) {
	now := r.now()
	ttl := r.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	r.mu.Lock()
	if entry, ok := r.cache[pricingURL]; ok {
		age := now().Sub(entry.fetchedAt)
		if entry.err == nil && age < ttl {
			r.mu.Unlock()
			return entry.ratios, nil
		}
		if entry.err != nil && age < negativeTTL {
			r.mu.Unlock()
			return nil, entry.err
		}
	}
	r.mu.Unlock()

	ratios, err := r.fetchGroupRatios(ctx, pricingURL)
	if err != nil {
		log.Printf("upstream pricing: fetch failed for %s (not retried for %s): %v",
			pricingURL, negativeTTL, err)
	}

	r.mu.Lock()
	if r.cache == nil {
		r.cache = make(map[string]cachedRatios)
	}
	r.cache[pricingURL] = cachedRatios{ratios: ratios, err: err, fetchedAt: now()}
	r.mu.Unlock()
	return ratios, err
}

func (r *Resolver) fetchGroupRatios(ctx context.Context, pricingURL string) (map[string]float64, error) {
	parsed, err := url.Parse(pricingURL)
	if err != nil {
		return nil, fmt.Errorf("parse pricing url: %w", err)
	}
	if r.RequireHTTPS && !strings.EqualFold(parsed.Scheme, "https") {
		return nil, fmt.Errorf("pricing url %q is not https", pricingURL)
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pricingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build pricing request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := r.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	if r.RequireHTTPS {
		// Go's default client follows up to 10 redirects and happily
		// downgrades https → http, which would let a hijacked upstream feed
		// us ratios over plaintext despite RequireHTTPS. Enforce https on
		// every hop. The caller's client is copied, never mutated: it may be
		// a shared client (tests inject http.DefaultClient).
		checked := *client
		checked.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if !strings.EqualFold(req.URL.Scheme, "https") {
				return fmt.Errorf("refusing redirect to non-https url %q", req.URL.Redacted())
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		client = &checked
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch pricing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fetch pricing: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPricingBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read pricing body: %w", err)
	}
	return ParseGroupRatios(body)
}
