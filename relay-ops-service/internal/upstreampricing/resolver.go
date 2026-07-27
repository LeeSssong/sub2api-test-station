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

	mu    sync.Mutex
	cache map[string]cachedRatios
}

// Lookup resolves accountName's multiplier from its upstream's live pricing.
// Every failure — unmapped account, unreadable config, unreachable upstream,
// unknown group name, non-positive ratio — returns (nil, false); it never
// returns a guessed or stale-beyond-TTL value.
func (r *Resolver) Lookup(ctx context.Context, accountName string) (*float64, bool) {
	cfg, err := LoadConfig(r.ConfigPath)
	if err != nil {
		return nil, false
	}

	for _, upstream := range cfg.Upstreams {
		group, mapped := upstream.Accounts[accountName]
		if !mapped {
			continue
		}
		ratios, err := r.groupRatios(ctx, upstream.PricingURL)
		if err != nil {
			return nil, false
		}
		ratio, found := ratios[group]
		if !found || ratio <= 0 {
			return nil, false
		}
		return &ratio, true
	}
	return nil, false
}

// groupRatios returns the ratio table for pricingURL, served from cache
// within TTL and fetched from the upstream otherwise.
func (r *Resolver) groupRatios(ctx context.Context, pricingURL string) (map[string]float64, error) {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	ttl := r.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, ok := r.cache[pricingURL]; ok && now().Sub(entry.fetchedAt) < ttl {
		return entry.ratios, nil
	}

	ratios, err := r.fetchGroupRatios(ctx, pricingURL)
	if err != nil {
		return nil, err
	}
	if r.cache == nil {
		r.cache = make(map[string]cachedRatios)
	}
	r.cache[pricingURL] = cachedRatios{ratios: ratios, fetchedAt: now()}
	return ratios, nil
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
