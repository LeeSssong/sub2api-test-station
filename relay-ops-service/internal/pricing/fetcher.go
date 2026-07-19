package pricing

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errUnsafeURL = errors.New("pricing URL resolves to an unsafe address")
var errBodyTooLarge = errors.New("pricing response exceeds size limit")

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type FetchResult struct {
	URL         string
	FetchedAt   time.Time
	ContentHash string
	ContentType string
	Body        []byte
}

type Fetcher struct {
	Client   *http.Client
	Resolver Resolver
	Now      func() time.Time
	MaxBytes int64
}

func IsUnsafeURL(err error) bool    { return errors.Is(err, errUnsafeURL) }
func IsBodyTooLarge(err error) bool { return errors.Is(err, errBodyTooLarge) }

func ValidateRemoteURL(ctx context.Context, resolver Resolver, rawURL string) error {
	return validateRemoteURL(ctx, resolver, rawURL)
}

func (f Fetcher) Fetch(ctx context.Context, rawURL, previousHash string) (FetchResult, bool, error) {
	resolver := f.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := validateRemoteURL(ctx, resolver, rawURL); err != nil {
		return FetchResult{}, false, err
	}
	base := f.Client
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	client.Timeout = 10 * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("pricing redirect limit exceeded")
		}
		return validateRemoteURL(req.Context(), resolver, req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return FetchResult{}, false, fmt.Errorf("build pricing request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/html;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := client.Do(req)
	if err != nil {
		if IsUnsafeURL(err) {
			return FetchResult{}, false, errUnsafeURL
		}
		return FetchResult{}, false, fmt.Errorf("fetch pricing page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchResult{}, false, fmt.Errorf("pricing page returned HTTP %d", resp.StatusCode)
	}
	reader := io.Reader(resp.Body)
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		compressed, err := gzip.NewReader(resp.Body)
		if err != nil {
			return FetchResult{}, false, fmt.Errorf("decode pricing gzip")
		}
		defer compressed.Close()
		reader = compressed
	}
	limit := f.MaxBytes
	if limit <= 0 {
		limit = 2 << 20
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return FetchResult{}, false, fmt.Errorf("read pricing page")
	}
	if int64(len(body)) > limit {
		return FetchResult{}, false, errBodyTooLarge
	}
	hashBytes := sha256.Sum256(body)
	hash := hex.EncodeToString(hashBytes[:])
	now := time.Now().UTC()
	if f.Now != nil {
		now = f.Now().UTC()
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	result := FetchResult{URL: finalURL, FetchedAt: now, ContentHash: hash, ContentType: resp.Header.Get("Content-Type")}
	if hash == previousHash {
		return result, false, nil
	}
	result.Body = body
	return result, true, nil
}

func validateRemoteURL(ctx context.Context, resolver Resolver, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return errUnsafeURL
	}
	host := parsed.Hostname()
	addresses := make([]net.IPAddr, 0, 1)
	if ip := net.ParseIP(host); ip != nil {
		addresses = append(addresses, net.IPAddr{IP: ip})
	} else {
		addresses, err = resolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return errUnsafeURL
		}
	}
	for _, address := range addresses {
		ip := address.IP
		if ip == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return errUnsafeURL
		}
	}
	return nil
}
